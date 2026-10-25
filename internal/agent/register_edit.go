// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import "github.com/mauriziofonte/gash/internal/invoke"

// registerEdit binds agent_replace (E1).
func (m *Module) registerEdit(t *invoke.Table) {
	t.Handle("agent_replace", m.replace)
}
