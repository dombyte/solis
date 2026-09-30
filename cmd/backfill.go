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
func runBackfill(args []string, configPath string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("backfill", flag.ContinueOnError)
	fs.SetOutput(stderr)
	years := fs.Int("years", 0, "closed years to recompute in addition to the current year")
	force := fs.Bool("force", false, "also recompute periods without complete daily history "+
		"(overwrites stored values with partial sums) and rebuild the total baseline from "+
		"all daily rows, pre-cutover years included")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0 // -h/--help printed the usage
		}
		return 1
	}
	opts := maintenance.Options{Years: *years, Force: *force}
	if err := backfill(opts, configPath, stdout, stderr); err != nil {
		_, _ = fmt.Fprintf(stderr, "backfill: %v\n", err)
		return 1
	}
	return 0
}

func backfill(opts maintenance.Options, configPath string, stdout, stderr io.Writer) error {
	cfg, err := loadConfig(configPath)
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
	return maintenance.RunBackfill(ctx, env, opts)
}
