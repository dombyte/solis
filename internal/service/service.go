// Package service is the read-side business logic behind the HTTP API: current values
// from the cache, history from the read-only store and the health snapshot. It never
// touches Modbus, the poller or the aggregator (no direct reads, no PollNow).
package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/dombyte/solis/internal/util"

	"github.com/rs/zerolog"

	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/history"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
)

// Sentinel errors mapped to HTTP status codes by the handlers' ErrorMapper.
var (
	// ErrUnknownKey: the key is not in the register table (404).
	ErrUnknownKey = errors.New("unknown register key")
	// ErrWrongKind: the operation does not apply to this register (400).
	ErrWrongKind = errors.New("unsupported for register")
	// ErrNoData: the key exists but has no value yet (404).
	ErrNoData = errors.New("no data")
	// ErrInvalidRange: bad start/end parameters (400).
	ErrInvalidRange = errors.New("invalid time range")
)

// KeyError adds the key to a sentinel error.
type KeyError struct {
	// Key is the register key.
	Key string
	// Detail explains the error.
	Detail string
	// Err is the sentinel.
	Err error
}

func (e *KeyError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("%v: %s", e.Err, e.Key)
	}
	return fmt.Sprintf("%v: %s (%s)", e.Err, e.Key, e.Detail)
}

// Unwrap returns the sentinel.
func (e *KeyError) Unwrap() error { return e.Err }

// ReadStore is the read-only history view.
type ReadStore = storage.ReadStore

// CacheReader reads current values.
type CacheReader interface {
	Get(key string) *solis.Value
}

// HealthSnapshotter returns the supervisor's snapshot.
type HealthSnapshotter interface {
	Snapshot() health.Snapshot
}

// Registry resolves register metadata.
type Registry interface {
	ByKey(key string) (solis.Register, bool)
	All() []solis.Register
}

// StatusDecoder decodes stored status values.
type StatusDecoder interface {
	DecodeStatus(key string, raw uint16) any
}

// Deps are the service dependencies.
type Deps struct {
	Store    ReadStore
	Cache    CacheReader
	Health   HealthSnapshotter
	Registry Registry
	Decoder  StatusDecoder
	Log      zerolog.Logger
}

// ReadService serves reads for the HTTP layer.
type ReadService struct {
	d Deps
}

// ErrMissingDependency is returned by NewReadService for a nil dependency.
var ErrMissingDependency = errors.New("service: missing dependency")

// NewReadService creates the service; every dependency is required (review HTTP-L8).
func NewReadService(d Deps) (*ReadService, error) {
	if err := util.RequireAll(ErrMissingDependency,
		util.Requirement{Name: "Store", OK: d.Store != nil},
		util.Requirement{Name: "Cache", OK: d.Cache != nil},
		util.Requirement{Name: "Health", OK: d.Health != nil},
		util.Requirement{Name: "Registry", OK: d.Registry != nil},
		util.Requirement{Name: "Decoder", OK: d.Decoder != nil},
	); err != nil {
		return nil, err
	}
	return &ReadService{d: d}, nil
}

// Health returns the last published health snapshot (never blocks).
func (s *ReadService) Health() health.Snapshot {
	return s.d.Health.Snapshot()
}

// Keys returns every register sorted by key.
func (s *ReadService) Keys() []solis.Register {
	regs := s.d.Registry.All()
	sort.Slice(regs, func(i, j int) bool { return regs[i].Key < regs[j].Key })
	return regs
}

// Register returns the register definition of key.
func (s *ReadService) Register(key string) (solis.Register, error) {
	reg, ok := s.d.Registry.ByKey(key)
	if !ok {
		return solis.Register{}, &KeyError{Key: key, Err: ErrUnknownKey}
	}
	return reg, nil
}

// Current returns the latest cached value of key.
func (s *ReadService) Current(key string) (*solis.Value, error) {
	if _, err := s.Register(key); err != nil {
		return nil, err
	}
	v := s.d.Cache.Get(key)
	if v == nil {
		s.d.Log.Debug().Str("key", key).Msg("cache miss")
		return nil, &KeyError{Key: key, Detail: "no current value", Err: ErrNoData}
	}
	s.d.Log.Debug().Str("key", key).Msg("cache hit")
	return v, nil
}

// requireStore checks the register kind of key.
func (s *ReadService) requireStore(key string, want solis.Store) error {
	reg, err := s.Register(key)
	if err != nil {
		return err
	}
	if reg.Store != want {
		return &KeyError{
			Key: key, Detail: "not a " + want.String() + " register", Err: ErrWrongKind,
		}
	}
	return nil
}

// historyFunc reads the rows of key between start and end from one history table.
type historyFunc[T any] func(ctx context.Context, key string, start, end time.Time) (T, error)

// readHistory wraps get with the register-kind check for want and debug logging.
func readHistory[T any](s *ReadService, want solis.Store, get historyFunc[T]) historyFunc[T] {
	return func(ctx context.Context, key string, start, end time.Time) (T, error) {
		if err := s.requireStore(key, want); err != nil {
			var zero T
			return zero, err
		}
		s.d.Log.Debug().Str("key", key).Stringer("store", want).Time("start", start).
			Time("end", end).Msg("getting history")
		return get(ctx, key, start, end)
	}
}

// DailyHistory returns daily rows of a daily key.
func (s *ReadService) DailyHistory(ctx context.Context, key string, start, end time.Time) (
	[]*history.DailyDataPoint, error,
) {
	return readHistory(s, solis.StoreDaily, s.d.Store.GetDailyHistory)(ctx, key, start, end)
}

// MonthlyHistory returns monthly rows of a monthly key.
func (s *ReadService) MonthlyHistory(ctx context.Context, key string, start, end time.Time) (
	[]*history.MonthlyDataPoint, error,
) {
	return readHistory(s, solis.StoreMonthly, s.d.Store.GetMonthlyHistory)(ctx, key, start, end)
}

// YearlyHistory returns yearly rows of a yearly key.
func (s *ReadService) YearlyHistory(ctx context.Context, key string, start, end time.Time) (
	[]*history.YearlyDataPoint, error,
) {
	return readHistory(s, solis.StoreYearly, s.d.Store.GetYearlyHistory)(ctx, key, start, end)
}

// Total returns the stored lifetime value of a total key.
func (s *ReadService) Total(ctx context.Context, key string) (*history.TotalDataPoint, error) {
	if err := s.requireStore(key, solis.StoreTotal); err != nil {
		return nil, err
	}
	s.d.Log.Debug().Str("key", key).Msg("getting total")
	dp, err := s.d.Store.GetTotalHistory(ctx, key)
	if err != nil {
		return nil, err
	}
	if dp == nil {
		return nil, &KeyError{Key: key, Detail: "no total data", Err: ErrNoData}
	}
	return dp, nil
}

// StatusEntry is one decoded status in the history.
type StatusEntry struct {
	Timestamp     string `json:"timestamp"`
	StatusDecoded any    `json:"status_decoded"`
}

// StatusHistory is the decoded change history of a status register, newest first.
type StatusHistory struct {
	Key     string        `json:"key"`
	Name    string        `json:"name"`
	History []StatusEntry `json:"history"`
	// Before is the change active at the start of a range (the newest one before it), so
	// a client knows the state between the start and the first listed change. Only for a
	// range, and only when such a change is stored.
	Before *StatusEntry `json:"before,omitempty"`
}

// StatusHistory decodes the stored changes of a status key in [start, end] (the newest
// 1000) plus its current value when that falls in the range and is not stored yet.
func (s *ReadService) StatusHistory(ctx context.Context, key string, start, end time.Time) (
	StatusHistory, error,
) {
	if err := s.requireStore(key, solis.StoreStatus); err != nil {
		return StatusHistory{}, err
	}
	s.d.Log.Debug().Str("key", key).Time("start", start).Time("end", end).
		Msg("getting status history")
	reg, _ := s.d.Registry.ByKey(key)
	out := StatusHistory{Key: key, Name: reg.Name, History: []StatusEntry{}}
	points, err := s.d.Store.GetErrorHistory(ctx, key, start, end)
	if err != nil {
		// History is best effort; the current value is still useful.
		s.d.Log.Warn().Err(err).Str("key", key).Msg("status history unavailable")
	}
	// Cleared states (no active bits) are kept: "fault cleared" is history too.
	for _, p := range points {
		out.History = append(out.History, s.decodeEntry(key, p))
	}
	var before *history.ErrorDataPoint
	if !start.IsZero() {
		before = s.statusBefore(ctx, key, start)
	}
	if cur := s.unstoredCurrent(key, newestStored(points, before), start, end); cur != nil {
		out.History = append(out.History, *cur)
	}
	// Both sources use period.TimestampLayout, so string order is time order.
	sort.SliceStable(out.History, func(i, j int) bool {
		return out.History[i].Timestamp > out.History[j].Timestamp
	})
	if before != nil {
		e := s.decodeEntry(key, before)
		out.Before = &e
	}
	return out, nil
}

// statusBefore is the change active at start, or nil (none stored, or the read failed:
// like the history itself, it is best effort).
func (s *ReadService) statusBefore(ctx context.Context, key string,
	start time.Time,
) *history.ErrorDataPoint {
	p, err := s.d.Store.GetLastErrorBefore(ctx, key, start)
	if err != nil {
		s.d.Log.Warn().Err(err).Str("key", key).Msg("status before range unavailable")
		return nil
	}
	return p
}

// newestStored is the stored state active at the end of the range: its newest change
// (points are oldest first), else the change active at its start, else nil.
func newestStored(points []*history.ErrorDataPoint,
	before *history.ErrorDataPoint,
) *history.ErrorDataPoint {
	if len(points) > 0 {
		return points[len(points)-1]
	}
	return before
}

// decodeEntry decodes one stored status change.
func (s *ReadService) decodeEntry(key string, p *history.ErrorDataPoint) StatusEntry {
	return StatusEntry{
		Timestamp:     p.Timestamp,
		StatusDecoded: s.d.Decoder.DecodeStatus(key, uint16(p.RawValue)),
	}
}

// unstoredCurrent is the cached state of key when it lies in [start, end] and differs
// from the newest stored state, else nil. Comparing with the state before the range when
// the range holds no change keeps an unchanged state from showing up as a new change at
// the latest poll.
func (s *ReadService) unstoredCurrent(key string, newest *history.ErrorDataPoint,
	start, end time.Time,
) *StatusEntry {
	v := s.d.Cache.Get(key)
	if v == nil || v.StatusDecoded == nil || (newest != nil && newest.RawValue == v.RawValue) ||
		v.Timestamp.Before(start) || v.Timestamp.After(end) {
		return nil
	}
	return &StatusEntry{
		Timestamp:     v.Timestamp.UTC().Format(period.TimestampLayout),
		StatusDecoded: v.StatusDecoded,
	}
}

// maxTime is the upper bound of an unbounded history query.
func maxTime() time.Time {
	const farFuture = 9999
	return time.Date(farFuture, time.December, 31, 0, 0, 0, 0, time.UTC)
}
