package storage

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/dombyte/solis/internal/period"
)

// Meta table keys: cutover date, closed-period watermarks, baselines, retention watermark.
const (
	metaCutover        = "v3_cutover_date"
	metaClosedDaily    = "closed:daily:" // + daily key (poller)
	metaClosedNetDaily = "closed:netdaily"
	metaClosedMonthly  = "closed:monthly"
	metaClosedYearly   = "closed:yearly"
	metaBaseline       = "baseline:" // + total key
	metaBaselineYear   = "baseline_year"
	metaPurgedBefore   = "purged_before" // earliest day retention cleanup kept
	metaOffset         = "offset:"       // + OffsetKey(level, period, key)
	floatBits          = 64
)

// metaState is the in-memory copy of the meta table.
type metaState struct {
	cutover      string
	closedDaily  map[string]string
	frozen       map[period.Level]string // Daily = net daily
	baselineYear string
	baseline     map[string]float64
	offsets      map[string]float64 // OffsetKey -> cutover offset
	purgedBefore string
}

func newMetaState() metaState {
	return metaState{
		closedDaily: make(map[string]string),
		frozen:      make(map[period.Level]string),
		baseline:    make(map[string]float64),
		offsets:     make(map[string]float64),
	}
}

func (m metaState) clone() metaState {
	c := m
	c.closedDaily = maps.Clone(m.closedDaily)
	c.frozen = maps.Clone(m.frozen)
	c.baseline = maps.Clone(m.baseline)
	c.offsets = maps.Clone(m.offsets)
	return c
}

func frozenKey(l period.Level) (string, error) {
	switch l {
	case period.Daily:
		return metaClosedNetDaily, nil
	case period.Monthly:
		return metaClosedMonthly, nil
	case period.Yearly:
		return metaClosedYearly, nil
	default:
		return "", fmt.Errorf("storage: level %s cannot be frozen", l)
	}
}

// loadMeta reads the meta table into memory.
func (s *Storage) loadMeta(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM meta`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	m := newMetaState()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return err
		}
		if err := m.apply(k, v); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.meta = m
	s.mu.Unlock()
	return nil
}

// apply sets one meta row on the state.
func (m *metaState) apply(k, v string) error {
	switch k {
	case metaCutover:
		m.cutover = v
	case metaBaselineYear:
		m.baselineYear = v
	case metaPurgedBefore:
		m.purgedBefore = v
	case metaClosedNetDaily:
		m.frozen[period.Daily] = v
	case metaClosedMonthly:
		m.frozen[period.Monthly] = v
	case metaClosedYearly:
		m.frozen[period.Yearly] = v
	default:
		return m.applyPrefixed(k, v)
	}
	return nil
}

// applyPrefixed sets a per-key meta row (daily close watermark or total baseline).
func (m *metaState) applyPrefixed(k, v string) error {
	if day, ok := strings.CutPrefix(k, metaClosedDaily); ok {
		m.closedDaily[day] = v
		return nil
	}
	target := m.baseline
	key, ok := strings.CutPrefix(k, metaBaseline)
	if !ok {
		if key, ok = strings.CutPrefix(k, metaOffset); !ok {
			return nil
		}
		target = m.offsets
	}
	f, err := strconv.ParseFloat(v, floatBits)
	if err != nil {
		return fmt.Errorf("storage: meta %s: %w", k, err)
	}
	target[key] = f
	return nil
}

func putMeta(tx *txn, k, v string) error {
	_, err := tx.ExecContext(tx.ctx, `INSERT INTO meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v)
	if err != nil {
		return fmt.Errorf("storage: write meta %s: %w", k, err)
	}
	return nil
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, floatBits)
}

// advance sets watermark k to v when v is later; returns the new value.
func advance(tx *txn, k, cur, v string) (string, error) {
	if v <= cur {
		return cur, nil
	}
	return v, putMeta(tx, k, v)
}

// EnsureCutover records the v3 cutover on first start and initialises the
// watermarks so pre-cutover monthly/yearly rows stay frozen as authoritative history.
// It returns the cutover day and whether it was created now.
func (s *Storage) EnsureCutover(ctx context.Context, p period.Period) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.meta.cutover != "" {
		return s.meta.cutover, false, nil
	}
	next := s.meta.clone()
	err := s.withTx(ctx, func(tx *txn) error { return s.writeCutover(tx, &next, p) })
	if err != nil {
		return "", false, err
	}
	s.meta = next
	return p.Day, true, nil
}

// cutoverMargin keeps the previous day writable so a first start inside the rollover
// window can still finish yesterday.
const cutoverMargin = -2

// writeCutover freezes everything before the last closed day's month and year. Freezing
// relative to that day (not to today) keeps a month or year open while one of its days
// is still writable: a cutover on the 1st inside the rollover window must not freeze the
// month whose last day is still being finished (review AGG-L3).
func (s *Storage) writeCutover(tx *txn, m *metaState, p period.Period) error {
	closedDay, err := period.AddDays(p.Day, cutoverMargin)
	if err != nil {
		return err
	}
	frozenMonth, err := period.AddMonths(closedDay[:len("2006-01")], -1)
	if err != nil {
		return err
	}
	frozenYear, err := period.AddYears(closedDay[:len("2006")], -1)
	if err != nil {
		return err
	}
	if err := putMeta(tx, metaCutover, p.Day); err != nil {
		return err
	}
	m.cutover = p.Day
	marks := []struct {
		level period.Level
		key   string
		value string
	}{
		{period.Monthly, metaClosedMonthly, frozenMonth},
		{period.Yearly, metaClosedYearly, frozenYear},
		{period.Daily, metaClosedNetDaily, closedDay},
	}
	for _, mk := range marks {
		if m.frozen[mk.level], err = advance(tx, mk.key, m.frozen[mk.level], mk.value); err != nil {
			return err
		}
	}
	return s.finishCutover(tx, m, p, closedDay)
}

// finishCutover records the cutover offsets and the per-key day marks.
func (s *Storage) finishCutover(tx *txn, m *metaState, p period.Period, closedDay string) error {
	if err := s.writeCutoverOffsets(tx, m, p); err != nil {
		return err
	}
	prevYear, err := period.AddYears(p.Year, -1)
	if err != nil {
		return err
	}
	return s.initDayMarks(tx, m, closedDay, prevYear)
}

// writeCutoverOffsets keeps the inverter-reported rows of the months and years that stay
// open at cutover (review AGG-M2): the periods before are frozen, but these would
// otherwise be replaced by daily sums that miss whatever the daily rows lack. For every
// computed key whose stored value is ahead of its daily sum up to today, the difference is
// recorded; the aggregator adds it on top of the growing daily sum, so the period
// continues from the inverter's value. `solis backfill` overrides it (clears the offset
// and writes the pure daily sum).
func (s *Storage) writeCutoverOffsets(tx *txn, m *metaState, p period.Period) error {
	spans, err := openSpans(m, p)
	if err != nil {
		return err
	}
	for _, sp := range spans {
		if err := s.periodOffsets(tx, m, sp); err != nil {
			return err
		}
	}
	return nil
}

// openSpans lists every month and year after the frozen watermarks through today, each
// with the daily rows summed for it (period start .. min(period end, today)).
func openSpans(m *metaState, p period.Period) ([]offsetSpan, error) {
	months, err := period.MonthsBetween(m.frozen[period.Monthly], p.Month)
	if err != nil {
		return nil, err
	}
	years, err := period.YearsBetween(m.frozen[period.Yearly], p.Year)
	if err != nil {
		return nil, err
	}
	var spans []offsetSpan
	for _, l := range []struct {
		level  period.Level
		keys   []string
		bounds func(string) (string, string, error)
	}{{period.Monthly, months, period.MonthBounds}, {period.Yearly, years, period.YearBounds}} {
		for _, k := range l.keys {
			first, last, err := l.bounds(k)
			if err != nil {
				return nil, err
			}
			spans = append(spans, offsetSpan{
				level: l.level, key: k, from: first,
				today: min(last, p.Day),
			})
		}
	}
	return spans, nil
}

// offsetSpan is one cutover period and the daily rows summed for it (from..today).
type offsetSpan struct {
	level            period.Level
	key, from, today string
}

// periodOffsets records the offset of every computed key of one cutover period.
func (s *Storage) periodOffsets(tx *txn, m *metaState, sp offsetSpan) error {
	l, key, from, today := sp.level, sp.key, sp.from, sp.today
	for _, e := range s.keys.Edges(l) {
		stored, ok, err := periodValue(tx.ctx, tx, l, e.Target, key)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		sum, err := sumDaily(tx.ctx, tx, e.Source, from, today)
		if err != nil {
			return err
		}
		if err := putOffset(tx, m, OffsetKey(l, key, e.Target), stored-sum); err != nil {
			return err
		}
	}
	return nil
}

// putOffset records a positive offset (a value at or below the daily sum needs none).
func putOffset(tx *txn, m *metaState, key string, off float64) error {
	if off <= 0 {
		return nil
	}
	m.offsets[key] = off
	return putMeta(tx, metaOffset+key, formatFloat(off))
}

func (s *Storage) initDayMarks(tx *txn, m *metaState, closedDay, prevYear string) error {
	var err error
	for _, k := range s.keys.DailyKeys() {
		if m.closedDaily[k], err = advance(tx, metaClosedDaily+k, m.closedDaily[k],
			closedDay); err != nil {
			return err
		}
	}
	if m.baselineYear == "" {
		m.baselineYear = prevYear
		return putMeta(tx, metaBaselineYear, prevYear)
	}
	return nil
}

// CloseState returns a copy of the persisted close/baseline state.
func (s *Storage) CloseState(_ context.Context) (CloseState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return CloseState{
		Cutover:       s.meta.cutover,
		FrozenMonth:   s.meta.frozen[period.Monthly],
		FrozenYear:    s.meta.frozen[period.Yearly],
		FrozenNetDay:  s.meta.frozen[period.Daily],
		ClosedThrough: s.closedThrough(),
		BaselineYear:  s.meta.baselineYear,
		Baseline:      maps.Clone(s.meta.baseline),
		Offsets:       maps.Clone(s.meta.offsets),
	}, nil
}

// closedThrough is the minimum closed day over all daily keys; caller holds mu.
func (s *Storage) closedThrough() string {
	low := ""
	for i, k := range s.keys.DailyKeys() {
		d := s.meta.closedDaily[k]
		if d == "" {
			return ""
		}
		if i == 0 || d < low {
			low = d
		}
	}
	return low
}
