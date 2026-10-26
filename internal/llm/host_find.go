// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/remote"
)

// findFallback is the POSIX sh llm_find for a host without the gash
// binary: the name search (PATTERN [PATH] [--type f|d] [-n N]) with the
// local output, byte for byte: find with the same pruned names, the paths
// sorted in the walk's order (each directory's entries in byte order: "/"
// mapped below every byte for sort), cut at N. --contains and any option
// of a later version need the binary.
func findFallback(_ *invoke.Call, nonce string, args []string) (*remote.Plan, bool) {
	pattern, fileType, limit := "", "f", "100"
	havePattern, havePath, ended := false, false, false
	path := "."
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !ended {
			switch a {
			case "--":
				ended = true
				continue
			case "--type", "-n", "--limit":
				next := args[i+1:]
				if len(next) == 0 {
					return argError("missing_value", a, a+" needs a value")
				}
				if a == "--type" {
					fileType = next[0]
				} else {
					limit = next[0]
				}
				i++
				continue
			}
			if strings.HasPrefix(a, "-") {
				return nil, false
			}
		}
		switch {
		case !havePattern:
			pattern, havePattern = a, true
		case havePath:
			return argError("unexpected_argument", a, "Pass at most one PATH")
		default:
			path, havePath = a, true
		}
	}
	if pattern == "" {
		return argError("missing_pattern", "", "Provide a name pattern as first argument, e.g. '*.php' or '*'")
	}
	if fileType != "f" && fileType != "d" {
		return argError("invalid_type", fileType, "Use --type f (files) or --type d (directories)")
	}
	if !uintRE.MatchString(limit) {
		return argError("invalid_limit", "Value '"+limit+"' is not a non-negative integer below 1000000000", "Provide a plain integer for limit")
	}
	script := fbHeader("P", path, "PAT", pattern, "T", fileType, "L", limit) + `
abs=$(gash_resolve "$P") || exit 1
gash_forbidden "$abs" && gash_err forbidden_path protected "$abs"
[ -e "$abs" ] || gash_err path_not_found exists "$abs"
gash_forbidden "$abs" names && gash_err forbidden_path subdir "$abs"
[ "$L" -eq 0 ] && exit 0
gash_tools find tr sort head
find "$abs" \( -name node_modules -o -name vendor -o -name .git -o -name __pycache__ -o -name "*$NL*" \) -prune -o -type "$T" -name "$PAT" -print 2>/dev/null |
  tr / '\001' | sort | tr '\001' / | head -n "$L"
exit 0`
	return &remote.Plan{Script: script, Finish: fbFinish(nonce, nil)}, true
}
