// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package compatgen

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The release platform of the bootstrap ($OSTYPE/$HOSTTYPE): Go's names.
func bootPlatform(t *testing.T) string {
	t.Helper()
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64":
		return runtime.GOOS + "_" + runtime.GOARCH
	}
	t.Skip("no Gash release for this platform")
	return ""
}

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// bootRelease writes a file:// release of version 2.1.0 (the tree's
// VERSION) holding files, with checksums.txt unless sums is "-" (sums
// "" lists the archive's hash, anything else is listed as it is).
func bootRelease(t *testing.T, files map[string][]byte, sums string) string {
	t.Helper()
	d := t.TempDir()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range []string{"gash", "gash-hosts-helper"} {
		if b, ok := files[name]; ok {
			// The owner of a release built by someone else: never the
			// owner of the installed file.
			require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(b)), Typeflag: tar.TypeReg,
				Uid: 4242, Gid: 4242, Uname: "builder", Gname: "builder"}))
			_, err := tw.Write(b)
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	asset := "gash_2.1.0_" + bootPlatform(t) + ".tar.gz"
	require.NoError(t, os.WriteFile(filepath.Join(d, asset), buf.Bytes(), 0o644))
	if sums != "-" {
		if sums == "" {
			sums = sum(buf.Bytes())
		}
		require.NoError(t, os.WriteFile(filepath.Join(d, "checksums.txt"), []byte(sums+"  "+asset+"\n"), 0o644))
	}
	return d
}

// bootFetch runs __gash_fetch MODE in the tree dir with HOME home and
// prints "rc=N reason=R" after what the fetch printed.
func bootFetch(t *testing.T, dir, home, mode string, env ...string) string {
	t.Helper()
	cmd := []string{"HOME=" + home, "PATH=/usr/bin:/bin", "LANG=C.UTF-8"}
	script := "source '" + dir + "/lib/core/output.sh'; source '" + dir + "/lib/compat/bootstrap.sh'; " +
		"__gash_fetch " + mode + "; echo \"rc=$? reason=$__GASH_FETCH_REASON\""
	out, err := bashEnv(append(cmd, env...), script)
	require.NoError(t, err, out)
	return out
}

func bashEnv(env []string, script string) (string, error) {
	c := execBash(script)
	c.Env = env
	out, err := c.CombinedOutput()
	return string(out), err
}

// The bootstrap fetch (upgrade.md 4): a file:// release verified against
// checksums.txt or the tree's pins, the hosts helper installed only when
// verified, GASH_REQUIRE_SIGNED=1 refusing an unpinned binary, the fetch
// lock (waited for; stale after 10 minutes), the one interactive line, and
// nothing left behind by a failure.
func TestBootstrapFetch(t *testing.T) {
	needBash(t)
	plat := bootPlatform(t)
	gash := []byte("#!/bin/sh\necho \"fake $*\"\n")
	helper := []byte("#!/bin/sh\nexit 0\n")
	both := map[string][]byte{"gash": gash, "gash-hosts-helper": helper}
	pins := func(dir string, lines ...string) {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "release"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "release/SHA256SUMS"), []byte(strings.Join(lines, "\n")+"\n"), 0o644))
	}
	gashPin, helperPin := sum(gash)+"  "+plat+"/gash", sum(helper)+"  "+plat+"/gash-hosts-helper"

	for _, tc := range []struct {
		name   string
		sums   string
		pins   []string
		env    []string
		reason string
		helper bool
	}{
		{name: "checksums.txt", helper: true},
		{name: "checksum mismatch", sums: "00", reason: "checksum mismatch for gash_2.1.0_" + plat + ".tar.gz"},
		{name: "no checksums.txt", sums: "-", reason: "download failed: file://"},
		{name: "pinned", sums: "-", pins: []string{gashPin, helperPin}, helper: true},
		{name: "pin mismatch", pins: []string{"00  " + plat + "/gash"}, reason: "the binary does not match release/SHA256SUMS"},
		{name: "helper pin mismatch", pins: []string{gashPin, "00  " + plat + "/gash-hosts-helper"},
			reason: "gash-hosts-helper does not match release/SHA256SUMS"},
		{name: "helper unpinned, checksums.txt", pins: []string{gashPin}, helper: true},
		{name: "helper unpinned, no checksums.txt", sums: "-", pins: []string{gashPin}},
		{name: "helper unpinned, bad checksums.txt", sums: "00", pins: []string{gashPin}, reason: "checksum mismatch"},
		{name: "signed, unpinned", env: []string{"GASH_REQUIRE_SIGNED=1"},
			reason: "this tree pins no binary for " + strings.Replace(plat, "_", "/", 1) + " (GASH_REQUIRE_SIGNED=1)"},
		{name: "signed, helper unpinned", pins: []string{gashPin}, env: []string{"GASH_REQUIRE_SIGNED=1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, home := tree(t), t.TempDir()
			if tc.pins != nil {
				pins(dir, tc.pins...)
			}
			rel := bootRelease(t, both, tc.sums)
			out := bootFetch(t, dir, home, "quiet", append(tc.env, "GASH_RELEASE_URL=file://"+rel+"/")...)
			vdir := filepath.Join(home, ".local/share/gash/versions/2.1.0")
			if tc.reason != "" {
				require.Contains(t, out, "rc=1 reason="+tc.reason)
				left, _ := os.ReadDir(vdir)
				require.Empty(t, left, "a failed fetch leaves nothing behind")
				return
			}
			require.Equal(t, "rc=0 reason=\n", out)
			require.FileExists(t, filepath.Join(vdir, "gash"))
			ownedByMe(t, filepath.Join(vdir, "gash"))
			if tc.helper {
				require.FileExists(t, filepath.Join(vdir, "gash-hosts-helper"))
				ownedByMe(t, filepath.Join(vdir, "gash-hosts-helper"))
			} else {
				require.NoFileExists(t, filepath.Join(vdir, "gash-hosts-helper"), "an unverified helper is never installed")
			}
			left, _ := os.ReadDir(vdir)
			for _, e := range left {
				require.False(t, strings.HasPrefix(e.Name(), "."), e.Name())
			}
		})
	}

	// Headless, the first call downloads nothing: binary_not_found, and
	// nothing is installed (3.4.2). With GASH_AUTO_FETCH=1 it fetches,
	// then runs the binary.
	dir, home := tree(t), t.TempDir()
	rel := bootRelease(t, both, "")
	env := []string{"HOME=" + home, "PATH=/usr/bin:/bin", "LANG=C.UTF-8", "GASH_RELEASE_URL=file://" + rel}
	out, err := bashEnv(env, "GASH_HEADLESS=1; source '"+dir+"/gash.sh'; gash_username")
	require.Error(t, err)
	require.Regexp(t, `^\{"error":"binary_not_found","details":"gash 2\.1\.0 for [a-z]+/[a-z0-9]+ is not installed \(headless mode does not download it; GASH_AUTO_FETCH=1 allows that\)","action":"STOP","recoverable":true,"hint":"Run: bash .*/install\.sh --repair"\}\n$`, out)
	require.NoDirExists(t, filepath.Join(home, ".local/share/gash/versions"))
	out, err = bashEnv(append(env, "GASH_AUTO_FETCH=1"), "GASH_HEADLESS=1; source '"+dir+"/gash.sh'; gash_username")
	require.NoError(t, err, out)
	require.Equal(t, "fake gash_username\n", out)

	// GASH_AUTO_FETCH=0: no download at an interactive load either, one
	// line that says why; the installer's __gash_fetch still fetches.
	dir, home = tree(t), t.TempDir()
	out, err = bashEnv([]string{"HOME=" + home, "PATH=/usr/bin:/bin", "LANG=C.UTF-8", "GASH_RELEASE_URL=file://" + rel, "GASH_AUTO_FETCH=0"},
		"source '"+dir+"/lib/core/output.sh'; source '"+dir+"/lib/compat/bootstrap.sh'; __gash_auto_fetch loud; echo \"rc=$?\"")
	require.NoError(t, err, out)
	require.Regexp(t, `^Warning: gash 2\.1\.0 is not installed \(GASH_AUTO_FETCH=0 turns the download off\)\. Run: bash .*/install\.sh --repair\nrc=1\n$`, out)
	require.NoDirExists(t, filepath.Join(home, ".local/share/gash/versions"))
	out = bootFetch(t, dir, home, "quiet", "GASH_RELEASE_URL=file://"+rel, "GASH_AUTO_FETCH=0")
	require.Equal(t, "rc=0 reason=\n", out)

	// The interactive line, success and failure.
	dir, home = tree(t), t.TempDir()
	out = bootFetch(t, dir, home, "loud", "GASH_RELEASE_URL=file://"+rel)
	require.Equal(t, "OK: Installed gash 2.1.0 for "+strings.Replace(plat, "_", "/", 1)+"\nrc=0 reason=\n", out)
	out = bootFetch(t, tree(t), t.TempDir(), "loud", "GASH_RELEASE_URL=file:///nonexistent")
	require.Regexp(t, `^Warning: gash 2\.1\.0 is not installed and could not be downloaded \(download failed: file:///nonexistent/gash_2\.1\.0_`+
		plat+`\.tar\.gz\)\. Run: bash .*/install\.sh --repair\nrc=1 `, out)

	// A held lock is waited for; a lock older than 10 minutes is stale.
	dir, home = tree(t), t.TempDir()
	lock := filepath.Join(home, ".local/share/gash/versions/2.1.0/.lock")
	require.NoError(t, os.MkdirAll(lock, 0o755))
	go func() { time.Sleep(1500 * time.Millisecond); _ = os.Remove(lock) }()
	start := time.Now()
	out = bootFetch(t, dir, home, "quiet", "GASH_RELEASE_URL=file://"+rel)
	require.Equal(t, "rc=0 reason=\n", out)
	require.Greater(t, time.Since(start), time.Second, "the fetch waited for the lock")
	require.NoDirExists(t, lock)

	dir, home = tree(t), t.TempDir()
	lock = filepath.Join(home, ".local/share/gash/versions/2.1.0/.lock")
	require.NoError(t, os.MkdirAll(lock, 0o755))
	old := time.Now().Add(-11 * time.Minute)
	require.NoError(t, os.Chtimes(lock, old, old))
	start = time.Now()
	out = bootFetch(t, dir, home, "quiet", "GASH_RELEASE_URL=file://"+rel)
	require.Equal(t, "rc=0 reason=\n", out)
	require.Less(t, time.Since(start), 30*time.Second)
	require.NoDirExists(t, lock)
	require.FileExists(t, filepath.Join(home, ".local/share/gash/versions/2.1.0/gash"))

	// A lock holds the time it was taken: one older than 10 minutes is
	// stale at once (3.4: no find needed to tell).
	dir, home = tree(t), t.TempDir()
	lock = filepath.Join(home, ".local/share/gash/versions/2.1.0/.lock")
	require.NoError(t, os.MkdirAll(lock, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(lock, "at"), []byte(strconv.FormatInt(time.Now().Add(-11*time.Minute).Unix(), 10)+"\n"), 0o644))
	start = time.Now()
	out = bootFetch(t, dir, home, "quiet", "GASH_RELEASE_URL=file://"+rel)
	require.Equal(t, "rc=0 reason=\n", out)
	require.Less(t, time.Since(start), 3*time.Second, "an old lock is stale at once")
	require.NoDirExists(t, lock)
}

func execBash(script string) *exec.Cmd {
	return exec.Command("bash", "--noprofile", "--norc", "-c", script)
}

// ownedByMe: the file belongs to the user who fetched it, whatever owner
// the archive recorded (as root, tar -x keeps that one).
func ownedByMe(t *testing.T, path string) {
	t.Helper()
	st, err := os.Stat(path)
	require.NoError(t, err)
	sys, ok := st.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	require.Equal(t, os.Getuid(), int(sys.Uid), path)
}
