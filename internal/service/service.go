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

	"github.com/rs/zerolog"

	"github.com/dombyte/solis/internal/health"
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

// NewReadService creates the service.
func NewReadService(d Deps) *ReadService {
	return &ReadService{d: d}
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
		return &KeyError{Key: key, Detail: "not a " + want.String() + " register", Err: ErrWrongKind}
	}
	return nil
}

// DailyHistory returns daily rows of a daily key.
func (s *ReadService) DailyHistory(ctx context.Context, key string, start, end time.Time) (
	[]*storage.DailyDataPoint, error) {
	if err := s.requireStore(key, solis.StoreDaily); err != nil {
		return nil, err
	}
	s.d.Log.Debug().Str("key", key).Str("start", start.Format(time.RFC3339)).Str("end", end.Format(time.RFC3339)).Msg("getting daily history")
	return s.d.Store.GetDailyHistory(ctx, key, start, end)
}

// MonthlyHistory returns monthly rows of a monthly key.
func (s *ReadService) MonthlyHistory(ctx context.Context, key string, start, end time.Time) (
	[]*storage.MonthlyDataPoint, error) {
	if err := s.requireStore(key, solis.StoreMonthly); err != nil {
		return nil, err
	}
	s.d.Log.Debug().Str("key", key).Str("start", start.Format(time.RFC3339)).Str("end", end.Format(time.RFC3339)).Msg("getting monthly history")
	return s.d.Store.GetMonthlyHistory(ctx, key, start, end)
}

// YearlyHistory returns yearly rows of a yearly key.
func (s *ReadService) YearlyHistory(ctx context.Context, key string, start, end time.Time) (
	[]*storage.YearlyDataPoint, error) {
	if err := s.requireStore(key, solis.StoreYearly); err != nil {
		return nil, err
	}
	s.d.Log.Debug().Str("key", key).Str("start", start.Format(time.RFC3339)).Str("end", end.Format(time.RFC3339)).Msg("getting yearly history")
	return s.d.Store.GetYearlyHistory(ctx, key, start, end)
}

// Total returns the stored lifetime value of a total key.
func (s *ReadService) Total(ctx context.Context, key string) (*storage.TotalDataPoint, error) {
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
}

// StatusHistory decodes the stored changes of a status key plus its current value.
func (s *ReadService) StatusHistory(ctx context.Context, key string) (StatusHistory, error) {
	if err := s.requireStore(key, solis.StoreStatus); err != nil {
		return StatusHistory{}, err
	}
	s.d.Log.Debug().Str("key", key).Msg("getting status history")
	reg, _ := s.d.Registry.ByKey(key)
	out := StatusHistory{Key: key, Name: reg.Name, History: []StatusEntry{}}
	points, err := s.d.Store.GetErrorHistory(ctx, key, time.Time{}, maxTime())
	if err != nil {
		// History is best effort; the current value is still useful.
		s.d.Log.Warn().Err(err).Str("key", key).Msg("status history unavailable")
	}
	// Cleared states (no active bits) are kept: "fault cleared" is history too.
	for _, p := range points {
		out.History = append(out.History, StatusEntry{Timestamp: p.Timestamp,
			StatusDecoded: s.d.Decoder.DecodeStatus(key, uint16(p.RawValue))})
	}
	if v := s.d.Cache.Get(key); v != nil && v.StatusDecoded != nil {
		out.History = append(out.History, StatusEntry{
			Timestamp: v.Timestamp.Format(time.RFC3339), StatusDecoded: v.StatusDecoded})
	}
	sort.SliceStable(out.History, func(i, j int) bool {
		return out.History[i].Timestamp > out.History[j].Timestamp
	})
	return out, nil
}

// maxTime is the upper bound of an unbounded history query.
func maxTime() time.Time {
	const farFuture = 9999
	return time.Date(farFuture, time.December, 31, 0, 0, 0, 0, time.UTC)
}
