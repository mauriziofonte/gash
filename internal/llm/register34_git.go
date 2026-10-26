// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import "github.com/mauriziofonte/gash/internal/invoke"

// registerGit binds the 3.4 functions of the area: llm_git_overview,
// llm_git_show.
func (m *Module) registerGit(t *invoke.Table) {
	t.Handle("llm_git_overview", m.hostable("llm_git_overview", m.gitOverview))
	t.Handle("llm_git_show", m.gitShow)
}
