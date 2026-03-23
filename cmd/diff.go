package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff <id>",
	Short: "Show the combined diff for a ticket's branch",
	Long: `Show the combined diff for a ticket's branch against its merge base with the default branch.

Looks for a branch named lore/<id>. If none exists, reports that no branch has been
created for this ticket yet (agents create the branch when they begin work).`,
	Args: cobra.ExactArgs(1),
	RunE: runDiff,
}

func init() {
	rootCmd.AddCommand(diffCmd)
}

func runDiff(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	branch := "lore/" + id

	// Verify the ticket exists.
	if _, err := loadTicket(ctx, gitRoot, id); err != nil {
		return fmt.Errorf("lore diff: %w", err)
	}

	// Check if the branch exists.
	out, err := execGit(ctx, gitRoot, "rev-parse", "--verify", branch)
	if err != nil {
		fmt.Printf("no branch %q found for ticket %s\n", branch, id)
		fmt.Printf("agents create this branch when they claim and begin work.\n")
		return nil
	}
	_ = out

	// Find the default branch (main or master).
	base := defaultBranch(gitRoot)

	// Find merge base so the diff shows only changes introduced by this branch.
	mergeBase, err := execGit(ctx, gitRoot, "merge-base", base, branch)
	if err != nil {
		// Fall back to diffing from the branch tip if merge-base fails.
		mergeBase = []byte(base)
	}
	mergeBaseRef := strings.TrimSpace(string(mergeBase))

	// Stream the diff directly to stdout so paging works naturally.
	gitCmd := exec.CommandContext(ctx, "git", "-C", gitRoot, "diff", mergeBaseRef+".."+branch)
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr
	return gitCmd.Run()
}

// defaultBranch returns "main" if it exists in the repo, otherwise "master".
func defaultBranch(gitRoot string) string {
	for _, name := range []string{"main", "master"} {
		out, err := exec.Command("git", "-C", gitRoot, "rev-parse", "--verify", name).Output()
		if err == nil && len(out) > 0 {
			return name
		}
	}
	return "main"
}
