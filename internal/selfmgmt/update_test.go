// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// A toy 3.x tree for the flow: the compat bootstrap marker, VERSION, and a
// gash.sh that sets GASH_VERSION from VERSION and whose gash_username asks
// the binary of that version, as the generated one does. BREAK is replaced
// by the line that breaks a tag.
const toyGashSh = `GASH_VERSION=unknown
[[ -r "${BASH_SOURCE[0]%/*}/VERSION" ]] && IFS= read -r GASH_VERSION <"${BASH_SOURCE[0]%/*}/VERSION"
gash_username() { "${XDG_DATA_HOME:-$HOME/.local/share}/gash/versions/$GASH_VERSION/gash" version --json >/dev/null && echo user; }
BREAK
`

func bashRun(t *testing.T, dir string, env []string, script string) string {
	t.Helper()
	cmd := exec.Command("bash", "--noprofile", "--norc", "-ec", script)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s\n%s", script, out)
	return strings.TrimSpace(string(out))
}

// upgrade.md 3.6 steps 4 to 7 on toy trees: a target whose load fails
// (headless output, a non-zero interactive load, a wrong GASH_VERSION)
// is checked out and then left again, the previous binary in place; a good
// one is installed, recorded and the old binaries pruned to two versions;
// gash update --rollback goes back and forth.
func TestUpdate_SmokeRollback(t *testing.T) {
	for _, p := range []string{"git", "bash"} {
		if _, err := exec.LookPath(p); err != nil {
			t.Skip("no " + p)
		}
	}
	if platform() == "" {
		t.Skip("no Gash release for this platform")
	}
	d := t.TempDir()
	home, origin, inst, rel := d+"/home", d+"/origin", d+"/home/.gash", d+"/rel"
	require.NoError(t, os.MkdirAll(rel, 0o755))
	bash, err := exec.LookPath("bash")
	require.NoError(t, err)
	env := []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t"}
	callEnv := map[string]string{"GASH_DIR": inst, "GASH_RELEASE_URL": "file://" + rel, "BASH": bash}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		callEnv[k] = v
	}

	// tag adds a commit of the toy tree with VERSION v and the break line.
	tag := func(v, brk string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(origin+"/lib/compat", 0o755))
		require.NoError(t, os.WriteFile(origin+"/lib/compat/bootstrap.sh", nil, 0o644))
		require.NoError(t, os.WriteFile(origin+"/VERSION", []byte(v+"\n"), 0o644))
		require.NoError(t, os.WriteFile(origin+"/gash.sh", []byte(strings.Replace(toyGashSh, "BREAK", brk, 1)), 0o644))
		bashRun(t, origin, env, "git add -A; git commit -q -m "+v+"; git tag -a v"+v+" -m v"+v)
		_, a := relDir(t, v, map[string][]byte{binGash: fakeGash(v)}, "")
		asset := "gash_" + v + "_" + platform() + ".tar.gz"
		require.NoError(t, os.WriteFile(rel+"/"+asset, a, 0o644))
		f, err := os.OpenFile(rel+"/checksums.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		require.NoError(t, err)
		_, _ = f.WriteString(hexSum(a) + "  " + asset + "\n")
		require.NoError(t, f.Close())
	}
	headOf := func() string {
		return bashRun(t, inst, env, "git describe --tags --exact-match HEAD")
	}
	clean := func() {
		t.Helper()
		require.Empty(t, bashRun(t, inst, env, "git status --porcelain"))
	}
	vdir := home + "/.local/share/gash/versions/"

	require.NoError(t, os.MkdirAll(home, 0o755))
	bashRun(t, d, env, "git init -q origin")
	tag("3.0.0", "")
	bashRun(t, d, env, "git clone -q origin home/.gash 2>/dev/null; git -C home/.gash -c advice.detachedHead=false checkout -q v3.0.0")
	require.NoError(t, os.MkdirAll(vdir+"3.0.0", 0o755))
	require.NoError(t, os.WriteFile(vdir+"3.0.0/gash", fakeGash("3.0.0"), 0o755))

	// No update yet: nothing to roll back to.
	_, errOut, rc := callModule(t, New(), "gash_upgrade", callEnv)
	require.Equal(t, 0, rc, errOut)
	require.Contains(t, errOut, "already up-to-date (v3.0.0")
	_, errOut, rc = runUpdate(t, callEnv, "--rollback")
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, "Error: No previous version of "+inst+" to return to")

	for _, bad := range []struct{ v, brk, reason string }{
		{"3.0.1", "echo noise >&2", "the headless load printed output"},
		{"3.0.2", "if [[ $- == *i* ]]; then return 4; fi", "the interactive load returned 4"},
		{"3.0.3", "GASH_VERSION=3.0.0", `GASH_VERSION is "3.0.0", not "3.0.3"`},
		{"3.0.4", "unset -f gash_username", "gash_username did not answer"},
		{"3.0.5", "return 7", "the headless load returned 7"},
	} {
		tag(bad.v, bad.brk)
		_, errOut, rc = callModule(t, New(), "gash_upgrade", callEnv)
		require.Equal(t, 1, rc, bad.v)
		require.Contains(t, errOut, "OK: Installed gash "+bad.v+" for ", bad.v)
		require.Contains(t, errOut, "Error: v"+bad.v+" failed its smoke test ("+bad.reason, bad.v)
		require.Contains(t, errOut, "Info: Went back to v3.0.0; nothing else was changed.\n", bad.v)
		require.NotContains(t, errOut, "Upgraded", bad.v)
		require.Equal(t, "v3.0.0", headOf(), bad.v)
		clean()
		require.FileExists(t, vdir+"3.0.0/gash")
		// The next case needs a release above the broken one.
		bashRun(t, origin, env, "git tag -d v"+bad.v+" >/dev/null")
	}

	// A user's own directory under versions/ is never pruned.
	require.NoError(t, os.MkdirAll(vdir+"9.9.9", 0o755))
	require.NoError(t, os.WriteFile(vdir+"9.9.9/notes", nil, 0o644))
	require.NoError(t, os.MkdirAll(vdir+"mine", 0o755))

	tag("3.1.0", "")
	_, errOut, rc = runUpdate(t, callEnv)
	require.Equal(t, 0, rc, errOut)
	require.Contains(t, errOut, "OK: Installed gash 3.1.0 for ")
	require.Contains(t, errOut, "(verified against checksums.txt).\n")
	require.Contains(t, errOut, "OK: Upgraded Gash from v3.0.0 to v3.1.0.\n")
	require.Equal(t, "v3.1.0", headOf())
	clean()
	for _, v := range []string{"3.0.1", "3.0.2", "3.0.3", "3.0.4", "3.0.5"} {
		require.NoDirExists(t, vdir+v, "two versions are kept")
	}
	for _, v := range []string{"3.0.0", "3.1.0", "9.9.9", "mine"} {
		require.DirExists(t, vdir+v)
	}
	st, err := os.ReadFile(home + "/.local/state/gash/update")
	require.NoError(t, err)
	require.Contains(t, string(st), "dir="+inst+"\n")
	require.Contains(t, string(st), "previous_name=v3.0.0\n")

	_, errOut, rc = runUpdate(t, callEnv, "--rollback")
	require.Equal(t, 0, rc, errOut)
	require.Contains(t, errOut, "OK: Rolled back Gash from v3.1.0 to v3.0.0.\n")
	require.NotContains(t, errOut, "Installed gash")
	require.Equal(t, "v3.0.0", headOf())
	clean()
	_, errOut, rc = runUpdate(t, callEnv, "--rollback")
	require.Equal(t, 0, rc, errOut)
	require.Contains(t, errOut, "OK: Rolled back Gash from v3.0.0 to v3.1.0.\n")
	require.Equal(t, "v3.1.0", headOf())

	// The rollback reinstalls a binary that went missing, verified.
	require.NoError(t, os.RemoveAll(vdir+"3.0.0"))
	_, errOut, rc = runUpdate(t, callEnv, "--rollback")
	require.Equal(t, 0, rc, errOut)
	require.Contains(t, errOut, "OK: Installed gash 3.0.0 for ")
	require.Equal(t, "v3.0.0", headOf())

	// A dirty tree is refused before anything.
	require.NoError(t, os.WriteFile(inst+"/new", nil, 0o644))
	_, errOut, rc = runUpdate(t, callEnv, "--rollback")
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, "has local modifications")
	require.Equal(t, "v3.0.0", headOf())
	require.NoError(t, os.Remove(inst+"/new"))

	// A binary that cannot be verified stops the update before the
	// checkout.
	tag("3.2.0", "")
	require.NoError(t, os.WriteFile(rel+"/checksums.txt", []byte("00  gash_3.2.0_"+platform()+".tar.gz\n"), 0o644))
	_, errOut, rc = runUpdate(t, callEnv)
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, "Error: Could not install the gash binary of v3.2.0 (checksum mismatch for gash_3.2.0_")
	require.Contains(t, errOut, "); nothing was changed.\n")
	require.Equal(t, "v3.0.0", headOf())
	require.NoFileExists(t, vdir+"3.2.0/gash")

	_, errOut, rc = runUpdate(t, callEnv, "now")
	require.Equal(t, 2, rc)
	require.Equal(t, "Error: Unexpected argument: now (usage: gash update [--rollback])\n", errOut)
}

func runUpdate(t *testing.T, env map[string]string, args ...string) (string, string, int) {
	t.Helper()
	tbl := invoke.NewTable()
	New().Register(tbl)
	h, ok := tbl.LookupCommand("update")
	require.True(t, ok)
	var out, errb bytes.Buffer
	e := invoke.EnvMap(env)
	rc := h(&invoke.Call{Name: "update", Args: args, Env: e, Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, false, false), Handlers: tbl})
	return out.String(), errb.String(), rc
}
