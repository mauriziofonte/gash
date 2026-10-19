// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package config

import "strings"

// FilePath is the configuration file 2.x reads: ${GASH_ENV_FILE:-$HOME/.gash_env}.
// An empty GASH_ENV_FILE counts as unset.
func FilePath(home, envFile string) string {
	if envFile != "" {
		return envFile
	}
	return home + "/.gash_env"
}

// ShownPath is how 2.x names a path in its messages: ${path/#$HOME/\~}.
// HOME is used as an unquoted glob pattern (2.x did not quote it), the
// longest matching prefix is replaced by "~", and the match is not limited
// to whole path components: with HOME=/home/u, /home/user2/x is shown as
// ~ser2/x. An empty HOME matches the empty prefix, so the path gets a
// leading "~".
func ShownPath(path, home string) string {
	if home == "" {
		return "~" + path
	}
	if n := prefixMatch(home, path); n >= 0 {
		return "~" + path[n:]
	}
	return path
}

// prefixMatch is Bash's match_upattern for an anchored-at-start pattern
// (${var/#pat/...}): the length of the matched prefix, or -1. As in Bash, a
// pattern of fixed length (umatchlen) is tried at that length only, after a
// first-character test (match_pattern_char); other patterns take the
// longest matching prefix. The two length rules disagree with the matcher
// on brackets such as "[!]x]", which then never match, as in Bash.
func prefixMatch(pat, s string) int {
	if !globMatch(quickPattern(pat), s) {
		return -1
	}
	mlen := umatchlen(pat)
	if mlen > len(s) || !matchPatternChar(pat, s) {
		return -1
	}
	n := len(s)
	if mlen != -1 {
		n = mlen
	}
	for ; n >= 0; n-- {
		if globMatch(pat, s[:n]) {
			return n
		}
		if mlen != -1 {
			break
		}
	}
	return -1
}

// quickPattern is the pattern match_upattern first tests against the whole
// string: pat followed by "*", unless pat already starts and ends with "*"
// (even an escaped one) or ends with an unescaped "*".
func quickPattern(pat string) string {
	n := len(pat)
	if pat[0] == '*' && pat[n-1] == '*' {
		return pat
	}
	if pat[n-1] == '*' && n >= 2 && pat[n-2] == '\\' {
		escaped := true
		for i := n - 3; i >= 0 && pat[i] == '\\'; i-- {
			escaped = !escaped
		}
		if escaped {
			return pat + "*"
		}
		return pat
	}
	if pat[n-1] != '*' {
		return pat + "*"
	}
	return pat
}

// matchPatternChar is Bash's match_pattern_char: can pat match at the start
// of s at all?
func matchPatternChar(pat, s string) bool {
	var s0, p1 byte
	if s != "" {
		s0 = s[0]
	}
	if len(pat) > 1 {
		p1 = pat[1]
	}
	switch c := pat[0]; c {
	case '\\':
		return s0 == p1
	case '?':
		return p1 == '(' || s != ""
	case '*':
		return true
	case '+', '!', '@':
		return p1 == '(' || s0 == c
	case '[':
		return s != ""
	default:
		return s0 == c
	}
}

// umatchlen is Bash's umatchlen: the number of characters pat matches when
// that number is fixed, -1 otherwise. Its bracket scan reads the first
// character after "[" without testing it for "]" (so "[]x]" is one bracket)
// but stops at the "]" right after "[!" or "[^".
func umatchlen(pat string) int {
	matlen := 0
	inCclass, inCollsym, inEquiv := false, false, false
	at := func(i int) byte {
		if i < len(pat) {
			return pat[i]
		}
		return 0
	}
	i := 0
	for i < len(pat) {
		c := pat[i]
		i++
		switch c {
		default:
			matlen++
		case '\\':
			if i >= len(pat) {
				return matlen + 1
			}
			matlen++
			i++
		case '?', '+', '!', '@':
			if at(i) == '(' {
				return -1
			}
			matlen++
		case '*':
			return -1
		case '[':
			bracklen := 1
			wc := at(i)
			i++
			bad := false
			for {
				switch {
				case wc == 0:
					i--
					matlen += bracklen
					bad = true
				case wc == '\\':
					bracklen++
					if at(i) == 0 || at(i+1) == 0 {
						i++
						matlen += bracklen
						bad = true
					} else {
						i++
					}
				case wc == '[' && at(i) == ':':
					i++
					bracklen++
					inCclass = true
				case inCclass && wc == ':' && at(i) == ']':
					i++
					bracklen++
					inCclass = false
				case wc == '[' && at(i) == '.':
					i++
					bracklen++
					if at(i) == ']' {
						i++
						bracklen++
					}
					inCollsym = true
				case inCollsym && wc == '.' && at(i) == ']':
					i++
					bracklen++
					inCollsym = false
				case wc == '[' && at(i) == '=':
					i++
					bracklen++
					if at(i) == ']' {
						i++
						bracklen++
					}
					inEquiv = true
				case inEquiv && wc == '=' && at(i) == ']':
					i++
					bracklen++
					inEquiv = false
				default:
					bracklen++
				}
				if bad {
					break
				}
				wc = at(i)
				i++
				if wc == ']' {
					break
				}
			}
			if !bad {
				matlen++
			}
		}
	}
	return matlen
}

// ExpandTilde is the SSH key path rule ${key/#\~/$HOME}: a leading "~" is
// replaced by HOME (so "~user/k" becomes HOME+"user/k"). HOME is an unquoted
// replacement string, processed as Bash 5.2 (the S1 reference) does with
// patsub_replacement on: "&" stands for the matched "~", "\&" for a literal
// "&" and "\\" for one backslash.
func ExpandTilde(key, home string) string {
	if !strings.HasPrefix(key, "~") {
		return key
	}
	var b strings.Builder
	for i := 0; i < len(home); i++ {
		c := home[i]
		switch {
		case c == '\\' && i+1 < len(home) && (home[i+1] == '&' || home[i+1] == '\\'):
			b.WriteByte(home[i+1])
			i++
		case c == '&':
			b.WriteByte('~')
		default:
			b.WriteByte(c)
		}
	}
	return b.String() + key[1:]
}

// globMatch reports whether s matches the Bash pattern p as a whole: a
// port of Bash's strmatch (lib/glob/sm_loop.c) for the C locale, extglob
// off, no flags ("*" and "?" also match "/" and a leading "."; ranges by
// byte value, as with globasciiranges).
func globMatch(p, s string) bool {
	for len(p) > 0 {
		switch p[0] {
		case '*':
			for len(p) > 0 && p[0] == '*' {
				p = p[1:]
			}
			if p == "" {
				return true
			}
			for i := 0; i <= len(s); i++ {
				if globMatch(p, s[i:]) {
					return true
				}
			}
			return false
		case '?':
			if s == "" {
				return false
			}
			p, s = p[1:], s[1:]
		case '[':
			if s == "" {
				return false
			}
			end := brackmatch(p, 1, s[0])
			if end < 0 {
				return false
			}
			p, s = p[end:], s[1:]
		case '\\':
			// A trailing backslash matches a backslash that ends the
			// string, and nothing else.
			if len(p) == 1 {
				return s == "\\"
			}
			if s == "" || s[0] != p[1] {
				return false
			}
			p, s = p[2:], s[1:]
		default:
			if s == "" || s[0] != p[0] {
				return false
			}
			p, s = p[1:], s[1:]
		}
	}
	return s == ""
}

// brackmatch is Bash's BRACKMATCH: pat[p:] follows a "[" and test is the
// character to match. It returns the index just past the bracket expression
// when test matches, -1 when it does not. An expression without its closing
// "]" makes the "[" an ordinary character: brackmatch then returns p when
// test is "[", -1 otherwise. Kept from Bash: the "]" right after an
// equivalence class that did not match is a member, not the end, so where
// the expression ends depends on test.
func brackmatch(pat string, p int, test byte) int {
	at := func(i int) byte {
		if i < len(pat) {
			return pat[i]
		}
		return 0
	}
	savep := p
	unterminated := func() int {
		if test == '[' {
			return savep
		}
		return -1
	}
	not := at(p) == '!' || at(p) == '^'
	if not {
		p++
	}
	c := at(p)
	p++
	matched := false
	for {
		cstart, cend := c, c //nolint:ineffassign // range parser: cend's symmetric init pairs with cstart and is set per branch before use

		// Equivalence class [=c=] (single character only).
		if c == '[' && at(p) == '=' && at(p+2) == '=' && at(p+3) == ']' {
			pc := at(p + 1)
			p += 4
			if test == pc {
				p++
				matched = true
				break
			}
			c = at(p)
			p++
			if c == 0 {
				return unterminated()
			}
			continue
		}

		// Character class [:name:].
		if c == '[' && at(p) == ':' {
			hit := false
			end := p + 1
			for at(end) != 0 && !(at(end) == ':' && at(end+1) == ']') {
				end++
			}
			if at(end) != 0 {
				hit = classMatch(dequote(pat[p+1:end]), test)
				p = end + 2
			}
			if hit {
				p++
				matched = true
				break
			}
			c = at(p)
			p++
			if c == 0 {
				return unterminated()
			}
			if c == ']' {
				break
			}
			continue
		}

		// Collating symbol [.c.] (single characters; named symbols are
		// not known here and never match).
		if c == '[' && at(p) == '.' {
			var sym int
			p, sym = parseCollsym(pat, p)
			if sym < 0 {
				cstart = test + 1
			} else {
				cstart = byte(sym)
			}
		}

		if c == '\\' {
			if at(p) == 0 {
				return -1
			}
			cstart = at(p)
			p++
		}
		cend = cstart
		isrange := false

		if c == 0 {
			return unterminated()
		}
		c = at(p)
		p++
		if c == 0 {
			return unterminated()
		}

		if c == '-' && at(p) != ']' {
			cend = at(p)
			p++
			if cend == '\\' {
				cend = at(p)
				p++
			}
			if cend == 0 {
				return -1
			}
			if cend == '[' && at(p) == '.' {
				var sym int
				p, sym = parseCollsym(pat, p)
				if sym < 0 {
					cend = test - 1
				} else {
					cend = byte(sym)
				}
			}
			c = at(p)
			p++
			if cstart > cend {
				// An inverted range matches nothing; the rest of the
				// expression is still checked.
				if c == ']' {
					break
				}
				continue
			}
			isrange = true
		}

		if (!isrange && test == cstart) || (isrange && test >= cstart && test <= cend) {
			matched = true
			break
		}
		if c == ']' {
			break
		}
	}

	if !matched {
		if not {
			return p
		}
		return -1
	}

	// Skip the rest of an expression that already matched.
	p--
	c = at(p)
	brcnt := 1
	brchrp := -1
	for brcnt > 0 {
		if c == 0 {
			return unterminated()
		}
		oc := c
		c = at(p)
		p++
		switch {
		case c == '[' && (at(p) == '=' || at(p) == ':' || at(p) == '.'):
			brcnt++
			brchrp = p
			p++
			if c = at(p); c == 0 {
				return unterminated()
			}
		case c == ']' && brcnt > 1 && brchrp >= 0 && oc == at(brchrp):
			brcnt--
			brchrp = -1
		case c == ']' && brcnt == 1:
			brcnt--
		case c == '\\':
			if at(p) == 0 {
				return -1
			}
			p++
		}
	}
	if not {
		return -1
	}
	return p
}

// parseCollsym is Bash's PARSE_COLLSYM: pat[p] is the "." after "[". It
// returns the index after ".]" and the symbol (-1 when invalid or
// unterminated, in which case the index is that of the end of pattern).
func parseCollsym(pat string, p int) (int, int) {
	p++
	end := strings.Index(pat[min(p, len(pat)):], ".]")
	if p > len(pat) || end < 0 {
		return len(pat), -1
	}
	name := pat[p : p+end]
	if len(name) == 1 {
		return p + end + 2, int(name[0])
	}
	return p + end + 2, -1
}

// dequote removes the backslash quoting of a character class name.
func dequote(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// classMatch is the C-locale meaning of the POSIX character classes.
func classMatch(name string, c byte) bool {
	isUpper := c >= 'A' && c <= 'Z'
	isLower := c >= 'a' && c <= 'z'
	isDigit := c >= '0' && c <= '9'
	isAlpha := isUpper || isLower
	isPunct := c >= '!' && c <= '~' && !isAlpha && !isDigit
	switch name {
	case "alnum":
		return isAlpha || isDigit
	case "alpha":
		return isAlpha
	case "ascii":
		return c < 0x80
	case "blank":
		return c == ' ' || c == '\t'
	case "cntrl":
		return c < 0x20 || c == 0x7f
	case "digit":
		return isDigit
	case "graph":
		return c > ' ' && c < 0x7f
	case "lower":
		return isLower
	case "print":
		return c >= ' ' && c < 0x7f
	case "punct":
		return isPunct
	case "space":
		return isSpace(c)
	case "upper":
		return isUpper
	case "word":
		return isAlpha || isDigit || c == '_'
	case "xdigit":
		return isDigit || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
	}
	return false
}

// isSpace is [[:space:]] in the C locale: the only whitespace 3.x trims
// (LEDGER CORE-08).
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\v' || c == '\f' || c == '\r'
}
