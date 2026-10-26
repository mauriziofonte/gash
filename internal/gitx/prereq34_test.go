// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

// noGit is Debian with sudo and every program but git; onMac is macOS
// whose only git is the /usr/bin stub of the Command Line Tools.
func noGit() *prereq.System {
	return &prereq.System{OS: platform.OS{GOOS: "linux", ID: "debian", Name: "Debian GNU/Linux 12 (bookworm)"},
		Manager: prereq.Apt, Sudo: true,
		LookPath: func(name string, _ []string) (string, bool) { return "/usr/bin/" + name, name != "git" }}
}

func onMac() *prereq.System {
	return &prereq.System{OS: platform.OS{GOOS: "darwin", Name: "macOS 15.1"}, Manager: prereq.Brew, Brew: "/opt/homebrew/bin/brew",
		LookPath: func(name string, _ []string) (string, bool) { return "/usr/bin/" + name, true },
		Xcode:    func(string) bool { return false }}
}

func runGit(t *testing.T, env map[string]string, fn string, args ...string) (string, int) {
	t.Helper()
	e := invoke.EnvMap(env)
	var out, errb bytes.Buffer
	c := &invoke.Call{Name: fn, Args: args, Env: e, Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, false, false)}
	tbl := invoke.NewTable()
	(&Module{Run: runner.NewFake(t)}).Register(tbl)
	rc := tbl.Run(c)
	require.Empty(t, out.String())
	return errb.String(), rc
}

// The git functions and the gl family report a missing git, the macOS
// stub included, and start nothing (3.4: "Not in a git repository." for
// git_*, status 127 and nothing on stderr for gl*).
func TestMissingGit(t *testing.T) {
	restore := prereq.SetSystem(noGit())
	for _, fn := range []string{"git_list_tags", "git_add_tag", "git_delete_tag", "git_dump_revisions", "git_apply_patch",
		"git_repair", "git_sync", "gl", "gla", "glo", "glg", "gls"} {
		args := []string{"x"}
		switch fn {
		case "git_list_tags", "git_repair", "git_sync", "gl", "gla", "glo", "glg", "gls":
			args = nil
		case "git_apply_patch":
			args = []string{"x", "-y"}
		}
		errOut, rc := runGit(t, nil, fn, args...)
		require.Equal(t, 1, rc, fn)
		require.Equal(t, "Error: "+fn+" needs git, which is not installed.\n"+
			"  git installs and updates Gash, and runs the git functions (package git on Debian GNU/Linux 12).\n"+
			"  Install it:  sudo apt-get install -y git\n"+
			"  Or run:      gash_doctor --install git\n"+
			"  On macOS git comes with the Command Line Tools: xcode-select --install.\n", errOut, fn)
	}
	errOut, rc := runGit(t, nil, "glf", "main.go")
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, "Error: glf needs git, which is not installed.\n")
	errOut, rc = runGit(t, map[string]string{"GASH_HEADLESS": "1"}, "git_sync")
	require.Equal(t, 1, rc)
	require.Equal(t, `{"error":"git_not_found","details":"git_sync needs git, which is not installed","action":"CONTINUE","recoverable":true,`+
		`"hint":"Install git: sudo apt-get install -y git. On macOS git comes with the Command Line Tools: xcode-select --install","missing":"git","install":"sudo apt-get install -y git"}`+"\n", errOut)
	restore()

	defer prereq.SetSystem(onMac())()
	errOut, rc = runGit(t, nil, "gl")
	require.Equal(t, 1, rc)
	require.Equal(t, "Error: gl needs git, which is not installed.\n"+
		"  git installs and updates Gash, and runs the git functions (package git on macOS 15.1).\n"+
		"  Install it:  brew install git\n"+
		"  Or run:      gash_doctor --install git\n"+
		"  On macOS git comes with the Command Line Tools: xcode-select --install.\n", errOut)
}
