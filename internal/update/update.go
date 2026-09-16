// Package update checks GitHub for a newer awsx release and installs it the
// way this copy was installed: through Homebrew, by replacing the binary that
// came out of a release archive, or not at all for `go install` builds, which
// the Go toolchain owns.
package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	repo    = "davidsgoncalves/awsx"
	binary  = "awsx"
	cask    = "davidsgoncalves/tap/awsx"
	modPath = "github.com/davidsgoncalves/awsx/cmd/awsx"

	latestURL   = "https://api.github.com/repos/" + repo + "/releases/latest"
	downloadURL = "https://github.com/" + repo + "/releases/download/"

	// maxArchive caps what is read from the network and out of the archive.
	// The released binary is a few megabytes; anything near this is wrong.
	maxArchive = 64 << 20
)

// Method is how this copy of awsx was installed, which decides how it can be
// replaced.
type Method int

const (
	// MethodArchive is an unpacked release archive (scripts/install.sh, or a
	// manual download): awsx can replace the binary itself.
	MethodArchive Method = iota
	// MethodBrew is a Homebrew cask: brew owns the files and must do it.
	MethodBrew
	// MethodGoInstall is a `go install` build: no release archive is involved
	// and the user reinstalls with the toolchain.
	MethodGoInstall
)

// Release is the published release awsx compares itself against.
type Release struct {
	Tag string
}

// Latest reads the newest published release from the GitHub API.
func Latest(ctx context.Context) (Release, error) {
	return latestFrom(ctx, latestURL)
}

func latestFrom(ctx context.Context, url string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return Release{}, fmt.Errorf("a API do GitHub recusou a consulta (%s); o limite por IP costuma liberar em uma hora", resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("consulta de releases falhou: %s", resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Release{}, err
	}
	if body.TagName == "" {
		return Release{}, errors.New("a resposta do GitHub não trouxe tag_name")
	}
	return Release{Tag: body.TagName}, nil
}

// Compare orders two version strings by their numeric parts, ignoring a
// leading "v" and any pre-release suffix, so "v0.5.0-rc1" and "v0.5.0" compare
// equal. Parts that are missing or non-numeric count as zero, which puts an
// unstamped "dev" build below every release.
func Compare(a, b string) int {
	pa, pb := parts(a), parts(b)
	for i := range 3 {
		switch {
		case pa[i] < pb[i]:
			return -1
		case pa[i] > pb[i]:
			return 1
		}
	}
	return 0
}

func parts(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out [3]int
	for i, f := range strings.SplitN(v, ".", 3) {
		n, err := strconv.Atoi(f)
		if err != nil {
			return out
		}
		out[i] = n
	}
	return out
}

// Installed classifies the running binary, returning the resolved path so the
// caller can show it and, for MethodArchive, write over it.
func Installed() (Method, string, error) {
	exe, err := os.Executable()
	if err != nil {
		return MethodArchive, "", err
	}
	// Homebrew puts a symlink in bin and the real file under Caskroom, so the
	// link has to be followed before the path says anything.
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return Detect(exe, GoBinDirs()), exe, nil
}

// brewPrefixes are the directory trees a Homebrew install can land in.
var brewPrefixes = []string{
	"/opt/homebrew/",
	"/usr/local/Caskroom/",
	"/usr/local/Cellar/",
	"/home/linuxbrew/",
}

// Detect classifies an installation from the resolved binary path and the
// directories a `go install` build would land in.
func Detect(exe string, goBins []string) Method {
	for _, p := range brewPrefixes {
		if strings.HasPrefix(exe, p) {
			return MethodBrew
		}
	}
	dir := filepath.Dir(exe)
	for _, b := range goBins {
		if b != "" && filepath.Clean(b) == dir {
			return MethodGoInstall
		}
	}
	return MethodArchive
}

// GoBinDirs are the directories `go install` writes to, in the order the
// toolchain prefers them.
func GoBinDirs() []string {
	var out []string
	if b := os.Getenv("GOBIN"); b != "" {
		out = append(out, b)
	}
	if p := os.Getenv("GOPATH"); p != "" {
		for _, entry := range filepath.SplitList(p) {
			out = append(out, filepath.Join(entry, "bin"))
		}
	}
	if h, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(h, "go", "bin"))
	}
	return out
}

// BrewUpgradeArgs is the brew invocation that upgrades the cask.
func BrewUpgradeArgs() []string { return []string{"upgrade", "--cask", cask} }

// GoInstallLine is the command that reinstalls a `go install` build.
func GoInstallLine() string { return "go install " + modPath + "@latest" }

// InstallerLine is the command that reinstalls through the release archive,
// for when awsx cannot write over its own binary.
func InstallerLine() string {
	return "curl -fsSL https://raw.githubusercontent.com/" + repo + "/main/scripts/install.sh | sudo sh"
}

// ArchiveName is the release asset for a platform, matching the name template
// in .goreleaser.yaml.
func ArchiveName(goos, goarch string) (string, error) {
	var os_ string
	switch goos {
	case "darwin":
		os_ = "Darwin"
	case "linux":
		os_ = "Linux"
	default:
		return "", fmt.Errorf("sem release para %s", goos)
	}
	var arch string
	switch goarch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("sem release para %s/%s", goos, goarch)
	}
	return fmt.Sprintf("%s_%s_%s.tar.gz", binary, os_, arch), nil
}

// Install downloads the release archive for this platform, verifies it against
// the release checksums and replaces dest with the binary inside it. A
// permission failure is wrapped so the caller can tell it apart from a
// download problem.
func Install(ctx context.Context, tag, dest string) error {
	name, err := ArchiveName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	archive, err := fetch(ctx, downloadURL+tag+"/"+name)
	if err != nil {
		return fmt.Errorf("baixar %s: %w", name, err)
	}
	sums, err := fetch(ctx, downloadURL+tag+"/checksums.txt")
	if err != nil {
		return fmt.Errorf("baixar checksums.txt: %w", err)
	}
	if err := verify(archive, string(sums), name); err != nil {
		return err
	}
	bin, err := extract(archive)
	if err != nil {
		return err
	}
	return replace(dest, bin)
}

func fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxArchive))
}

// verify checks data against the sha256 listed for name in a checksums.txt.
func verify(data []byte, sums, name string) error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	for line := range strings.SplitSeq(strings.TrimSpace(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		if fields[0] != got {
			return fmt.Errorf("checksum de %s não confere (esperado %s, obtido %s)", name, fields[0], got)
		}
		return nil
	}
	return fmt.Errorf("checksums.txt não lista %s", name)
}

// extract pulls the awsx binary out of the release tarball.
func extract(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("o arquivo não contém %q", binary)
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(h.Name) != binary || h.Typeflag != tar.TypeReg {
			continue
		}
		return io.ReadAll(io.LimitReader(tr, maxArchive))
	}
}

// replace writes bin over dest through a temporary file in the same directory,
// so the swap is a rename and never leaves a half-written binary behind.
func replace(dest string, bin []byte) error {
	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, "."+binary+"-*")
	if err != nil {
		return fmt.Errorf("escrever em %s: %w", dir, err)
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(bin); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o755); err != nil {
		return err
	}
	return os.Rename(name, dest)
}
