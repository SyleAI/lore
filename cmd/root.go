package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/loreteam/lore/internal/config"
	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/gitcmd"
	"github.com/spf13/cobra"
)

var (
	flagJSON  bool
	flagQuiet bool
)

// rootCmd is the base command for the lore CLI.
var rootCmd = &cobra.Command{
	Use:     "lore",
	Short:   "Lore — AI-assisted ticket and knowledge management for git repositories",
	Version: "0.1.0-dev",
	Long: `Lore is a CLI tool for managing AI-assisted development workflows.
It stores all state in git object storage, making it fully offline-capable
and requiring no external database.`,
	SilenceErrors: true,
	SilenceUsage:  true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		// Try to find git root; commands that need it will check gitRootFromContext.
		gitRoot, err := gitcmd.FindGitRoot(".")
		if err != nil {
			return nil
		}

		loreDir := gitRoot + "/.lore"

		// Load config (falls back to defaults if .lore/config.yaml does not exist yet).
		cfg, err := config.Load(loreDir)
		if err != nil {
			cfg = config.Default()
		}

		// Build emitter based on config.
		// Always append to the file log (so `lore events --follow` works),
		// then optionally also write to stdout.
		fileEmitter := event.NewFileEmitter(loreDir + "/events.log")
		var emitter event.Emitter
		if flagQuiet {
			emitter = fileEmitter
		} else if flagJSON || cfg.Events.Adapter == "stdout" {
			emitter = event.NewFanoutEmitter(fileEmitter, event.NewStdoutEmitter())
		} else {
			emitter = fileEmitter
			if a := cfg.Events.Adapter; a != "" && a != "noop" && a != "none" {
				fmt.Fprintf(os.Stderr, "warning: unknown event adapter %q, events disabled\n", a)
			}
		}

		ctx = context.WithValue(ctx, gitRootKey{}, gitRoot)
		ctx = context.WithValue(ctx, loreDirKey{}, loreDir)
		ctx = context.WithValue(ctx, configKey{}, cfg)
		ctx = context.WithValue(ctx, event.EmitterKey, emitter)
		cmd.SetContext(ctx)

		return nil
	},
}

// context key types — unexported to prevent collisions.
type gitRootKey struct{}
type loreDirKey struct{}
type configKey struct{}

// Execute runs the root command. Errors are printed to stderr before exiting.
func Execute() {
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "output events as newline-delimited JSON")
	rootCmd.PersistentFlags().BoolVar(&flagQuiet, "quiet", false, "suppress progress output and events")
}

// gitRootFromContext retrieves the git root from the command context.
// Returns empty string if not set.
func gitRootFromContext(ctx context.Context) string {
	v, _ := ctx.Value(gitRootKey{}).(string)
	return v
}

// loreDirFromContext retrieves the .lore directory path from the command context.
// Returns empty string if not set.
func loreDirFromContext(ctx context.Context) string {
	v, _ := ctx.Value(loreDirKey{}).(string)
	return v
}
