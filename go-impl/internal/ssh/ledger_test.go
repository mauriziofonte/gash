// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ssh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/runner"
)

// SYS-21: only the exact "Could not open a connection" text starts an
// agent; a stale socket's refusal (status 2) goes to the unlock, whose
// lines are info, and the marker is set.
func TestLedger_SYS21(t *testing.T) {
	fx := newFixture(t, "SSH:~/.ssh/key1=pw\n", "ssh-add", "ssh-agent")
	fx.env["SSH_AUTH_SOCK"] = "/tmp/dead.sock"
	m, h := tm(runner.NewFake(t, list("Error connecting to agent: Connection refused\n", 2),
		runner.Expect{PTY: true, Argv: []string{"ssh-add", "--", fx.key("key1")}, Dialog: []runner.DialogStep{
			{Out: "Error connecting to agent: Connection refused\r\n"},
		}}))
	require.Equal(t, result{"SSH: Error connecting to agent: Connection refused\n", 0}, fx.call(m, nil))
	require.Equal(t, "export GASH_SSH_AUTOUNLOCK_RAN=1\n", h.String())
}

// SYS-22: the agent search chmods an existing runtime directory to 700.
func TestLedger_SYS22(t *testing.T) {
	fx := newFixture(t, "SSH:~/.ssh/key1=pw\n", "ssh-add", "ssh-agent")
	run := filepath.Join(fx.home, "run")
	require.NoError(t, os.Mkdir(run, 0o700))
	require.NoError(t, os.Chmod(run, 0o755))
	fx.env["XDG_RUNTIME_DIR"] = run
	m, _ := tm(runner.NewFake(t, list(refused, 2),
		runner.Expect{Argv: []string{"ssh-agent", "-a", filepath.Join(run, agentSock), "-s"}},
		runner.Expect{Argv: []string{"ssh-agent", "-s"}},
		list(refused, 2)))
	require.Equal(t, 0, fx.call(m, nil).rc)
	st, err := os.Stat(run)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), st.Mode().Perm())
}

// SYS-35: the parsed entries, never shell words: passphrases with glob
// characters reach ssh-add as they are, nothing else is printed.
func TestLedger_SYS35(t *testing.T) {
	fx := newFixture(t, "SSH:~/.ssh/key1=pa*ss\nSSH:~/.ssh/key2=a?b\nSSH:~/.ssh/key3=[x]\n", "ssh-add")
	quiet := func(key, pass string) runner.Expect {
		return runner.Expect{PTY: true, Argv: []string{"ssh-add", "--", key}, Dialog: []runner.DialogStep{
			{Out: "Enter passphrase for " + key + ":"}, {In: pass + "\r"}, {Out: "\r\n"},
		}}
	}
	m, h := tm(runner.NewFake(t, list(noIdentities, 1), quiet(fx.key("key1"), "pa*ss"), quiet(fx.key("key2"), "a?b"), quiet(fx.key("key3"), "[x]")))
	r := fx.call(m, nil)
	require.Equal(t, result{"SSH: Enter passphrase for " + fx.key("key1") + ":\nSSH: Enter passphrase for " + fx.key("key2") + ":\n" +
		"SSH: Enter passphrase for " + fx.key("key3") + ":\n", 0}, r)
	require.Equal(t, "export GASH_SSH_AUTOUNLOCK_RAN=1\n", h.String())
}

// SYS-37: ssh-add on a terminal instead of expect: no expect needed, the
// passphrase typed once and found in no argv, environment or file.
func TestLedger_SYS37(t *testing.T) {
	fx := newFixture(t, "SSH:~/.ssh/key1=pw-secret\n", "ssh-add")
	f := runner.NewFake(t, list(noIdentities, 1), runner.Expect{PTY: true, Argv: []string{"ssh-add", "--", fx.key("key1")}, Dialog: []runner.DialogStep{
		{Out: "Enter passphrase for " + fx.key("key1") + ": "}, {In: "pw-secret\r"},
		{Out: "\r\nIdentity added: " + fx.key("key1") + " (user@host)\r\n"},
		// Read after the closing pattern: not in the transcript.
	}})
	m, _ := tm(f)
	r := fx.call(m, nil)
	require.Equal(t, 0, r.rc)
	require.Contains(t, r.err, "SSH: Identity added: "+fx.key("key1")+" (user@host)\n")
	for _, c := range f.Calls() {
		require.NotContains(t, strings.Join(c.Argv, " "), "pw-secret")
		require.NotContains(t, strings.Join(c.Env, " "), "pw-secret")
	}
	ents, err := os.ReadDir(fx.env["TMPDIR"])
	require.NoError(t, err)
	require.Empty(t, ents)
}

// SYS-39: only the two assignments of the agent's output are taken, as
// quoted exports; nothing of it runs.
func TestLedger_SYS39(t *testing.T) {
	fx := newFixture(t, "SSH:~/.ssh/key1=pw\n", "ssh-add", "ssh-agent")
	marker := filepath.Join(fx.home, "marker")
	out := ": >" + marker + "; SSH_AUTH_SOCK='/tmp/x y.sock'; export SSH_AUTH_SOCK;\nSSH_AGENT_PID=$(touch " + marker + ");\n"
	m, h := tm(runner.NewFake(t, list(refused, 2),
		runner.Expect{Argv: []string{"ssh-agent", "-a", filepath.Join(fx.home, ".ssh", agentSock), "-s"}, Stdout: out},
		list(noIdentities, 1, "SSH_AUTH_SOCK=/tmp/x y.sock"), unlocked(fx.key("key1"), "pw")))
	require.Equal(t, 0, fx.call(m, nil).rc)
	require.Equal(t, "export SSH_AUTH_SOCK='/tmp/x y.sock'\nexport GASH_SSH_AUTOUNLOCK_RAN=1\n", h.String())
	_, err := os.Stat(marker)
	require.True(t, os.IsNotExist(err))
}

// realSSHAdd is an ssh-add on PATH for the real terminal driver: -l says
// the agent is empty; a key ending in "slow" asks for a PIN and hangs; any
// other key asks for its passphrase without echo.
const realSSHAdd = `#!/bin/sh
[ "$1" = -l ] && { echo "The agent has no identities."; exit 1; }
case "$2" in
*slow) printf 'Enter PIN for %s: ' "$2"; exec sleep 30 ;;
esac
stty -echo
printf 'Enter passphrase for %s: ' "$2"
read -r p
printf '\n'
[ "$p" = pw ] && echo "Identity added: $2 (x)" || echo "Bad passphrase"
`

// The real terminal: the dialog, the 20 s wait (shortened) that moves on
// to the next key, the transcript joining the hung prompt with the next
// spawn line, and the hung ssh-add hung up at the end.
func TestRealPTY(t *testing.T) {
	fx := newFixture(t, "SSH:~/.ssh/key1=pw\n")
	require.NoError(t, os.WriteFile(filepath.Join(fx.home, ".ssh", "keyslow"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(fx.bin, "ssh-add"), []byte(realSSHAdd), 0o755))
	fx.env["PATH"] = fx.bin + ":/usr/bin:/bin"
	m, h := tm(runner.Exec{})
	r := fx.call(m, nil)
	require.Equal(t, result{"SSH: Enter passphrase for " + fx.key("key1") + ": \nSSH: Identity added: " + fx.key("key1") + " (x)\n", 0}, r)
	require.Equal(t, "export GASH_SSH_AUTOUNLOCK_RAN=1\n", h.String())

	require.NoError(t, os.WriteFile(filepath.Join(fx.home, ".gash_env"), []byte("SSH:~/.ssh/keyslow=pw\nSSH:~/.ssh/key1=pw\n"), 0o600))
	m, _ = tm(runner.Exec{})
	m.wait = 300 * time.Millisecond
	start := time.Now()
	r = fx.call(m, nil)
	require.Less(t, time.Since(start), 10*time.Second)
	require.Equal(t, result{"SSH: Enter PIN for " + fx.key("keyslow") + ": spawn ssh-add -- " + fx.key("key1") + "\n" +
		"SSH: Enter passphrase for " + fx.key("key1") + ": \nSSH: Identity added: " + fx.key("key1") + " (x)\n", 0}, r)
}
