// Package cli builds the awsx command tree. The root command runs the
// interactive TUI; subcommands are reserved for future versions.
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
	"github.com/davidsgoncalves/awsx/internal/config"
	"github.com/davidsgoncalves/awsx/internal/deps"
	"github.com/davidsgoncalves/awsx/internal/profiles"
	"github.com/davidsgoncalves/awsx/internal/tui"
)

// NewRootCmd returns the awsx root command. With no subcommand it runs the TUI.
func NewRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "awsx",
		Short:         "Interactive AWS login and EC2 access via SSM Session Manager",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ps, err := profiles.Parse(config.ConfigPath())
			if err != nil {
				return fmt.Errorf("read profiles: %w", err)
			}
			if len(ps) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(),
					"Nenhum perfil AWS foi encontrado. Configure a AWS CLI (aws configure sso) antes de continuar.")
				return err
			}
			return tui.Run(tui.Deps{
				Profiles: ps,
				Checks:   deps.Check(),
				NewClients: func(ctx context.Context, profile string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, string, error) {
					c, region, err := awsx.NewClients(ctx, profile)
					if err != nil {
						return nil, nil, nil, "", err
					}
					return c, c, c, region, nil
				},
				NewCLI: func(profile string) (awsx.Login, awsx.Sessioner) {
					cli := awsx.CLI{Profile: profile}
					return cli, cli
				},
			})
		},
	}
}
