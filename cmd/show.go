package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/loreteam/lore/internal/ticket"
	"github.com/loreteam/lore/internal/ticketops"
	"github.com/spf13/cobra"
)

var showJSON bool

var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a ticket and its full thread",
	Args:  cobra.ExactArgs(1),
	RunE:  runShow,
}

func init() {
	showCmd.Flags().BoolVar(&showJSON, "json", false, "output as JSON (images as base64 data URIs)")
	rootCmd.AddCommand(showCmd)
}

func runShow(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	t, err := loadTicket(ctx, gitRoot, args[0])
	if err != nil {
		return fmt.Errorf("lore show: %w", err)
	}

	entries, err := loadThread(ctx, gitRoot, t)
	if err != nil {
		return fmt.Errorf("lore show: load thread: %w", err)
	}

	if showJSON {
		return printTicketJSON(ctx, gitRoot, t, entries)
	}

	printTicket(t, entries)
	return nil
}

func printTicketJSON(_ context.Context, gitRoot string, t *ticket.Ticket, entries []*ticket.ThreadEntry) error {
	type jsonEntry struct {
		ID         string `json:"id"`
		Kind       string `json:"kind"`
		Author     string `json:"author"`
		Timestamp  string `json:"ts"`
		Text       string `json:"text,omitempty"`
		QuestionID string `json:"question_id,omitempty"`
		ImageData  string `json:"image_data,omitempty"`
		ImageMIME  string `json:"image_mime,omitempty"`
		Caption    string `json:"caption,omitempty"`
	}

	jsonEntries := make([]jsonEntry, 0, len(entries))
	for _, e := range entries {
		je := jsonEntry{
			ID:         e.ID,
			Kind:       string(e.Kind),
			Author:     e.Author,
			Timestamp:  e.Timestamp.Format("2006-01-02T15:04:05Z"),
			Text:       e.Text,
			QuestionID: e.QuestionID,
			Caption:    e.Caption,
			ImageMIME:  e.ImageMIME,
		}
		if e.Kind == ticket.EntryKindImage && e.ImageSHA != "" {
			imgData, err := ticketops.LoadBlob(gitRoot, e.ImageSHA)
			if err == nil {
				je.ImageData = "data:" + e.ImageMIME + ";base64," + base64.StdEncoding.EncodeToString(imgData)
			}
		}
		jsonEntries = append(jsonEntries, je)
	}

	out := map[string]any{
		"id":     t.ID,
		"desc":   t.Desc,
		"status": string(t.Status),
		"agent":  t.Agent,
		"thread": jsonEntries,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	_, err = fmt.Fprintln(os.Stdout, string(data))
	return err
}
