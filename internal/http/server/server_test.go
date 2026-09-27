package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/health"
)

func TestServeProbeStop(t *testing.T) {
	s := New(&config.AppSettings{Port: 1, Timeout: time.Second},
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "ok")
		}), zerolog.Nop())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, s.Serve(ln))

	resp, err := http.Get("http://" + ln.Addr().String())
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	assert.Equal(t, "ok", string(body))

	st, _ := s.Probe()
	assert.Equal(t, health.Healthy, st)
	require.NoError(t, s.Stop(context.Background()))
	st, _ = s.Probe()
	assert.Equal(t, health.Healthy, st, "a clean shutdown is not a failure")
}

func TestProbeReportsUnexpectedServeError(t *testing.T) {
	s := New(&config.AppSettings{Port: 1, Timeout: time.Second}, http.NotFoundHandler(),
		zerolog.Nop())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, ln.Close()) // Serve on a closed listener fails immediately
	require.NoError(t, s.Serve(ln))
	require.Eventually(t, func() bool {
		st, _ := s.Probe()
		return st == health.Failed
	}, time.Second, time.Millisecond)
}

func TestStartBusyPortFails(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port
	s := New(&config.AppSettings{Port: port, Timeout: time.Second}, http.NotFoundHandler(),
		zerolog.Nop())
	assert.Error(t, s.Start())
}
