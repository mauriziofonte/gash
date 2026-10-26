// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import "github.com/mauriziofonte/gash/internal/invoke"

// registerRuntime binds the 3.4 functions of the area: llm_tools,
// llm_runtime, llm_lint (agent_test, the test runner, is a write tool:
// internal/agent).
func (m *Module) registerRuntime(t *invoke.Table) {
	t.Handle("llm_tools", m.hostable("llm_tools", m.tools))
	t.Handle("llm_runtime", m.runtime)
	t.Handle("llm_lint", m.hostable("llm_lint", m.lint))
}
