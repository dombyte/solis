package main

import (
	"context"
	"os"

	"github.com/dombyte/solis/internal/app"
	"github.com/dombyte/solis/internal/logging"
)

// runServer runs one application lifetime (server mode).
func runServer() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	root := logging.New(os.Stderr, cfg.App.Debug, true)
	for _, w := range cfg.Warnings {
		root.Warn().Str("component", "config").Msg(w)
	}
	return app.Run(context.Background(), cfg, root)
}
