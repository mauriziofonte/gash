// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package pathguard

import "strings"

// matchGlob reports whether name matches the shell pattern pat the way bash
// does in `[[ name == pat ]]` (fnmatch without FNM_PATHNAME): "*" matches any
// run of characters including "/", "?" any single character, and "[...]" a
// bracket class with ranges and a leading "!" or "^" negation. A backslash
// escapes the next character. The secret tables are matched on base names (no
// "/"), so the FNM_PATHNAME difference never shows.
func matchGlob(pat, name string) bool {
	return globMatch([]byte(pat), []byte(name))
}

// MatchGlobSegment reports whether name matches the shell pattern pat (the
// same fnmatch rules matchGlob uses). Exported for llm_exec glob expansion so
// one matcher decides for both packages.
func MatchGlobSegment(pat, name string) bool {
	return matchGlob(pat, name)
}

func globMatch(pat, s []byte) bool {
	for len(pat) > 0 {
		switch pat[0] {
		case '*':
			// Collapse consecutive stars.
			for len(pat) > 0 && pat[0] == '*' {
				pat = pat[1:]
			}
			if len(pat) == 0 {
				return true
			}
			for i := 0; i <= len(s); i++ {
				if globMatch(pat, s[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(s) == 0 {
				return false
			}
			pat, s = pat[1:], s[1:]
		case '[':
			if len(s) == 0 {
				return false
			}
			matched, rest, ok := matchClass(pat, s[0])
			if !ok {
				// A malformed class is treated as a literal '['.
				if s[0] != '[' {
					return false
				}
				pat, s = pat[1:], s[1:]
				continue
			}
			if !matched {
				return false
			}
			pat, s = rest, s[1:]
		case '\\':
			if len(pat) >= 2 {
				if len(s) == 0 || s[0] != pat[1] {
					return false
				}
				pat, s = pat[2:], s[1:]
			} else {
				if len(s) == 0 || s[0] != '\\' {
					return false
				}
				pat, s = pat[1:], s[1:]
			}
		default:
			if len(s) == 0 || s[0] != pat[0] {
				return false
			}
			pat, s = pat[1:], s[1:]
		}
	}
	return len(s) == 0
}

// matchClass evaluates a "[...]" bracket expression at the head of pat
// against the byte c. It returns whether c matched, the pattern remaining
// after the class, and ok=false when the class is not terminated.
func matchClass(pat []byte, c byte) (matched bool, rest []byte, ok bool) {
	i := 1
	negate := false
	if i < len(pat) && (pat[i] == '!' || pat[i] == '^') {
		negate = true
		i++
	}
	found := false
	first := true
	for i < len(pat) {
		if pat[i] == ']' && !first {
			return found != negate, pat[i+1:], true
		}
		first = false
		lo := pat[i]
		if lo == '\\' && i+1 < len(pat) {
			i++
			lo = pat[i]
		}
		// A range lo-hi, unless the '-' is last before ']'.
		if i+2 < len(pat) && pat[i+1] == '-' && pat[i+2] != ']' {
			hi := pat[i+2]
			if hi == '\\' && i+3 < len(pat) {
				hi = pat[i+3]
				i++
			}
			if c >= lo && c <= hi {
				found = true
			}
			i += 3
			continue
		}
		if c == lo {
			found = true
		}
		i++
	}
	return false, nil, false
}

// IcaseGlob turns a glob into a case-insensitive one for tools without
// -iname (grep --exclude): ".env" -> ".[eE][nN][vV]". Ported from
// __llm_icase_glob.
func IcaseGlob(in string) string {
	var b strings.Builder
	for i := 0; i < len(in); i++ {
		c := in[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			lo := c | 0x20
			up := c &^ 0x20
			b.WriteByte('[')
			b.WriteByte(lo)
			b.WriteByte(up)
			b.WriteByte(']')
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}
