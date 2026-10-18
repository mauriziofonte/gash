// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package llm holds the llm_* functions of the 2.x lib/modules/llm module
// that read files and the system (llm_tree, llm_find, llm_grep, llm_config,
// llm_env, llm_ports, llm_procs) and the llm_exec handler. The llm_db_*
// family lives in internal/db.
package llm

import (
	"errors"
	"os"
	"regexp"
	"syscall"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/runner"
)

// Module is the llm handler set with its process and filesystem seams.
type Module struct {
	run      runner.Runner
	getwd    func() (string, error)
	rootHome func() string
	readFile func(string) ([]byte, error)
}

// New returns the module wired to the real system.
func New() *Module {
	return &Module{
		run:      runner.Exec{},
		getwd:    syscall.Getwd,
		rootHome: pathguard.RootHome,
		readFile: os.ReadFile,
	}
}

// Register binds the llm functions to t.
func Register(t *invoke.Table) { New().Register(t) }

// Register binds the functions of m to t.
func (m *Module) Register(t *invoke.Table) {
	t.Handle("llm_tree", m.tree)
	t.Handle("llm_find", m.find)
	t.Handle("llm_grep", m.grep)
}

// helpAnywhere is __llm_needs_help: --help or -h anywhere among the
// arguments prints the help and ends the call.
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

// refusal writes a pathguard or llmexec refusal as 2.x printed it.
func refusal(c *invoke.Call, err error) int {
	var r *pathguard.Refusal
	if !errors.As(err, &r) {
		return fail(c, "internal_error", err.Error(), "")
	}
	return c.Fail(envelopeOf(r.Code, r.Details, r.Action, r.Hint))
}

// envelopeOf is the envelope of a refusal that carries its 2.x action: the
// code's fixed action when they agree (always, for the codes of this
// package's guards), the 2.x action otherwise.
func envelopeOf(code, details, action, hint string) contract.Envelope {
	if a, ok := contract.ActionOf(code); ok && string(a) == action {
		return contract.New(code, details, hint)
	}
	return contract.Legacy(code, details, action, hint)
}

var uintRE = regexp.MustCompile(`^[0-9]{1,9}$`)

// validUint is __llm_validate_uint: value must be 1 to 9 ASCII digits,
// otherwise invalid_<name> is written and false returned.
func validUint(c *invoke.Call, name, value string) bool {
	if uintRE.MatchString(value) {
		return true
	}
	fail(c, "invalid_"+name, "Value '"+value+"' is not a positive integer below 1000000000",
		"Provide a plain integer for "+name)
	return false
}

// atoi is $((10#value)) for a value validUint accepted.
func atoi(value string) int {
	n := 0
	for i := 0; i < len(value); i++ {
		n = n*10 + int(value[i]-'0')
	}
	return n
}

// guard is the path guard of the call: the physical working directory
// (realpath -m resolves relative paths from getcwd) and the caller's HOME.
func (m *Module) guard(c *invoke.Call) *pathguard.Guard {
	cwd, err := m.getwd()
	if err != nil {
		cwd = c.Env.Get("PWD")
	}
	return pathguard.NewGuard(pathguard.OSFS{}, cwd, c.Env.Get("HOME"), m.rootHome())
}

// validatePath is __llm_validate_path; on a refusal the envelope is
// written and ok is false.
func (m *Module) validatePath(c *invoke.Call, path string) (string, bool) {
	safe, err := m.guard(c).ValidatePath(path)
	if err != nil {
		refusal(c, err)
		return "", false
	}
	return safe, true
}

// pathTaker is __llm_take_path: the first positional is the PATH, a second
// one is unexpected_argument.
type pathTaker struct {
	path string
	set  bool
}

func (p *pathTaker) take(c *invoke.Call, v string) bool {
	if p.set {
		fail(c, "unexpected_argument", v, "Pass at most one PATH")
		return false
	}
	p.path, p.set = v, true
	return true
}
