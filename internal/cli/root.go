// Package cli wires the cobra command tree. One file per command; this file
// holds the root command, the shared App state, and global flag handling.
package cli

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stozo04/speediance-cli/internal/config"
	"github.com/stozo04/speediance-cli/internal/version"
)

// App holds process-wide state shared by every command: resolved config, a
// stderr logger, and the values of the global persistent flags. Config is
// resolved lazily so commands that need no credentials (version, completion)
// never fail on a malformed config.json.
type App struct {
	configPath string // --config
	verbose    bool   // --verbose/-v

	cfg    *config.Config
	logger *slog.Logger
}

// NewRootCmd builds the root command and registers every subcommand. A fresh
// App and command tree per call keeps tests isolated from sticky global flags
// (GOAL.md §16).
func NewRootCmd() *cobra.Command {
	app := &App{}

	root := &cobra.Command{
		Use:   "speediance-cli",
		Short: "Read workouts and push training programs to your Speediance machine",
		Long: "speediance-cli is a drop-in CLI for the (unofficial) Speediance / Gym Monster\n" +
			"cloud API. It reads completed workouts and pushes custom training programs that\n" +
			"appear on the machine.",
		// Runtime failures print one error line to stderr ourselves; never dump
		// usage on them, and never let cobra also print the error (GOAL.md §12).
		SilenceUsage:  true,
		SilenceErrors: true,
		// Default action with no subcommand: show help (exit 0).
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		// Initialize the stderr logger before any command body runs.
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			app.logger = newLogger(cmd.ErrOrStderr(), app.verbose)
			return nil
		},
	}

	root.PersistentFlags().StringVar(&app.configPath, "config", "",
		"path to config.json (overrides discovery and "+config.EnvConfig+")")
	root.PersistentFlags().BoolVarP(&app.verbose, "verbose", "v", false,
		"verbose logging to stderr")

	// Root --version: short one-liner, no subcommand needed (GOAL.md §6, §9.8).
	root.Version = version.Info().String()
	root.SetVersionTemplate("{{.Version}}\n")

	addCommands(app, root)
	return root
}

// addCommands registers every subcommand on root. Each command lives in its own
// file and exposes a newXxxCmd(app) constructor.
func addCommands(app *App, root *cobra.Command) {
	root.AddCommand(
		newLoginCmd(app),
		newWorkoutsCmd(app),
		newSessionCmd(app),
		newLibraryCmd(app),
		newPushCmd(app),
		newSyncCmd(app),
		newConfigCmd(app),
		newVersionCmd(app),
		newCompletionCmd(),
	)
}

// resolveConfig loads configuration once and caches it on the App. Errors are
// tagged with the config exit code so main reports them consistently.
func (a *App) resolveConfig() (*config.Config, error) {
	if a.cfg != nil {
		return a.cfg, nil
	}
	cfg, err := config.Load(config.Options{ConfigPath: a.configPath})
	if err != nil {
		return nil, withCode(ExitConfig, err)
	}
	a.cfg = cfg
	return cfg, nil
}

// newLogger returns a slog text logger writing to w (stderr). Default level is
// Warn; --verbose drops to Debug; LOG_LEVEL overrides either (GOAL.md §13).
func newLogger(w io.Writer, verbose bool) *slog.Logger {
	level := slog.LevelWarn
	if verbose {
		level = slog.LevelDebug
	}
	if v, ok := os.LookupEnv("LOG_LEVEL"); ok {
		if parsed, ok := parseLevel(v); ok {
			level = parsed
		}
	}
	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})
	return slog.New(h)
}

func parseLevel(s string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelWarn, false
	}
}

// fprintln writes a human line to the given writer, ignoring write errors (a
// broken stdout/stderr pipe is not actionable at the CLI layer).
func fprintln(w io.Writer, args ...any) {
	_, _ = fmt.Fprintln(w, args...)
}

// fprintf is the Printf-style sibling of fprintln.
func fprintf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}
