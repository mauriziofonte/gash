// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// fakeGash is a stand-in release binary: it answers `version --json`
// with version.
func fakeGash(version string) []byte {
	return []byte("#!/bin/sh\necho '{\"version\":\"" + version + "\",\"os\":\"x\",\"arch\":\"y\",\"path\":\"\"}'\n")
}

func hexSum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// tgz is a release archive holding files at its root.
func tgz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range []string{binGash, binHelper} {
		b, ok := files[name]
		if !ok {
			continue
		}
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(b)), Typeflag: tar.TypeReg}))
		_, err := tw.Write(b)
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

// relDir writes a file:// release directory for version: the archive and,
// unless sums is "-", checksums.txt (its hash for the archive, or sums).
func relDir(t *testing.T, version string, files map[string][]byte, sums string) (string, []byte) {
	t.Helper()
	d := t.TempDir()
	asset := "gash_" + version + "_" + platform() + ".tar.gz"
	a := tgz(t, files)
	require.NoError(t, os.WriteFile(filepath.Join(d, asset), a, 0o644))
	if sums != "-" {
		if sums == "" {
			sums = hexSum(a)
		}
		require.NoError(t, os.WriteFile(filepath.Join(d, "checksums.txt"), []byte("0000  other.tar.gz\n"+sums+"  "+asset+"\n"), 0o644))
	}
	return d, a
}

func fetchCall(home, url string, extra ...string) *invoke.Call {
	m := map[string]string{"HOME": home, "PATH": os.Getenv("PATH"), "GASH_RELEASE_URL": url}
	for i := 0; i+1 < len(extra); i += 2 {
		m[extra[i]] = extra[i+1]
	}
	e := invoke.EnvMap(m)
	var out, errb bytes.Buffer
	return &invoke.Call{Env: e, Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, false, false)}
}

// upgrade.md 3.6 step 4 and the bootstrap fetch it mirrors (upgrade.md 4):
// the binary of the target version goes to versions/<v>/ only once it is
// verified, against the tree's pins (release/SHA256SUMS) or the release's
// checksums.txt; the hosts helper only when it is verified too; the
// binary must answer its version; a failure leaves nothing behind.
func TestUpdate_FetchVerify(t *testing.T) {
	if platform() == "" {
		t.Skip("no Gash release for this platform")
	}
	gash, helper := fakeGash("3.0.1"), []byte("#!/bin/sh\nexit 0\n")
	both := map[string][]byte{binGash: gash, binHelper: helper}
	pinned := map[string]string{binGash: hexSum(gash), binHelper: hexSum(helper)}
	gashOnly := map[string]string{binGash: hexSum(gash)}

	for _, tc := range []struct {
		name    string
		files   map[string][]byte
		sums    string // "" correct, "-" none, else the listed hash
		pins    map[string]string
		env     []string
		how     string
		helper  bool
		failure string
	}{
		{name: "checksums.txt", files: both, how: "checksums.txt", helper: true},
		{name: "checksum mismatch", files: both, sums: "deadbeef", failure: "checksum mismatch for gash_3.0.1_" + platform() + ".tar.gz"},
		{name: "no checksums.txt", files: both, sums: "-", failure: "download failed: "},
		{name: "pinned, no checksums.txt", files: both, sums: "-", pins: pinned, how: "release/SHA256SUMS", helper: true},
		{name: "pin mismatch", files: both, sums: "-", pins: map[string]string{binGash: "00"}, failure: "the binary does not match release/SHA256SUMS"},
		{name: "helper pin mismatch", files: both, pins: map[string]string{binGash: hexSum(gash), binHelper: "00"},
			failure: "gash-hosts-helper does not match release/SHA256SUMS"},
		{name: "helper unpinned, checksums.txt", files: both, pins: gashOnly, how: "release/SHA256SUMS", helper: true},
		{name: "helper unpinned, no checksums.txt", files: both, sums: "-", pins: gashOnly, how: "release/SHA256SUMS"},
		{name: "helper unpinned, bad checksums.txt", files: both, sums: "deadbeef", pins: gashOnly, failure: "checksum mismatch"},
		{name: "signed, unpinned", files: both, env: []string{"GASH_REQUIRE_SIGNED", "1"}, failure: "this tree pins no binary for "},
		{name: "signed, helper unpinned", files: both, pins: gashOnly, env: []string{"GASH_REQUIRE_SIGNED", "1"}, how: "release/SHA256SUMS"},
		{name: "wrong version", files: map[string][]byte{binGash: fakeGash("3.0.9")}, failure: "the downloaded binary is version 3.0.9, not 3.0.1"},
		{name: "not a binary", files: map[string][]byte{binGash: []byte("#!/bin/sh\nexit 3\n")}, failure: "does not answer gash version --json"},
		{name: "no gash in the archive", files: map[string][]byte{binHelper: helper}, failure: "cannot extract"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rel, _ := relDir(t, "3.0.1", tc.files, tc.sums)
			home := t.TempDir()
			c := fetchCall(home, "file://"+rel, tc.env...)
			how, err := New().installBinary(c, release{version: "3.0.1", pins: tc.pins})
			dir := filepath.Join(home, ".local/share/gash/versions/3.0.1")
			left, _ := os.ReadDir(dir)
			if tc.failure != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.failure)
				require.Empty(t, left, "a failed fetch leaves nothing behind")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.how, how)
			require.FileExists(t, filepath.Join(dir, binGash))
			if tc.helper {
				require.FileExists(t, filepath.Join(dir, binHelper))
			} else {
				require.NoFileExists(t, filepath.Join(dir, binHelper))
			}
			st, err := os.Stat(filepath.Join(dir, binGash))
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0o755), st.Mode().Perm())

			// Installed and verified: nothing is downloaded again.
			how, err = New().installBinary(fetchCall(home, "file:///nonexistent"), release{version: "3.0.1", pins: tc.pins})
			require.NoError(t, err)
			require.Empty(t, how)
		})
	}

	// A binary in place that is not the target's is replaced.
	rel, _ := relDir(t, "3.0.1", both, "")
	home := t.TempDir()
	dir := filepath.Join(home, ".local/share/gash/versions/3.0.1")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, binGash), fakeGash("3.0.0"), 0o755))
	how, err := New().installBinary(fetchCall(home, "file://"+rel), release{version: "3.0.1"})
	require.NoError(t, err)
	require.Equal(t, "checksums.txt", how)

	// Over HTTP, with the default URL shape replaced by GASH_RELEASE_URL.
	srv := httptest.NewServer(http.FileServer(http.Dir(rel)))
	defer srv.Close()
	home = t.TempDir()
	how, err = New().installBinary(fetchCall(home, srv.URL+"/"), release{version: "3.0.1"})
	require.NoError(t, err)
	require.Equal(t, "checksums.txt", how)
	_, err = New().installBinary(fetchCall(t.TempDir(), srv.URL+"/missing"), release{version: "3.0.1"})
	require.ErrorContains(t, err, "download failed: "+srv.URL+"/missing/gash_3.0.1_")
}

// The fetch lock of upgrade.md 4: mkdir versions/<v>/.lock, shared with
// the bootstrap; a held lock is waited for, a lock older than 10 minutes
// is stale and taken over.
func TestUpdate_FetchLock(t *testing.T) {
	if platform() == "" {
		t.Skip("no Gash release for this platform")
	}
	defer func(w, p time.Duration) { lockWait, lockPoll = w, p }(lockWait, lockPoll)
	lockWait, lockPoll = 300*time.Millisecond, 50*time.Millisecond
	rel, _ := relDir(t, "3.0.1", map[string][]byte{binGash: fakeGash("3.0.1")}, "")
	home := t.TempDir()
	lock := filepath.Join(home, ".local/share/gash/versions/3.0.1/.lock")
	require.NoError(t, os.MkdirAll(lock, 0o755))

	_, err := New().installBinary(fetchCall(home, "file://"+rel), release{version: "3.0.1"})
	require.ErrorContains(t, err, "another download holds "+lock)
	require.DirExists(t, lock, "a live lock is not ours to remove")

	// Released while waiting.
	go func() { time.Sleep(100 * time.Millisecond); _ = os.Remove(lock) }()
	_, err = New().installBinary(fetchCall(home, "file://"+rel), release{version: "3.0.1"})
	require.NoError(t, err)
	require.NoDirExists(t, lock)

	// Stale.
	require.NoError(t, os.Remove(filepath.Join(home, ".local/share/gash/versions/3.0.1/gash")))
	require.NoError(t, os.Mkdir(lock, 0o755))
	old := time.Now().Add(-11 * time.Minute)
	require.NoError(t, os.Chtimes(lock, old, old))
	_, err = New().installBinary(fetchCall(home, "file://"+rel), release{version: "3.0.1"})
	require.NoError(t, err)
	require.NoDirExists(t, lock)
}

func TestUpdate_ParsePins(t *testing.T) {
	sums := "aa  linux_amd64/gash\nbb  linux_amd64/gash-hosts-helper\r\ncc  darwin_arm64/gash\nbad line\n"
	require.Equal(t, map[string]string{binGash: "aa", binHelper: "bb"}, parsePins(sums, "linux_amd64"))
	require.Equal(t, map[string]string{binGash: "cc"}, parsePins(sums, "darwin_arm64"))
	require.Empty(t, parsePins(sums, "linux_arm64"))
}
