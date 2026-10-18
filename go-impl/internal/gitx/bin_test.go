// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/app"
)

// TestMain lets the test binary stand in for the gash binary behind the
// generated shims (GASH_BIN): with GITX_TEST_AS_GASH=1 it is app.Main.
func TestMain(m *testing.M) {
	if os.Getenv("GITX_TEST_AS_GASH") == "1" {
		os.Exit(app.Main(os.Args[1:], os.Environ(), os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

// LEDGER GIT-09 (forced divergence): the 2.x git_dump_revisions leaves
// its loop variable commit in the caller's shell; through the shim the
// function runs in the binary and the caller's commit is untouched.
func TestLedger_GIT09(t *testing.T) {
	for _, p := range []string{"bash", "git"} {
		if _, err := exec.LookPath(p); err != nil {
			t.Skip(p + " not available")
		}
	}
	root, err := filepath.Abs("../../..")
	require.NoError(t, err)
	compat, err := filepath.Abs("../../testdata/compat")
	require.NoError(t, err)
	if _, err := os.Stat(filepath.Join(root, "lib/modules/git.sh")); err != nil {
		t.Skip("the 2.x sources are not in this tree")
	}
	self, err := os.Executable()
	require.NoError(t, err)

	base := t.TempDir()
	setup := exec.Command("bash", "--noprofile", "--norc", "-euc",
		`git init -q w && cd w && git config user.email a@b && git config user.name n &&
echo a >f && git add f && git commit -qm i && echo b >f && git commit -qam j`)
	setup.Dir = base
	setup.Env = []string{"HOME=" + base, "PATH=/usr/bin:/bin", "GIT_CONFIG_NOSYSTEM=1"}
	out, err := setup.CombinedOutput()
	require.NoError(t, err, string(out))

	run := func(src string) string {
		t.Helper()
		cmd := exec.Command("bash", "--noprofile", "--norc", "-c",
			src+`; commit=mine; git_dump_revisions f 2>/dev/null; echo "commit=$commit"`)
		cmd.Dir = filepath.Join(base, "w")
		cmd.Env = []string{"HOME=" + base, "PATH=/usr/bin:/bin", "GIT_CONFIG_NOSYSTEM=1",
			"GASH_BIN=" + self, "GITX_TEST_AS_GASH=1"}
		out, err := cmd.Output()
		require.NoError(t, err)
		return string(out)
	}
	leaked := run(`for f in output utils validation; do source "` + root + `/lib/core/$f.sh"; done; source "` + root + `/lib/modules/git.sh"`)
	require.True(t, strings.HasPrefix(leaked, "commit=") && leaked != "commit=mine\n", leaked)
	require.Equal(t, "commit=mine\n", run(`source "`+compat+`/lib/compat/bootstrap.sh"; source "`+compat+`/lib/modules/git.sh"`))
	_, err = os.Stat(filepath.Join(base, "w/f.002."+strings.TrimSpace(strings.TrimPrefix(leaked, "commit="))))
	require.NoError(t, err, "the shim run wrote the files too")
}
