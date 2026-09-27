package logging

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestParseLevel(t *testing.T) {
	tests := map[string]zerolog.Level{
		"DEBUG": zerolog.DebugLevel, "debug": zerolog.DebugLevel,
		"INFO": zerolog.InfoLevel, "Warn": zerolog.WarnLevel,
		"error": zerolog.ErrorLevel, "FATAL": zerolog.FatalLevel,
		"": zerolog.InfoLevel, "nonsense": zerolog.InfoLevel,
	}
	for in, want := range tests {
		assert.Equal(t, want, ParseLevel(in), in)
	}
}

func TestNew_JSONRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "WARN", false)
	log.Info().Msg("hidden")
	log.Warn().Msg("shown")
	assert.NotContains(t, buf.String(), "hidden")
	assert.Contains(t, buf.String(), `"message":"shown"`)
}

func TestNew_PrettyAndComponent(t *testing.T) {
	var buf bytes.Buffer
	log := Component(New(&buf, "DEBUG", true), "poller")
	log.Debug().Msg("hello")
	assert.Contains(t, buf.String(), "hello")
	assert.Contains(t, buf.String(), "poller")
}

func TestNew_IsIndependentPerInstance(t *testing.T) {
	var a, b bytes.Buffer
	la := New(&a, "ERROR", false)
	lb := New(&b, "DEBUG", false)
	la.Info().Msg("x")
	lb.Info().Msg("y")
	assert.Empty(t, a.String())
	assert.Contains(t, b.String(), "y")
}
