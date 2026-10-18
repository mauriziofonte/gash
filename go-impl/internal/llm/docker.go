// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
)

// dockerCheck is llm_docker_check [PATH]: docker_compose_check PATH --json,
// PATH "." when empty or absent; later words are ignored, as 2.x passed
// only "${1:-.}". No path guard, as in 2.x (LEDGER LLM-20); the binary
// cannot change the caller's shell variables (LLM-21).
func (m *Module) dockerCheck(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	path := c.Arg(0)
	if path == "" {
		path = "."
	}
	sub := *c
	sub.Name = "docker_compose_check"
	sub.Args = []string{path, "--json"}
	if c.Handlers == nil {
		return c.Fail(invoke.NotPorted(sub.Name))
	}
	return c.Handlers.Run(&sub)
}
