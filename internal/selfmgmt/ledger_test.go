// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/compatgen"
	"github.com/mauriziofonte/gash/internal/prereq"
)

// LEDGER SELF-15: every self-management command refuses an unknown option
// or an unexpected argument with rc 2 before any side effect: no file
// written, no git call (the fake runner expects no start). 3.0: SELF-15
// fixed; 2.x ignored the words (gash_env_init --bogus created the file).
func TestLedger_SELF15(t *testing.T) {
	d := t.TempDir()
	home := filepath.Join(d, "home")
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".gash/lib/core"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gash/gash.sh"), nil, 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(home, ".gash/.git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gash/.gash_env.template"), []byte("T\n"), 0o644))
	env := map[string]string{"HOME": home, "GASH_DIR": home + "/.gash"}
	for _, tc := range []struct{ name, usage string }{
		{"gash_env_init", "gash_env_init [--force]"},
		{"gash_username", "gash_username"},
		{"gash_upgrade", "gash_upgrade"},
		{"gash_uninstall", "gash_uninstall"},
		{"gash_doctor", doctorUsage},
	} {
		for arg, what := range map[string]string{"--bogus": "Unknown option", "word": "Unexpected argument"} {
			args := []string{arg}
			if tc.name == "gash_env_init" {
				args = []string{"--force", arg}
			}
			out, errOut, rc := call(t, tc.name, env, args...)
			require.Equal(t, 2, rc, "%s %s", tc.name, arg)
			require.Empty(t, out, tc.name)
			require.Equal(t, "Error: "+what+": "+arg+" (usage: "+tc.usage+")\n", errOut, tc.name)
		}
	}
	require.NoFileExists(t, filepath.Join(home, ".gash_env"))
	require.DirExists(t, filepath.Join(home, ".gash"))
	// The uninstall steps the stub runs refuse too, so the stub stops.
	for _, step := range []string{"check", "remove", "env-remove"} {
		_, _, rc := call(t, "gash_uninstall", map[string]string{"HOME": home, "GASH_DIR": home + "/.gash", UninstallStepVar: step}, "x")
		require.Equal(t, 2, rc, step)
	}
	require.DirExists(t, filepath.Join(home, ".gash"))
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

// LEDGER SELF-04: uninstall edits the file a linked profile points to and
// leaves the link in place, the target's mode kept; the backup next to the
// profile holds the text before the edit (3.0: SELF-04 fixed).
func TestLedger_SELF04(t *testing.T) {
	d := t.TempDir()
	home := filepath.Join(d, "home")
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".gash/lib/core"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gash/gash.sh"), nil, 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(d, "dotfiles"), 0o755))
	block := "a\n# >>> GASH START >>>\nsource ~/.gashrc\n# <<< GASH END <<<\nb\n"
	require.NoError(t, os.WriteFile(filepath.Join(d, "dotfiles/bashrc"), []byte(block), 0o600))
	require.NoError(t, os.Symlink("../dotfiles/bashrc", filepath.Join(home, ".bashrc")))
	_, errOut, rc := callModule(t, New(), "gash_uninstall",
		map[string]string{"HOME": home, "PATH": "/usr/bin:/bin", "LANG": "C.UTF-8", UninstallStepVar: "remove"})
	require.Equal(t, 0, rc, errOut)
	link, err := os.Readlink(filepath.Join(home, ".bashrc"))
	require.NoError(t, err)
	require.Equal(t, "../dotfiles/bashrc", link)
	b, err := os.ReadFile(filepath.Join(d, "dotfiles/bashrc"))
	require.NoError(t, err)
	require.Equal(t, "a\nb\n", string(b))
	st, err := os.Stat(filepath.Join(d, "dotfiles/bashrc"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), st.Mode().Perm())
	b, err = os.ReadFile(filepath.Join(home, ".bashrc.gash-uninstall.bak"))
	require.NoError(t, err)
	require.Equal(t, block, string(b))
	entries, err := os.ReadDir(filepath.Join(d, "dotfiles"))
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temporary file left")
}

// doctorTree writes a 3.x install of version 3.0.0 into a new directory: the
// generated files as this binary generates them, the other shipped files,
// and release/SHA256SUMS pinning the fake binary at exe for this platform.
func doctorTree(t *testing.T, exe string) string {
	t.Helper()
	d := t.TempDir()
	files, err := compatgen.Generate(false)
	require.NoError(t, err)
	for _, f := range files {
		p := filepath.Join(d, f.Path)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, f.Data, 0o644))
	}
	for _, f := range []string{"VERSION", "quotes/list.txt", ".gash_env.template"} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(d, f)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(d, f), []byte("3.0.0\n"), 0o644))
	}
	require.NoError(t, os.WriteFile(exe, []byte("the binary"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(d, "release"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(d, "release/SHA256SUMS"),
		[]byte(fileSHA256(exe)+"  "+runtime.GOOS+"_"+runtime.GOARCH+"/gash\n"+strings.Repeat("0", 64)+"  other_os/gash\n"), 0o644))
	return d
}

// doctorModule is the module as a 3.0.0 release binary at exe.
func doctorModule(exe string) *Module {
	m := New()
	m.version = "3.0.0"
	m.executable = func() (string, error) { return exe, nil }
	return m
}

func doctorEnv(t *testing.T, dir string) map[string]string {
	return map[string]string{"HOME": t.TempDir(), "GASH_DIR": dir, "PATH": t.TempDir(), "BASH_VERSION": "5.2.21(1)-release",
		"GASH_VERSION": "3.0.0"}
}

// LEDGER SELF-17: on a 3.x install the doctor checks the shipped files, the
// binary against the hash release/SHA256SUMS pins, and every generated
// file against what this binary generates: a healthy tree passes; a
// removed shim is its MISSING line, an edited one and a binary that does
// not match are their own lines, each an issue.
func TestLedger_SELF17(t *testing.T) {
	defer prereq.SetSystem(rockySystem(allBut()))()
	exe := filepath.Join(t.TempDir(), "gash")
	d := doctorTree(t, exe)
	out, errOut, rc := callModule(t, doctorModule(exe), "gash_doctor", doctorEnv(t, d))
	require.Equal(t, 0, rc, errOut)
	plat := runtime.GOOS + "_" + runtime.GOARCH
	require.Contains(t, out, "Binary:")
	require.Contains(t, out, "Shims:")
	require.Contains(t, errOut, "OK:   "+plat+" — matches release/SHA256SUMS\n")
	require.Contains(t, errOut, "OK:   lib/core/utils.sh\n")
	require.True(t, strings.HasSuffix(errOut, "\nOK: All checks passed.\n"), errOut)

	require.NoError(t, os.Remove(filepath.Join(d, "lib/core/utils.sh")))
	require.NoError(t, os.WriteFile(filepath.Join(d, "bash_completion"), []byte("# edited\n"), 0o644))
	require.NoError(t, os.WriteFile(exe, []byte("another binary"), 0o755))
	_, errOut, rc = callModule(t, doctorModule(exe), "gash_doctor", doctorEnv(t, d))
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, "Error:   lib/core/utils.sh — MISSING\n")
	require.Contains(t, errOut, "Error:   bash_completion — differs from gash 3.0.0 (regenerate: gash __compat generate --out "+d+")\n")
	require.Contains(t, errOut, "Error:   "+plat+" — does not match release/SHA256SUMS\n")
	require.True(t, strings.HasSuffix(errOut, "\nWarning: 3 issue(s) found.\n"), errOut)

	// The hosts helper next to the binary is checked against its own pin
	// when the tree has one (upgrade.md 2: the helper is pinned too).
	exe = filepath.Join(t.TempDir(), "gash")
	d = doctorTree(t, exe)
	helper := filepath.Join(filepath.Dir(exe), "gash-hosts-helper")
	require.NoError(t, os.WriteFile(helper, []byte("the helper"), 0o755))
	sums, err := os.ReadFile(filepath.Join(d, "release/SHA256SUMS"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(d, "release/SHA256SUMS"),
		append(sums, []byte(fileSHA256(helper)+"  "+plat+"/gash-hosts-helper\n")...), 0o644))
	_, errOut, rc = callModule(t, doctorModule(exe), "gash_doctor", doctorEnv(t, d))
	require.Equal(t, 0, rc, errOut)
	require.Contains(t, errOut, "OK:   gash-hosts-helper — matches release/SHA256SUMS\n")
	require.NoError(t, os.WriteFile(helper, []byte("another helper"), 0o755))
	_, errOut, rc = callModule(t, doctorModule(exe), "gash_doctor", doctorEnv(t, d))
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, "Error:   gash-hosts-helper — does not match release/SHA256SUMS\n")
	require.NoError(t, os.Remove(helper))
	_, errOut, rc = callModule(t, doctorModule(exe), "gash_doctor", doctorEnv(t, d))
	require.Equal(t, 0, rc, errOut)
	require.Contains(t, errOut, "Info:   gash-hosts-helper — not installed next to the binary\n")

	// A development build checks neither.
	_, errOut, _ = callModule(t, New(), "gash_doctor", doctorEnv(t, d))
	require.Contains(t, errOut, "Info:   not checked (development build)\n")
}

// LEDGER SELF-06: the doctor returns 1 when a check fails and refuses an
// unknown option with status 2 before running any check (3.0: SELF-06
// fixed).
func TestLedger_SELF06(t *testing.T) {
	defer prereq.SetSystem(rockySystem(allBut()))()
	exe := filepath.Join(t.TempDir(), "gash")
	d := doctorTree(t, exe)
	require.NoError(t, os.Remove(filepath.Join(d, "lib/modules/php.sh")))
	_, errOut, rc := callModule(t, doctorModule(exe), "gash_doctor", doctorEnv(t, d))
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, "Warning:   php — not found\n")
	require.True(t, strings.HasSuffix(errOut, "\nWarning: 1 issue(s) found.\n"), errOut)

	out, errOut, rc := callModule(t, doctorModule(exe), "gash_doctor", doctorEnv(t, d), "--bogus")
	require.Equal(t, 2, rc)
	require.Empty(t, out)
	require.Equal(t, "Error: Unknown option: --bogus (usage: "+doctorUsage+")\n", errOut)
}

// LEDGER SELF-10: tagSigned takes a tag object as signed only when, after
// the header block, a BEGIN marker line is followed by the matching END
// line at the end (3.0: SELF-10 fixed; 2.x matched the marker as text).
func TestLedger_SELF10(t *testing.T) {
	head := "object 3c98a1da386c6a10a42fde470165ebf153867e47\ntype commit\ntag v1.1\ntagger T <t@e> 1700000000 +0000\n\n"
	pgp := "-----BEGIN PGP SIGNATURE-----\n\niQ\n-----END PGP SIGNATURE-----\n"
	ssh := "-----BEGIN SSH SIGNATURE-----\nU1NI\n-----END SSH SIGNATURE-----\n"
	for obj, want := range map[string]bool{
		head + "rel\n" + pgp: true,
		head + "rel\n" + ssh: true,
		head + pgp:           true,
		head + "quoting -----BEGIN PGP SIGNATURE----- in text\n":                      false,
		head + "see\n-----BEGIN PGP SIGNATURE-----\nno end\n":                         false,
		head + "rel\n-----BEGIN PGP SIGNATURE-----\nx\n-----END SSH SIGNATURE-----\n": false,
		"-----BEGIN PGP SIGNATURE-----\n-----END PGP SIGNATURE-----\n":                false, // no header block
		"": false,
	} {
		require.Equal(t, want, tagSigned([]byte(obj)), "%q", obj)
	}
}

// LEDGER SELF-13: gash_inspiring_quote tells a positional word from an
// unknown option (3.0: SELF-13 fixed; 2.x called both an unknown option).
func TestLedger_SELF13(t *testing.T) {
	for a, want := range map[string]string{
		"foo":     "Error: Unexpected argument: foo (usage: gash_inspiring_quote [--source])\n",
		"--bogus": "Error: Unknown option: --bogus (usage: gash_inspiring_quote [--source])\n",
	} {
		out, errOut, rc := call(t, "gash_inspiring_quote", nil, a)
		require.Empty(t, out)
		require.Equal(t, want, errOut)
		require.Equal(t, 1, rc)
	}
}
