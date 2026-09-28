package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/dombyte/solis/internal/app"
	"github.com/dombyte/solis/internal/logging"
	"github.com/dombyte/solis/internal/maintenance"
)

// runBackfill parses `backfill --years N` and runs the job; exit code 0/1.
func runBackfill(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("backfill", flag.ContinueOnError)
	fs.SetOutput(stderr)
	years := fs.Int("years", 0, "closed years to recompute in addition to the current year")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0 // -h/--help printed the usage
		}
		return 1
	}
	if err := backfill(*years, stdout, stderr); err != nil {
		_, _ = fmt.Fprintf(stderr, "backfill: %v\n", err)
		return 1
	}
	return 0
}

func backfill(years int, stdout, stderr io.Writer) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	log := logging.Component(logging.New(stderr, cfg.App.Debug, true), "maintenance")
	env, err := app.CreateBackfillEnv(cfg, stdout, log)
	if err != nil {
		return err
	}
	// Ctrl-C/SIGTERM cancels the job; its single transaction then rolls back.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return maintenance.RunBackfill(ctx, env, years)
}
