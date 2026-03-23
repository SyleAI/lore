package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/graph"
	"github.com/loreteam/lore/internal/policy"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	newTitle       string
	newDescription string
	newPriority    int
	newFiles       []string
	newFile        string
)

var newCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new ticket",
	Long: `Create a new ticket and store it in git.

Without flags, opens an interactive wizard. With --title, runs non-interactively.
Use --file to load a ticket from a YAML file.`,
	RunE: runNew,
}

func init() {
	newCmd.Flags().StringVar(&newTitle, "title", "", "ticket title (skips wizard)")
	newCmd.Flags().StringVar(&newDescription, "description", "", "ticket description")
	newCmd.Flags().IntVar(&newPriority, "priority", 0, "initial system priority 0-100 (0 = let lore score compute)")
	newCmd.Flags().StringSliceVar(&newFiles, "files", nil, "related file paths")
	newCmd.Flags().StringVar(&newFile, "file", "", "load ticket from YAML file")
	rootCmd.AddCommand(newCmd)
}

func runNew(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	var t *ticket.Ticket

	switch {
	case newFile != "":
		data, err := os.ReadFile(newFile)
		if err != nil {
			return fmt.Errorf("lore new: read file: %w", err)
		}
		t = &ticket.Ticket{}
		if err := yaml.Unmarshal(data, t); err != nil {
			return fmt.Errorf("lore new: parse file: %w", err)
		}
		if t.Title == "" {
			return fmt.Errorf("lore new: ticket title is required")
		}
		if t.Status != "" && !ticket.ValidStatus(t.Status) {
			return fmt.Errorf("lore new: invalid status %q (valid: open, in-progress, blocked, ready, closed)", t.Status)
		}

	case newTitle != "":
		t = &ticket.Ticket{
			Title:       newTitle,
			Description: newDescription,
			Priority:    newPriority,
			Files:       newFiles,
		}

	default:
		var err error
		t, err = newWizard()
		if err != nil {
			return fmt.Errorf("lore new: %w", err)
		}
	}

	id, err := ticket.NewID()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	t.ID = id
	if t.Status == "" {
		t.Status = ticket.StatusOpen
	}
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.Priority < 0 || t.Priority > 100 {
		t.Priority = 0
	}

	if err := saveTicket(ctx, gitRoot, t); err != nil {
		return fmt.Errorf("lore new: %w", err)
	}

	// Auto-score: compute system priority for the new ticket if not already set.
	if t.Priority == 0 {
		scoreNewTicket(ctx, gitRoot, t)
	}

	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventTicketCreated, map[string]any{
		"id":    t.ID,
		"title": t.Title,
	}))

	fmt.Printf("created ticket %s\n", t.ID)
	return nil
}

func newWizard() (*ticket.Ticket, error) {
	r := bufio.NewReader(os.Stdin)

	fmt.Print("Title: ")
	title, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read title: %w", err)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	fmt.Print("Description (blank to skip): ")
	desc, _ := r.ReadString('\n')
	desc = strings.TrimSpace(desc)

	fmt.Print("Priority 0-100 [0=auto]: ")
	priStr, _ := r.ReadString('\n')
	priStr = strings.TrimSpace(priStr)
	priority := 0
	if priStr != "" {
		if p, convErr := strconv.Atoi(priStr); convErr == nil && p >= 0 && p <= 100 {
			priority = p
		} else {
			fmt.Fprintf(os.Stderr, "invalid priority %q, defaulting to 0\n", priStr)
		}
	}

	fmt.Print("Related files (comma-separated, blank to skip): ")
	filesStr, _ := r.ReadString('\n')
	var files []string
	for _, f := range strings.Split(strings.TrimSpace(filesStr), ",") {
		if f = strings.TrimSpace(f); f != "" {
			files = append(files, f)
		}
	}

	return &ticket.Ticket{
		Title:       title,
		Description: desc,
		Priority:    priority,
		Files:       files,
	}, nil
}

// scoreNewTicket computes and writes back a system score for a newly created ticket.
// Failures are non-fatal — scoring is best-effort on new ticket creation.
func scoreNewTicket(ctx context.Context, gitRoot string, t *ticket.Ticket) {
	pol, err := policy.Load(ctx, gitRoot)
	if err != nil {
		return
	}
	idx, err := graph.Load(ctx, gitRoot)
	if err != nil {
		return
	}
	score := graph.UpdateTicket(idx, t, pol)
	if score == 0 {
		// A new ticket with no files, no age, and no blockers scores 0 — same as
		// the default priority it was just saved with. Skip the redundant CAS write.
		return
	}
	_, _ = casUpdate(ctx, gitRoot, t.ID, func(t *ticket.Ticket) error {
		t.Priority = score
		return nil
	})
	_ = graph.Save(ctx, gitRoot, idx)
}
