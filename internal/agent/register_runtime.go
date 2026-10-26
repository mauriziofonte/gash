// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import "github.com/mauriziofonte/gash/internal/invoke"

// registerRuntime binds agent_test (L2).
func (m *Module) registerRuntime(t *invoke.Table) {
	t.Handle("agent_test", m.test)
}
