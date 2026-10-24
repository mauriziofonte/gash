// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"os"
	"path/filepath"
	"strings"
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

// LEDGER SELF-02: uninstall keeps a ~/.gashrc that holds the user's own
// lines as ~/.gashrc.gash-uninstall.bak (same bytes), removes one an
// installer wrote, and keeps and lists the profile backups sed made
// (3.0: SELF-02 fixed; 2.x removed all of them).
func TestLedger_SELF02(t *testing.T) {
	tmpl2x, err := knownGashrc.ReadFile("gashrc/2x.gashrc")
	require.NoError(t, err)
	tmpl1x, err := knownGashrc.ReadFile("gashrc/1x-1538e53.gashrc")
	require.NoError(t, err)
	custom := strings.Replace(string(tmpl2x), `__gash_home="${GASH_DIR:-$HOME/.gash}"`, `__gash_home="${GASH_DIR:-/opt/my\ gash}"`, 1)
	require.NotEqual(t, string(tmpl2x), custom)
	block := "# before\n# >>> GASH START >>>\nif [ -f ~/.gashrc ]; then\n  source ~/.gashrc\nfi\n# <<< GASH END <<<\n# after\n"

	uninstallIn := func(t *testing.T, gashrc string) (string, string) {
		t.Helper()
		d := t.TempDir()
		home := filepath.Join(d, "home")
		require.NoError(t, os.MkdirAll(filepath.Join(home, ".gash/lib/core"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(home, ".gash/gash.sh"), nil, 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte(block), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(home, ".gashrc"), []byte(gashrc), 0o644))
		_, errOut, rc := callModule(t, New(), "gash_uninstall",
			map[string]string{"HOME": home, "PATH": "/usr/bin:/bin", "LANG": "C.UTF-8", UninstallStepVar: "remove"})
		require.Equal(t, 0, rc, errOut)
		return home, errOut
	}

	t.Run("modified", func(t *testing.T) {
		mine := string(tmpl2x) + "alias mine='echo mine'\n"
		home, errOut := uninstallIn(t, mine)
		_, err := os.Lstat(filepath.Join(home, ".gashrc"))
		require.True(t, os.IsNotExist(err))
		b, err := os.ReadFile(filepath.Join(home, ".gashrc.gash-uninstall.bak"))
		require.NoError(t, err)
		require.Equal(t, mine, string(b))
		require.Contains(t, errOut, "Info: Saved your ~/.gashrc as ~/.gashrc.gash-uninstall.bak\n")
		require.Contains(t, errOut, "Info: Backups kept: ~/.bashrc.gash-uninstall.bak, ~/.gashrc.gash-uninstall.bak\n")
	})
	for name, pristine := range map[string]string{"2.x": string(tmpl2x), "custom dir": custom, "1.x": string(tmpl1x)} {
		t.Run("pristine "+name, func(t *testing.T) {
			home, errOut := uninstallIn(t, pristine)
			for _, gone := range []string{".gashrc", ".gashrc.gash-uninstall.bak", ".gash"} {
				_, err := os.Lstat(filepath.Join(home, gone))
				require.True(t, os.IsNotExist(err), gone)
			}
			require.Contains(t, errOut, "Info: Removing ~/.gashrc...\n")
			require.Contains(t, errOut, "Info: Backups kept: ~/.bashrc.gash-uninstall.bak\n")
		})
	}
	t.Run("profile backup", func(t *testing.T) {
		home, _ := uninstallIn(t, string(tmpl2x))
		b, err := os.ReadFile(filepath.Join(home, ".bashrc.gash-uninstall.bak"))
		require.NoError(t, err)
		require.Equal(t, block, string(b))
		b, err = os.ReadFile(filepath.Join(home, ".bashrc"))
		require.NoError(t, err)
		require.Equal(t, "# before\n# after\n", string(b))
	})
}

// The embedded 2.x template is the repository's .gashrc: a change to one
// must reach the other, or uninstall would keep a pristine file as the
// user's.
func TestKnownGashrcIsRepositoryTemplate(t *testing.T) {
	repo, err := os.ReadFile("../../.gashrc")
	require.NoError(t, err)
	tmpl, err := knownGashrc.ReadFile("gashrc/2x.gashrc")
	require.NoError(t, err)
	require.Equal(t, string(repo), string(tmpl))
	require.True(t, pristineGashrc(repo))
	require.False(t, pristineGashrc(append(repo, "x\n"...)))
	require.False(t, pristineGashrc(nil))
}

// LEDGER SELF-01: versions compare as semver, so gash_upgrade can tell a
// checkout newer than the latest release (a pre-release of the next one
// included) from an older one (3.0: SELF-01 fixed; 2.x compared tags for
// equality only). The end-to-end proof with a mirror is TestLedger_SELF01
// in internal/handlers.
func TestLedger_SELF01(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"v3.0.0-rc.1", "v2.1.0", 1},
		{"v2.1.0", "v3.0.0-rc.1", -1},
		{"v3.0.0-rc.1", "v3.0.0", -1},
		{"v3.0.0", "v3.0.0-rc.1", 1},
		{"v3.0.0-rc.2", "v3.0.0-rc.10", -1},
		{"v3.0.0-alpha", "v3.0.0-alpha.1", -1},
		{"v3.0.0-alpha.1", "v3.0.0-beta", -1},
		{"v3.0.0-1", "v3.0.0-alpha", -1},
		{"v2.0.0", "v2.1.0", -1},
		{"v1.10.0", "v1.9.1", 1},
		{"2.1", "v2.1.0", 0},
		{"v2.1.0", "v2.1.0", 0},
		{"v2.1.0+build.5", "v2.1.0", 0},
		{"v10.0.0", "v9.99.99", 1},
	} {
		got, ok := compareVersions(tc.a, tc.b)
		require.True(t, ok, tc.a+" vs "+tc.b)
		require.Equal(t, tc.want, got, tc.a+" vs "+tc.b)
	}
	for _, bad := range []string{"", "main", "v", "v1..2", "release-1"} {
		_, ok := compareVersions(bad, "v1.0.0")
		require.False(t, ok, bad)
	}
}
