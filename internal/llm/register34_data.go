// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import "github.com/mauriziofonte/gash/internal/invoke"

// registerData binds the 3.4 functions of the area: llm_json, llm_csv,
// llm_dotenv, llm_date (the llm_db_* functions register in internal/db).
func (m *Module) registerData(t *invoke.Table) {
	t.Handle("llm_json", m.hostable("llm_json", m.json))
	t.Handle("llm_csv", m.hostable("llm_csv", m.csv))
	t.Handle("llm_dotenv", m.hostable("llm_dotenv", m.dotenv))
	t.Handle("llm_date", m.date)
}
