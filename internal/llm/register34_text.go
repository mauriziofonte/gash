// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import "github.com/mauriziofonte/gash/internal/invoke"

// registerText binds the 3.4 functions of the area: llm_stat, llm_count,
// llm_assert, llm_read, llm_log, llm_hash, llm_textstats, llm_textcheck.
func (m *Module) registerText(t *invoke.Table) {
	_ = t
}
