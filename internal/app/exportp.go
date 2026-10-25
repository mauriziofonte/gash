// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package app

import (
	"errors"
	"strconv"
	"strings"
)

// decl is one declaration as `declare -p` or `export -p` prints it.
type decl struct {
	name, flags, value string
	set                bool // has a value (declare -x NAME alone has none)
}

// scalar reports a variable that is not an array (an array never reaches
// a program's environment).
func (d decl) scalar() bool { return !strings.ContainsAny(d.flags, "aA") }

func (d decl) exported() bool { return strings.ContainsRune(d.flags, 'x') }

// parseDecls reads what `declare -p NAMES` or `export -p` prints, in Bash
// or in POSIX mode. Array values are returned undecoded.
func parseDecls(text string) ([]decl, error) {
	var out []decl
	s := text
	for len(s) > 0 {
		if s[0] == '\n' {
			s = s[1:]
			continue
		}
		var word string
		if word, s = cutWord(s); word != "declare" && word != "export" {
			return nil, errors.New("declare -p: unexpected " + strconv.Quote(word))
		}
		d := decl{}
		if strings.HasPrefix(s, "-") {
			d.flags, s = cutWord(s)
		}
		if word == "export" {
			d.flags += "x"
		}
		i := 0
		for i < len(s) && s[i] != '=' && s[i] != '\n' && s[i] != ' ' {
			i++
		}
		d.name, s = s[:i], s[i:]
		if d.name == "" {
			return nil, errors.New("declare -p: missing name")
		}
		if !strings.HasPrefix(s, "=") {
			out = append(out, d)
			s = skipLine(s)
			continue
		}
		var ok bool
		d.value, s, ok = cutValue(s[1:])
		if !ok {
			return nil, errors.New("declare -p: bad value of " + d.name)
		}
		d.set = true
		out = append(out, d)
	}
	return out, nil
}

// parseExportP is the environment a program the shell runs gets, from
// what `export -p` prints: "NAME=value" entries of the exported scalar
// variables that have a value.
func parseExportP(text string) ([]string, error) {
	decls, err := parseDecls(text)
	if err != nil {
		return nil, err
	}
	var env []string
	for _, d := range decls {
		if d.set && d.scalar() && d.exported() {
			env = append(env, d.name+"="+d.value)
		}
	}
	return env, nil
}

// cutWord returns the text up to the next space and the rest after it.
func cutWord(s string) (string, string) {
	i := strings.IndexAny(s, " \n")
	if i < 0 {
		return s, ""
	}
	if s[i] == ' ' {
		return s[:i], s[i+1:]
	}
	return s[:i], s[i:]
}

func skipLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return ""
}

// cutValue decodes one shell word, the value of an assignment, up to the
// unquoted newline that ends it: "..." with \ before $ ` " \ and newline,
// $'...' with the ANSI-C escapes, '...', bare text, or an array's
// (...) list, returned undecoded.
func cutValue(s string) (value, rest string, ok bool) {
	var b strings.Builder
	depth := 0
	for len(s) > 0 {
		c := s[0]
		switch {
		case c == '\n' && depth == 0:
			return b.String(), s[1:], true
		case c == '"':
			end := 1
			for ; end < len(s) && s[end] != '"'; end++ {
				if s[end] == '\\' && end+1 < len(s) {
					switch s[end+1] {
					case '$', '`', '"', '\\':
						b.WriteByte(s[end+1])
						end++
						continue
					case '\n':
						end++
						continue
					}
				}
				b.WriteByte(s[end])
			}
			if end >= len(s) {
				return "", "", false
			}
			s = s[end+1:]
		case c == '$' && len(s) > 1 && s[1] == '\'':
			n, used, good := ansiC(s[2:])
			if !good {
				return "", "", false
			}
			b.WriteString(n)
			s = s[2+used:]
		case c == '\'':
			end := strings.IndexByte(s[1:], '\'')
			if end < 0 {
				return "", "", false
			}
			b.WriteString(s[1 : 1+end])
			s = s[end+2:]
		case c == '\\' && len(s) > 1:
			if s[1] != '\n' {
				b.WriteByte(s[1])
			}
			s = s[2:]
		default:
			if c == '(' {
				depth++
			} else if c == ')' && depth > 0 {
				depth--
			}
			b.WriteByte(c)
			s = s[1:]
		}
	}
	return b.String(), "", depth == 0
}

// ansiC decodes the body of $'...' up to its closing quote: the value and
// the bytes used, the quote included.
func ansiC(s string) (string, int, bool) {
	var b strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '\'' {
			return b.String(), i + 1, true
		}
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			i++
			continue
		}
		e := s[i+1]
		i += 2
		switch e {
		case 'a':
			b.WriteByte('\a')
		case 'b':
			b.WriteByte('\b')
		case 'e', 'E':
			b.WriteByte(0x1b)
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'v':
			b.WriteByte('\v')
		case '\\', '\'', '"', '?':
			b.WriteByte(e)
		case '0', '1', '2', '3', '4', '5', '6', '7':
			v, n := int(e-'0'), 1
			for n < 3 && i < len(s) && s[i] >= '0' && s[i] <= '7' {
				v = v*8 + int(s[i]-'0')
				i++
				n++
			}
			b.WriteByte(byte(v))
		case 'x', 'u', 'U':
			limit := map[byte]int{'x': 2, 'u': 4, 'U': 8}[e]
			v, n := 0, 0
			for n < limit && i < len(s) && isHex(s[i]) {
				v = v*16 + hexVal(s[i])
				i++
				n++
			}
			switch {
			case n == 0:
				b.WriteByte('\\')
				b.WriteByte(e)
			case e == 'x':
				b.WriteByte(byte(v))
			default:
				b.WriteString(string(rune(v)))
			}
		case 'c':
			if i < len(s) {
				b.WriteByte(s[i] & 0x1f)
				i++
			}
		default:
			b.WriteByte('\\')
			b.WriteByte(e)
		}
	}
	return "", 0, false
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	}
	return int(c-'A') + 10
}

// execShellLevel is the SHLVL Bash 4.4's exec gives the program from the
// shell's: an integer (blanks around it allowed) less one, at least 0; 0
// for anything else; 1 past 999.
func execShellLevel(v string) string {
	n, err := strconv.ParseInt(strings.Trim(v, " \t\n"), 10, 64)
	switch {
	case err != nil || n < 1:
		n = 0
	case n > 1000:
		n = 1
	default:
		n--
	}
	return strconv.FormatInt(n, 10)
}
