package cmd

import (
	"fmt"

	"github.com/loreteam/lore/internal/ui"
	"github.com/spf13/cobra"
)

var uiPort int

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Start the local web UI",
	Long: `ui starts a local web server and opens the Lore dashboard in your browser.

The server binds to localhost only and is not exposed to the network.
Default port is 7890, configurable via --port or .lore/config.yaml.`,
	RunE: runUI,
}

func init() {
	uiCmd.Flags().IntVar(&uiPort, "port", 7890, "port to listen on")
	rootCmd.AddCommand(uiCmd)
}

func runUI(cmd *cobra.Command, args []string) error {
	gitRoot, err := requireGitRoot(cmd)
	if err != nil {
		return err
	}

	srv := ui.NewServer(gitRoot, uiPort)
	fmt.Printf("Starting Lore UI — open http://localhost:%d in your browser\n", uiPort)
	return srv.Start(cmd.Context())
}
