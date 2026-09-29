package app

import (
	"context"
	"io"

	"github.com/rs/zerolog"

	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/database"
	"github.com/dombyte/solis/internal/maintenance"
	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/storage"
	"github.com/dombyte/solis/internal/util"
)

// CreateBackfillEnv wires the `solis backfill` job: a verified backup (rotated to
// max_backups like the server's), then migrations and storage are opened only after the
// job holds the exclusive lock. cmd only dispatches; concrete types live here.
func CreateBackfillEnv(cfg *config.AppConfig, out io.Writer, log zerolog.Logger) (
	maintenance.Env, error,
) {
	reg, err := solis.NewRegistry()
	if err != nil {
		return maintenance.Env{}, err
	}
	clock := util.NewRealClock()
	st := cfg.Storage
	backupCfg := &database.BackupConfig{Enabled: true, MaxBackups: st.MaxBackups}
	return maintenance.Env{
		DBPath: st.Path,
		Backup: func(ctx context.Context) (string, error) {
			path, err := database.WriteBackup(ctx, st.Path, clock.Now(), log)
			if err == nil { // rotate like the server does, so repeated jobs don't pile up
				err = database.CleanupBackups(st.Path, st.MaxBackups, log)
			}
			return path, err
		},
		OpenStore: func(ctx context.Context) (maintenance.Store, func() error, error) {
			mgr := database.NewManager(DatabaseSettings(st), backupCfg, clock, log)
			if err := mgr.Prepare(ctx); err != nil {
				return nil, nil, err
			}
			s, err := storage.New(ctx, StorageSettings(st), reg, clock, log)
			if err != nil {
				return nil, nil, err
			}
			return s, s.Close, nil
		},
		Registry: reg, Now: clock.Now(), Out: out,
	}, nil
}
