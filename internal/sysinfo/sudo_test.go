// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// Root needs no sudo; cached credentials make every privileged read go
// through `sudo cat` and `sudo find`, with the same output.
func TestSudoReads(t *testing.T) {
	fx := newFixture(t, etcTree)
	require.Equal(t, result{llmWant["auth"], "", 0}, fx.call(t, nil, "auth", "--llm"))
	require.Empty(t, fx.sudo(t))

	fx.uid = 1000
	fx.env["SUDO_CACHED"] = "0"
	require.Equal(t, result{llmWant["auth"], "", 0}, fx.call(t, nil, "auth", "--llm"))
	require.Equal(t, "sudo -n true\nsudo cat R/etc/sudoers\nsudo find R/etc/sudoers.d -type f\nsudo cat R/etc/sudoers.d/alice\n"+
		"sudo cat R/etc/ssh/sshd_config\nsudo find R/etc/ssh/sshd_config.d -name *.conf\nsudo cat R/etc/ssh/sshd_config.d/50-local.conf\n", fx.sudo(t))
}

// No cached credentials and no terminal (or headless): no prompt, the
// reads are the user's.
func TestSudoUnavailable(t *testing.T) {
	fx := newFixture(t, etcTree)
	fx.uid = 1000
	require.Equal(t, result{llmWant["auth"], "", 0}, fx.call(t, fx.tty(), "auth", "--llm", "--no-color"))
	require.Equal(t, "sudo -n true\nsudo -v\n", fx.sudo(t))

	fx = newFixture(t, etcTree)
	fx.uid = 1000
	require.Equal(t, result{llmWant["auth"], "", 0}, fx.call(t, nil, "auth", "--llm"))
	require.Equal(t, "sudo -n true\n", fx.sudo(t))

	fx = newFixture(t, etcTree)
	fx.uid = 1000
	fx.env["GASH_HEADLESS"] = "1"
	require.Equal(t, result{llmWant["auth"], "", 0}, fx.call(t, fx.tty(), "auth", "--llm"))
	require.Equal(t, "sudo -n true\n", fx.sudo(t))
}

// On a terminal, verbose mode says why it asks, and what failed (3.0:
// SYS-15 fixed: a section that reads as root, network asks nothing).
func TestSudoPrompt(t *testing.T) {
	fx := newFixture(t)
	fx.uid = 1000
	r := fx.call(t, fx.tty(), "identity")
	require.Equal(t, term.SeqInfo+"•"+term.SeqOff+" Elevated privileges needed for full system enumeration.\n"+
		term.SeqWarning+"! Warning:"+term.SeqOff+" Could not obtain sudo. Some data will be limited.\n", r.err)
	require.Equal(t, "sudo -n true\nsudo -v\n", fx.sudo(t))

	// --no-color empties the colors, the glyphs stay (2.x locals).
	fx = newFixture(t)
	fx.uid = 1000
	r = fx.call(t, fx.tty(), "identity", "--no-color")
	require.Equal(t, "• Elevated privileges needed for full system enumeration.\n"+
		"! Warning: Could not obtain sudo. Some data will be limited.\n", r.err)

	fx = newFixture(t, etcTree)
	fx.uid = 1000
	fx.env["SUDO_V"] = "0"
	r = fx.call(t, term.Fixed(invoke.EnvMap(fx.env), true, false, false), "auth")
	require.Equal(t, "Info: Elevated privileges needed for full system enumeration.\n", r.err)
	require.True(t, strings.HasPrefix(fx.sudo(t), "sudo -n true\nsudo -v\nsudo cat R/etc/sudoers\n"), fx.sudo(t))
}

// The keep-alive refreshes the credentials until Release, then stops.
func TestSudoKeepAlive(t *testing.T) {
	fx := newFixture(t)
	fx.uid = 1000
	fx.env["SUDO_CACHED"] = "0"
	fx.m.tick = 5 * time.Millisecond
	e := invoke.EnvMap(fx.env)
	c := &invoke.Call{Name: "sysinfo", Env: e, Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Term: term.Fixed(e, false, false, false)}
	s := fx.m.Begin(c, false, true)
	require.True(t, s.sudo)
	require.Eventually(t, func() bool { return strings.Count(fx.sudo(t), "sudo -n true\n") >= 4 }, 10*time.Second, 5*time.Millisecond)
	s.Release()
	s.Release()
	n := fx.sudo(t)
	time.Sleep(30 * time.Millisecond)
	require.Equal(t, n, fx.sudo(t))
	require.Equal(t, strings.Count(n, "\n"), strings.Count(n, "sudo -n true\n"))
}

// Collect is a collector as ai_sysinfo runs it: compact, every header at
// once, nothing found never said.
func TestCollect(t *testing.T) {
	fx := newFixture(t, tree{"etc/dovecot/": "", "etc/postfix/main.cf": "# only\n"})
	e := invoke.EnvMap(fx.env)
	var w bytes.Buffer
	c := &invoke.Call{Name: "ai_sysinfo", Env: e, Stdin: strings.NewReader(""), Stdout: &w, Stderr: &w, Term: term.Fixed(e, true, true, true)}
	s := fx.m.Begin(c, false, false)
	defer s.Release()
	s.Collect(&w, "mail")
	s.Collect(&w, "network")
	require.Equal(t, "=== MAIL ===\n--- Postfix\n--- Dovecot\n=== NETWORK ===\n--- resolv.conf\n--- hosts\n", w.String())
	_, err := os.Stat(fx.env["SUDO_LOG"])
	require.True(t, os.IsNotExist(err))
}
