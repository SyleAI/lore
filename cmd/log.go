package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var logReasoningFlag bool

var logCmd = &cobra.Command{
	Use:   "log <id>",
	Short: "Show execution trace for a ticket",
	Args:  cobra.ExactArgs(1),
	RunE:  runLog,
}

func init() {
	logCmd.Flags().BoolVar(&logReasoningFlag, "reasoning", false, "include full question text and answers")
	rootCmd.AddCommand(logCmd)
}

func runLog(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	t, err := loadTicket(ctx, gitRoot, args[0])
	if err != nil {
		return fmt.Errorf("lore log: %w", err)
	}

	fmt.Printf("Execution log for %s: %s\n\n", t.ID, t.Title)
	fmt.Printf("  Status:   %s\n", t.Status)
	if t.Agent != "" {
		fmt.Printf("  Agent:    %s\n", t.Agent)
	}
	fmt.Printf("  Attempts: %d\n\n", t.Attempts)

	// Merge checkpoints and questions into a single timeline sorted by time.
	type entry struct {
		ts   time.Time
		kind string // "checkpoint" or "question"
		text string
	}

	var entries []entry
	for _, cp := range t.Checkpoints {
		entries = append(entries, entry{ts: cp.Timestamp, kind: "checkpoint", text: cp.Message})
	}
	for _, q := range t.Questions {
		var sb strings.Builder
		fmt.Fprintf(&sb, "[%s] %s", q.Type, q.Text)
		if q.Blocking {
			sb.WriteString(" (blocking)")
		}
		if logReasoningFlag && q.Answer != "" {
			fmt.Fprintf(&sb, "\n    Answer: %s", q.Answer)
			if q.AnsweredBy != "" {
				fmt.Fprintf(&sb, " — %s", q.AnsweredBy)
			}
		} else if q.Answer != "" {
			sb.WriteString(" ✓")
		}
		entries = append(entries, entry{ts: q.AskedAt, kind: "question", text: sb.String()})
	}

	// Insertion-sort by timestamp (slices are small).
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].ts.Before(entries[j-1].ts); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}

	if len(entries) == 0 {
		fmt.Println("  (no entries yet)")
		return nil
	}

	for _, e := range entries {
		prefix := "●"
		if e.kind == "question" {
			prefix = "?"
		}
		fmt.Printf("  %s %s  %s\n", e.ts.Local().Format("2006-01-02 15:04"), prefix, e.text)
	}
	return nil
}
