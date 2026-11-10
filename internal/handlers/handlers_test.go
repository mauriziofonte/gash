// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/registry"
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
	// Every function the generated Bash delegates to the binary has a
	// handler, and nothing else does (the card, gash, is the one stub that
	// is not delegated; gash_unload's override runs in Bash). Derived from
	// the registry, so a new function only needs its registry entry and
	// its Handle line.
	var want []string
	for _, f := range registry.Functions() {
		if (f.Delegated() && f.Name != "gash_unload") || f.Name == "gash" {
			want = append(want, f.Name)
		}
	}
	require.Equal(t, want, tbl.Names())
	require.Contains(t, tbl.Names(), "llm_find")
	require.Contains(t, tbl.Names(), "agent_replace")
	require.Equal(t, []string{"env load", "env perms", "help", "json arr", "json str", "output box", "output choose",
		"output download", "output table", "output wrap", "prereq fail"}, tbl.CompatOps())
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
	require.Equal(t, "gash shell colors\nUsage: gash shell colors\nDisplays all available terminal colors (256 colors).\n", out)
}

func TestCompatHelp(t *testing.T) {
	tbl := Table()
	h, _, rest, ok := tbl.LookupCompat([]string{"help", "all_colors"})
	require.True(t, ok)
	var out bytes.Buffer
	e := invoke.EnvMap(nil)
	rc := h(&invoke.Call{Args: rest, Env: e, Stdout: &out, Stderr: &out, Term: term.Fixed(e, false, false, false)})
	require.Equal(t, 0, rc)
	require.Contains(t, out.String(), "  gash shell colors\n")

	out.Reset()
	rc = h(&invoke.Call{Args: []string{"mycommand"}, Env: e, Stdout: &out, Stderr: &out, Term: term.Fixed(e, false, false, false)})
	require.Equal(t, 1, rc)
	require.Empty(t, out.String())
}

func TestCardUnknownCommand(t *testing.T) {
	// A word that is no namespace is an unknown command: the envelope when
	// stderr is not a terminal, with the way to the namespaces.
	_, errOut, rc := run(t, "gash", map[string]string{"LANG": "C.UTF-8"}, false, "nope")
	require.Equal(t, 1, rc)
	require.Equal(t, `{"error":"unknown_command","details":"Unknown command: gash nope","action":"RETRY","recoverable":true,"hint":"Run: gash (namespaces) or gash help --search WORD"}`+"\n", errOut)

	_, errOut, rc = run(t, "gash", map[string]string{"LANG": "C.UTF-8"}, true, "--no-color", "gti")
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, `"details":"Unknown command: gash gti","action":"RETRY","recoverable":true,"hint":"Did you mean gash git?"`)

	// The former card topics open their namespace page.
	out, errOut, rc := run(t, "gash", nil, false, "navigation")
	require.Equal(t, 0, rc)
	require.Empty(t, errOut)
	require.Contains(t, out, "\n  gash shell ")

	out, _, rc = run(t, "gash", nil, false, "git", "-h")
	require.Equal(t, 0, rc)
	require.Contains(t, out, "\n  gash git ")

	out, _, rc = run(t, "gash", nil, false, "-h")
	require.Equal(t, 0, rc)
	require.Contains(t, out, "FOR AI AGENTS")

	_, errOut, rc = run(t, "gash", nil, false, "migrate", "--x")
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, "Error: gash migrate: Gash 3 does not migrate a Gash 1.x configuration;")
}

func TestUsername(t *testing.T) {
	out, _, rc := run(t, "gash_username", map[string]string{"PATH": "/usr/bin:/bin"}, false)
	require.Equal(t, 0, rc)
	require.NotEmpty(t, strings.TrimSpace(out))
}
