// Package config resolves runtime settings from the environment: config file
// path, region, and the debug flag. It never resolves credentials.
package config

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNoRegion means no region could be resolved from env or the profile.
var ErrNoRegion = errors.New("no region configured for the selected profile; set one in ~/.aws/config or AWS_REGION")

// ConfigPath returns the AWS shared config path, honoring AWS_CONFIG_FILE.
func ConfigPath() string {
	if p := os.Getenv("AWS_CONFIG_FILE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".aws", "config")
	}
	return filepath.Join(home, ".aws", "config")
}

// DebugEnabled reports whether verbose logging is on.
func DebugEnabled() bool {
	return os.Getenv("AWSX_DEBUG") == "true"
}

// ResolveRegion returns the effective region: env override first, then the
// profile's configured region. ErrNoRegion if neither is set.
func ResolveRegion(profileRegion string) (string, error) {
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r, nil
	}
	if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
		return r, nil
	}
	if profileRegion != "" {
		return profileRegion, nil
	}
	return "", ErrNoRegion
}
