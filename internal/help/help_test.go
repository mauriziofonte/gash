// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package help

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/term"
)

const repoRoot = "../.."

// run2x loads the 2.x gash.sh of the checkout in an interactive shell
// (the palette set) with a temporary HOME and runs script.
func run2x(t *testing.T, script string) []byte {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repoRoot, "lib/core/help.sh")); err != nil {
		t.Skip("the 2.x sources are not in this tree")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	root, err := filepath.Abs(repoRoot)
	require.NoError(t, err)
	cmd := exec.Command("bash", "--noprofile", "--norc", "-i", "-c",
		`source "$1/gash.sh" >/dev/null 2>&1; `+script, "_", root)
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "LANG=C.UTF-8", "TERM=dumb"}
	var out bytes.Buffer
	cmd.Stdout = &out
	require.NoError(t, cmd.Run())
	return out.Bytes()
}

// Display writes what __gash_help_display wrote, for every registered
// function, with and without colors.
func TestDisplayMatches2x(t *testing.T) {
	for _, color := range []bool{false, true} {
		gate := "__gash_use_color() { return 1; }"
		if color {
			gate = "__gash_use_color() { return 0; }"
		}
		out := run2x(t, gate+`
			for f in "${!__GASH_HELP_REGISTRY[@]}"; do
				printf '%s\0' "$f"; __gash_help_display "$f"; printf '\0'
			done`)
		fields := strings.Split(string(out), "\x00")
		fields = fields[:len(fields)-1]
		require.NotEmpty(t, fields)
		for i := 0; i < len(fields); i += 2 {
			f, ok := registry.Lookup(fields[i])
			require.True(t, ok, fields[i])
			var b bytes.Buffer
			require.True(t, Display(&b, color, f))
			require.Equal(t, fields[i+1], b.String(), "%s color=%v", f.Name, color)
		}
	}
}

func call(env map[string]string, stdoutTTY bool, name string) (*invoke.Call, *bytes.Buffer) {
	var out bytes.Buffer
	e := invoke.EnvMap(env)
	return &invoke.Call{Name: name, Env: e, Stdout: &out, Stderr: &out, Term: term.Fixed(e, false, stdoutTTY, false)}, &out
}

func TestNeedsFallbackAndArgs(t *testing.T) {
	c, out := call(nil, false, "x")
	require.False(t, Needs(c, "prog", "prog ARG", "Does things.", ""))
	require.False(t, Needs(c, "prog", "prog ARG", "Does things.", "file"))
	require.Empty(t, out.String())

	require.True(t, Needs(c, "prog", "prog ARG", "Does things.", "--help"))
	require.Equal(t, "prog\nUsage: prog ARG\nDoes things.\n", out.String())

	c, out = call(nil, true, "x")
	require.True(t, Needs(c, "prog", "prog ARG", "Does things.", "-h"))
	require.Equal(t, term.SeqAccent+"prog"+term.SeqOff+"\n"+term.SeqBold+"Usage:"+term.SeqOff+" prog ARG\nDoes things.\n", out.String())
}

// all_colors -h: the registered page when the caller's shell had the
// registry, the fallback (with "256 colors") when it had not.
func TestNeedsOfRegistryState(t *testing.T) {
	c, out := call(nil, false, "all_colors")
	require.True(t, NeedsOf(c, "-h"))
	require.Contains(t, out.String(), "  Display all 256 terminal colors\n")

	c, out = call(map[string]string{"GASH__HELP": "0"}, false, "all_colors")
	require.True(t, NeedsOf(c, "-h"))
	require.Equal(t, "all_colors\nUsage: all_colors\nDisplays all available terminal colors (256 colors).\n", out.String())
}

// LEDGER SELF-11: the card and help lists sort names by bytes, the order
// of sort(1) under LC_ALL=C, in every locale: a_b a_c ab, where
// collations that ignore "_" give a_b ab a_c.
func TestLedger_SELF11(t *testing.T) {
	fns := []*registry.Function{{Name: "ab"}, {Name: "a_c"}, {Name: "a_b"}}
	slices.SortFunc(fns, byName)
	require.Equal(t, "a_b", fns[0].Name)
	require.Equal(t, "a_c", fns[1].Name)
	require.Equal(t, "ab", fns[2].Name)

	var names []string
	for _, f := range Registered() {
		names = append(names, f.Name)
	}
	if _, err := exec.LookPath("sort"); err != nil {
		t.Skip("sort not available")
	}
	cmd := exec.Command("sort")
	cmd.Env = []string{"LC_ALL=C"}
	cmd.Stdin = strings.NewReader(strings.Join(names, "\n") + "\n")
	out, err := cmd.Output()
	require.NoError(t, err)
	require.Equal(t, string(out), strings.Join(names, "\n")+"\n")
}
