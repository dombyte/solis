package poller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dombyte/solis/internal/eventbus"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
)

// readAll reads every planned block within PollTimeout; a partial poll is discarded.
func (p *Poller) readAll(ctx context.Context, r Reader, at time.Time) (
	map[string]*solis.Value, error) {
	pctx, cancel := context.WithTimeout(ctx, p.d.Settings.PollTimeout)
	defer cancel()
	values := make(map[string]*solis.Value)
	blocks := p.d.Registry.Blocks()
	for i, b := range blocks {
		raw, err := p.readBlock(pctx, r, b)
		if err != nil {
			return nil, fmt.Errorf("block %d@%d: %w", i+1, b.Start, err)
		}
		for k, v := range p.d.Decoder.DecodeBlock(b, raw, at) {
			values[k] = v
		}
		if p.d.Settings.BlockInterval > 0 && i < len(blocks)-1 {
			if err := p.sleep(pctx, p.d.Settings.BlockInterval); err != nil {
				return nil, err
			}
		}
	}
	return values, nil
}

// readBlock reads one block with BlockAttempts retries.
func (p *Poller) readBlock(ctx context.Context, r Reader, b solis.Block) ([]uint16, error) {
	var err error
	for attempt := 0; attempt <= p.d.Settings.BlockAttempts; attempt++ {
		if attempt > 0 {
			if !r.IsConnected() {
				return nil, fmt.Errorf("modbus disconnected: %w", err)
			}
			if sErr := p.sleep(ctx, p.d.Settings.BlockRetryDelay); sErr != nil {
				return nil, errors.Join(err, sErr)
			}
		}
		var raw []uint16
		if raw, err = r.ReadRegisters(ctx, b.Start, b.Count); err == nil {
			return raw, nil
		}
		p.Beat()
	}
	return nil, err
}

// sleep waits d on the injected clock or until ctx is done.
func (p *Poller) sleep(ctx context.Context, d time.Duration) error {
	t := p.d.Clock.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C():
		return nil
	}
}

// persist runs the write path: forced closes, day attribution, status changes, one
// storage transaction, then cache replacement and PeriodClosed events.
func (p *Poller) persist(ctx context.Context, values map[string]*solis.Value, now time.Time) error {
	forced, err := p.attr.ForceClose(now)
	if err != nil {
		return err
	}
	for k, d := range forced {
		p.pendingCloses[k] = maxDay(p.pendingCloses[k], d)
	}
	w, cacheVals := p.attribute(values, now)
	w.Status = p.statusChanges(values, now)
	for k, d := range p.pendingCloses {
		w.Close = append(w.Close, storage.DayClose{Key: k, Day: d})
	}
	sctx, cancel := p.storageContext(ctx)
	defer cancel()
	if err := p.d.Store.WritePoll(sctx, w); err != nil && !storage.IsRejection(err) {
		return err
	} else if err != nil {
		p.d.Log.Warn().Err(err).Msg("storage rejected late or foreign writes")
	}
	p.committed(w)
	p.d.Cache.ReplaceDomain(eventbus.DomainPoller, cacheVals, now)
	return nil
}

// attribute builds the daily rows and the cache view; discarded daily values keep the
// last accepted value in the cache (the cache stays the comparison base).
func (p *Poller) attribute(values map[string]*solis.Value, now time.Time) (
	storage.PollWrite, map[string]*solis.Value) {
	var w storage.PollWrite
	cacheVals := make(map[string]*solis.Value, len(values))
	for k, v := range values {
		reg, _ := p.d.Registry.ByKey(k)
		if reg.Store != solis.StoreDaily {
			cacheVals[k] = v
			continue
		}
		a := p.attr.Attribute(k, v.DecodedValue, now)
		if a.Warn != "" {
			p.d.Log.Warn().Str("key", k).Msg(a.Warn)
		}
		if a.Closed != "" {
			p.pendingCloses[k] = maxDay(p.pendingCloses[k], a.Closed)
		}
		if a.Decision == Discard {
			if prev, ok := p.lastDaily[k]; ok {
				cacheVals[k] = prev
			}
			continue
		}
		w.Daily = append(w.Daily, storage.DailyRow{Key: k, Day: a.Day, Value: v.DecodedValue,
			Raw: v.RawValue})
		p.lastDaily[k] = v
		cacheVals[k] = v
	}
	return w, cacheVals
}

// statusChanges returns status rows whose value differs from the last stored one.
func (p *Poller) statusChanges(values map[string]*solis.Value, now time.Time) []storage.StatusRow {
	var rows []storage.StatusRow
	for k, v := range values {
		reg, _ := p.d.Registry.ByKey(k)
		if reg.Store != solis.StoreStatus {
			continue
		}
		if last, ok := p.lastStatus[k]; ok && last == v.RawValue {
			continue
		}
		rows = append(rows, storage.StatusRow{Key: k, Raw: v.RawValue, At: now})
	}
	return rows
}

// committed updates loop state after a successful write and emits PeriodClosed once per
// closed day (on the first closing key or the forced close).
func (p *Poller) committed(w storage.PollWrite) {
	for _, s := range w.Status {
		p.lastStatus[s.Key] = s.Raw
	}
	for _, c := range w.Close {
		delete(p.pendingCloses, c.Key)
		if p.emitted[c.Day] {
			continue
		}
		p.markEmitted(c.Day)
		p.d.Bus.Publish(eventbus.Event{Kind: eventbus.PeriodClosed, Day: c.Day,
			At: p.d.Clock.Now()})
		p.d.Log.Info().Str("day", c.Day).Msg("period closed")
	}
}

// emittedMemory bounds the set of days a PeriodClosed was already emitted for.
const emittedMemory = 8

func (p *Poller) markEmitted(day string) {
	if len(p.emitted) >= emittedMemory {
		p.emitted = make(map[string]bool)
	}
	p.emitted[day] = true
}

func maxDay(a, b string) string {
	if a > b {
		return a
	}
	return b
}
