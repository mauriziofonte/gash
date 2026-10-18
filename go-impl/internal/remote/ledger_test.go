// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

const boxEnv = "HOST:box=deploy@box.example\n"

// argv is the arguments of the stub call named name in a log, in order.
func argv(log, name string) [][]string {
	var calls [][]string
	var cur *[]string
	for _, l := range strings.Split(log, "\n") {
		switch {
		case l == "ssh" || l == "scp" || l == "rsync":
			calls = append(calls, nil)
			if l == name {
				cur = &calls[len(calls)-1]
			} else {
				cur = nil
			}
		case cur != nil && strings.HasPrefix(l, "  [") && strings.HasSuffix(l, "]"):
			*cur = append(*cur, l[3:len(l)-1])
		}
	}
	var out [][]string
	for _, c := range calls {
		if c != nil {
			out = append(out, c)
		}
	}
	return out
}

// LEDGER REM-01 (rc.A reproduced): unknown flags and a fourth positional
// are ignored, also with --delete.
func TestLedger_REM01_2x(t *testing.T) {
	r := newScene(t, boxEnv).same(headless, "remote_sync_push", "box", "./src", "/tmp/build", "--delete", "-n")
	require.Equal(t, 0, r.RC)
	a := argv(r.Log, "rsync")[0]
	require.Contains(t, a, "--delete")
	require.NotContains(t, a, "--dry-run")

	r = newScene(t, boxEnv).same(headless, "remote_sync_push", "box", "./src", "/tmp/build", "--delete", "--exclude", ".env")
	require.Equal(t, []string{"--delete", "--", "./src", "deploy@box.example:/tmp/build"}, argv(r.Log, "rsync")[0][4:])

	for _, typo := range []string{"--delte", "--forse"} {
		r = newScene(t, boxEnv).same(headless, "remote_sync_push", "box", "./src", "/tmp/build", typo)
		require.Equal(t, 0, r.RC, typo)
		require.NotContains(t, argv(r.Log, "rsync")[0], "--delete")
	}

	r = newScene(t, boxEnv).same(headless, "remote_push", "box", "./x", "/tmp/x", "--dry-run", "extra")
	require.Equal(t, 0, r.RC)
	require.Equal(t, [][]string{{"-o", "ConnectTimeout=10", "-o", "BatchMode=yes", "--", "./x", "deploy@box.example:/tmp/x"}}, argv(r.Log, "scp"))

	r = newScene(t, boxEnv).same(headless, "remote_sync_pull", "box", "/srv/d", "./d", "--delete", "--dryrun")
	require.Equal(t, 0, r.RC)
	require.Contains(t, argv(r.Log, "rsync")[0], "--delete")
}

// LEDGER REM-02 (rc.A reproduced): an option-like target is a valid HOST
// entry and reaches ssh in option position, in the probe too.
func TestLedger_REM02_2x(t *testing.T) {
	env := "HOST:opt=-oProxyCommand=x@h\n"
	r := newScene(t, env).same(plain, "remote_hosts")
	require.Contains(t, r.Out, "-oProxyCommand=x@h")

	s := newScene(t, env)
	s.files["ssh.status"] = "255 255"
	r = s.same(plain, "remote_exec", "opt", "true")
	calls := argv(r.Log, "ssh")
	require.Len(t, calls, 2)
	require.Equal(t, []string{"-oProxyCommand=x@h", "unset HISTFILE HISTSIZE; true"}, calls[0][14:])
	require.Equal(t, []string{"-oProxyCommand=x@h", "exit 0"}, calls[1][16:])

	// Interactively a raw option-like target is accepted too.
	r = newScene(t, "").same(plain, "remote_exec", "-oX=y@h", "true")
	require.Equal(t, "-oX=y@h", argv(r.Log, "ssh")[0][14])
}

// LEDGER REM-03 (rc.A reproduced): the tiers and the --delete safe set
// read the literal string.
func TestLedger_REM03_2x(t *testing.T) {
	r := newScene(t, boxEnv).same(headless, "remote_sync_push", "box", "./src", "$HOME", "--delete")
	require.Equal(t, 0, r.RC)
	require.Equal(t, "deploy@box.example:$HOME", argv(r.Log, "rsync")[0][len(argv(r.Log, "rsync")[0])-1])
	for _, p := range []string{"/e?c/shadow", "/e[t]c/shadow"} {
		require.Equal(t, "allow", classifyPath(p), p)
	}
	for _, p := range []string{"$HOME", "${HOME}", "*", "./*", "$(echo ~)", "`pwd`"} {
		require.Equal(t, "allow", classifyPath(p), p)
		require.True(t, safeForDelete(p), p)
	}
}

// LEDGER REM-04 (forced divergence): script arguments are quoted
// POSIX-style, so sh and bash on the remote parse the same words, which
// are the caller's.
func TestLedger_REM04(t *testing.T) {
	vectors := []string{"a b", "x\ny", "é", "~", "", "$(id)", "it's", "--", "-c", "`id`", "\\", "*"}
	cmd := historyOff + "bash -s --" + scriptArgs(vectors)
	require.False(t, strings.HasSuffix(cmd, " "))
	for _, sh := range []string{"sh", "bash", "dash"} {
		if _, err := exec.LookPath(sh); err != nil {
			continue
		}
		require.Equal(t, vectors, parseArgs(t, sh, cmd), sh)
	}
	// 2.x quoted with printf %q: Bash-only, with a trailing space.
	got := bash2x(t, `printf '%q ' "a b" $'x\ny'`)
	require.Equal(t, `a\ b $'x\ny' `, got)
}

// LEDGER REM-05 (rc.A reproduced): HOME goes unquoted into the rsync -e
// string, so a HOME with a space splits ControlPath.
func TestLedger_REM05_2x(t *testing.T) {
	s := newScene(t, boxEnv)
	s.homeName = "John Doe"
	home := s.home()
	r := s.same(headless, "remote_sync_push", "box", "./s", "/tmp/d")
	e := argv(r.Log, "rsync")[0][2]
	require.Contains(t, e, "-o ControlPath="+home+"/.ssh/cm-%C -o")
	words := strings.Fields(e)
	require.Contains(t, words, "Doe/.ssh/cm-%C", "rsync splits the -e string on spaces")
}

// LEDGER REM-06 (rc.A reproduced): the local --delete check of
// remote_sync_pull speaks of the remote home.
func TestLedger_REM06_2x(t *testing.T) {
	r := newScene(t, boxEnv).same(headless, "remote_sync_pull", "box", "/srv", "./", "--delete")
	require.Equal(t, 1, r.RC)
	require.Equal(t, "Error: rsync --delete targeting './' is outside the safe set (below /tmp, /var/tmp, /home/<user>/ or the remote home; never the home itself or .ssh). Refused in headless mode without --force.\n", r.Err)
	require.Empty(t, r.Log)
}

// LEDGER REM-07 (rc.A reproduced): remote_exec creates ~/.ssh with the
// caller's umask.
func TestLedger_REM07_2x(t *testing.T) {
	old := syscall.Umask(0o022)
	defer syscall.Umask(old)
	r := newScene(t, boxEnv).same(plain, "remote_exec", "box", "true")
	require.Contains(t, r.Tree, "home/.ssh drwxr-xr-x")
}

// LEDGER REM-08 (rc.A reproduced): a HOST target u@h:22 is accepted and
// scp reads its port as a path.
func TestLedger_REM08_2x(t *testing.T) {
	r := newScene(t, "HOST:x=u@h:22\n").same(headless, "remote_push", "x", "./f", "/tmp/f")
	require.Equal(t, [][]string{{"-o", "ConnectTimeout=10", "-o", "BatchMode=yes", "--", "./f", "u@h:22:/tmp/f"}}, argv(r.Log, "scp"))
}
