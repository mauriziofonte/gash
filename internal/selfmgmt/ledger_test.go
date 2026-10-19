// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/runner"
)

// LEDGER SELF-15, reproduced in rc.A: the self-management commands accept
// and ignore words they do not know.
func TestLedger_SELF15_2x(t *testing.T) {
	t.Run("gash_env_init", func(t *testing.T) {
		d := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(d, "home"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(d, ".gash_env.template"), []byte("T\n"), 0o644))
		m := New()
		m.run = runner.NewFake(t,
			runner.Expect{Argv: []string{"cp", d + "/.gash_env.template", d + "/home/.gash_env"}},
			runner.Expect{Argv: []string{"chmod", "600", d + "/home/.gash_env"}})
		out, errOut, rc := callModule(t, m, "gash_env_init", map[string]string{"HOME": d + "/home", "GASH_DIR": d}, "--bogus")
		require.Equal(t, 0, rc)
		require.Empty(t, out)
		require.Equal(t, "OK: Created "+d+"/home/.gash_env\nInfo: Edit with your SSH keys and database credentials\n", errOut)
	})
	t.Run("gash_username", func(t *testing.T) {
		p := &fakeProcs{have: map[string]bool{"whoami": true}, out: map[string]string{"whoami": "alice"}, ran: []string{"whoami"}}
		out, rc := run(t, p.module(t, "", nil), nil, "--bogus")
		require.Equal(t, 0, rc)
		require.Equal(t, "alice\n", out)
	})
}
