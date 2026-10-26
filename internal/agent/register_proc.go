// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import "github.com/mauriziofonte/gash/internal/invoke"

// registerProc binds agent_kill (P2), agent_docker_exec (P5).
func (m *Module) registerProc(t *invoke.Table) {
	t.Handle("agent_kill", m.kill)
	t.Handle("agent_docker_exec", m.dockerExec)
}
