// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The pattern dialect of `grep -i -- PATTERN` in 2.x history_grep and
// process_find: GNU grep's basic regular expressions (with the \+ \? \|
// \{m,n\} \( \) extensions, POSIX classes and the GNU backslash classes),
// matched case-insensitively with Unicode simple folding in every locale
// (LEDGER SYS-38). breToRE2 translates one to a Go regexp.

// grepError is a pattern GNU grep refuses: the text after "grep: ".
type grepError struct{ msg string }

func (e *grepError) Error() string { return e.msg }

// errNoRE2 is a pattern GNU grep accepts but Go regexp cannot express
// (a back-reference, word-start and word-end anchors, a count over
// 1000): rc.A refuses it with "Error: Invalid pattern: P".
var errNoRE2 = errors.New("no Go regexp equivalent")

const (
	breDupMax = 0x7fff // RE_DUP_MAX of GNU regex
	re2DupMax = 1000   // the largest Go regexp repeat count
)

// GNU grep's messages for the patterns it refuses.
const (
	msgUnmatchedParen   = "Unmatched ( or \\("
	msgUnmatchedClose   = "Unmatched ) or \\)"
	msgUnmatchedBrace   = "Unmatched \\{"
	msgBadBrace         = "Invalid content of \\{\\}"
	msgTooBig           = "Regular expression too big"
	msgUnmatchedBracket = "Unmatched [, [^, [:, [., or [="
	msgBadClass         = "Invalid character class name"
	msgBadRange         = "Invalid range end"
	msgBadCollation     = "Invalid collation character"
	msgTrailingBS       = "Trailing backslash"
	msgBadBackref       = "Invalid back reference"
	msgColonClass       = "character class syntax is [[:space:]], not [:space:]"
)

// compileBRE compiles a grep -i pattern. Newlines separate patterns, as
// grep reads them; an empty one matches every line.
func compileBRE(pattern string) (*regexp.Regexp, error) {
	parts := strings.Split(pattern, "\n")
	alts := make([]string, 0, len(parts))
	for _, p := range parts {
		re, err := breToRE2(p)
		if err != nil {
			return nil, err
		}
		alts = append(alts, "(?:"+re+")")
	}
	re, err := regexp.Compile("(?i)" + strings.Join(alts, "|"))
	if err != nil {
		return nil, errNoRE2
	}
	return re, nil
}

// breAtom is one translated atom and whether a repetition applies to it
// already (Go refuses a repetition of a repetition: it is grouped first).
type breAtom struct {
	re       string
	repeated bool
}

// breToRE2 translates one GNU basic regular expression.
func breToRE2(p string) (string, error) {
	type frame struct {
		alts  []string  // finished alternatives of the group
		atoms []breAtom // the current alternative
	}
	stack := []*frame{{}}
	groups := 0 // closed groups, for back-references
	cur := func() *frame { return stack[len(stack)-1] }
	// start is the context where a repetition is a literal and ^ is an
	// anchor: the start of the pattern, after \( or \|; afterAnchor is
	// right after such a ^, where a repetition is still literal.
	start, afterAnchor := true, false
	push := func(re string) {
		f := cur()
		f.atoms = append(f.atoms, breAtom{re: re})
		start, afterAnchor = false, false
	}
	repeat := func(op string) {
		f := cur()
		a := &f.atoms[len(f.atoms)-1]
		if a.repeated {
			a.re = "(?:" + a.re + ")"
		}
		a.re += op
		a.repeated = true
	}
	join := func(f *frame) string {
		var b strings.Builder
		for _, a := range f.atoms {
			b.WriteString(a.re)
		}
		return strings.Join(append(f.alts, b.String()), "|")
	}

	for i := 0; i < len(p); {
		ch := p[i]
		switch {
		case ch == '\\':
			if i+1 >= len(p) {
				return "", &grepError{msgTrailingBS}
			}
			r, size := utf8.DecodeRuneInString(p[i+1:])
			next := i + 1 + size
			switch r {
			case '(':
				stack = append(stack, &frame{})
				start, afterAnchor = true, false
			case ')':
				if len(stack) == 1 {
					return "", &grepError{msgUnmatchedClose}
				}
				f := cur()
				stack = stack[:len(stack)-1]
				groups++
				push("(" + join(f) + ")")
			case '|':
				f := cur()
				var b strings.Builder
				for _, a := range f.atoms {
					b.WriteString(a.re)
				}
				f.alts = append(f.alts, b.String())
				f.atoms = nil
				start, afterAnchor = true, false
			case '{':
				if start || afterAnchor {
					push(`\{`)
					break
				}
				lo, hi, end, err := breInterval(p, next)
				if err != nil {
					return "", err
				}
				if lo > re2DupMax || hi > re2DupMax {
					return "", errNoRE2
				}
				switch {
				case hi < 0:
					repeat("{" + strconv.Itoa(lo) + ",}")
				case lo == hi:
					repeat("{" + strconv.Itoa(lo) + "}")
				default:
					repeat("{" + strconv.Itoa(lo) + "," + strconv.Itoa(hi) + "}")
				}
				next = end
			case '+', '?':
				if start || afterAnchor {
					push(regexp.QuoteMeta(string(r)))
					break
				}
				repeat(string(r))
			case '1', '2', '3', '4', '5', '6', '7', '8', '9':
				if int(r-'0') > groups {
					return "", &grepError{msgBadBackref}
				}
				return "", errNoRE2
			case '<', '>':
				return "", errNoRE2
			case 'w':
				push(`[\p{L}\p{N}_]`)
			case 'W':
				push(`[^\p{L}\p{N}_]`)
			case 's':
				push("[" + spaceClass + "]")
			case 'S':
				push("[^" + spaceClass + "]")
			case 'b':
				push(`\b`)
			case 'B':
				push(`\B`)
			case '`':
				push(`\A`)
			case '\'':
				push(`\z`)
			default:
				push(literal(p[i+1 : next]))
			}
			i = next
		case ch == '[':
			re, end, err := breBracket(p, i)
			if err != nil {
				return "", err
			}
			push(re)
			i = end
		case ch == '*':
			if start || afterAnchor {
				push(`\*`)
			} else {
				repeat("*")
			}
			i++
		case ch == '^':
			if start {
				f := cur()
				f.atoms = append(f.atoms, breAtom{re: "^"})
				start, afterAnchor = false, true
			} else {
				push(`\^`)
			}
			i++
		case ch == '$':
			rest := p[i+1:]
			if rest == "" || strings.HasPrefix(rest, `\)`) || strings.HasPrefix(rest, `\|`) {
				f := cur()
				f.atoms = append(f.atoms, breAtom{re: "$"})
				start, afterAnchor = false, false
			} else {
				push(`\$`)
			}
			i++
		case ch == '.':
			push(".")
			i++
		default:
			_, size := utf8.DecodeRuneInString(p[i:])
			push(literal(p[i : i+size]))
			i += size
		}
	}
	if len(stack) > 1 {
		return "", &grepError{msgUnmatchedParen}
	}
	return join(stack[0]), nil
}

// literal is the Go regexp of one character; a byte that is not UTF-8
// matches the replacement character Go reads in its place.
func literal(s string) string {
	if !utf8.ValidString(s) {
		return `\x{FFFD}`
	}
	return regexp.QuoteMeta(s)
}

// breInterval reads the bounds of \{m,n\} from p[i:] (after `\{`): lo,
// hi (-1 when open) and the index after `\}`.
func breInterval(p string, i int) (lo, hi, end int, err error) {
	readNum := func() (int, bool) {
		j := i
		for i < len(p) && p[i] >= '0' && p[i] <= '9' {
			i++
		}
		if j == i {
			return 0, false
		}
		n, e := strconv.Atoi(p[j:i])
		if e != nil || n > breDupMax {
			n = breDupMax + 1
		}
		return n, true
	}
	lo, hasLo := readNum()
	hi = lo
	if i < len(p) && p[i] == ',' {
		i++
		var hasHi bool
		hi, hasHi = readNum()
		if !hasHi {
			hi = -1
		}
		if !hasLo {
			lo = 0
		}
	} else if !hasLo {
		hi = -2 // no bounds at all
	}
	switch {
	case i >= len(p) || (p[i] == '\\' && i+1 >= len(p)):
		return 0, 0, 0, &grepError{msgUnmatchedBrace}
	case p[i] != '\\' || p[i+1] != '}':
		if !strings.Contains(p[i:], `\}`) {
			return 0, 0, 0, &grepError{msgUnmatchedBrace}
		}
		return 0, 0, 0, &grepError{msgBadBrace}
	case hi == -2:
		return 0, 0, 0, &grepError{msgBadBrace}
	case lo > breDupMax || hi > breDupMax:
		return 0, 0, 0, &grepError{msgTooBig}
	case hi >= 0 && hi < lo:
		return 0, 0, 0, &grepError{msgBadBrace}
	}
	return lo, hi, i + 2, nil
}

// spaceClass is [[:space:]] of a glibc UTF-8 locale (iswspace).
const spaceClass = `\t-\r \x{1680}\x{2000}-\x{2006}\x{2008}-\x{200A}\x{2028}\x{2029}\x{205F}\x{3000}`

// breClasses are the POSIX classes of a glibc UTF-8 locale, as Go class
// contents.
var breClasses = map[string]string{
	"alpha":  `\p{L}`,
	"upper":  `\p{Lu}`,
	"lower":  `\p{Ll}`,
	"digit":  `0-9`,
	"xdigit": `0-9A-Fa-f`,
	"alnum":  `\p{L}\p{N}`,
	"space":  spaceClass,
	"blank":  `\t \x{1680}\x{2000}-\x{2006}\x{2008}-\x{200A}\x{205F}\x{3000}`,
	"punct":  `\p{P}\p{S}`,
	"print":  `\P{C}`,
	"graph":  `\p{L}\p{M}\p{N}\p{P}\p{S}`,
	"cntrl":  `\p{Cc}`,
}

// breBracket translates the bracket expression at p[i] ('['): the Go
// class and the index after its closing ']'. Range ends are compared
// after the upper-casing of grep -i (glibc's case-folded matching), in
// code point order.
func breBracket(p string, i int) (string, int, error) {
	j := i + 1
	neg := false
	if j < len(p) && p[j] == '^' {
		neg = true
		j++
	}
	body := j
	var parts []string
	special := false // a class, an equivalence class, a symbol or a range
	// element reads one element at p[j]: a character (or a collating
	// symbol [.c.]), or a class or equivalence class (cls set).
	element := func() (r rune, cls string, err error) {
		if p[j] == '[' && j+1 < len(p) && (p[j+1] == ':' || p[j+1] == '=' || p[j+1] == '.') {
			kind := p[j+1]
			end := strings.Index(p[j+2:], string(kind)+"]")
			if end < 0 {
				return 0, "", &grepError{msgUnmatchedBracket}
			}
			name := p[j+2 : j+2+end]
			j += 2 + end + 2
			special = true
			if kind == ':' {
				c, ok := breClasses[name]
				if !ok {
					return 0, "", &grepError{msgBadClass}
				}
				return 0, c, nil
			}
			rr, size := utf8.DecodeRuneInString(name)
			if name == "" || size != len(name) {
				return 0, "", &grepError{msgBadCollation}
			}
			if kind == '=' {
				return 0, classChar(rr), nil
			}
			return rr, "", nil
		}
		rr, size := utf8.DecodeRuneInString(p[j:])
		j += size
		return rr, "", nil
	}
	for first := true; ; first = false {
		if j >= len(p) {
			return "", 0, &grepError{msgUnmatchedBracket}
		}
		if p[j] == ']' && !first {
			break
		}
		lo, cls, err := element()
		if err != nil {
			return "", 0, err
		}
		isRange := j+1 < len(p) && p[j] == '-' && p[j+1] != ']'
		if cls != "" {
			if isRange {
				return "", 0, &grepError{msgBadRange}
			}
			parts = append(parts, cls)
			continue
		}
		if !isRange {
			parts = append(parts, classChar(lo))
			continue
		}
		j++ // the '-'
		if j+1 < len(p) && p[j] == '[' && (p[j+1] == ':' || p[j+1] == '=') {
			return "", 0, &grepError{msgBadRange}
		}
		hi, _, err := element()
		if err != nil {
			return "", 0, err
		}
		special = true
		lo, hi = unicode.ToUpper(lo), unicode.ToUpper(hi)
		if hi < lo {
			return "", 0, &grepError{msgBadRange}
		}
		parts = append(parts, classChar(lo)+"-"+classChar(hi))
	}
	// GNU grep refuses [:alpha:] written without the outer brackets: a
	// bracket of plain characters that starts and ends with ':' and holds
	// another character.
	if content := p[body:j]; !special && len(content) >= 2 && content[0] == ':' &&
		content[len(content)-1] == ':' && strings.Trim(content, ":") != "" {
		return "", 0, &grepError{msgColonClass}
	}
	cls := strings.Join(parts, "")
	if neg {
		return "[^" + cls + "]", j + 1, nil
	}
	return "[" + cls + "]", j + 1, nil
}

// classChar is one character inside a Go class.
func classChar(r rune) string {
	if r == utf8.RuneError {
		return `\x{FFFD}`
	}
	return `\x{` + strconv.FormatInt(int64(r), 16) + `}`
}
