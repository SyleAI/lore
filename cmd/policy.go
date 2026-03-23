package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/loreteam/lore/internal/policy"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var policyCmd = &cobra.Command{
	Use:   "policy",
	Short: "Show or modify lore policy",
	Long: `policy prints the current policy document stored in refs/tickets/policy.

Sub-commands:
  lore policy set <key> <value>   set a dot-notation key (e.g. agents.max_concurrent 10)
  lore policy edit                open policy in $EDITOR
  lore policy reset               reset policy to built-in defaults`,
	RunE: runPolicy,
}

var policySetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a policy value using dot notation",
	Args:  cobra.ExactArgs(2),
	RunE:  runPolicySet,
}

var policyEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open policy in $EDITOR",
	RunE:  runPolicyEdit,
}

var policyResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset policy to built-in defaults",
	RunE:  runPolicyReset,
}

func init() {
	policyCmd.AddCommand(policySetCmd)
	policyCmd.AddCommand(policyEditCmd)
	policyCmd.AddCommand(policyResetCmd)
	rootCmd.AddCommand(policyCmd)
}

func runPolicy(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	p, err := policy.Load(ctx, gitRoot)
	if err != nil {
		return fmt.Errorf("lore policy: %w", err)
	}

	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("lore policy: marshal: %w", err)
	}
	fmt.Print(string(data))
	return nil
}

func runPolicySet(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	key, val := args[0], args[1]

	p, err := policy.Load(ctx, gitRoot)
	if err != nil {
		return fmt.Errorf("lore policy set: %w", err)
	}

	// Marshal → decode into map → set dot-notation key → remarshal → unmarshal back.
	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("lore policy set: marshal: %w", err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("lore policy set: unmarshal map: %w", err)
	}

	if err := setDotKey(m, strings.Split(key, "."), parseValue(val)); err != nil {
		return fmt.Errorf("lore policy set: %w", err)
	}

	// Remarshal and parse back into Policy for validation.
	updated, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("lore policy set: remarshal: %w", err)
	}
	var p2 policy.Policy
	if err := yaml.Unmarshal(updated, &p2); err != nil {
		return fmt.Errorf("lore policy set: invalid result: %w", err)
	}

	if err := policy.Save(ctx, gitRoot, &p2); err != nil {
		return fmt.Errorf("lore policy set: %w", err)
	}
	fmt.Printf("policy: %s = %s\n", key, val)
	return nil
}

func runPolicyEdit(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	p, err := policy.Load(ctx, gitRoot)
	if err != nil {
		return fmt.Errorf("lore policy edit: %w", err)
	}

	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("lore policy edit: marshal: %w", err)
	}

	// Write to temp file.
	tmp, err := os.CreateTemp("", "lore-policy-*.yaml")
	if err != nil {
		return fmt.Errorf("lore policy edit: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("lore policy edit: write temp: %w", err)
	}
	tmp.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	editorCmd := exec.Command(editor, tmpName)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr
	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("lore policy edit: editor: %w", err)
	}

	edited, err := os.ReadFile(tmpName)
	if err != nil {
		return fmt.Errorf("lore policy edit: read temp: %w", err)
	}
	var p2 policy.Policy
	if err := yaml.Unmarshal(edited, &p2); err != nil {
		return fmt.Errorf("lore policy edit: invalid YAML: %w", err)
	}

	if err := policy.Save(ctx, gitRoot, &p2); err != nil {
		return fmt.Errorf("lore policy edit: %w", err)
	}
	fmt.Println("policy saved")
	return nil
}

func runPolicyReset(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	if err := policy.Save(ctx, gitRoot, policy.Default()); err != nil {
		return fmt.Errorf("lore policy reset: %w", err)
	}
	fmt.Println("policy reset to defaults")
	return nil
}

// setDotKey sets a nested key in a map[string]any using a dot-split path.
func setDotKey(m map[string]any, parts []string, val any) error {
	if len(parts) == 0 {
		return fmt.Errorf("empty key")
	}
	if len(parts) == 1 {
		m[parts[0]] = val
		return nil
	}
	sub, ok := m[parts[0]]
	if !ok {
		sub = map[string]any{}
	}
	subMap, ok := sub.(map[string]any)
	if !ok {
		return fmt.Errorf("key %q is not a map", parts[0])
	}
	if err := setDotKey(subMap, parts[1:], val); err != nil {
		return err
	}
	m[parts[0]] = subMap
	return nil
}

// parseValue attempts to parse val as bool, int, float64, then falls back to string.
func parseValue(val string) any {
	if val == "true" {
		return true
	}
	if val == "false" {
		return false
	}
	if i, err := strconv.ParseInt(val, 10, 64); err == nil {
		return int(i)
	}
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return f
	}
	return val
}
