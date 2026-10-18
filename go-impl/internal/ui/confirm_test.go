// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ui

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/term"
)

// run2xTTY runs body after the 2.x output.sh and utils.sh in a Bash whose
// controlling terminal is a pseudo-terminal (script(1)) on which input is
// typed; it returns what body wrote to stderr and its status. Skipped
// without script(1).
func run2xTTY(t *testing.T, input, mode, body string) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("script(1) not available")
	}
	root, err := filepath.Abs(repoRoot)
	require.NoError(t, err)
	if _, err := os.Stat(filepath.Join(root, "lib/core/output.sh")); err != nil {
		t.Skip("the 2.x sources are not in this tree")
	}
	dir := t.TempDir()
	src := "source " + shq(root+"/lib/core/output.sh") + "; source " + shq(root+"/lib/core/utils.sh") + "\n" +
		in2x(mode, body+" 2>"+shq(dir+"/err")+"; echo $? >"+shq(dir+"/rc"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "s.sh"), []byte(src), 0o600))
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "script", "-qec", "bash --noprofile --norc "+shq(dir+"/s.sh"), "/dev/null")
	cmd.Env = []string{"HOME=" + dir, "PATH=/usr/bin:/bin", "TERM=dumb", "LANG=C.UTF-8"}
	cmd.Stdin = strings.NewReader(input)
	require.NoError(t, cmd.Run())
	errOut, err := os.ReadFile(filepath.Join(dir, "err"))
	require.NoError(t, err)
	rc, err := os.ReadFile(filepath.Join(dir, "rc"))
	require.NoError(t, err)
	return string(errOut), strings.TrimSpace(string(rc))
}

var confirmInputs = []string{"y\n", "Y\n", " y \n", "\ty\n", "n\n", "\n", "yes\n", "YES\n", "Yes\n", "yeS\n", "ye\n", "yy\n", "y y\n", "no\n", "x\n"}

// needs_confirm_prompt and __gash_confirm_plan on a terminal: the same
// question bytes, the same answer for each input.
func TestConfirmMatches2x(t *testing.T) {
	for _, mode := range []string{"off", "on"} {
		for _, in := range confirmInputs {
			s := modeStyle(mode, true, 80)
			tt := term.Fixed(nil, true, true, true).WithTTY(in)

			want, rc := run2xTTY(t, in, mode, "needs_confirm_prompt 'Delete it?'")
			var b bytes.Buffer
			ok := s.Confirm(tt, &b, "Delete it?")
			require.Equal(t, want, b.String(), "%q %s", in, mode)
			require.Equal(t, rc == "0", ok, "%q %s", in, mode)

			want, rc = run2xTTY(t, in, mode, "__gash_confirm_plan 0")
			b.Reset()
			ok = s.ConfirmPlan(tt.WithTTY(in), &b, "0")
			require.Equal(t, want, b.String(), "plan %q %s", in, mode)
			require.Equal(t, rc == "0", ok, "plan %q %s", in, mode)
		}
	}
}

func TestConfirmRefusals(t *testing.T) {
	s := modeStyle("off", true, 80)
	tty := term.Fixed(nil, true, true, true).WithTTY("y\n")
	var b bytes.Buffer

	// Headless: no question, no message.
	h := s
	h.Headless = true
	require.False(t, h.Confirm(tty, &b, "Go?"))
	require.Empty(t, b.String())
	require.False(t, h.ConfirmPlan(tty, &b, "0"))
	require.Equal(t, "Error: No terminal to ask for confirmation: pass -y to go ahead.\n", b.String())
	b.Reset()

	// -y goes ahead without a terminal, in headless mode too.
	require.True(t, h.ConfirmPlan(term.Fixed(nil, false, false, false), &b, "1"))
	require.Empty(t, b.String())

	// The end of input before a newline refuses, even after a y.
	require.False(t, s.Confirm(tty.WithTTY("y"), &b, "Go?"))
	require.Equal(t, "Go? [y/N] ", b.String())
	b.Reset()
	require.False(t, s.ConfirmPlan(tty.WithTTY(""), &b, "0"))
	require.Equal(t, "Do you want to continue? [Y/n] \n", b.String())
	b.Reset()

	// No terminal: the plan refuses before asking.
	require.False(t, s.ConfirmPlan(tty.WithoutTTY(), &b, "0"))
	require.Equal(t, "Error: No terminal to ask for confirmation: pass -y to go ahead.\n", b.String())
}

// LEDGER CORE-01: without a terminal needs_confirm_prompt refuses after
// the question, and stderr holds only the question (2.x added the shell's
// "bash: /dev/tty: No such device or address").
func TestLedger_CORE01(t *testing.T) {
	var b bytes.Buffer
	ok := modeStyle("off", true, 80).Confirm(term.Fixed(nil, false, false, false), &b, "Continue?")
	require.False(t, ok)
	require.Equal(t, "Continue? [y/N] ", b.String())

	// 2.x, in a session without a controlling terminal.
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid not available")
	}
	root, err := filepath.Abs(repoRoot)
	require.NoError(t, err)
	cmd := exec.Command("setsid", "-w", "bash", "--noprofile", "--norc", "-c",
		`source "$1/lib/core/output.sh"; source "$1/lib/core/utils.sh"; needs_confirm_prompt Continue? 2>&1; echo " rc=$?"`, "_", root)
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "TERM=dumb"}
	cmd.Stdin = strings.NewReader("")
	got, err := cmd.Output()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(got), "Continue? [y/N] "), string(got))
	require.True(t, strings.HasSuffix(string(got), " rc=1\n"), string(got))
}
