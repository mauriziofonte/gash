// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import "github.com/mauriziofonte/gash/internal/invoke"

// registerNet binds the 3.4 functions of the area: llm_http, llm_dns,
// llm_tls, llm_tcp (llm_ports --port is in system.go's handler).
func (m *Module) registerNet(t *invoke.Table) {
	t.Handle("llm_http", m.http)
	t.Handle("llm_dns", m.dns)
	t.Handle("llm_tls", m.tlsCheck)
	t.Handle("llm_tcp", m.tcp)
}
