// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/prereq"
)

// TestGitCommitXcodeStub: on macOS without the Command Line Tools
// /usr/bin/git is a stub, reported as git_not_found with the install
// command before anything runs (3.4 before: the stub ran, and its
// failure read as a directory outside a repository).
func TestGitCommitXcodeStub(t *testing.T) {
	t.Cleanup(prereq.SetSystem(&prereq.System{
		OS:      platform.OS{GOOS: "darwin", ID: "macos", Name: "macOS 15.1"},
		Manager: prereq.Brew, Brew: "/opt/homebrew/bin/brew", Env: []string{"PATH=/usr/bin:/bin"},
		LookPath: func(name string, _ []string) (string, bool) { return "/usr/bin/" + name, true },
		Xcode:    func(string) bool { return false },
	}))
	d := physDir(t)
	r := commitCall(t, d, physDir(t), "-m", "x", "-A")
	require.Equal(t, result{"", `{"error":"git_not_found","details":"agent_git_commit needs git, which is not installed","action":"CONTINUE","recoverable":true,` +
		`"hint":"Install git: brew install git. On macOS git comes with the Command Line Tools: xcode-select --install","missing":"git","install":"brew install git"}` + "\n", 1}, r)
}
