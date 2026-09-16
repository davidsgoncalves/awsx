// Package version reports which awsx build is running. Releases stamp the
// value in at link time; any other build falls back to what the Go toolchain
// recorded, which is the module version for `go install` and nothing for a
// local `go build`.
package version

import "runtime/debug"

// Version is set by the release build with
// -X github.com/davidsgoncalves/awsx/internal/version.Version=<tag>.
var Version = ""

// Current is the running version, or "dev" when the build carries none.
func Current() string {
	return resolve(Version, buildVersion)
}

// resolve picks the stamped version, then the build-recorded one, then "dev".
func resolve(stamped string, fromBuild func() string) string {
	if stamped != "" {
		return stamped
	}
	if v := fromBuild(); v != "" && v != "(devel)" {
		return v
	}
	return "dev"
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return info.Main.Version
}
