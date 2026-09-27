package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/dombyte/solis/internal/database"
	"github.com/dombyte/solis/internal/logging"
	"github.com/dombyte/solis/internal/maintenance"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
	"github.com/dombyte/solis/internal/utils"
)

// runBackfill parses `backfill --years N` and runs the job; exit code 0/1.
func runBackfill(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("backfill", flag.ContinueOnError)
	fs.SetOutput(stderr)
	years := fs.Int("years", 0, "closed years to recompute in addition to the current year")
	if err := fs.Parse(args); err != nil {
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
	reg, err := solis.NewRegistry()
	if err != nil {
		return err
	}
	clock := utils.NewRealClock()
	st := &cfg.Storage
	backupCfg := &database.BackupConfig{Enabled: true, MaxBackups: st.MaxBackups}
	return maintenance.RunBackfill(context.Background(), maintenance.Env{
		DBPath: st.Path,
		Backup: func() (string, error) { return database.CreateBackup(st.Path, backupCfg, log) },
		OpenStore: func() (maintenance.Store, func() error, error) {
			mgr := database.NewManager(st, backupCfg, clock, log)
			if err := mgr.Prepare(context.Background()); err != nil {
				return nil, nil, err
			}
			s, err := storage.New(st, reg, clock, log)
			if err != nil {
				return nil, nil, err
			}
			return s, s.Close, nil
		},
		Registry: reg, Now: clock.Now(), Out: stdout, Log: log,
	}, years)
}
