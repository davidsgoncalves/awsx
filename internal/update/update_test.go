package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompare_OrdersByNumericParts(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.5.0", "v0.6.0", -1},
		{"0.6.0", "v0.6.0", 0},
		{"v0.10.0", "v0.9.0", 1},
		{"v1.0.0", "v0.99.99", 1},
		{"v0.6.0-rc1", "v0.6.0", 0},
		{"dev", "v0.1.0", -1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestDetect_ClassifiesByPath(t *testing.T) {
	goBins := []string{"/home/u/go/bin"}
	cases := []struct {
		exe  string
		want Method
	}{
		{"/opt/homebrew/Caskroom/awsx/0.5.0/awsx", MethodBrew},
		{"/usr/local/Caskroom/awsx/0.5.0/awsx", MethodBrew},
		{"/home/linuxbrew/.linuxbrew/bin/awsx", MethodBrew},
		{"/home/u/go/bin/awsx", MethodGoInstall},
		{"/usr/local/bin/awsx", MethodArchive},
		{"/home/u/.local/bin/awsx", MethodArchive},
	}
	for _, c := range cases {
		if got := Detect(c.exe, goBins); got != c.want {
			t.Errorf("Detect(%q) = %v, want %v", c.exe, got, c.want)
		}
	}
}

func TestArchiveName_MatchesTheReleaseTemplate(t *testing.T) {
	name, err := ArchiveName("darwin", "arm64")
	if err != nil || name != "awsx_Darwin_arm64.tar.gz" {
		t.Fatalf("got %q, %v", name, err)
	}
	name, err = ArchiveName("linux", "amd64")
	if err != nil || name != "awsx_Linux_x86_64.tar.gz" {
		t.Fatalf("got %q, %v", name, err)
	}
	if _, err := ArchiveName("windows", "amd64"); err == nil {
		t.Fatal("windows should have no release archive")
	}
}

func TestVerify_AcceptsTheListedSum(t *testing.T) {
	data := []byte("archive bytes")
	sum := sha256.Sum256(data)
	sums := "deadbeef  other.tar.gz\n" + hex.EncodeToString(sum[:]) + "  awsx_Linux_x86_64.tar.gz\n"
	if err := verify(data, sums, "awsx_Linux_x86_64.tar.gz"); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestVerify_RejectsAMismatch(t *testing.T) {
	sums := "deadbeef  awsx_Linux_x86_64.tar.gz\n"
	if err := verify([]byte("archive bytes"), sums, "awsx_Linux_x86_64.tar.gz"); err == nil {
		t.Fatal("a wrong checksum must not pass")
	}
}

func TestVerify_RejectsAMissingEntry(t *testing.T) {
	if err := verify([]byte("x"), "deadbeef  other.tar.gz\n", "awsx_Linux_x86_64.tar.gz"); err == nil {
		t.Fatal("an unlisted archive must not pass")
	}
}

func TestExtract_PullsTheBinaryOut(t *testing.T) {
	got, err := extract(tarball(t, "awsx", []byte("#!/bin/true")))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if string(got) != "#!/bin/true" {
		t.Fatalf("got %q", got)
	}
}

func TestExtract_FailsWithoutTheBinary(t *testing.T) {
	if _, err := extract(tarball(t, "README.md", []byte("hi"))); err == nil {
		t.Fatal("an archive without awsx must fail")
	}
}

func TestReplace_SwapsTheBinaryInPlace(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "awsx")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replace(dest, []byte("new")); err != nil {
		t.Fatalf("replace: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "new" {
		t.Fatalf("got %q, %v", got, err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("binary is not executable: %v", info.Mode())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary file left behind: %v", entries)
	}
}

func TestReplace_FailsOnAnUnwritableDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if err := replace(filepath.Join(dir, "awsx"), []byte("new")); err == nil {
		t.Fatal("writing into a read-only directory must fail")
	}
}

func TestLatestFrom_ReadsTheTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.6.0"}`))
	}))
	defer srv.Close()
	rel, err := latestFrom(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("latestFrom: %v", err)
	}
	if rel.Tag != "v0.6.0" {
		t.Fatalf("tag = %q", rel.Tag)
	}
}

func TestLatestFrom_ExplainsARateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	_, err := latestFrom(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), "limite") {
		t.Fatalf("err = %v, want a rate-limit explanation", err)
	}
}

// tarball builds a gzipped tar holding one regular file.
func tarball(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
