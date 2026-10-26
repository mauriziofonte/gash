// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
	"github.com/mauriziofonte/gash/internal/ui"
)

// gitReport is the prerequisite report of a missing git on rockySystem.
func gitReport(need string) string {
	return "Error: " + need + " needs git, which is not installed.\n" +
		"  git installs and updates Gash, and runs the git functions (package git on Rocky Linux 9.4).\n" +
		"  Install it:  sudo dnf install -y git\n" +
		"  Or run:      gash_doctor --install git\n" +
		"  On macOS git comes with the Command Line Tools: xcode-select --install.\n"
}

// fakeCheckout is an install directory that is a git checkout.
func fakeCheckout(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".git"), 0o755))
	return dir
}

// Without git, gash_upgrade and gash update stop on the prerequisite
// report before anything runs (3.4: cleanCheckout read the empty output of
// a git that did not start as a clean tree, and the fetch then blamed the
// network).
func TestUpgradeWithoutGit(t *testing.T) {
	defer prereq.SetSystem(rockySystem(allBut("git")))()
	dir := fakeCheckout(t)
	m := New()
	m.run = runner.NewFake(t)
	_, errOut, rc := callModule(t, m, "gash_upgrade", map[string]string{"GASH_DIR": dir})
	require.Equal(t, 1, rc)
	require.Equal(t, gitReport("gash_upgrade"), errOut)
	require.NotContains(t, errOut, "network")

	_, errOut, rc = callModule(t, m, "gash_upgrade", map[string]string{"GASH_DIR": dir, "GASH_HEADLESS": "1"})
	require.Equal(t, 1, rc)
	require.Equal(t, `{"error":"git_not_found","details":"gash_upgrade needs git, which is not installed","action":"CONTINUE","recoverable":true,`+
		`"hint":"Install git: sudo dnf install -y git. On macOS git comes with the Command Line Tools: xcode-select --install","missing":"git","install":"sudo dnf install -y git"}`+"\n", errOut)

	_, errOut, rc = runUpdate(t, map[string]string{"GASH_DIR": dir}, "--rollback")
	require.Equal(t, 1, rc)
	require.Equal(t, gitReport("gash update --rollback"), errOut)
}

// A git status that fails is no proof of a clean tree: the upgrade stops
// there (3.4: its empty output counted as clean).
func TestUpgradeStatusFailsClosed(t *testing.T) {
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	dir := fakeCheckout(t)
	m := New()
	m.run = runner.NewFake(t, runner.Expect{Argv: []string{"git", "-C", dir, "status", "--porcelain"},
		Stderr: "fatal: detected dubious ownership in repository at '" + dir + "'\n", Status: 128})
	_, errOut, rc := callModule(t, m, "gash_upgrade", map[string]string{"GASH_DIR": dir, "PATH": bin})
	require.Equal(t, 1, rc)
	require.Equal(t, "Error: Cannot tell whether the working tree at "+dir+" has local modifications "+
		"(git status failed: fatal: detected dubious ownership in repository at '"+dir+"'); not upgrading.\n", errOut)
}

// A signed tag whose verifier is not installed names that program, with
// the install command for the default one (3.4: the warning blamed the
// keyring whatever failed).
func TestVerifyTagMissingVerifier(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	d := t.TempDir()
	env := []string{"HOME=" + d, "PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t"}
	// Tag objects that carry a signature block, made without a key.
	bashRun(t, d, env, `git init -q repo && cd repo && git commit -q --allow-empty -m one
for kind in PGP SSH; do
  obj=$(printf 'object %s\ntype commit\ntag v-%s\ntagger T <t@t> 1700000000 +0000\n\nrel\n-----BEGIN %s SIGNATURE-----\nx\n-----END %s SIGNATURE-----\n' "$(git rev-parse HEAD)" "$kind" "$kind" "$kind" | git mktag)
  git update-ref "refs/tags/v-$kind" "$obj"
done`)
	repo := d + "/repo"
	run := func(tag string) string {
		t.Helper()
		e := invoke.EnvMap(map[string]string{"HOME": d, "PATH": os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM": "1"})
		var buf bytes.Buffer
		c := &invoke.Call{Name: "gash_upgrade", Env: e, Term: term.Fixed(e, false, false, false)}
		require.True(t, New().verifyTag(c, ui.StyleOf(c.Term, false).Printer(&buf), repo, tag))
		return buf.String()
	}

	restore := prereq.SetSystem(rockySystem(allBut("gpg", "ssh-keygen")))
	require.Equal(t, "Warning: v-PGP is signed, but its signature cannot be checked: gpg is not installed.\n"+
		"Hint: Install gnupg2: sudo dnf install -y gnupg2\n", run("v-PGP"))
	require.Equal(t, "Warning: v-SSH is signed, but its signature cannot be checked: ssh-keygen is not installed.\n"+
		"Hint: Install openssh-clients: sudo dnf install -y openssh-clients\n", run("v-SSH"))

	// A program the user configured is named as configured.
	bashRun(t, repo, env, "git config gpg.program /nonexistent/gpg2")
	require.Equal(t, "Warning: v-PGP is signed, but its signature cannot be checked: /nonexistent/gpg2 is not installed.\n", run("v-PGP"))
	bashRun(t, repo, env, "git config --unset gpg.program")
	restore()

	// The verifier is there: the signature itself does not verify.
	defer prereq.SetSystem(rockySystem(allBut()))()
	require.Equal(t, "Warning: v-PGP is signed, but the signature could not be verified (is the key in your keyring?).\n", run("v-PGP"))
}

// uninstallHome is a HOME with a Gash install and a .bashrc that loads it.
func uninstallHome(t *testing.T) (home, bashrc string) {
	t.Helper()
	home = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".gash/lib/core"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gash/gash.sh"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gashrc"), []byte("# mine\n"), 0o644))
	bashrc = "a\n# >>> GASH START >>>\nsource ~/.gashrc\n# <<< GASH END <<<\nb\n"
	require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte(bashrc), 0o644))
	return home, bashrc
}

// gash_uninstall removes nothing when the profile edit cannot run or
// fails: without sed it stops on the prerequisite report before the
// question, and a failing sed stops it before the install is removed
// (3.4: it went on and left the profile loading a removed Gash).
func TestUninstallStopsWithoutProfileEdit(t *testing.T) {
	home, bashrc := uninstallHome(t)
	restore := prereq.SetSystem(rockySystem(allBut("sed")))
	_, errOut, rc := callModule(t, New(), "gash_uninstall", map[string]string{"HOME": home, UninstallStepVar: "check"})
	restore()
	require.Equal(t, 1, rc)
	require.Equal(t, "Error: gash_uninstall needs sed, which is not installed.\n"+
		"  sed is used by the installer and gash_uninstall (package sed on Rocky Linux 9.4).\n"+
		"  Install it:  sudo dnf install -y sed\n"+
		"  Or run:      gash_doctor --install sed\n", errOut)
	require.DirExists(t, filepath.Join(home, ".gash"))

	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "sed"), []byte("#!/bin/sh\necho 'sed: -e expression #1, char 3: unknown command' >&2\nexit 1\n"), 0o755))
	_, errOut, rc = callModule(t, New(), "gash_uninstall", map[string]string{"HOME": home, "PATH": bin, UninstallStepVar: "remove"})
	require.Equal(t, 1, rc)
	require.Equal(t, "Info: Removing Gash block from: "+home+"/.bashrc\n"+
		"sed: -e expression #1, char 3: unknown command\n"+
		"Error: Gash was not uninstalled: the Gash block is still in "+home+"/.bashrc.\n", errOut)
	require.DirExists(t, filepath.Join(home, ".gash"))
	require.FileExists(t, filepath.Join(home, ".gashrc"))
	b, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	require.NoError(t, err)
	require.Equal(t, bashrc, string(b))
}
