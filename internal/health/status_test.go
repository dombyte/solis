package health_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/dombyte/solis/internal/health"
	"github.com/dombyte/solis/internal/health/mocks"
	"github.com/dombyte/solis/internal/util/clocktest"
)

func TestStatus(t *testing.T) {
	rep := mocks.NewMockReporter(t)
	t0 := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	clk := clocktest.New(t0)
	s := health.NewStatus(rep, clk)
	assert.Equal(t, health.Healthy, s.State())
	assert.True(t, s.LastBeat().IsZero())

	clk.Advance(time.Second)
	s.Beat()
	assert.True(t, s.LastBeat().Equal(t0.Add(time.Second)))

	rep.EXPECT().Report(health.Recovering, "modbus down").Once()
	s.Set(health.Recovering, "modbus down")
	s.Set(health.Recovering, "still down") // unchanged: no report
	assert.Equal(t, health.Recovering, s.State())

	rep.EXPECT().Report(health.Healthy, "").Once()
	s.Set(health.Healthy, "")
}
