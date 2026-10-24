// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// Image interpolation with Compose's own rules (LEDGER DOCK-13), for the
// images the YAML-subset reader returns (compose v1, or a `config` that
// failed); images that came from `docker compose config` are already
// interpolated and are never interpolated again.
//
// The template grammar is compose-go's: $$ is a literal $, $VAR and
// ${VAR} the value (empty when unset), ${VAR:-D} D when VAR is unset or
// empty, ${VAR-D} D when unset, ${VAR:?E} and ${VAR?E} an error with E,
// ${VAR:+R} and ${VAR+R} R when set (and non-empty for :+), else empty.
// D, E and R are templates themselves. Values come from the caller's
// exported environment first, then DIR/.env (the environment wins, as in
// Compose); a variable the shell did not export is out of a binary's
// sight (DOCK-12).

// errTemplate is a malformed ${...}.
var errTemplate = errors.New("invalid interpolation format")

// interpolate expands the template s, looking each name up with get.
func interpolate(s string, get func(string) (string, bool)) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); {
		c := s[i]
		if c != '$' || i+1 == len(s) {
			b.WriteByte(c)
			i++
			continue
		}
		switch n := s[i+1]; {
		case n == '$':
			b.WriteByte('$')
			i += 2
		case n == '{':
			end := closingBrace(s, i+2)
			if end < 0 {
				return "", errTemplate
			}
			v, err := braced(s[i+2:end], get)
			if err != nil {
				return "", err
			}
			b.WriteString(v)
			i = end + 1
		case nameStart(n):
			j := i + 2
			for j < len(s) && nameChar(s[j]) {
				j++
			}
			v, _ := get(s[i+1 : j])
			b.WriteString(v)
			i = j
		default:
			b.WriteByte('$')
			i++
		}
	}
	return b.String(), nil
}

// closingBrace is the index of the '}' that closes a "${" whose body
// starts at from, nested ${...} skipped; -1 when there is none.
func closingBrace(s string, from int) int {
	depth := 0
	for i := from; i < len(s); i++ {
		switch {
		case s[i] == '$' && i+1 < len(s) && s[i+1] == '$':
			i++
		case s[i] == '$' && i+1 < len(s) && s[i+1] == '{':
			depth++
			i++
		case s[i] == '}':
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

// braced expands the body of one ${...}.
func braced(body string, get func(string) (string, bool)) (string, error) {
	j := 0
	for j < len(body) && nameChar(body[j]) {
		j++
	}
	name := body[:j]
	if name == "" || !nameStart(name[0]) {
		return "", errTemplate
	}
	rest := body[j:]
	val, set := get(name)
	if rest == "" {
		return val, nil
	}
	op, arg := rest[:1], rest[1:]
	if rest[0] == ':' {
		if len(rest) < 2 {
			return "", errTemplate
		}
		op, arg = rest[:2], rest[2:]
	}
	switch op {
	case ":-":
		if set && val != "" {
			return val, nil
		}
		return interpolate(arg, get)
	case "-":
		if set {
			return val, nil
		}
		return interpolate(arg, get)
	case ":?", "?":
		if set && (op == "?" || val != "") {
			return val, nil
		}
		msg, err := interpolate(arg, get)
		if err != nil {
			return "", err
		}
		return "", errors.New("required variable " + name + " is missing a value: " + msg)
	case ":+":
		if set && val != "" {
			return interpolate(arg, get)
		}
		return "", nil
	case "+":
		if set {
			return interpolate(arg, get)
		}
		return "", nil
	}
	return "", errTemplate
}

func nameStart(c byte) bool { return c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }
func nameChar(c byte) bool  { return nameStart(c) || '0' <= c && c <= '9' }

// dotenv reads DIR/.env with Compose's dotenv rules: blank lines and
// # comments skipped, an optional "export " prefix, KEY=VALUE with blanks
// around KEY and before VALUE ignored; a single-quoted value is literal, a
// double-quoted one takes the \n \r \t \\ \" escapes and interpolation,
// both may span lines; an unquoted value ends at a blank followed by #
// and is interpolated. Interpolation inside the file sees the earlier
// definitions, then the environment. The last definition wins. A quoted
// value that never closes is an error, as Compose refuses the whole file
// over it (F-09); the definitions before it are returned with it.
func dotenv(path string, env func(string) (string, bool)) (map[string]string, error) {
	vars := map[string]string{}
	if st, err := os.Stat(path); err != nil || !st.Mode().IsRegular() {
		return vars, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return vars, nil
	}
	get := func(n string) (string, bool) {
		if v, ok := vars[n]; ok {
			return v, true
		}
		return env(n)
	}
	all := strings.ReplaceAll(strings.ReplaceAll(string(data), "\x00", ""), "\r\n", "\n")
	text := all
	for text != "" {
		lineNo := strings.Count(all[:len(all)-len(text)], "\n") + 1
		var line string
		line, text = cutLine(text)
		line = strings.TrimLeft(line, " \t")
		if line == "" || line[0] == '#' {
			continue
		}
		if strings.HasPrefix(line, "export ") || strings.HasPrefix(line, "export\t") {
			line = strings.TrimLeft(line[len("export"):], " \t")
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimRight(line[:eq], " \t")
		if !envKey(key) {
			continue
		}
		raw := strings.TrimLeft(line[eq+1:], " \t")
		var val string
		switch {
		case raw != "" && raw[0] == '\'':
			body, rest, ok := quoted(raw[1:], text, '\'')
			if !ok {
				return vars, &dotenvError{path: path, line: lineNo, key: key}
			}
			val, text = body, rest
		case raw != "" && raw[0] == '"':
			body, rest, ok := quoted(raw[1:], text, '"')
			if !ok {
				return vars, &dotenvError{path: path, line: lineNo, key: key}
			}
			text = rest
			v, err := interpolate(unescape(body), get)
			if err != nil {
				v = unescape(body)
			}
			val = v
		default:
			for i := 1; i < len(raw); i++ {
				if raw[i] == '#' && (raw[i-1] == ' ' || raw[i-1] == '\t') {
					raw = raw[:i]
					break
				}
			}
			raw = strings.TrimRight(raw, " \t")
			v, err := interpolate(raw, get)
			if err != nil {
				v = raw
			}
			val = v
		}
		vars[key] = val
	}
	return vars, nil
}

// dotenvError is a quoted value of the .env file at path that opens on
// line and never closes.
type dotenvError struct {
	path string
	line int
	key  string
}

func (e *dotenvError) Error() string {
	return "line " + strconv.Itoa(e.line) + ": unterminated quoted value of " + e.key
}

// cutLine splits off the first line of s (without its newline).
func cutLine(s string) (string, string) {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

// quoted finds the closing quote q of a value that starts in first and
// may continue on the lines of rest: the body, what is left of the input
// after the line of the closing quote, and whether a closing quote came.
// A backslash escapes the next byte inside double quotes.
func quoted(first, rest string, q byte) (string, string, bool) {
	all := first
	if rest != "" {
		all = first + "\n" + rest
	}
	for i := 0; i < len(all); i++ {
		switch {
		case q == '"' && all[i] == '\\':
			i++
		case all[i] == q:
			_, after := cutLine(all[i+1:])
			return all[:i], after, true
		}
	}
	return "", rest, false
}

// unescape applies the escapes of a double-quoted dotenv value.
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '\\', '"':
			b.WriteByte(s[i])
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// envKey is a dotenv variable name: letters, digits, '_', '.' and '-'.
func envKey(k string) bool {
	if k == "" {
		return false
	}
	for i := 0; i < len(k); i++ {
		if c := k[i]; !nameChar(c) && c != '.' && c != '-' {
			return false
		}
	}
	return true
}
