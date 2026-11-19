package main

import (
	"context"
	"io"

	"github.com/dombyte/solis/internal/app"
	"github.com/dombyte/solis/internal/logging"
)

// runServer runs the application (server mode) until ctx is cancelled or it fails.
func runServer(ctx context.Context, configPath string, stderr io.Writer) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	root := logging.New(stderr, cfg.App.Debug, true)
	for _, w := range cfg.Warnings {
		root.Warn().Str("component", "config").Msg(w)
	}
	return app.Run(ctx, cfg, buildInfo(), root)
}
