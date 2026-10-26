// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package runcmd holds three top-level commands of the binary for agents
// (plan S7, S8, S11):
//
//   - gash runtime php|node|composer|npm|npx VERSION [--path]: a runtime
//     version resolved to its binary without nvm.sh, cached;
//   - gash run [budget] [--test-summary] [--runtime NAME:VERSION] [--safe]
//     -- CMD...: CMD run with its output shaped, its exit status kept;
//   - gash agent-functions [--json]: the llm_* and agent_* functions, for
//     a wrapper that calls `gash <fn>` directly.
//
// They parse their own words (no cobra: gash run starts a program per
// call) and answer `__complete` for the shell completion.
package runcmd

import (
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
)

// Module holds the seams of the commands.
type Module struct {
	run runner.Runner
}

// New returns the commands wired to the real system.
func New() *Module { return &Module{run: runner.Exec{}} }

// Register binds the commands to t.
func Register(t *invoke.Table) { New().Register(t) }

// Register binds the commands of m to t.
func (m *Module) Register(t *invoke.Table) {
	t.HandleCommand("runtime", m.runtime)
	t.HandleCommand("run", m.runCmd)
	t.HandleCommand("agent-functions", m.agentFunctions)
}

func fail(c *invoke.Call, code, details, hint string) int {
	return c.Fail(contract.New(code, details, hint))
}

// completing reports whether the call is a completion request
// (`gash __complete WORD ARGS... PARTIAL`) and returns the words before
// the one being completed and that partial word.
func completing(c *invoke.Call) ([]string, string, bool) {
	if len(c.Args) == 0 || c.Args[0] != cobra.ShellCompRequestCmd {
		return nil, "", false
	}
	words := c.Args[1:]
	if len(words) == 0 {
		return nil, "", true
	}
	return words[:len(words)-1], words[len(words)-1], true
}

// complete writes the candidates that start with partial and the
// directive line cobra reads back.
func complete(w io.Writer, partial string, directive cobra.ShellCompDirective, candidates ...string) int {
	var b strings.Builder
	for _, cand := range candidates {
		if strings.HasPrefix(cand, partial) {
			b.WriteString(cand + "\n")
		}
	}
	b.WriteString(":" + strconv.Itoa(int(directive)) + "\n")
	_, _ = io.WriteString(w, b.String())
	return 0
}

func isHelp(a string) bool { return a == "--help" || a == "-h" }
