// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ssh

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
	"github.com/mauriziofonte/gash/internal/ui"
)

// fakeHandoff records what the binary hands back.
type fakeHandoff struct {
	bytes.Buffer
	closed bool
}

func (f *fakeHandoff) Close() error { f.closed = true; return nil }

func tm(f runner.Runner) (*Module, *fakeHandoff) {
	h := &fakeHandoff{}
	return &Module{run: f, open: func() (io.WriteCloser, error) { return h, nil }, wait: 2 * time.Second, settle: 2 * time.Second}, h
}

type result struct {
	err string
	rc  int
}

// fixture is a HOME with key files, a settings file and a PATH holding
// the ssh tools.
type fixture struct {
	home, bin string
	env       map[string]string
	// out is the stdout of the last call (empty but for the help).
	out string
}

func newFixture(t *testing.T, settings string, tools ...string) *fixture {
	t.Helper()
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".ssh"), 0o700))
	for _, k := range []string{"key1", "key2", "key3"} {
		require.NoError(t, os.WriteFile(filepath.Join(home, ".ssh", k), nil, 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gash_env"), []byte(settings), 0o600))
	bin := filepath.Join(home, "bin")
	require.NoError(t, os.Mkdir(bin, 0o700))
	for _, n := range tools {
		require.NoError(t, os.WriteFile(filepath.Join(bin, n), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	}
	return &fixture{home: home, bin: bin, env: map[string]string{
		"HOME": home, "PATH": bin, "LANG": "C.UTF-8", "GASH__HANDOFF": "3", "TMPDIR": t.TempDir(),
	}}
}

func (fx *fixture) key(n string) string { return filepath.Join(fx.home, ".ssh", n) }

func (fx *fixture) call(m *Module, tt *term.Term, args ...string) result {
	e := invoke.EnvMap(fx.env)
	if tt == nil {
		tt = term.Fixed(e, false, false, false)
	}
	var out, errb bytes.Buffer
	c := &invoke.Call{Name: "gash_ssh_auto_unlock", Args: args, Env: e, Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb, Term: tt}
	tbl := invoke.NewTable()
	m.Register(tbl)
	rc := tbl.Run(c)
	fx.out = out.String()
	return result{errb.String(), rc}
}

func list(out string, status int, env ...string) runner.Expect {
	return runner.Expect{Argv: []string{"ssh-add", "-l"}, Stdout: out, Status: status, EnvHas: env}
}

const (
	noIdentities = "The agent has no identities.\n"
	refused      = "Could not open a connection to your authentication agent.\n"
)

// unlocked is the PTY dialog of an encrypted key with the right
// passphrase.
func unlocked(key, pass string) runner.Expect {
	return runner.Expect{PTY: true, Argv: []string{"ssh-add", "--", key}, Dialog: []runner.DialogStep{
		{Out: "Enter passphrase for " + key + ": "}, {In: pass + "\r"},
		{Out: "\r\nIdentity added: " + key + " (user@host)\r\n"},
	}}
}

func TestRunOnce(t *testing.T) {
	fx := newFixture(t, "SSH:~/.ssh/key1=pw\n", "ssh-add")
	fx.env["GASH_SSH_AUTOUNLOCK_RAN"] = "1"
	m, h := tm(runner.NewFake(t))
	require.Equal(t, result{"", 0}, fx.call(m, nil))
	require.Empty(t, h.String())

	r := fx.call(m, nil, "--help")
	require.Equal(t, 0, r.rc)
	require.Contains(t, fx.out, "gash_ssh_auto_unlock")
}

func TestNoKeys(t *testing.T) {
	fx := newFixture(t, "# none\n", "ssh-add")
	m, h := tm(runner.NewFake(t))
	require.Equal(t, result{"", 0}, fx.call(m, nil))
	require.Equal(t, "export GASH_SSH_AUTOUNLOCK_RAN=1\n", h.String())
	require.True(t, h.closed)
}

func TestNoSSHAdd(t *testing.T) {
	fx := newFixture(t, "SSH:~/.ssh/key1=pw\n")
	m, h := tm(runner.NewFake(t))
	require.Equal(t, result{"Error: ssh-add is not available; cannot auto-unlock SSH keys.\n", 0}, fx.call(m, nil))
	require.Empty(t, h.String())
	require.True(t, h.closed)
}

func TestUnlock(t *testing.T) {
	fx := newFixture(t, "SSH:~/.ssh/key1=pw\nSSH:~/.ssh/key2=pw2\n", "ssh-add")
	m, h := tm(runner.NewFake(t, list(noIdentities, 1), unlocked(fx.key("key1"), "pw"), unlocked(fx.key("key2"), "pw2")))
	r := fx.call(m, nil)
	require.Equal(t, result{"SSH: Enter passphrase for " + fx.key("key1") + ": \nSSH: Identity added: " + fx.key("key1") + " (user@host)\n" +
		"SSH: Enter passphrase for " + fx.key("key2") + ": \nSSH: Identity added: " + fx.key("key2") + " (user@host)\n", 0}, r)
	require.Equal(t, "export GASH_SSH_AUTOUNLOCK_RAN=1\n", h.String())

	// On a color terminal: the glyph and color of each level.
	m, _ = tm(runner.NewFake(t, list(noIdentities, 1), runner.Expect{PTY: true, Argv: []string{"ssh-add", "--", fx.key("key1")}, Dialog: []runner.DialogStep{
		{Out: "Agent pid 9\r\nThe agent has no identities.\r\nBad passphrase, try again for k: "},
	}}, unlocked(fx.key("key2"), "pw2")))
	tt := term.Fixed(invoke.EnvMap(fx.env), true, true, true)
	r = fx.call(m, tt)
	require.Equal(t, "\x1b[38;5;75m•\x1b[0m \x1b[38;5;39mSSH:\x1b[0m Agent pid 9\n"+
		"\x1b[38;5;220m!\x1b[0m \x1b[38;5;39mSSH:\x1b[0m The agent has no identities.\n"+
		"\x1b[38;5;196m✗\x1b[0m \x1b[38;5;39mSSH:\x1b[0m Bad passphrase, try again for k: spawn ssh-add -- "+fx.key("key2")+"\n"+
		"\x1b[38;5;75m•\x1b[0m \x1b[38;5;39mSSH:\x1b[0m Enter passphrase for "+fx.key("key2")+": \n"+
		"\x1b[38;5;82m✓\x1b[0m \x1b[38;5;39mSSH:\x1b[0m Identity added: "+fx.key("key2")+" (user@host)\n", r.err)
}

// The fingerprints the agent holds: those keys are skipped, KEY.pub
// fingerprinted when it is there.
func TestSkipLoaded(t *testing.T) {
	fx := newFixture(t, "SSH:~/.ssh/key1=pw\nSSH:~/.ssh/key2=pw2\nSSH:~/.ssh/key3=\n", "ssh-add", "ssh-keygen")
	require.NoError(t, os.WriteFile(fx.key("key2")+".pub", nil, 0o600))
	keygen := func(file, out string, status int) runner.Expect {
		return runner.Expect{Argv: []string{"ssh-keygen", "-lf", file}, Stdout: out, Stderr: "noise\n", Status: status}
	}
	f := runner.NewFake(t, list("256 SHA256:fp-key1 a@b (ED25519)\n256 SHA256:fp-key2 c@d (ED25519)\n", 0),
		keygen(fx.key("key1"), "256 SHA256:fp-key1 a@b (ED25519)\n", 0),
		keygen(fx.key("key2")+".pub", "256 SHA256:fp-key2 c@d (ED25519)\n", 0),
		keygen(fx.key("key3"), "256 SHA256:fp-key3 e@f (ED25519)\n", 1),
		// An empty passphrase is typed as an empty line.
		unlocked(fx.key("key3"), ""))
	m, h := tm(f)
	r := fx.call(m, nil)
	require.Equal(t, 0, r.rc)
	require.Contains(t, r.err, "SSH: Identity added: "+fx.key("key3"))
	require.Equal(t, "export GASH_SSH_AUTOUNLOCK_RAN=1\n", h.String())

	// Everything loaded: no ssh-add run, the marker set.
	fx = newFixture(t, "SSH:~/.ssh/key1=pw\n", "ssh-add", "ssh-keygen")
	m, h = tm(runner.NewFake(t, list("256 SHA256:fp-key1 a@b (ED25519)\n", 0), keygen(fx.key("key1"), "256 SHA256:fp-key1 a@b (ED25519)", 0)))
	require.Equal(t, result{"", 0}, fx.call(m, nil))
	require.Equal(t, "export GASH_SSH_AUTOUNLOCK_RAN=1\n", h.String())

	// Without ssh-keygen nothing is skipped.
	fx = newFixture(t, "SSH:~/.ssh/key1=pw\n", "ssh-add")
	m, _ = tm(runner.NewFake(t, list("256 SHA256:fp-key1 a@b (ED25519)\n", 0), unlocked(fx.key("key1"), "pw")))
	require.Equal(t, 0, fx.call(m, nil).rc)
}

func agentOut(sock string) string {
	return "SSH_AUTH_SOCK=" + sock + "; export SSH_AUTH_SOCK;\nSSH_AGENT_PID=4242; export SSH_AGENT_PID;\necho Agent pid 4242;\n"
}

func TestAgentStart(t *testing.T) {
	fx := newFixture(t, "SSH:~/.ssh/key1=pw\n", "ssh-add", "ssh-agent")
	run := filepath.Join(fx.home, "run")
	require.NoError(t, os.Mkdir(run, 0o700))
	fx.env["XDG_RUNTIME_DIR"] = run
	sock := filepath.Join(run, agentSock)
	f := runner.NewFake(t, list(refused, 2), runner.Expect{Argv: []string{"ssh-agent", "-a", sock, "-s"}, Stdout: agentOut(sock)},
		list(noIdentities, 1, "SSH_AUTH_SOCK="+sock, "SSH_AGENT_PID=4242"))
	e := unlocked(fx.key("key1"), "pw")
	e.EnvHas = []string{"SSH_AUTH_SOCK=" + sock}
	f.Expect(e)
	m, h := tm(f)
	r := fx.call(m, nil)
	require.Equal(t, 0, r.rc)
	require.Equal(t, "export SSH_AUTH_SOCK="+sock+"\nexport SSH_AGENT_PID=4242\nexport GASH_SSH_AUTOUNLOCK_RAN=1\n", h.String())

	// No socket on the fixed path: anywhere (ssh-agent -s), and the
	// caller's directory falls back to ~/.ssh when XDG_RUNTIME_DIR is not
	// a directory.
	fx.env["XDG_RUNTIME_DIR"] = filepath.Join(fx.home, "nope")
	ssh := filepath.Join(fx.home, ".ssh", agentSock)
	m, h = tm(runner.NewFake(t, list(refused, 2),
		runner.Expect{Argv: []string{"ssh-agent", "-a", ssh, "-s"}, Stdout: "echo Agent pid 1;\n\n", Status: 1},
		runner.Expect{Argv: []string{"ssh-agent", "-s"}, Stdout: agentOut("/tmp/ssh-x/agent.1")},
		list(noIdentities, 1, "SSH_AUTH_SOCK=/tmp/ssh-x/agent.1"), unlocked(fx.key("key1"), "pw")))
	require.Equal(t, 0, fx.call(m, nil).rc)
	require.Equal(t, "export SSH_AUTH_SOCK=/tmp/ssh-x/agent.1\nexport SSH_AGENT_PID=4242\nexport GASH_SSH_AUTOUNLOCK_RAN=1\n", h.String())
}

func TestAgentFails(t *testing.T) {
	fx := newFixture(t, "SSH:~/.ssh/key1=pw\n", "ssh-add", "ssh-agent")
	sock := filepath.Join(fx.home, ".ssh", agentSock)
	m, h := tm(runner.NewFake(t, list(refused, 2),
		runner.Expect{Argv: []string{"ssh-agent", "-a", sock, "-s"}, Status: 1},
		runner.Expect{Argv: []string{"ssh-agent", "-s"}, Status: 1},
		list(refused, 2)))
	require.Equal(t, result{"Error: ssh-agent is not running and could not be started automatically.\n", 0}, fx.call(m, nil))
	require.Empty(t, h.String())

	// Without ssh-agent the same error, the variables an agent set kept.
	fx = newFixture(t, "SSH:~/.ssh/key1=pw\n", "ssh-add")
	m, h = tm(runner.NewFake(t, list(refused, 2), list(refused, 2)))
	require.Equal(t, result{"Error: ssh-agent is not running and could not be started automatically.\n", 0}, fx.call(m, nil))
	require.Empty(t, h.String())

	fx = newFixture(t, "SSH:~/.ssh/key1=pw\n", "ssh-add", "ssh-agent")
	sock = filepath.Join(fx.home, ".ssh", agentSock)
	m, h = tm(runner.NewFake(t, list(refused, 2), runner.Expect{Argv: []string{"ssh-agent", "-a", sock, "-s"}, Stdout: agentOut("/x")},
		list(refused, 2, "SSH_AUTH_SOCK=/x")))
	require.Equal(t, 0, fx.call(m, nil).rc)
	require.Equal(t, "export SSH_AUTH_SOCK=/x\nexport SSH_AGENT_PID=4242\n", h.String())
}

// The agent on the fixed socket is reused while it answers; a dead one is
// removed and replaced.
func TestAgentReuse(t *testing.T) {
	fx := newFixture(t, "SSH:~/.ssh/key1=pw\n", "ssh-add", "ssh-agent")
	run, err := os.MkdirTemp("/tmp", "gs") // a socket path stays short
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(run) })
	fx.env["XDG_RUNTIME_DIR"] = run
	sock := filepath.Join(run, agentSock)
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	l.(*net.UnixListener).SetUnlinkOnClose(false)
	require.NoError(t, l.Close())

	alive := runner.Expect{Argv: []string{"ssh-add", "-l"}, EnvHas: []string{"SSH_AUTH_SOCK=" + sock}, Status: 1}
	m, h := tm(runner.NewFake(t, list(refused, 2), alive, list(noIdentities, 1, "SSH_AUTH_SOCK="+sock), unlocked(fx.key("key1"), "pw")))
	require.Equal(t, 0, fx.call(m, nil).rc)
	require.Equal(t, "export SSH_AUTH_SOCK="+sock+"\nexport GASH_SSH_AUTOUNLOCK_RAN=1\n", h.String())

	dead := alive
	dead.Status = 2
	m, _ = tm(runner.NewFake(t, list(refused, 2), dead, runner.Expect{Argv: []string{"ssh-agent", "-a", sock, "-s"}, Stdout: agentOut(sock)},
		list(noIdentities, 1), unlocked(fx.key("key1"), "pw")))
	require.Equal(t, 0, fx.call(m, nil).rc)
	_, err = os.Lstat(sock)
	require.True(t, os.IsNotExist(err))
}

// A direct run unlocks without a hand-off.
func TestDirectRun(t *testing.T) {
	fx := newFixture(t, "SSH:~/.ssh/key1=pw\n", "ssh-add")
	delete(fx.env, "GASH__HANDOFF")
	m, h := tm(runner.NewFake(t, list(noIdentities, 1), unlocked(fx.key("key1"), "pw")))
	require.Equal(t, 0, fx.call(m, nil).rc)
	require.Empty(t, h.String())
	require.False(t, h.closed)
}

func TestAgentAssignments(t *testing.T) {
	for in, want := range map[string]string{
		agentOut("/tmp/a"): "SSH_AUTH_SOCK=/tmp/a SSH_AGENT_PID=4242",
		"SSH_AUTH_SOCK='/tmp/a b'; export SSH_AUTH_SOCK": "SSH_AUTH_SOCK=/tmp/a b",
		`SSH_AUTH_SOCK="/tmp/q"`:                         "SSH_AUTH_SOCK=/tmp/q",
		"export SSH_AUTH_SOCK=/e SSH_AGENT_PID=7":        "SSH_AUTH_SOCK=/e SSH_AGENT_PID=7",
		"SSH_AUTH_SOCK=/a b/c; export SSH_AUTH_SOCK":     "",
		"SSH_AUTH_SOCK=$(id)":                            "",
		"SSH_AUTH_SOCK=/a$HOME":                          "",
		`SSH_AUTH_SOCK=/a\ b`:                            "",
		"SSH_AGENT_PID=12x":                              "",
		"SSH_AUTH_SOCK=/r >/tmp/f":                       "",
		"SSH_AUTH_SOCK=/bg &":                            "",
		"OTHER=1; SSH_AUTH_SOCK=":                        "SSH_AUTH_SOCK=",
		"if then\nSSH_AUTH_SOCK=/l":                      "SSH_AUTH_SOCK=/l",
	} {
		var got []string
		for _, a := range agentAssignments(in) {
			got = append(got, a[0]+"="+a[1])
		}
		require.Equal(t, want, strings.Join(got, " "), in)
	}
}

func TestPrintTranscript(t *testing.T) {
	fx := newFixture(t, "")
	e := invoke.EnvMap(fx.env)
	var b bytes.Buffer
	p := ui.NewPrinter(&b, term.Fixed(e, false, false, false), false)
	printTranscript(p, "spawn ssh-add -- /k\r\n\r\nAgent pid 1\r\r\nIdentity added: x\nCould not open a connection to your authentication agent.\r\nother\n\n\n")
	require.Equal(t, "SSH: Agent pid 1\r\nSSH: Identity added: x\nSSH: Could not open a connection to your authentication agent.\nSSH: other\n", b.String())
}
