package storage

import (
	"context"
	"time"

	"github.com/dombyte/solis/internal/history"
	"github.com/dombyte/solis/internal/period"
	"github.com/dombyte/solis/internal/solis"
)

// KeyLookup is the register metadata storage needs for validation and scaling.
type KeyLookup interface {
	ByKey(key string) (solis.Register, bool)
	DailyKeys() []string
}

// PollerStore is the poller's write domain: non-net daily rows (max of the open day),
// status changes and per-key day closes.
type PollerStore interface {
	// WritePoll persists one poll cycle in a single transaction.
	WritePoll(ctx context.Context, w PollWrite) error
	// Seed returns the state needed to seed day attribution and status change detection
	// after a restart (one query per table, not per key).
	Seed(ctx context.Context, days []string) (Seed, error)
}

// AggregatorStore is the aggregator's write domain: computed monthly/yearly/total rows,
// net daily rows, freeze marks and the total baseline.
type AggregatorStore interface {
	// HasDailyData reports whether any daily row exists (cold-start gate).
	HasDailyData(ctx context.Context) (bool, error)
	// SumDaily sums daily values of key for from <= date <= to ("" from = unbounded).
	SumDaily(ctx context.Context, key, from, to string) (float64, error)
	// CloseState returns watermarks, cutover and baseline.
	CloseState(ctx context.Context) (CloseState, error)
	// WriteComputed upserts computed rows, applies freezes and a baseline fold atomically.
	WriteComputed(ctx context.Context, w ComputedWrite) error
}

// ReadStore is the read-only view used by the HTTP service.
type ReadStore interface {
	GetDailyHistory(ctx context.Context, key string, start, end time.Time) (
		[]*history.DailyDataPoint, error)
	GetMonthlyHistory(ctx context.Context, key string, start, end time.Time) (
		[]*history.MonthlyDataPoint, error)
	GetYearlyHistory(ctx context.Context, key string, start, end time.Time) (
		[]*history.YearlyDataPoint, error)
	GetTotalHistory(ctx context.Context, key string) (*history.TotalDataPoint, error)
	GetErrorHistory(ctx context.Context, key string, start, end time.Time) (
		[]*history.ErrorDataPoint, error)
}

// DailyRow is one daily max-write.
type DailyRow struct {
	// Key is a non-net daily register key.
	Key string
	// Day is the attributed day key.
	Day string
	// Value is the decoded value.
	Value float64
	// Raw is the raw value.
	Raw float64
}

// StatusRow is one status/fault change.
type StatusRow struct {
	// Key is a status register key.
	Key string
	// Raw is the raw register value.
	Raw float64
	// At is the poll instant.
	At time.Time
}

// DayClose advances the closed-day watermark of one daily key.
type DayClose struct {
	// Key is the daily register key.
	Key string
	// Day is the last closed day.
	Day string
}

// PollWrite is everything one poll cycle persists.
type PollWrite struct {
	// Daily are the max-writes of the open days.
	Daily []DailyRow
	// Status are the changed status registers.
	Status []StatusRow
	// Close are the day closes detected in this poll.
	Close []DayClose
}

// Seed is the persisted state the poller seeds itself from.
type Seed struct {
	// Daily maps day -> key -> stored value for the requested days.
	Daily map[string]map[string]float64
	// Status maps status key -> last stored raw value.
	Status map[string]float64
	// Closed maps daily key -> closed-through watermark.
	Closed map[string]string
}

// PeriodRow is one computed upsert (REPLACE for open periods).
type PeriodRow struct {
	// Level is Daily (net keys only), Monthly, Yearly or Total.
	Level period.Level
	// Key is the computed register key.
	Key string
	// Period is the period key ("" for Total).
	Period string
	// Value is the full-precision value.
	Value float64
}

// Freeze marks a period final: later writes into it are rejected.
type Freeze struct {
	// Level is Daily (net keys), Monthly or Yearly.
	Level period.Level
	// Period is the last frozen period key.
	Period string
}

// BaselineFold adds a closed year's sums to the total baseline (idempotent per year).
type BaselineFold struct {
	// Year is the closed year.
	Year string
	// Add maps total key -> sum of the year's daily rows.
	Add map[string]float64
}

// ComputedWrite is one aggregator run's persistence.
type ComputedWrite struct {
	// Rows are the upserts.
	Rows []PeriodRow
	// Freezes are applied after the rows.
	Freezes []Freeze
	// Folds are applied last, oldest year first.
	Folds []BaselineFold
	// At timestamps total rows.
	At time.Time
}

// CloseState is the persisted close/baseline state.
type CloseState struct {
	// Cutover is the v3 cutover day ("" before EnsureCutover).
	Cutover string
	// FrozenMonth is the last frozen month.
	FrozenMonth string
	// FrozenYear is the last frozen year.
	FrozenYear string
	// FrozenNetDay is the last frozen net daily day.
	FrozenNetDay string
	// ClosedThrough is the minimum closed day over all daily keys ("" if any is open).
	ClosedThrough string
	// BaselineYear is the last year folded into the baseline.
	BaselineYear string
	// Baseline maps total key -> baseline value.
	Baseline map[string]float64
}
