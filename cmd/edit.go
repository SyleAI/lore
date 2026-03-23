package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var editCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Edit a ticket in $EDITOR",
	Args:  cobra.ExactArgs(1),
	RunE:  runEdit,
}

func init() {
	rootCmd.AddCommand(editCmd)
}

func runEdit(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	t, err := loadTicket(ctx, gitRoot, id)
	if err != nil {
		return fmt.Errorf("lore edit: %w", err)
	}

	original, err := ticket.Marshal(t)
	if err != nil {
		return fmt.Errorf("lore edit: marshal: %w", err)
	}
	tmp, err := os.CreateTemp("", "lore-edit-*.yaml")
	if err != nil {
		return fmt.Errorf("lore edit: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(original); err != nil {
		tmp.Close()
		return fmt.Errorf("lore edit: write temp file: %w", err)
	}
	tmp.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	editorCmd := exec.Command(editor, tmpPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr
	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("lore edit: editor: %w", err)
	}

	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("lore edit: read temp file: %w", err)
	}
	if string(edited) == string(original) {
		fmt.Println("no changes")
		return nil
	}

	t, err = ticket.Unmarshal(edited)
	if err != nil {
		return fmt.Errorf("lore edit: parse edited ticket: %w", err)
	}
	t.UpdatedAt = time.Now().UTC()

	if err := saveTicket(ctx, gitRoot, t); err != nil {
		return fmt.Errorf("lore edit: %w", err)
	}

	fmt.Printf("updated ticket %s\n", id)
	return nil
}
