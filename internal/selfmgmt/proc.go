// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"context"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
)

// ctxOf is the context of the call; Background when the caller gave none.
func ctxOf(c *invoke.Call) context.Context {
	if c.Ctx != nil {
		return c.Ctx
	}
	return context.Background()
}

// lookPath is `type -P NAME` (and `command -v NAME` for a program) in the
// caller's PATH.
func (m *Module) lookPath(c *invoke.Call, name string) (string, bool) {
	if m.lookPathFn != nil {
		return m.lookPathFn(name)
	}
	p, err := runner.LookPath(name, c.Env.Child(), "")
	return p, err == nil
}

// output is $(NAME ARGS 2>/dev/null) || true: the program run with the
// caller's exported environment and stdin closed, its stdout as command
// substitution leaves it (trailing newlines removed) whatever its status;
// "" when it does not start.
func (m *Module) output(c *invoke.Call, name string, args ...string) string {
	res, err := m.run.Run(ctxOf(c), runner.Cmd{Path: name, Args: args, Options: runner.Options{Env: c.Env.Child()}})
	if err != nil {
		return ""
	}
	return runner.CommandSubst(res.Stdout)
}
