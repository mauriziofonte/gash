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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/remote"
	"github.com/mauriziofonte/gash/internal/runner"
)

// Module is the agent handler set with its process and filesystem seams.
type Module struct {
	run      runner.Runner
	getwd    func() (string, error)
	rootHome func() string
	readFile func(string) ([]byte, error)
	getpid   func() int
	exe      func() (string, error)
}

// New returns the module wired to the real system.
func New() *Module {
	return &Module{
		run:      runner.Exec{},
		getwd:    syscall.Getwd,
		rootHome: pathguard.RootHome,
		readFile: os.ReadFile,
		getpid:   os.Getpid,
		exe:      os.Executable,
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

// helpAnywhere: --help or -h anywhere among the arguments prints the help
// and ends the call, as on every llm_* line.
func helpAnywhere(c *invoke.Call) bool {
	for _, a := range c.Args {
		if a == "--help" || a == "-h" {
			help.NeedsOf(c, "--help")
			return true
		}
	}
	return false
}

// fail writes the envelope of code (with its fixed action) and returns 1.
func fail(c *invoke.Call, code, details, hint string) int {
	return c.Fail(contract.New(code, details, hint))
}

// refusal writes a pathguard refusal with its action.
func refusal(c *invoke.Call, err error) int {
	var r *pathguard.Refusal
	if !errors.As(err, &r) {
		return fail(c, "internal_error", err.Error(), "")
	}
	if a, ok := contract.ActionOf(r.Code); ok && string(a) == r.Action {
		return c.Fail(contract.New(r.Code, r.Details, r.Hint))
	}
	return c.Fail(contract.Legacy(r.Code, r.Details, r.Action, r.Hint))
}

// guard is the path guard of the call, as llm_* builds it: the physical
// working directory and the caller's HOME.
func (m *Module) guard(c *invoke.Call) *pathguard.Guard {
	cwd, err := m.getwd()
	if err != nil {
		cwd = c.Env.Get("PWD")
	}
	return pathguard.NewGuard(pathguard.OSFS{}, cwd, c.Env.Get("HOME"), m.rootHome())
}

// target validates one explicit write target: the llm_* path rails
// (traversal, ~user, the protected trees), the secret-file rule, which a
// write refuses outright, and the transfer path policy of remote writes
// (internal/remote): a DENY path (system binaries, /etc/passwd, /boot,
// ...) is write_path_denied (FATAL), an ASK path (/etc, /var, /usr/local,
// /root, any .ssh) is sensitive_path (STOP: ask the user) unless force or
// the target is the caller's own work (askWaived).
// It returns the resolved absolute path; on a refusal the envelope is
// written and ok is false.
func (m *Module) target(c *invoke.Call, g *pathguard.Guard, path string, force bool) (string, bool) {
	safe, err := g.ValidatePath(path)
	if err != nil {
		refusal(c, err)
		return "", false
	}
	if !filepath.IsAbs(safe) {
		safe = filepath.Join(g.Cwd, safe)
	}
	if g.IsSecretFile(path) || g.IsSecretFile(safe) {
		fail(c, "secret_file_blocked", path, "agent_* functions never write secret files")
		return "", false
	}
	kind, detail, _ := strings.Cut(remote.ClassifyLocal(safe), ":")
	switch {
	case kind == "deny":
		fail(c, "write_path_denied", safe+" (matched: "+detail+")", "This path is never written, not even with --force")
		return "", false
	case kind == "ask" && !force && !askWaived(g, safe, detail):
		fail(c, "sensitive_path", safe+" (matched: "+detail+")", "Ask the user; with their agreement run again with --force")
		return "", false
	}
	return safe, true
}

// askWaived reports an ASK target that is the caller's own work: below
// their HOME (root's /root included), or below the working directory the
// agent runs in (a project in /var/www), unless that directory is /, an
// ASK root itself or below /etc. A .ssh component is never waived.
func askWaived(g *pathguard.Guard, safe, detail string) bool {
	if detail == ".ssh" {
		return false
	}
	if h := strings.TrimSuffix(g.Home, "/"); h != "" && within(safe, h) {
		return true
	}
	cwd := strings.TrimSuffix(g.Cwd, "/")
	if cwd == "" || within(cwd, "/etc") {
		return false
	}
	for _, root := range []string{"/var", "/usr/local", "/root", "/usr"} {
		if cwd == root {
			return false
		}
	}
	return within(safe, cwd)
}

// within reports p == dir or p below dir.
func within(p, dir string) bool { return p == dir || strings.HasPrefix(p, dir+"/") }
