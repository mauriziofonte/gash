// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import "strings"

// resolvedAliases are the spellings /etc, /var and /tmp take once a path is
// resolved on macOS, where the three are symlinks into /private: a target
// that resolves to /private/etc/hosts is judged as /etc/hosts.
var resolvedAliases = []string{"/private/etc", "/private/var", "/private/tmp"}

// resolvedSafePrefixes are per-user temporary trees inside an ASK root,
// allowed as /tmp and /var/tmp are: macOS gives each user one below
// /var/folders (its $TMPDIR).
var resolvedSafePrefixes = []string{"/var/folders"}

// UnaliasResolved is abs with a macOS /private alias of /etc, /var or /tmp
// spelled as the path it stands for (resolvedAliases); other paths are
// returned unchanged.
func UnaliasResolved(abs string) string {
	for _, a := range resolvedAliases {
		if under(abs, a) {
			return strings.TrimPrefix(abs, "/private")
		}
	}
	return abs
}

// classifyResolved is classifyPath for a resolved absolute path: the
// /private aliases unaliased first, and a per-user temporary tree that an
// ASK prefix would cover allowed.
func classifyResolved(abs string) string {
	abs = UnaliasResolved(abs)
	v := classifyPath(abs)
	if strings.HasPrefix(v, "ask:/") {
		for _, p := range resolvedSafePrefixes {
			if under(abs, p) {
				return "allow"
			}
		}
	}
	return v
}

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
	return classifyResolved(clean)
}
