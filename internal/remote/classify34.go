// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import "strings"

// ClassifyLocal applies the transfer path policy (the DENY and ASK tiers of
// classifyPath) to a local write target that is an absolute, already
// resolved path no shell will expand: "allow", "deny:<prefix>" or
// "ask:<prefix>". The metacharacter rule protects a remote shell and does
// not apply, so those characters are neutralized first (no tier entry
// contains one, so no verdict changes otherwise). The agent_* write family
// uses it for every local target, the same policy remote writes follow.
func ClassifyLocal(abs string) string {
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune("*?[]{}$`;&|<>()!'\"\\\n~", r) {
			return '_'
		}
		return r
	}, abs)
	return classifyPath(clean)
}
