// Package cli builds the awsx command tree. The root command runs the
// interactive TUI; subcommands are reserved for future versions.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewRootCmd returns the awsx root command. With no subcommand it runs the TUI.
func NewRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "awsx",
		Short:         "Interactive AWS login and EC2 access via SSM Session Manager",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Rewired to the TUI in Task 15.
			fmt.Fprintln(cmd.OutOrStdout(), "awsx: TUI not wired yet")
			return nil
		},
	}
}
