package aws

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"

	"github.com/davidsgoncalves/awsx/internal/profiles"
)

// ephemeralProfileName is the profile written into the temporary config file.
const ephemeralProfileName = "_awsx"

// Ephemeral is a temporary AWS config describing a single sso-session /
// account / role / region. Both the SDK and the aws CLI child resolve SSO
// credentials from the cached token via this config; AWSX never handles the
// raw credentials. The temp directory is removed by Close.
type Ephemeral struct {
	ConfigPath string
	Profile    string
	dir        string
}

// WriteEphemeralProfile writes a temporary AWS config file (never
// ~/.aws/config) for the chosen session/account/role/region.
func WriteEphemeralProfile(session profiles.SSOSession, accountID, roleName, region string) (*Ephemeral, error) {
	dir, err := os.MkdirTemp("", "awsx-")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "config")

	content := fmt.Sprintf(`[sso-session %s]
sso_start_url = %s
sso_region = %s
sso_registration_scopes = sso:account:access

[profile %s]
sso_session = %s
sso_account_id = %s
sso_role_name = %s
region = %s
`, session.Name, session.StartURL, session.Region,
		ephemeralProfileName, session.Name, accountID, roleName, region)

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return &Ephemeral{ConfigPath: path, Profile: ephemeralProfileName, dir: dir}, nil
}

// Close removes the temporary directory.
func (e *Ephemeral) Close() error {
	if e == nil || e.dir == "" {
		return nil
	}
	return os.RemoveAll(e.dir)
}

// Clients builds SDK clients that resolve SSO credentials from the cached token
// via the ephemeral profile, returning the clients and the resolved region.
func (e *Ephemeral) Clients(ctx context.Context) (*Clients, string, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithSharedConfigFiles([]string{e.ConfigPath}),
		awsconfig.WithSharedConfigProfile(e.Profile),
	)
	if err != nil {
		return nil, "", err
	}
	return newClientsFromConfig(cfg), cfg.Region, nil
}
