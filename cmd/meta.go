package cmd

import (
	"fmt"
	"sort"

	"github.com/loreteam/lore/internal/ticket"
	"github.com/spf13/cobra"
)

var metaCmd = &cobra.Command{
	Use:   "meta <ticket-id>",
	Short: "Read or write ticket metadata",
	Long: `meta reads all metadata fields on a ticket.

Sub-commands:
  lore meta set <id> <key> <value>   set a metadata field
  lore meta get <id> <key>           get a single metadata field
  lore meta del <id> <key>           delete a metadata field`,
	Args: cobra.ExactArgs(1),
	RunE: runMeta,
}

var metaSetCmd = &cobra.Command{
	Use:   "set <ticket-id> <key> <value>",
	Short: "Set a metadata field on a ticket",
	Args:  cobra.ExactArgs(3),
	RunE:  runMetaSet,
}

var metaGetCmd = &cobra.Command{
	Use:   "get <ticket-id> <key>",
	Short: "Get a metadata field from a ticket",
	Args:  cobra.ExactArgs(2),
	RunE:  runMetaGet,
}

var metaDelCmd = &cobra.Command{
	Use:   "del <ticket-id> <key>",
	Short: "Delete a metadata field from a ticket",
	Args:  cobra.ExactArgs(2),
	RunE:  runMetaDel,
}

func init() {
	metaCmd.AddCommand(metaSetCmd)
	metaCmd.AddCommand(metaGetCmd)
	metaCmd.AddCommand(metaDelCmd)
	rootCmd.AddCommand(metaCmd)
}

func runMeta(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	t, err := loadTicket(ctx, gitRoot, args[0])
	if err != nil {
		return fmt.Errorf("lore meta: %w", err)
	}

	if len(t.Metadata) == 0 {
		fmt.Printf("ticket %s has no metadata\n", t.ID)
		return nil
	}

	keys := make([]string, 0, len(t.Metadata))
	for k := range t.Metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%-20s %v\n", k, t.Metadata[k])
	}
	return nil
}

func runMetaSet(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id, key, val := args[0], args[1], args[2]

	_, err = casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		if t.Metadata == nil {
			t.Metadata = make(map[string]any)
		}
		t.Metadata[key] = parseValue(val)
		return nil
	})
	if err != nil {
		return fmt.Errorf("lore meta set: %w", err)
	}
	fmt.Printf("ticket %s: %s = %s\n", id, key, val)
	return nil
}

func runMetaGet(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	t, err := loadTicket(ctx, gitRoot, args[0])
	if err != nil {
		return fmt.Errorf("lore meta get: %w", err)
	}

	v, ok := t.Metadata[args[1]]
	if !ok {
		return fmt.Errorf("lore meta get: key %q not found", args[1])
	}
	fmt.Println(v)
	return nil
}

func runMetaDel(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	id, key := args[0], args[1]

	_, err = casUpdate(ctx, gitRoot, id, func(t *ticket.Ticket) error {
		if _, ok := t.Metadata[key]; !ok {
			return fmt.Errorf("key %q not found", key)
		}
		delete(t.Metadata, key)
		return nil
	})
	if err != nil {
		return fmt.Errorf("lore meta del: %w", err)
	}
	fmt.Printf("ticket %s: deleted %s\n", id, key)
	return nil
}

