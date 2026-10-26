// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import "github.com/mauriziofonte/gash/internal/invoke"

// registerProc binds the 3.4 functions of the area: llm_wait, llm_service,
// llm_docker_ps, llm_docker_logs, llm_docker_exec, llm_sysinfo.
func (m *Module) registerProc(t *invoke.Table) {
	t.Handle("llm_wait", m.wait)
	t.Handle("llm_service", m.service)
	t.Handle("llm_docker_ps", m.dockerPS)
	t.Handle("llm_docker_logs", m.dockerLogs)
	t.Handle("llm_docker_exec", m.dockerExec)
	t.Handle("llm_sysinfo", m.sysinfoLLM)
}
