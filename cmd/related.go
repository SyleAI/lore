package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/store"
	"github.com/spf13/cobra"
)

var relatedCmd = &cobra.Command{
	Use:   "related <ticket-id>",
	Short: "Show tickets related by shared files or semantic similarity",
	Args:  cobra.ExactArgs(1),
	RunE:  runRelated,
}

func init() {
	rootCmd.AddCommand(relatedCmd)
}

func runRelated(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	t, err := loadTicket(ctx, gitRoot, id)
	if err != nil {
		return fmt.Errorf("lore related: %w", err)
	}

	s, err := openStore(gitRoot)
	if err != nil {
		return fmt.Errorf("lore related: %w", err)
	}
	defer s.Close()

	byFile, err := s.FindRelatedByFiles(id)
	if err != nil {
		return fmt.Errorf("lore related: %w", err)
	}

	similar := findSimilarTickets(ctx, s, id, ticketQueryText(t))

	// Append semantic-only results not already shown by file-based.
	seen := make(map[string]bool, len(byFile))
	for _, r := range byFile {
		seen[r.ID] = true
	}
	var semanticOnly []*store.Row
	for _, r := range similar {
		if !seen[r.ID] {
			semanticOnly = append(semanticOnly, r)
		}
	}

	if len(byFile) == 0 && len(semanticOnly) == 0 {
		fmt.Println("no related tickets found")
		return nil
	}

	printTicketSection("Related (shared files)", byFile)
	printTicketSection("Related (semantic)", semanticOnly)
	return nil
}
