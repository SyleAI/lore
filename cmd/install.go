package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install <source>",
	Short: "Install an orchestrator into this repository",
	Long: `install copies or clones an orchestrator into .lore/orchestrators/<name>/.

Sources accepted:
  ./path or /abs/path   — local directory
  user/repo             — GitHub shorthand (clones https://github.com/user/repo)
  https://...           — full git URL`,
	Args: cobra.ExactArgs(1),
	RunE: runInstall,
}

func init() {
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	source := args[0]
	orchestratorsDir := filepath.Join(gitRoot, ".lore", "orchestrators")

	// Clone or copy into a temp staging dir first, then read manifest for name.
	staging, err := os.MkdirTemp("", "lore-install-*")
	if err != nil {
		return fmt.Errorf("lore install: create staging dir: %w", err)
	}
	defer os.RemoveAll(staging)

	stagingDest := filepath.Join(staging, "orch")

	if isLocalPath(source) {
		expanded := expandHome(source)
		if err := copyDir(expanded, stagingDest); err != nil {
			return fmt.Errorf("lore install: copy %s: %w", source, err)
		}
	} else {
		url, err := resolveGitURL(source)
		if err != nil {
			return fmt.Errorf("lore install: %w", err)
		}
		fmt.Printf("cloning %s …\n", url)
		c := exec.CommandContext(cmd.Context(), "git", "clone", "--depth=1", url, stagingDest)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("lore install: git clone %s: %w", url, err)
		}
	}

	// Read manifest to determine canonical name.
	manifest, err := readManifest(stagingDest)
	name := ""
	if err != nil || manifest.Name == "" {
		// Fall back to basename of source, stripping trailing slash and .git suffix.
		name = filepath.Base(strings.TrimSuffix(source, "/"))
		name = strings.TrimSuffix(name, ".git")
		if name == "." || name == "" {
			name = "orchestrator"
		}
	} else {
		name = manifest.Name
	}

	dest := filepath.Join(orchestratorsDir, name)

	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("lore install: orchestrator %q already installed at %s (remove it first)", name, dest)
	}

	if err := os.MkdirAll(orchestratorsDir, 0755); err != nil {
		return fmt.Errorf("lore install: create orchestrators dir: %w", err)
	}

	if err := copyDir(stagingDest, dest); err != nil {
		return fmt.Errorf("lore install: install to %s: %w", dest, err)
	}

	fmt.Printf("installed %s → %s\n", name, filepath.Join(".lore", "orchestrators", name))
	fmt.Printf("run with: lore run %s\n", name)
	return nil
}

// isLocalPath reports whether source looks like a filesystem path.
func isLocalPath(source string) bool {
	return strings.HasPrefix(source, "./") ||
		strings.HasPrefix(source, "../") ||
		strings.HasPrefix(source, "/") ||
		strings.HasPrefix(source, "~/")
}

// resolveGitURL turns a GitHub shorthand (user/repo) into a full HTTPS URL,
// leaving full URLs unchanged. Returns an empty string if the source is not
// a recognized format.
func resolveGitURL(source string) (string, error) {
	if strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "git@") || strings.HasPrefix(source, "http://") {
		return source, nil
	}
	// Accept exactly user/repo — one slash, no dots in the owner segment.
	// Dots are allowed in the repo name (e.g. user/my.tool).
	parts := strings.SplitN(source, "/", 3)
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" &&
		!strings.Contains(parts[0], ".") {
		return "https://github.com/" + source, nil
	}
	return "", fmt.Errorf("unrecognized source %q — use a local path (./dir), GitHub shorthand (user/repo), or a full git URL", source)
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

// skipDirs are directories that should never be copied as part of an orchestrator.
var skipDirs = map[string]bool{
	".git":          true,
	".venv":         true,
	"venv":          true,
	"node_modules":  true,
	"__pycache__":   true,
	".mypy_cache":   true,
	".pytest_cache": true,
}

// copyDir recursively copies src into dst, merging into dst if it already exists.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Skip non-distributable directories.
		topLevel := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		if skipDirs[topLevel] {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		target := filepath.Join(dst, rel)

		// Handle symlinks by recreating them.
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		}

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
