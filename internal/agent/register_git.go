// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import "github.com/mauriziofonte/gash/internal/invoke"

// registerGit binds agent_git_commit (G2).
func (m *Module) registerGit(t *invoke.Table) {
	t.Handle("agent_git_commit", m.gitCommit)
}
