// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import "github.com/mauriziofonte/gash/internal/invoke"

// registerText binds the 3.4 functions of the area: llm_stat, llm_count,
// llm_assert, llm_read, llm_log, llm_hash, llm_textstats, llm_textcheck.
func (m *Module) registerText(t *invoke.Table) {
	t.Handle("llm_stat", m.hostable("llm_stat", m.stat))
	t.Handle("llm_count", m.hostable("llm_count", m.count))
	t.Handle("llm_assert", m.hostable("llm_assert", m.assert))
	t.Handle("llm_read", m.hostable("llm_read", m.read))
	t.Handle("llm_log", m.hostable("llm_log", m.log))
	t.Handle("llm_hash", m.hostable("llm_hash", m.hash))
	t.Handle("llm_textstats", m.hostable("llm_textstats", m.textstats))
	t.Handle("llm_textcheck", m.hostable("llm_textcheck", m.textcheck))
}
