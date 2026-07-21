// Package deps detects the external runtime dependencies AWSX needs and
// produces platform-appropriate install hints. It makes no assumptions about
// the host beyond querying PATH and the provided GOOS.
package deps

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Dependency is an external binary AWSX relies on.
type Dependency struct {
	Name   string
	Binary string
	Found  bool
	Path   string
}

var required = []struct{ name, binary string }{
	{"AWS CLI", "aws"},
	{"Session Manager Plugin", "session-manager-plugin"},
}

// Check looks up each required dependency on PATH.
func Check() []Dependency {
	out := make([]Dependency, 0, len(required))
	for _, r := range required {
		path, err := exec.LookPath(r.binary)
		out = append(out, Dependency{
			Name:   r.name,
			Binary: r.binary,
			Found:  err == nil,
			Path:   path,
		})
	}
	return out
}

// BrewAvailable reports whether Homebrew is on PATH.
func BrewAvailable() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

// GOOS returns the current operating system. Wrapper kept so callers pass it
// into InstallHint, keeping that function pure and testable.
func GOOS() string { return runtime.GOOS }

// InstallHint returns a human-readable install instruction for binary on goos.
func InstallHint(binary, goos string, hasBrew bool) string {
	switch binary {
	case "aws":
		switch goos {
		case "darwin":
			if hasBrew {
				return "Install with: brew install awscli"
			}
			return "Download the official AWS CLI v2 installer: https://awscli.amazonaws.com/AWSCLIV2.pkg"
		default: // linux
			return "Install AWS CLI v2 (official): curl 'https://awscli.amazonaws.com/awscli-exe-linux-$(uname -m).zip' -o awscliv2.zip && unzip awscliv2.zip && sudo ./aws/install"
		}
	case "session-manager-plugin":
		switch goos {
		case "darwin":
			if hasBrew {
				return "Install with: brew install --cask session-manager-plugin"
			}
			return "Install the session-manager-plugin (official docs): https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html"
		default: // linux
			return "Install the session-manager-plugin (official docs): https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html"
		}
	}
	return fmt.Sprintf("Install %q and ensure it is on your PATH.", binary)
}
