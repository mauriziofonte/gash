// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package chrome

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// SYS-23: a symlinked ROOT is not resolved: the link's target does not
// start with it, so nothing counts as installed and every kept version,
// the current one too, is listed for removal.
func TestLedger_SYS23(t *testing.T) {
	fx := newFixture(t)
	fx.root = filepath.Join(fx.dir, "opt", "real")
	fx.version(t, "139.0.7258.5", "139.0.7258.5")
	fx.version(t, "140.0.7339.16", "140.0.7339.16")
	rootlink := filepath.Join(fx.dir, "opt", "rootlink")
	require.NoError(t, os.Symlink("real", rootlink))
	require.NoError(t, os.Symlink(filepath.Join(rootlink, "140.0.7339.16", binRel), fx.link))
	fx.env["GASH_CHROME_ROOT"] = rootlink
	r, rc := fx.call(t, nil, "--dry-run")
	require.Equal(t, 0, rc)
	require.Equal(t, head+"Installed: none\n"+
		"The following NEW packages will be installed:\n  chrome-headless-shell 141.0.7390.37 (from playwright-core 1.56.0)\n"+
		"The following versions will be REMOVED:\n  139.0.7258.5 140.0.7339.16\n"+
		"Need to get 2.0 KiB of archives from "+zipURL+"\n", r.out)

	// The same tree through the real ROOT: the version is found.
	fx.env["GASH_CHROME_ROOT"] = fx.root
	require.NoError(t, os.Remove(fx.link))
	require.NoError(t, os.Symlink(filepath.Join(fx.root, "140.0.7339.16", binRel), fx.link))
	r, _ = fx.call(t, nil, "--dry-run")
	require.Contains(t, r.out, "Installed: 140.0.7339.16\n")
	require.Contains(t, r.out, "Kept for rollback: 140.0.7339.16\n")
}

// SYS-24: the version check is a substring test, and an executable
// already at ROOT/V is linked as it is, unverified.
func TestLedger_SYS24(t *testing.T) {
	fx := newFixture(t)
	bogus := fx.version(t, target, "bogus")
	r, rc := fx.call(t, nil, "-y")
	require.Equal(t, 0, rc)
	require.Contains(t, r.err, "Info: Verified: Chrome-Headless-Shell 141.0.7390.37\n")
	require.Contains(t, r.err, "OK: Installed: Chrome-Headless-Shell bogus -> T/opt/chs/141.0.7390.37/"+binRel+"\n")
	dest, err := os.Readlink(fx.link)
	require.NoError(t, err)
	require.Equal(t, bogus, dest)
	require.Equal(t, "sudo ln -sfn T/opt/chs/141.0.7390.37/"+binRel+" T/bin/chs\n", fx.sudo(t))

	fx = newFixture(t)
	fx.env["MOCK_VERSION"] = "Chrome-Headless-Shell 141.0.7390.37-extra"
	r, rc = fx.call(t, nil, "-y")
	require.Equal(t, 0, rc)
	require.Contains(t, r.err, "Info: Verified: Chrome-Headless-Shell 141.0.7390.37-extra\n")
}

// SYS-25: plain sudo, also headless and with a ROOT the user can write.
func TestLedger_SYS25(t *testing.T) {
	fx := newFixture(t)
	fx.env["GASH_HEADLESS"] = "1"
	_, rc := fx.call(t, nil, "-y")
	require.Equal(t, 0, rc)
	tmp := regexp.MustCompile(`chrome-headless-shell\.[0-9]+`)
	require.Equal(t, "sudo mkdir -p T/opt/chs/141.0.7390.37\n"+
		"sudo cp -R --no-preserve=ownership T/tmp/chrome-headless-shell.N/x/chrome-headless-shell-linux64 T/opt/chs/141.0.7390.37/\n"+
		"sudo ln -sfn T/opt/chs/141.0.7390.37/"+binRel+" T/bin/chs\n", tmp.ReplaceAllString(fx.sudo(t), "chrome-headless-shell.N"))
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
