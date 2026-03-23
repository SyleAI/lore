package cmd

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"time"

	"github.com/loreteam/lore/internal/event"
	"github.com/loreteam/lore/internal/gitcmd"
	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var (
	updateImagePath string
	updateFrom      string
)

var updateCmd = &cobra.Command{
	Use:   "update <ticket-id> <message>",
	Short: "Append a progress update to a ticket thread",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runUpdate,
}

func init() {
	updateCmd.Flags().StringVar(&updateImagePath, "image", "", "attach an image file to the update")
	updateCmd.Flags().StringVar(&updateFrom, "from", "", "agent identity (defaults to config agent_id or hostname)")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id := args[0]
	from := agentID(ctx, updateFrom)

	t, err := loadTicket(ctx, gitRoot, id)
	if err != nil {
		return fmt.Errorf("lore update: %w", err)
	}

	entryID, err := ticket.NewEntryID()
	if err != nil {
		return fmt.Errorf("lore update: %w", err)
	}

	if updateImagePath != "" {
		// Image attachment.
		imgData, err := os.ReadFile(updateImagePath)
		if err != nil {
			return fmt.Errorf("lore update: read image: %w", err)
		}
		imgSHA, err := gitcmd.WriteBlob(ctx, gitRoot, imgData)
		if err != nil {
			return fmt.Errorf("lore update: store image: %w", err)
		}

		mimeType := mime.TypeByExtension(filepath.Ext(updateImagePath))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}

		caption := ""
		if len(args) == 2 {
			caption = args[1]
		}

		entry := &ticket.ThreadEntry{
			ID:        entryID,
			Kind:      ticket.EntryKindImage,
			Author:    from,
			Timestamp: time.Now().UTC(),
			ImageSHA:  imgSHA,
			ImageMIME: mimeType,
			Caption:   caption,
		}
		if _, err := appendThread(ctx, gitRoot, t, entry); err != nil {
			return fmt.Errorf("lore update: %w", err)
		}
		fmt.Printf("image attached to ticket %s (%s)\n", id, imgSHA[:8])
		em := event.EmitterFromContext(ctx)
		em.Emit(ctx, event.New(event.EventTicketUpdated, map[string]any{
			"ticket_id": id,
			"kind":      "image",
			"from":      from,
		}))
		return nil
	}

	// Text update.
	if len(args) < 2 {
		return fmt.Errorf("lore update: message required (or use --image for image attachments)")
	}
	message := args[1]

	entry := &ticket.ThreadEntry{
		ID:        entryID,
		Kind:      ticket.EntryKindUpdate,
		Author:    from,
		Timestamp: time.Now().UTC(),
		Text:      message,
	}
	if _, err := appendThread(ctx, gitRoot, t, entry); err != nil {
		return fmt.Errorf("lore update: %w", err)
	}

	fmt.Printf("update added to ticket %s\n", id)
	em := event.EmitterFromContext(ctx)
	em.Emit(ctx, event.New(event.EventTicketUpdated, map[string]any{
		"ticket_id": id,
		"kind":      "update",
		"from":      from,
	}))
	return nil
}
