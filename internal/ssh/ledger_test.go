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

// 3.0: SYS-21 fixed: ssh-add -l exiting 2 is "no agent" whatever its
// text: a stale socket's refusal starts the agent search, and the unlock
// runs against the new agent.
func TestLedger_SYS21(t *testing.T) {
	fx := newFixture(t, "SSH:~/.ssh/key1=pw\n", "ssh-add", "ssh-agent")
	fx.env["SSH_AUTH_SOCK"] = "/tmp/dead.sock"
	sock := filepath.Join(fx.home, ".ssh", agentSock)
	m, h := tm(runner.NewFake(t, list("Error connecting to agent: Connection refused\n", 2),
		runner.Expect{Argv: []string{"ssh-agent", "-a", sock, "-s"}, Stdout: agentOut(sock)},
		list(noIdentities, 1, "SSH_AUTH_SOCK="+sock), unlocked(fx.key("key1"), "pw")))
	r := fx.call(m, nil)
	require.Equal(t, 0, r.rc)
	require.NotContains(t, r.err, "Error connecting")
	require.Contains(t, r.err, "SSH: Identity added: "+fx.key("key1"))
	require.Equal(t, "export SSH_AUTH_SOCK="+sock+"\nexport SSH_AGENT_PID=4242\nexport GASH_SSH_AUTOUNLOCK_RAN=1\n", h.String())

	// The search failing keeps the 2.x error line.
	fx = newFixture(t, "SSH:~/.ssh/key1=pw\n", "ssh-add", "ssh-agent")
	sock = filepath.Join(fx.home, ".ssh", agentSock)
	m, h = tm(runner.NewFake(t, list("Error connecting to agent: Connection refused\n", 2),
		runner.Expect{Argv: []string{"ssh-agent", "-a", sock, "-s"}, Status: 1},
		runner.Expect{Argv: []string{"ssh-agent", "-s"}, Status: 1},
		list("Error connecting to agent: Connection refused\n", 2)))
	require.Equal(t, result{"Error: ssh-agent is not running and could not be started automatically.\n", 0}, fx.call(m, nil))
	require.Empty(t, h.String())
}

// 3.0: SYS-22 fixed: the agent search makes private only a directory it
// created; an existing runtime directory keeps its mode, and one that
// group or others may write is refused for the socket (~/.ssh instead).
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
	require.Equal(t, os.FileMode(0o755), st.Mode().Perm())

	require.NoError(t, os.Chmod(run, 0o777))
	m, _ = tm(runner.NewFake(t, list(refused, 2),
		runner.Expect{Argv: []string{"ssh-agent", "-a", filepath.Join(fx.home, ".ssh", agentSock), "-s"}},
		runner.Expect{Argv: []string{"ssh-agent", "-s"}},
		list(refused, 2)))
	require.Equal(t, 0, fx.call(m, nil).rc)
	st, err = os.Stat(run)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o777), st.Mode().Perm())

	// A ~/.ssh the search creates is 0700.
	fx = newFixture(t, "SSH:~/key1=pw\n", "ssh-add", "ssh-agent")
	require.NoError(t, os.WriteFile(filepath.Join(fx.home, "key1"), nil, 0o600))
	require.NoError(t, os.RemoveAll(filepath.Join(fx.home, ".ssh")))
	m, _ = tm(runner.NewFake(t, list(refused, 2),
		runner.Expect{Argv: []string{"ssh-agent", "-a", filepath.Join(fx.home, ".ssh", agentSock), "-s"}},
		runner.Expect{Argv: []string{"ssh-agent", "-s"}},
		list(refused, 2)))
	require.Equal(t, 0, fx.call(m, nil).rc)
	st, err = os.Stat(filepath.Join(fx.home, ".ssh"))
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
