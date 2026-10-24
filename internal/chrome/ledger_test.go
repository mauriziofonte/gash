// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package chrome

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// 3.0: SYS-23 fixed: ROOT is resolved like the link, so a symlinked or
// relative ROOT finds the installed version and keeps it for rollback.
func TestLedger_SYS23(t *testing.T) {
	fx := newFixture(t)
	fx.root = filepath.Join(fx.dir, "opt", "real")
	fx.version(t, "139.0.7258.5", "139.0.7258.5")
	fx.version(t, "140.0.7339.16", "140.0.7339.16")
	rootlink := filepath.Join(fx.dir, "opt", "rootlink")
	require.NoError(t, os.Symlink("real", rootlink))
	require.NoError(t, os.Symlink(filepath.Join(rootlink, "140.0.7339.16", binRel), fx.link))
	plan := head + "Installed: 140.0.7339.16\n" +
		"The following packages will be upgraded:\n  chrome-headless-shell (140.0.7339.16 => 141.0.7390.37, from playwright-core 1.56.0)\n" +
		"Kept for rollback: 140.0.7339.16\nThe following versions will be REMOVED:\n  139.0.7258.5\n" +
		"Need to get 2.0 KiB of archives from " + zipURL + "\n"
	fx.env["GASH_CHROME_ROOT"] = rootlink
	r, rc := fx.call(t, nil, "--dry-run")
	require.Equal(t, 0, rc)
	require.Equal(t, plan, r.out)

	// A relative ROOT, from the directory above it.
	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(filepath.Join(fx.dir, "opt")))
	t.Cleanup(func() { _ = os.Chdir(wd) })
	fx.env["GASH_CHROME_ROOT"] = "rootlink"
	r, _ = fx.call(t, nil, "--dry-run")
	require.Equal(t, plan, r.out)
}

// 3.0: SYS-24 fixed: the --version line must name the target exactly,
// and an executable already at ROOT/V is linked only when its own
// --version passes that check; a bogus one is replaced.
func TestLedger_SYS24(t *testing.T) {
	fx := newFixture(t)
	bogus := fx.version(t, target, "bogus")
	r, rc := fx.call(t, nil, "-y")
	require.Equal(t, 0, rc)
	require.Contains(t, r.err, "Info: Verified: Chrome-Headless-Shell 141.0.7390.37\n")
	require.Contains(t, r.err, "Info: Replacing an unverified copy of 141.0.7390.37.\n")
	require.Contains(t, r.err, "OK: Installed: Chrome-Headless-Shell 141.0.7390.37 -> T/opt/chs/141.0.7390.37/"+binRel+"\n")
	dest, err := os.Readlink(fx.link)
	require.NoError(t, err)
	require.Equal(t, bogus, dest)
	b, err := os.ReadFile(bogus)
	require.NoError(t, err)
	require.NotContains(t, string(b), "bogus")

	// A verified copy already there is linked as it is.
	require.NoError(t, os.Remove(fx.link))
	fx.version(t, target, target)
	r, rc = fx.call(t, nil, "-y")
	require.Equal(t, 0, rc)
	require.NotContains(t, r.err, "Replacing")

	fx = newFixture(t)
	fx.env["MOCK_VERSION"] = "Chrome-Headless-Shell 141.0.7390.37-extra"
	r, rc = fx.call(t, nil, "-y")
	require.Equal(t, 1, rc)
	require.Contains(t, r.err, "Error: Version check failed: expected 141.0.7390.37, the binary says 'Chrome-Headless-Shell 141.0.7390.37-extra'.\n")
	require.Empty(t, fx.tree(t))
	for _, ok := range []string{"Chromium 141.0.7390.37", "Google Chrome for Testing 141.0.7390.37 ", "Chrome-Headless-Shell 141.0.7390.37"} {
		require.True(t, versionIs(ok, target), ok)
	}
	for _, bad := range []string{"141.0.7390.37", "Chromium 141.0.7390.370", "Chromium 1141.0.7390.37", "x 141.0.7390.37 y", "Chromium 141.0.7390.3"} {
		require.False(t, versionIs(bad, target), bad)
	}
}

// 3.0: SYS-25 fixed: no sudo when ROOT and the link's directory are
// writable; otherwise an unattended run checks `sudo -n true` first and
// runs every call as `sudo -n`, and a sudo that wants a password is a
// clear error before anything is downloaded.
func TestLedger_SYS25(t *testing.T) {
	fx := newFixture(t)
	fx.env["GASH_HEADLESS"] = "1"
	_, rc := fx.call(t, nil, "-y")
	require.Equal(t, 0, rc)
	require.Empty(t, fx.sudo(t))

	fx = newFixture(t)
	fx.env["GASH_HEADLESS"] = "1"
	fx.m.writable = func(string) bool { return false }
	r, rc := fx.call(t, nil, "-y")
	require.Equal(t, 0, rc, r.err)
	require.Contains(t, r.err, "Info: Installing into T/opt/chs/141.0.7390.37 (sudo)...\n")
	tmp := regexp.MustCompile(`chrome-headless-shell\.[0-9]+`)
	require.Equal(t, "sudo -n true\nsudo -n mkdir -p T/opt/chs/141.0.7390.37\n"+
		"sudo -n cp -R --no-preserve=ownership T/tmp/chrome-headless-shell.N/x/chrome-headless-shell-linux64 T/opt/chs/141.0.7390.37/\n"+
		"sudo -n ln -sfn T/opt/chs/141.0.7390.37/"+binRel+" T/bin/chs\n", tmp.ReplaceAllString(fx.sudo(t), "chrome-headless-shell.N"))

	fx = newFixture(t)
	fx.env["GASH_HEADLESS"] = "1"
	fx.env["MOCK_SUDO_PASSWORD"] = "1"
	fx.m.writable = func(string) bool { return false }
	r, rc = fx.call(t, nil, "-y")
	require.Equal(t, 1, rc)
	require.True(t, strings.HasSuffix(r.err, located+"Error: sudo needs a password: run interactively or configure NOPASSWD.\n"), r.err)
	require.Equal(t, "sudo -n true\n", fx.sudo(t))
	require.Empty(t, fx.tree(t))

	// On a terminal, plain sudo.
	fx = newFixture(t)
	fx.m.writable = func(string) bool { return false }
	e := invoke.EnvMap(fx.env)
	_, rc = fx.call(t, term.Fixed(e, true, true, true).WithTTY("\n"), "-y")
	require.Equal(t, 0, rc)
	require.True(t, strings.HasPrefix(fx.sudo(t), "sudo mkdir -p T/opt/chs/141.0.7390.37\n"), fx.sudo(t))
}

// SYS-36: no curl or jq on PATH; an error status of the registry is the
// network failure.
func TestLedger_SYS36(t *testing.T) {
	fx := newFixture(t)
	fx.env["PATH"] = fx.bin
	r, rc := fx.call(t, nil, "--dry-run")
	require.Equal(t, 0, rc)
	require.Equal(t, result{head + "Installed: none\n" + newPlan, tasks + located}, r)

	fx.srv.routes["GET "+npmLatest] = route{status: 503, body: pwJSON}
	r, rc = fx.call(t, nil, "--dry-run")
	require.Equal(t, 1, rc)
	require.Equal(t, result{"", "Resolving the latest Playwright release... failed\n" +
		"Error: Unable to read playwright-core from the npm registry (network?). Nothing changed.\n"}, r)
}
