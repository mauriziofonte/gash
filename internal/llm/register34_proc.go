// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import "github.com/mauriziofonte/gash/internal/invoke"

// registerProc binds the 3.4 functions of the area: llm_wait, llm_service,
// llm_docker_ps, llm_docker_logs, llm_docker_exec, llm_sysinfo.
func (m *Module) registerProc(t *invoke.Table) {
	_ = t
}
