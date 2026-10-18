// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ui

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/term"
)

const repoRoot = "../../.."

// run2x sources the 2.x lib/core/output.sh and utils.sh of the checkout in
// a non-interactive Bash with env and runs script; stdout is returned.
// The test is skipped when the 2.x sources or bash are missing.
func run2x(t *testing.T, env []string, script string) string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repoRoot, "lib/core/output.sh")); err != nil {
		t.Skip("the 2.x sources are not in this tree")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	root, err := filepath.Abs(repoRoot)
	require.NoError(t, err)
	cmd := exec.Command("bash", "--noprofile", "--norc", "-c",
		`source "$1/lib/core/output.sh"; source "$1/lib/core/utils.sh"; `+script, "_", root)
	cmd.Env = append([]string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "TERM=dumb"}, env...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	require.NoError(t, cmd.Run(), errb.String())
	return out.String()
}

// shq quotes s for Bash.
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// in2x wraps body for a color mode: off (no terminal), on (a color
// terminal) or shadow (a color terminal and a --no-color call, whose
// palette is shadowed while __gash_use_color stays true). The body runs
// inside a function, as the 2.x primitives ran inside their callers.
func in2x(mode, body string) string {
	pre := "__gash_use_color() { return 1; }; "
	locals := ""
	switch mode {
	case "on":
		pre = "__gash_use_color() { return 0; }; "
	case "shadow":
		pre = "__gash_use_color() { return 0; }; "
		locals = `eval "$__GASH_COLOR_OFF_LOCALS"; `
	}
	return pre + "__t() { " + locals + body + "\n}; __t"
}

// modeStyle is the Go style of a color mode.
func modeStyle(mode string, utf8 bool, width int) Style {
	s := Style{Width: width, Columns: width, G: term.ASCIIGlyphs}
	if utf8 {
		s.G, s.UTF8 = term.UnicodeGlyphs, true
	}
	switch mode {
	case "on":
		s.Color, s.P = true, term.Colors
	case "shadow":
		s.Color = true
	}
	return s
}

var colorModes = []string{"off", "on", "shadow"}
