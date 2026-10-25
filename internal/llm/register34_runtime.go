// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import "github.com/mauriziofonte/gash/internal/invoke"

// registerRuntime binds the 3.4 functions of the area: llm_tools,
// llm_runtime, llm_lint, llm_test.
func (m *Module) registerRuntime(t *invoke.Table) {
	_ = t
}
