// Package profiles reads AWS shared config profiles for selection in the TUI.
package profiles

import (
	"errors"
	"io/fs"
	"sort"
	"strings"

	"gopkg.in/ini.v1"
)

// Profile is a selectable AWS profile from the shared config file.
type Profile struct {
	Name   string
	Region string
	IsSSO  bool
}

// Parse reads an AWS config file and returns its profiles. SSO profiles are
// listed first, alphabetical within each group. A missing file yields an empty
// slice and no error.
func Parse(configPath string) ([]Profile, error) {
	f, err := ini.Load(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var out []Profile
	for _, s := range f.Sections() {
		name, ok := profileName(s.Name())
		if !ok {
			continue
		}
		out = append(out, Profile{
			Name:   name,
			Region: s.Key("region").String(),
			IsSSO:  s.HasKey("sso_session") || s.HasKey("sso_start_url"),
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsSSO != out[j].IsSSO {
			return out[i].IsSSO // SSO first
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// profileName maps a config section name to a profile name. In AWS config,
// the default profile is "[default]" and others are "[profile NAME]".
func profileName(section string) (string, bool) {
	if section == "default" {
		return "default", true
	}
	if rest, ok := strings.CutPrefix(section, "profile "); ok {
		return strings.TrimSpace(rest), true
	}
	return "", false // DEFAULT ini section, etc.
}
