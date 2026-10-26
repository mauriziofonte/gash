// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import "github.com/mauriziofonte/gash/internal/invoke"

// registerRemote binds the 3.4 functions of the area: llm_remote_run,
// llm_same.
func (m *Module) registerRemote(t *invoke.Table) {
	t.Handle("llm_remote_run", m.remoteRun)
	t.Handle("llm_same", m.same)
}
