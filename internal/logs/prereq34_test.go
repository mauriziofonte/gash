// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package logs

import (
	"bytes"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/term"
)

// debian is Debian 12 for a user with sudo, without the named programs.
func debian(missing ...string) *prereq.System {
	return &prereq.System{OS: platform.OS{GOOS: "linux", ID: "debian", Name: "Debian GNU/Linux 12 (bookworm)"},
		Manager: prereq.Apt, Sudo: !slices.Contains(missing, "sudo"), Root: slices.Contains(missing, "sudo"),
		LookPath: func(name string, _ []string) (string, bool) {
			return "/usr/bin/" + name, !slices.Contains(missing, name)
		}}
}

// rootRun is the check of a `gash logs clean --sudo` over ops: its
// verdict, status and stderr.
func rootRun(asJSON bool, ops []op) (bool, int, string) {
	e := invoke.EnvMap(map[string]string{"PATH": "/usr/bin"})
	var errb bytes.Buffer
	c := &invoke.Call{Name: "gash", Env: e, Stdout: &bytes.Buffer{}, Stderr: &errb, Term: term.Fixed(e, false, false, false)}
	r := &run{c: c, useSudo: true, asJSON: asJSON}
	ok := r.rootReady(ops)
	return ok, r.status, errb.String()
}

// `gash logs clean --sudo` checks sudo, and truncate when a root-owned
// live log is truncated, before it asks or changes anything (3.4: their
// absence was "N files could not be removed." after the other changes).
func TestCleanRootTools(t *testing.T) {
	rootDel := op{Path: "/var/log/syslog.1", needRoot: true}
	rootTrunc := op{Path: "/var/log/syslog", Truncate: true, needRoot: true}
	mine := op{Path: "/home/me/app.log", Truncate: true}

	restore := prereq.SetSystem(debian("sudo", "truncate"))
	ok, rc, errOut := rootRun(false, []op{mine, rootDel})
	require.False(t, ok)
	require.Equal(t, 1, rc)
	require.Equal(t, "Error: gash logs clean --sudo needs sudo, which is not installed.\n"+
		"  sudo runs the privileged steps of some functions (package sudo on Debian GNU/Linux 12).\n"+
		"  Install it:  apt-get install -y sudo\n"+
		"  Or run:      gash_doctor --install sudo\n", errOut)
	// Nothing root-owned: no sudo needed.
	ok, _, errOut = rootRun(false, []op{mine})
	require.True(t, ok)
	require.Empty(t, errOut)
	restore()

	restore = prereq.SetSystem(debian("truncate"))
	ok, _, errOut = rootRun(true, []op{rootDel, rootTrunc})
	require.False(t, ok)
	require.Equal(t, `{"error":"prerequisite_missing","details":"gash logs clean --sudo --truncate needs truncate, which is not installed",`+
		`"action":"STOP","recoverable":true,"hint":"Install coreutils: sudo apt-get install -y coreutils","missing":"truncate",`+
		`"install":"sudo apt-get install -y coreutils"}`+"\n", errOut)
	// Deletes alone need no truncate.
	ok, _, _ = rootRun(true, []op{rootDel})
	require.True(t, ok)
	restore()
}
