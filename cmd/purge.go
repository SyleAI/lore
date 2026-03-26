package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/loreteam/lore/internal/gitcmd"
	"github.com/spf13/cobra"
)

var purgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Delete all tickets and reset the event log",
	Long: `Purge removes all tickets (open and done) by deleting their git refs,
and truncates the events log. This cannot be undone.`,
	Args: cobra.NoArgs,
	RunE: runPurge,
}

var flagForce bool

func init() {
	purgeCmd.Flags().BoolVar(&flagForce, "force", false, "skip confirmation prompt")
	rootCmd.AddCommand(purgeCmd)
}

func runPurge(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	if !flagForce {
		fmt.Print("This will delete ALL tickets and clear the event log. Continue? [y/N] ")
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(strings.TrimSpace(response)) != "y" {
			fmt.Println("aborted")
			return nil
		}
	}

	// Collect all ticket refs (open and done)
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

	// Truncate events log
	loreDir := loreDirFromContext(ctx)
	if loreDir != "" {
		eventsLog := loreDir + "/events.log"
		if err := os.Truncate(eventsLog, 0); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: could not clear events log: %v\n", err)
		}
	}

	fmt.Printf("purged %d ticket(s)\n", deleted)
	return nil
}
