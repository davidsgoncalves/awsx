// Package state persists small, non-sensitive user preferences between runs —
// currently the last region chosen per profile or SSO account/role. It stores
// no credentials or tokens.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/davidsgoncalves/awsx/internal/logging"
)

// State is the persisted preference set.
type State struct {
	Regions map[string]string `json:"regions"`
}

// Path is the state file location (alongside the log).
func Path() string { return filepath.Join(logging.Dir(), "state.json") }

// Load reads the state file, returning an empty State when it is absent or
// unreadable (preferences are best-effort, never fatal).
func Load() *State {
	s := &State{Regions: map[string]string{}}
	data, err := os.ReadFile(Path())
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, s)
	if s.Regions == nil {
		s.Regions = map[string]string{}
	}
	return s
}

// Region returns the remembered region for key, or "".
func (s *State) Region(key string) string { return s.Regions[key] }

// SetRegion records the region for key (in memory; call Save to persist).
func (s *State) SetRegion(key, region string) {
	if s.Regions == nil {
		s.Regions = map[string]string{}
	}
	s.Regions[key] = region
}

// Save writes the state file. Errors are returned but are non-fatal to callers.
func (s *State) Save() error {
	if err := os.MkdirAll(logging.Dir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), data, 0o600)
}

// ProfileKey is the state key for the profile flow.
func ProfileKey(profile string) string { return "profile:" + profile }

// SSOKey is the state key for the SSO account/role flow.
func SSOKey(session, account, role string) string {
	return "sso:" + session + "/" + account + "/" + role
}
