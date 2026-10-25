// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package cli

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/buildinfo"
	"github.com/mauriziofonte/gash/internal/handlers"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

func run(args ...string) (string, string, int) {
	return runWith(handlers.Table(func(t *invoke.Table) {
		t.HandleCompat("echo args", func(c *invoke.Call) int {
			_, _ = c.Stdout.Write([]byte(strings.Join(c.Args, "|") + "\n"))
			return 4
		})
	}), args...)
}

func runWith(tbl *invoke.Table, args ...string) (string, string, int) {
	var out, errb bytes.Buffer
	env := invoke.EnvMap(map[string]string{"LANG": "C.UTF-8"})
	rc := Run(Process{
		Args: args, Env: env, Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb,
		Term: term.Fixed(env, false, false, false), Table: tbl,
		Executable: func() (string, error) { return "/opt/gash", nil },
	})
	return out.String(), errb.String(), rc
}

func TestVersion(t *testing.T) {
	out, errOut, rc := run("version")
	require.Equal(t, 0, rc, errOut)
	require.Equal(t, "gash "+buildinfo.Version+" "+runtime.GOOS+"/"+runtime.GOARCH+" /opt/gash\n", out)

	out, _, rc = run("version", "--json")
	require.Equal(t, 0, rc)
	require.Equal(t, `{"version":"`+buildinfo.Version+`","os":"`+runtime.GOOS+`","arch":"`+runtime.GOARCH+`","path":"/opt/gash"}`+"\n", out)

	_, errOut, rc = run("version", "--jsn")
	require.Equal(t, 1, rc)
	require.Equal(t, `{"error":"unknown_option","details":"unknown flag: --jsn","action":"RETRY","recoverable":true,"hint":"Run: gash version --help"}`+"\n", errOut)

	_, errOut, rc = run("version", "extra")
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, `{"error":"unexpected_argument","details":"unknown command \"extra\" for \"gash version\"",`)
}

// A 2.x function gets its arguments raw: flags, --, = forms and all.
func TestFunctionArgsRaw(t *testing.T) {
	out, errOut, rc := run("all_colors", "--no-such", "--", "a=b")
	require.Equal(t, 0, rc, errOut)
	require.True(t, strings.HasPrefix(out, "colour0\n"))

	// A registered function with no handler (an empty table: no module
	// registers anything) answers the not-implemented envelope.
	_, errOut, rc = runWith(invoke.NewTable(), "llm_tree", "--depth", "2")
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, `"details":"llm_tree is not implemented in this gash build"`)
}

// Words that are not commands go to the card, all of them, as the 2.x gash
// function received them.
func TestUnknownWordIsTheCard(t *testing.T) {
	for _, args := range [][]string{{"nope"}, {"--no-color", "nope"}, {"--no-color", "version", "nope"}, {"help"}, {"needs_help"}, {"flf"}} {
		_, errOut, rc := run(args...)
		require.Equal(t, 1, rc, args)
		require.Equal(t, "Error: Unknown topic: "+args[len(args)-1]+". Topics: files system nav git docker remote dev llm agent ai gash, all\n", errOut, args)
	}
	// --no-color before a command word does not select the command.
	_, errOut, _ := run("--no-color", "version")
	require.Contains(t, errOut, "Unknown topic: version.")

	out, _, rc := run("-h")
	require.Equal(t, 0, rc)
	require.Contains(t, out, "  gash  (module: gash)")
}

func TestCompatOps(t *testing.T) {
	out, _, rc := run("__compat", "echo", "args", "--x", "y")
	require.Equal(t, 4, rc)
	require.Equal(t, "--x|y\n", out)

	_, errOut, rc := run("__compat", "output", "nope", "--indent", "2")
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, `"details":"__compat output nope is not implemented in this gash build"`)

	out, _, rc = run("__compat", "output", "wrap", "7")
	require.Equal(t, 0, rc)
	require.Equal(t, "\n", out)

	out, _, rc = run("__compat", "help", "gash_username")
	require.Equal(t, 0, rc)
	require.Contains(t, out, "  gash_username  (module: gash)\n")
}

// gash update runs the selfmgmt flow (upgrade.md 3.6): it needs a git
// checkout, takes --rollback and nothing else, and has its own help; a
// build without the handler answers not-implemented.
func TestUpdateCommand(t *testing.T) {
	_, errOut, rc := run("update")
	require.Equal(t, 1, rc)
	require.Equal(t, "Error: Gash is not installed in /.gash as a git checkout. See https://github.com/mauriziofonte/gash.\n", errOut)
	_, errOut, rc = run("update", "--rollback")
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, "as a git checkout")

	_, errOut, rc = run("update", "--bogus")
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, `{"error":"unknown_option","details":"unknown flag: --bogus"`)
	_, errOut, rc = run("update", "now")
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, `"error":"unexpected_argument"`)

	out, _, rc := run("update", "--help")
	require.Equal(t, 0, rc)
	require.Contains(t, out, "gash update [--rollback]")
	require.Contains(t, out, "--rollback")
	require.Contains(t, out, "release/SHA256SUMS")

	_, errOut, rc = runWith(invoke.NewTable(), "update")
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, `{"error":"internal_error","details":"gash update is not implemented in this gash build"`)

	_, errOut, rc = run("init", "zsh")
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, `"error":"unexpected_argument"`)
}

// TestInitBash: gash init bash emits the interactive layer (prompt hook and
// terminal setup are always present) with status 0.
func TestInitBash(t *testing.T) {
	out, errOut, rc := run("init", "bash")
	require.Equal(t, 0, rc, errOut)
	require.Contains(t, out, "shopt -s checkwinsize")
	require.Contains(t, out, "__gash_prompt_command")
}

func TestComplete(t *testing.T) {
	out, _, rc := run("__complete", "vers")
	require.Equal(t, 0, rc)
	require.Contains(t, out, "version\t")
	out, _, _ = run("__complete", "version", "--")
	require.Contains(t, out, "--json\t")
	out, _, _ = run("__complete", "init", "")
	require.Contains(t, out, "bash")
}

func TestIsCommand(t *testing.T) {
	for _, w := range []string{"version", "init", "update", "hte", "__compat", "__complete", "llm_tree", "gash_username", "gl", "gash_unload", "node_upgrade"} {
		require.True(t, IsCommand(w), w)
	}
	for _, w := range []string{"gash", "help", "needs_help", "path_add", "nvm", "git", "flf", "--no-color", "migrate"} {
		require.False(t, IsCommand(w), w)
	}
}
