// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
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
