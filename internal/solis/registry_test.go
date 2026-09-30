package solis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/period"
)

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry()
	require.NoError(t, err)
	return r
}

// TestBlockPlanGolden pins the read plan: 3 reads instead of v2's 5.
// Update deliberately when register addresses change.
func TestBlockPlanGolden(t *testing.T) {
	r := newTestRegistry(t)
	assert.Equal(t, []Block{
		{Start: 33035, Count: 24}, // pv_energy_daily .. pv_total_power
		{Start: 33095, Count: 85}, // status, faults, grid_power, SOC, BMS, loads, dailies
		{Start: 33586, Count: 11}, // household/backup daily
	}, r.Blocks())
	for _, b := range r.Blocks() {
		assert.LessOrEqual(t, b.Count, uint16(MaxBlockLen))
	}
}

func TestRegistry_Contents(t *testing.T) {
	r := newTestRegistry(t)

	assert.Len(t, r.DailyKeys(), 8)
	assert.Len(t, r.ByStore(StoreStatus), 9)
	assert.Len(t, r.ByStore(StoreMonthly), 9)
	assert.Len(t, r.ByStore(StoreYearly), 9)
	assert.Len(t, r.ByStore(StoreTotal), 9)
	assert.Len(t, r.ByStore(StoreNone), 8)
	assert.Len(t, r.Edges(period.Monthly), 8)
	assert.Len(t, r.Edges(period.Yearly), 8)
	assert.Len(t, r.Edges(period.Total), 8)
	assert.Len(t, r.NetPairs(period.Daily), 1)
	assert.Len(t, r.Keys(), len(r.All()))

	grid, ok := r.ByKey("grid_power")
	require.True(t, ok)
	assert.Equal(t, uint16(33130), grid.Address)
	assert.Equal(t, uint16(2), grid.Count())

	net, ok := r.ByKey("grid_energy_daily")
	require.True(t, ok)
	assert.True(t, net.Net)
	assert.True(t, net.Computed())
	assert.Equal(t, StoreDaily, net.Store)
	assert.False(t, net.Polled())

	monthly, _ := r.ByKey("pv_energy_monthly")
	assert.Equal(t, "PV Energy Monthly", monthly.Name)
	assert.True(t, monthly.Computed())
	consumption, _ := r.ByKey("energy_consumption_monthly")
	assert.Equal(t, "Energy Consumption Monthly (Computed)", consumption.Name)

	byAddr, ok := r.ByAddress(33095)
	require.True(t, ok)
	assert.Equal(t, "solis_status", byAddr.Key)
	_, ok = r.ByAddress(1)
	assert.False(t, ok)
}

func TestNewRegistry_ValidationFailures(t *testing.T) {
	t.Parallel()
	daily := Register{Key: "d", Address: 10, DataType: Uint16, Scale: 1, Store: StoreDaily}
	monthly := Register{Key: "m", DataType: Uint32, Scale: 1, Store: StoreMonthly}
	edge := Edge{Level: period.Monthly, Source: "d", Target: "m"}
	tests := []struct {
		name  string
		regs  []Register
		edges []Edge
		nets  []NetPair
	}{
		{"duplicate key", []Register{daily, daily}, nil, nil},
		{"duplicate address", []Register{daily, {Key: "x", Address: 10, Scale: 1}}, nil, nil},
		{"overlap", []Register{
			{Key: "a", Address: 10, DataType: Uint32, Scale: 1},
			{Key: "b", Address: 11, Scale: 1},
		}, nil, nil},
		{"zero scale", []Register{{Key: "a", Address: 1}}, nil, nil},
		{"invalid store", []Register{{Key: "a", Address: 1, Scale: 1, Store: Store(99)}}, nil, nil},
		{
			"net polled",
			[]Register{{Key: "a", Address: 1, Scale: 1, Store: StoreDaily, Net: true}},
			nil, nil,
		},
		{"computed with address", []Register{{
			Key: "a", Address: 1, Scale: 1,
			Store: StoreMonthly,
		}}, nil, nil},
		{"status without address", []Register{{Key: "a", Scale: 1, Store: StoreStatus}}, nil, nil},
		{"computed without definition", []Register{daily, monthly}, nil, nil},
		{
			"edge bad source",
			[]Register{daily, monthly},
			[]Edge{{Level: period.Monthly, Source: "x", Target: "m"}},
			nil,
		},
		{
			"edge bad target level",
			[]Register{daily, monthly},
			[]Edge{{Level: period.Yearly, Source: "d", Target: "m"}},
			nil,
		},
		{"edge defined twice", []Register{daily, monthly}, []Edge{edge, edge}, nil},
		{"net bad source", []Register{daily, monthly, {
			Key: "n", Scale: 1, Store: StoreMonthly,
			Net: true,
		}}, []Edge{edge}, []NetPair{{
			Level: period.Monthly, Export: "m",
			Import: "zz", Target: "n",
		}}},
		{
			"net bad target",
			[]Register{daily, monthly},
			[]Edge{edge},
			[]NetPair{{Level: period.Monthly, Export: "m", Import: "m", Target: "m"}},
		}, // Near the top of the address space Address+Count must not wrap (review nit).
		{"runs past 65535", []Register{{Key: "a", Address: 65535, DataType: Uint32, Scale: 1}}, nil, nil},
		{"overlap across wrap", []Register{
			{Key: "a", Address: 65534, DataType: Uint16, Scale: 1},
			{Key: "b", Address: 65535, DataType: Uint16, Scale: 1},
			{Key: "c", Address: 65533, DataType: Uint32, Scale: 1},
		}, nil, nil},
		// A total whose source has no yearly edge would fold 0 into the baseline (AGG-L2).
		{"total without yearly source", []Register{daily, {
			Key: "t", DataType: Uint32, Scale: 1,
			Store: StoreTotal,
		}}, []Edge{{Level: period.Total, Source: "d", Target: "t"}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := newRegistry(tt.regs, tt.edges, tt.nets)
			assert.ErrorIs(t, err, ErrInvalidRegistry)
		})
	}

	_, err := newRegistry([]Register{daily, monthly}, []Edge{edge}, nil)
	assert.NoError(t, err)

	_, err = newRegistry([]Register{daily, daily}, nil, nil)
	var re *RegistryError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, daily.Key, re.Key)
	assert.Contains(t, re.Error(), "invalid register table: duplicate key")
}

func TestPlanBlocks(t *testing.T) {
	regs := []Register{
		{Key: "a", Address: 100, DataType: Uint16},
		{Key: "b", Address: 101, DataType: Uint32},
		{Key: "c", Address: 110, DataType: Uint16}, // gap 7
		{Key: "d", Address: 200, DataType: Uint16}, // gap too large
		{Key: "e", Address: 203, DataType: Uint16},
	}
	assert.Equal(t, []Block{{100, 11}, {200, 4}}, PlanBlocks(regs, 8, 125))
	assert.Equal(t, []Block{{100, 3}, {110, 1}, {200, 1}, {203, 1}}, PlanBlocks(regs, 1, 125))
	assert.Equal(t, []Block{{100, 3}, {110, 1}, {200, 4}}, PlanBlocks(regs, 8, 10))
	assert.Empty(t, PlanBlocks(nil, 8, 125))
}

func TestStoreAndTypeStrings(t *testing.T) {
	names := map[Store]string{
		StoreNone: "none", StoreDaily: "daily", StoreMonthly: "monthly",
		StoreYearly: "yearly", StoreTotal: "total", StoreStatus: "status", Store(9): "invalid",
	}
	for s, want := range names {
		assert.Equal(t, want, s.String())
	}
	for _, s := range []Store{StoreDaily, StoreMonthly, StoreYearly, StoreTotal} {
		l, ok := s.Level()
		require.True(t, ok)
		assert.Equal(t, s, StoreForLevel(l))
	}
	_, ok := StoreStatus.Level()
	assert.False(t, ok)

	types := map[DataType]string{
		Uint16: "Uint16", Int16: "Int16", Uint32: "Uint32",
		Int32: "Int32", Float32: "Float32", Bool: "Bool", DataType(42): "Unknown",
	}
	for d, want := range types {
		assert.Equal(t, want, d.String())
	}
	assert.Equal(t, uint16(2), Float32.Count())
	assert.Equal(t, uint16(1), Bool.Count())
}
