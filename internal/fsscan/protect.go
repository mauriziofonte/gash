// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/pathguard"
)

// userHidden is the user's PROTECT:all trees (the PROTECT: lines of
// ~/.gash_env, see internal/pathguard), resolved and as written: the
// scanners refuse a PATH in one and leave them out of every walk, as they
// do the protected locations (3.4.1). A PROTECT:content tree needs nothing
// here: the scanners show names and sizes, never content. nil when the
// file has no PROTECT:all line.
func userHidden(c *invoke.Call, home, pwd string) []string {
	rules := pathguard.Rules(c.Env.Get)
	if len(rules) == 0 {
		return nil
	}
	return pathguard.NewGuard(pathguard.OSFS{}, pwd, home, pathguard.RootHome()).WithRules(rules).Hidden
}
