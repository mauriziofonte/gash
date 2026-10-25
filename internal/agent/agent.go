// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package agent holds the agent_* family: the write tools for AI agents
// (plan D1, section 6.1). llm_* stays read-only with no exception; every
// change an agent makes through Gash is an agent_* call:
//
//   - the same foundation as llm_*: the JSON envelope, the error contract,
//     --format and the output budget (internal/llmfmt), --host, the
//     telemetry line, registry entries;
//   - --dry-run on every function, reporting what would change and
//     changing nothing;
//   - targets always explicit: no implicit glob, no implicit "everything"
//     (agent_git_commit has no default -A);
//   - the llm_* rails on secret files and blocked paths; never a force
//     push.
//
// The card lists the family under its own topic ("Write tools for AI
// agents") and AGENTS.md gives it its own section, so whoever enables only
// llm_* knows what stays out.
package agent

import (
	"os"
	"syscall"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/runner"
)

// Module is the agent handler set with its process and filesystem seams.
type Module struct {
	run      runner.Runner
	getwd    func() (string, error)
	rootHome func() string
	readFile func(string) ([]byte, error)
	getpid   func() int
}

// New returns the module wired to the real system.
func New() *Module {
	return &Module{
		run:      runner.Exec{},
		getwd:    syscall.Getwd,
		rootHome: pathguard.RootHome,
		readFile: os.ReadFile,
		getpid:   os.Getpid,
	}
}

// Register binds the agent functions to t.
func Register(t *invoke.Table) { New().Register(t) }

// Register binds the functions of m to t: one hook per area, each in its
// own file, so parallel work never edits the same lines.
func (m *Module) Register(t *invoke.Table) {
	m.registerEdit(t)
	m.registerGit(t)
	m.registerProc(t)
	m.registerRemote(t)
}
