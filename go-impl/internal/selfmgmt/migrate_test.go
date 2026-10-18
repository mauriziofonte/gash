// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

// call runs one function of a module whose runner expects no start.
func call(t *testing.T, name string, env map[string]string, args ...string) (string, string, int) {
	t.Helper()
	m := New()
	m.run = runner.NewFake(t)
	tbl := invoke.NewTable()
	m.Register(tbl)
	var out, errb bytes.Buffer
	e := invoke.EnvMap(env)
	c := &invoke.Call{Name: name, Args: args, Env: e, Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, false, false), Handlers: tbl}
	rc := tbl.Run(c)
	return out.String(), errb.String(), rc
}

// Decision D-6: every gash_migrate call but -h/--help, the 2.x options
// included, prints the pointer to the 2.x route on stderr, writes nothing
// and returns 1; gash migrate is the same call.
func TestSelfmgmt_MigratePointer(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gashrc"), []byte("# 1.x\n"), 0o644))
	env := map[string]string{"HOME": home, "XDG_STATE_HOME": filepath.Join(home, "state"), "TMPDIR": filepath.Join(home, "tmp")}
	want := "Error: gash_migrate: Gash 3 does not migrate a Gash 1.x configuration; check out v2.1.0 in " +
		home + "/.gash, open a new terminal, run gash_migrate there, then gash_upgrade\n"
	for _, args := range [][]string{{}, {"--yes"}, {"-y"}, {"--plain"}, {"--rollback"}, {"--rollback", "--yes"}, {"--status"}, {"--auto"}, {"--bogus"}, {"word"}} {
		out, errOut, rc := call(t, "gash_migrate", env, args...)
		require.Empty(t, out, "%q", args)
		require.Equal(t, want, errOut, "%q", args)
		require.Equal(t, 1, rc, "%q", args)
		out, errOut, rc = call(t, "gash", env, append([]string{"migrate"}, args...)...)
		require.Empty(t, out, "%q", args)
		require.Equal(t, want, errOut, "%q", args)
		require.Equal(t, 1, rc, "%q", args)
	}
	env["GASH_DIR"] = "/opt/gash"
	_, errOut, _ := call(t, "gash_migrate", env)
	require.Contains(t, errOut, " in /opt/gash, ")

	entries, err := os.ReadDir(home)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	b, err := os.ReadFile(filepath.Join(home, ".gashrc"))
	require.NoError(t, err)
	require.Equal(t, "# 1.x\n", string(b))

	for _, h := range []string{"-h", "--help"} {
		out, errOut, rc := call(t, "gash_migrate", env, h)
		require.Equal(t, 0, rc)
		require.Empty(t, errOut)
		require.Contains(t, out, "\n  gash_migrate  (module: gash)\n")
	}
}
