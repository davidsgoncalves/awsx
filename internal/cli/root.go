// Package cli builds the awsx command tree. The root command runs the
// interactive TUI; subcommands are reserved for future versions.
package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
	"github.com/davidsgoncalves/awsx/internal/config"
	"github.com/davidsgoncalves/awsx/internal/deps"
	"github.com/davidsgoncalves/awsx/internal/logging"
	"github.com/davidsgoncalves/awsx/internal/profiles"
	"github.com/davidsgoncalves/awsx/internal/state"
	"github.com/davidsgoncalves/awsx/internal/tui"
	"github.com/davidsgoncalves/awsx/internal/version"
)

// NewRootCmd returns the awsx root command. With no subcommand it runs the TUI.
func NewRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "awsx",
		Short:         "Interactive AWS login and EC2 access via SSM Session Manager",
		Version:       version.Current(),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath := config.ConfigPath()
			ps, err := profiles.Parse(configPath)
			if err != nil {
				return fmt.Errorf("read profiles: %w", err)
			}
			sessions, err := profiles.ParseSSOSessions(configPath)
			if err != nil {
				return fmt.Errorf("read sso sessions: %w", err)
			}
			if len(ps) == 0 && len(sessions) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(),
					"Nenhum perfil AWS foi encontrado. Configure a AWS CLI (aws configure sso) antes de continuar.")
				return err
			}
			logger := logging.New(config.DebugEnabled())
			defer func() { _ = logger.Close() }()

			return tui.Run(tui.Deps{
				Profiles:    ps,
				SSOSessions: sessions,
				Checks:      deps.Check(),
				Log:         logger,
				State:       state.Load(),
				NewClients: func(ctx context.Context, profile, region string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, awsx.ContainerLister, awsx.ECSLister, string, error) {
					c, resolved, err := awsx.NewClients(ctx, profile, region)
					if err != nil {
						return nil, nil, nil, nil, nil, nil, "", err
					}
					return c, c, c, c, c, c, resolved, nil
				},
				NewCLI: func(profile, region string) (awsx.Login, awsx.Sessioner) {
					cli := awsx.CLI{Profile: profile, Region: region}
					return cli, cli
				},
				NewDiscoverer: func(ctx context.Context, session profiles.SSOSession) (awsx.SSODiscoverer, error) {
					return awsx.NewSSOClient(ctx, session, time.Now())
				},
				NewSSOLogin: func(session profiles.SSOSession) awsx.Login {
					return awsx.SSOSessionLogin{Session: session.Name}
				},
				NewEphemeral: func(session profiles.SSOSession, accountID, roleName, region string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, awsx.ContainerLister, awsx.ECSLister, awsx.Sessioner, string, func(), error) {
					eph, err := awsx.WriteEphemeralProfile(session, accountID, roleName, region)
					if err != nil {
						return nil, nil, nil, nil, nil, nil, nil, "", nil, err
					}
					clients, resolved, err := eph.Clients(context.Background())
					if err != nil {
						_ = eph.Close()
						return nil, nil, nil, nil, nil, nil, nil, "", nil, err
					}
					sess := awsx.CLI{Profile: eph.Profile, Region: resolved, ConfigFile: eph.ConfigPath}
					cleanup := func() { _ = eph.Close() }
					return clients, clients, clients, clients, clients, clients, sess, resolved, cleanup, nil
				},
			})
		},
	}
}
