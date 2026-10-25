// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import "github.com/mauriziofonte/gash/internal/invoke"

// registerNet binds the 3.4 functions of the area: llm_http, llm_dns,
// llm_tls, llm_tcp.
func (m *Module) registerNet(t *invoke.Table) {
	_ = t
}
