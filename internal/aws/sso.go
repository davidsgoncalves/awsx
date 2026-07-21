package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/aws/aws-sdk-go-v2/service/sso"

	"github.com/davidsgoncalves/awsx/internal/profiles"
)

// ErrTokenExpiredOrMissing means no valid cached SSO token is available and the
// user must run `aws sso login` for the session.
var ErrTokenExpiredOrMissing = errors.New("sso token is missing or expired")

// Account is an AWS account reachable through an SSO session.
type Account struct {
	ID   string
	Name string
}

// Role is a permission set (role) available in an account via SSO.
type Role struct {
	Name string
}

// SSODiscoverer enumerates the accounts and roles a user can access with a
// valid SSO session token.
type SSODiscoverer interface {
	Accounts(ctx context.Context) ([]Account, error)
	Roles(ctx context.Context, accountID string) ([]Role, error)
}

type cachedToken struct {
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// readSSOToken returns the cached SSO access token for a session, or
// ErrTokenExpiredOrMissing when the cache file is absent or the token has
// expired at the given time. It only reads the token; it never handles the
// resulting AWS credentials.
func readSSOToken(sessionName string, now time.Time) (string, error) {
	path, err := ssocreds.StandardCachedTokenFilepath(sessionName)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", ErrTokenExpiredOrMissing
		}
		return "", err
	}
	var tok cachedToken
	if err := json.Unmarshal(data, &tok); err != nil {
		return "", fmt.Errorf("parse sso token cache: %w", err)
	}
	if tok.AccessToken == "" || !tok.ExpiresAt.After(now) {
		return "", ErrTokenExpiredOrMissing
	}
	return tok.AccessToken, nil
}

// SSOClient discovers accounts and roles for one SSO session.
type SSOClient struct {
	client *sso.Client
	token  string
}

// NewSSOClient builds an SSO discovery client for the session using its cached
// token. Returns ErrTokenExpiredOrMissing if the caller must log in first.
func NewSSOClient(ctx context.Context, session profiles.SSOSession, now time.Time) (*SSOClient, error) {
	token, err := readSSOToken(session.Name, now)
	if err != nil {
		return nil, err
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(session.Region))
	if err != nil {
		return nil, err
	}
	return &SSOClient{client: sso.NewFromConfig(cfg), token: token}, nil
}

// Accounts implements SSODiscoverer.
func (c *SSOClient) Accounts(ctx context.Context) ([]Account, error) {
	var out []Account
	p := sso.NewListAccountsPaginator(c.client, &sso.ListAccountsInput{AccessToken: &c.token})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, a := range page.AccountList {
			out = append(out, Account{ID: deref(a.AccountId), Name: deref(a.AccountName)})
		}
	}
	return out, nil
}

// Roles implements SSODiscoverer.
func (c *SSOClient) Roles(ctx context.Context, accountID string) ([]Role, error) {
	var out []Role
	p := sso.NewListAccountRolesPaginator(c.client, &sso.ListAccountRolesInput{
		AccessToken: &c.token,
		AccountId:   &accountID,
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range page.RoleList {
			out = append(out, Role{Name: deref(r.RoleName)})
		}
	}
	return out, nil
}
