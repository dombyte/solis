// Package main is the Solis monitor entry point. Without a subcommand it runs the server
// once and exits 0 on a clean (signal) shutdown and 1 on any error, including a fatal
// health escalation, so the container runtime's restart policy restarts it; `solis
// backfill --years N` runs a maintenance job and exits 0/1. `-config <path>` (before the
// subcommand) selects the configuration file, default ./config.yaml.
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

	_ "time/tzdata" // fallback zoneinfo so TZ=Europe/Berlin never silently becomes UTC

	"github.com/dombyte/solis/internal/app"
	"github.com/dombyte/solis/internal/buildinfo"
	"github.com/dombyte/solis/internal/config"
	"github.com/dombyte/solis/internal/logging"
)

const defaultConfigPath = "config.yaml"

// Build information, set by the Makefile and goreleaser via -ldflags "-X main.Version=…".
// -X can only set package-level string variables, hence the only globals in the binary.
//
//nolint:gochecknoglobals // written by the linker at build time, read-only afterwards
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
	GoVersion = "unknown"
)

// buildInfo collects the linker-set build information.
func buildInfo() buildinfo.Info {
	return buildinfo.Info{
		Version: Version, Commit: Commit, BuildDate: BuildDate, GoVersion: GoVersion,
	}
}

func main() {
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
}

// dispatch parses the global flags, selects the subcommand and returns the process exit
// code.
func dispatch(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("solis", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { printUsage(stderr) }
	configPath := fs.String("config", defaultConfigPath, "path of the configuration file")
	version := fs.Bool("version", false, "print the build information")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0 // -h/--help printed the usage
		}
		return 1
	}
	args = fs.Args()
	if *version {
		args = []string{"version"}
	}
	return runCommand(args, *configPath, stdout, stderr)
}

// runCommand runs the subcommand in args[0] (the server when args is empty).
func runCommand(args []string, configPath string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "serve" {
		return serve(stderr, func(ctx context.Context) error { return runServer(ctx, configPath) })
	}
	switch args[0] {
	case "backfill":
		return runBackfill(args[1:], configPath, stdout, stderr)
	case "help":
		printUsage(stdout)
		return 0
	case "version":
		_, _ = fmt.Fprintln(stdout, buildInfo().String())
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
  solis [-config PATH]                     run the server
  solis [-config PATH] backfill --years N  recompute monthly+yearly values (app must be stopped)
  solis version                            print the build information

  -config PATH  configuration file (default config.yaml)
`)
}

// serve runs the server once under a context cancelled by SIGINT/SIGTERM. There is no
// in-process restart: any error (startup, fatal health escalation, shutdown timeout) is
// exit code 1, and the container runtime (restart: unless-stopped) starts a fresh
// process. A panic on another goroutine crashes the process, which restarts it too.
func serve(stderr io.Writer, run func(context.Context) error) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := logging.New(stderr, "INFO", true)
	log.Info().Str("version", Version).Str("commit", Commit).Str("built", BuildDate).
		Str("go", GoVersion).Msg("starting")
	if err := run(ctx); err != nil {
		log.Error().Err(err).Msg("application stopped with an error, exiting with code 1")
		return 1
	}
	log.Info().Msg("clean shutdown")
	return 0
}

// loadConfig loads and validates the configuration file at path.
func loadConfig(configPath string) (*config.AppConfig, error) {
	cfg, err := config.LoadConfig(configPath, app.ConfigRules()...)
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	return cfg, nil
}
