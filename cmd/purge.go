package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/loreteam/lore/internal/gitcmd"
	"github.com/spf13/cobra"
)

var purgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Remove tickets, orchestrators, or everything",
	Long: `Purge removes lore state from the repository. This cannot be undone.

By default (no flags) purge removes everything: all tickets, the event log,
and all installed orchestrators.

Use --tickets or --orchestrators to limit scope.`,
	Args: cobra.NoArgs,
	RunE: runPurge,
}

var (
	flagForce        bool
	flagTickets      bool
	flagOrchestrators bool
)

func init() {
	purgeCmd.Flags().BoolVar(&flagForce, "force", false, "skip confirmation prompt")
	purgeCmd.Flags().BoolVar(&flagTickets, "tickets", false, "remove only tickets and the event log")
	purgeCmd.Flags().BoolVar(&flagOrchestrators, "orchestrators", false, "remove only installed orchestrators")
	rootCmd.AddCommand(purgeCmd)
}

func runPurge(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	// Determine scope.
	doTickets := flagTickets
	doOrchestrators := flagOrchestrators
	if !doTickets && !doOrchestrators {
		// No flags — purge everything.
		doTickets = true
		doOrchestrators = true
	}

	// Build confirmation message.
	if !flagForce {
		var what []string
		if doTickets {
			what = append(what, "all tickets and the event log")
		}
		if doOrchestrators {
			what = append(what, "all installed orchestrators")
		}
		fmt.Printf("This will delete %s. Continue? [y/N] ", strings.Join(what, " and "))
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(strings.TrimSpace(response)) != "y" {
			fmt.Println("aborted")
			return nil
		}
	}

	loreDir := loreDirFromContext(ctx)

	if doTickets {
		openRefs, err := gitcmd.ListRefs(ctx, gitRoot, "refs/tickets/open/")
		if err != nil {
			return fmt.Errorf("lore purge: %w", err)
		}
		doneRefs, err := gitcmd.ListRefs(ctx, gitRoot, "refs/tickets/done/")
		if err != nil {
			return fmt.Errorf("lore purge: %w", err)
		}

		deleted := 0
		for ref := range openRefs {
			if err := gitcmd.DeleteRef(ctx, gitRoot, ref); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to delete ref %s: %v\n", ref, err)
				continue
			}
			deleted++
		}
		for ref := range doneRefs {
			if err := gitcmd.DeleteRef(ctx, gitRoot, ref); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to delete ref %s: %v\n", ref, err)
				continue
			}
			deleted++
		}

		if loreDir != "" {
			eventsLog := loreDir + "/events.log"
			if err := os.Truncate(eventsLog, 0); err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "warning: could not clear events log: %v\n", err)
			}
		}

		fmt.Printf("purged %d ticket(s)\n", deleted)
	}

	if doOrchestrators {
		orchsDir := filepath.Join(gitRoot, ".lore", "orchestrators")
		entries, err := os.ReadDir(orchsDir)
		if os.IsNotExist(err) {
			fmt.Println("no orchestrators installed")
		} else if err != nil {
			return fmt.Errorf("lore purge: read orchestrators dir: %w", err)
		} else {
			count := 0
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				if err := os.RemoveAll(filepath.Join(orchsDir, entry.Name())); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not remove orchestrator %s: %v\n", entry.Name(), err)
					continue
				}
				count++
			}
			fmt.Printf("removed %d orchestrator(s)\n", count)
		}
	}

	return nil
}
