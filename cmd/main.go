// Package main is the Solis monitor entry point. Without a subcommand it runs the server
// inside a restart loop; `solis backfill --years N` runs a maintenance job and exits 0/1.
package main

import (
	"fmt"
	"io"
	"os"
	"time"

	_ "time/tzdata" // fallback zoneinfo so TZ=Europe/Berlin never silently becomes UTC

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/logging"
)

// Restart loop limits (spec §2.1: unchanged from v2).
const (
	maxRestarts  = 100
	restartDelay = 5 * time.Second
	configPath   = "config.yaml"
)

func main() {
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
}

// dispatch selects the subcommand and returns the process exit code.
func dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "serve" {
		return restartLoop(stderr, runServer, maxRestarts, restartDelay)
	}
	switch args[0] {
	case "backfill":
		return runBackfill(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	default:
		// #nosec G705 -- writes to the process's own stderr, not an HTTP response
		_, _ = fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printUsage(stderr)
		return 1
	}
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `usage:
  solis                     run the server
  solis backfill --years N  recompute monthly+yearly values (app must be stopped)
`)
}

// restartLoop runs run until it returns nil (clean shutdown). Errors and panics
// (including health-fatal escalations) restart the whole app after restartDelay; the
// shared ceiling of maxRestarts ends the process with exit code 1.
func restartLoop(stderr io.Writer, run func() error, limit int, delay time.Duration) int {
	log := logging.New(stderr, "INFO", true)
	for attempt := 1; attempt <= limit; attempt++ {
		err := runRecovered(run)
		if err == nil {
			log.Info().Msg("clean shutdown")
			return 0
		}
		log.Error().Err(err).Int("attempt", attempt).Dur("delay", delay).
			Msg("application stopped, restarting")
		time.Sleep(delay)
	}
	log.Error().Int("max", limit).Msg("maximum restart count reached, exiting")
	return 1
}

func runRecovered(run func() error) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	return run()
}

// loadConfig loads and validates config.yaml.
func loadConfig() (*config.AppConfig, error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	return cfg, nil
}
