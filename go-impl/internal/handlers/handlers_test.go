// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

func run(t *testing.T, name string, env map[string]string, tty bool, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	e := invoke.EnvMap(env)
	c := &invoke.Call{Name: name, Args: args, Env: e, Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, tty, false)}
	rc := Table().Run(c)
	return out.String(), errb.String(), rc
}

// Building the table checks every registration against the registry
// (Handle panics on an unknown or repeated name).
func TestTableBuilds(t *testing.T) {
	tbl := Table()
	require.Equal(t, []string{"all_colors", "gash", "gash_username", "llm_tree"}, tbl.Names())
	require.Equal(t, []string{"env load", "help", "json arr", "json str", "output box", "output choose",
		"output download", "output table", "output wrap"}, tbl.CompatOps())
}

func TestAllColors(t *testing.T) {
	out, errOut, rc := run(t, "all_colors", nil, false)
	require.Equal(t, 0, rc)
	require.Empty(t, errOut)
	require.True(t, strings.HasPrefix(out, "colour0\ncolour1\n"))

	out, _, _ = run(t, "all_colors", nil, true)
	require.True(t, strings.HasPrefix(out, "\x1b[38;5;0mcolour0\x1b[0m\n"))

	// The environment gate: NO_COLOR on a terminal.
	out, _, _ = run(t, "all_colors", map[string]string{"NO_COLOR": "1"}, true)
	require.True(t, strings.HasPrefix(out, "colour0\n"))

	out, _, rc = run(t, "all_colors", map[string]string{"GASH__HELP": "0"}, false, "-h")
	require.Equal(t, 0, rc)
	require.Equal(t, "all_colors\nUsage: all_colors\nDisplays all available terminal colors (256 colors).\n", out)
}

func TestCompatHelp(t *testing.T) {
	tbl := Table()
	h, _, rest, ok := tbl.LookupCompat([]string{"help", "all_colors"})
	require.True(t, ok)
	var out bytes.Buffer
	e := invoke.EnvMap(nil)
	rc := h(&invoke.Call{Args: rest, Env: e, Stdout: &out, Stderr: &out, Term: term.Fixed(e, false, false, false)})
	require.Equal(t, 0, rc)
	require.Contains(t, out.String(), "  all_colors  (module: utils)\n")

	out.Reset()
	rc = h(&invoke.Call{Args: []string{"mycommand"}, Env: e, Stdout: &out, Stderr: &out, Term: term.Fixed(e, false, false, false)})
	require.Equal(t, 1, rc)
	require.Empty(t, out.String())
}

func TestCardUnknownTopic(t *testing.T) {
	_, errOut, rc := run(t, "gash", map[string]string{"LANG": "C.UTF-8"}, false, "nope")
	require.Equal(t, 1, rc)
	require.Equal(t, "Error: Unknown topic: nope. Topics: files system nav git docker remote dev llm ai gash, all\n", errOut)

	_, errOut, rc = run(t, "gash", map[string]string{"LANG": "C.UTF-8"}, true, "--no-color", "x")
	require.Equal(t, 1, rc)
	require.Equal(t, "✗ Error: Unknown topic: x. Topics: files system nav git docker remote dev llm ai gash, all\n", errOut)

	_, errOut, rc = run(t, "gash", nil, true, "nope")
	require.Equal(t, 1, rc)
	require.Equal(t, "\x1b[38;5;196mx Error:\x1b[0m Unknown topic: nope. Topics: files system nav git docker remote dev llm ai gash, all\n", errOut)

	// Synonyms are topics; the card itself is not ported yet.
	_, errOut, rc = run(t, "gash", nil, false, "navigation")
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, `"details":"gash is not implemented in this gash build"`)

	out, _, rc := run(t, "gash", nil, false, "git", "-h")
	require.Equal(t, 0, rc)
	require.Contains(t, out, "\n  gash  (module: gash)\n")

	_, errOut, rc = run(t, "gash", nil, false, "migrate", "--x")
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, `"details":"gash_migrate is not implemented in this gash build"`)
}

func TestUsername(t *testing.T) {
	out, _, rc := run(t, "gash_username", map[string]string{"PATH": "/usr/bin:/bin"}, false)
	require.Equal(t, 0, rc)
	require.NotEmpty(t, strings.TrimSpace(out))
}
