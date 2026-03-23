package cmd

import (
	"fmt"
	"time"

	"github.com/loreteam/lore/internal/store"
	"github.com/spf13/cobra"
)

var signalsTrend bool

var signalsCmd = &cobra.Command{
	Use:   "signals",
	Short: "Show system health dashboard",
	Args:  cobra.NoArgs,
	RunE:  runSignals,
}

func init() {
	signalsCmd.Flags().BoolVar(&signalsTrend, "trend", false, "show tickets active in the last 7 days")
	rootCmd.AddCommand(signalsCmd)
}

func runSignals(cmd *cobra.Command, args []string) error {
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	s, err := openStore(gitRoot)
	if err != nil {
		return fmt.Errorf("lore signals: %w", err)
	}
	defer s.Close()

	counts, err := s.CountByStatus()
	if err != nil {
		return fmt.Errorf("lore signals: %w", err)
	}

	oldest, err := s.OldestOpen()
	if err != nil {
		return fmt.Errorf("lore signals: %w", err)
	}

	blockingCount, err := s.CountQuestions(store.QuestionFilter{Unanswered: true, BlockingOnly: true})
	if err != nil {
		return fmt.Errorf("lore signals: %w", err)
	}

	agents, err := s.ListAgents("")
	if err != nil {
		return fmt.Errorf("lore signals: %w", err)
	}

	fmt.Printf("System signals (%s):\n\n", time.Now().UTC().Format("2006-01-02"))
	fmt.Printf("  Tickets:\n")
	fmt.Printf("    open         %d\n", counts.Open)
	fmt.Printf("    in-progress  %d\n", counts.InProgress)
	fmt.Printf("    blocked      %d\n", counts.Blocked)
	fmt.Printf("    ready        %d\n", counts.Ready)
	fmt.Printf("    closed       %d\n", counts.Closed)

	if oldest != nil {
		createdAt, err := time.Parse(time.RFC3339, oldest.CreatedAt)
		if err == nil {
			fmt.Printf("\n  Oldest open: %-14s  \"%s\"  (%dd)\n", oldest.ID, oldest.Title, ageDays(createdAt))
		} else {
			fmt.Printf("\n  Oldest open: %-14s  \"%s\"\n", oldest.ID, oldest.Title)
		}
	}

	fmt.Printf("\n  Blocking unanswered questions: %d\n", blockingCount)
	fmt.Printf("  Active agents: %d\n", len(agents))

	if signalsTrend {
		recent, err := s.RecentlyUpdated(7)
		if err != nil {
			return fmt.Errorf("lore signals: %w", err)
		}
		fmt.Printf("\n  Activity last 7 days (%d tickets):\n", len(recent))
		fmt.Printf("    %-14s  %-12s  %s\n", "TICKET", "STATUS", "TITLE")
		for _, r := range recent {
			fmt.Printf("    %-14s  %-12s  %s\n", r.ID, r.Status, truncate(r.Title, 50))
		}
	}

	return nil
}
