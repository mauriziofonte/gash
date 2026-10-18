// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package render

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

// ParsePHPOverlay parses a global PHP overlay, php/<X.Y>.ini, which the
// user writes and the renderer never rewrites. The accepted subset of
// php.ini is: blank lines, ";" comments and "name = value" lines. Section
// headers, "#" comments, a name given twice and control characters are
// refused (invalid_state), never skipped: a pool must not silently differ
// from what the file says. The value is kept as written (trimmed), so it
// means in the pool what it means in php.ini. file names it in errors.
func ParsePHPOverlay(data []byte, file string) ([]INIValue, error) {
	text := string(data)
	if !utf8.ValidString(text) {
		return nil, overlayError(file, 0, "the file is not valid UTF-8")
	}
	var out []INIValue
	seen := map[string]int{}
	for i, line := range strings.Split(text, "\n") {
		n := i + 1
		if strings.ContainsFunc(line, func(r rune) bool { return unicode.IsControl(r) && r != '\t' }) {
			return nil, overlayError(file, n, "the line contains a control character")
		}
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, ";"):
			continue
		case strings.HasPrefix(line, "["):
			return nil, overlayError(file, n, "sections are not supported; write name = value lines only")
		case strings.HasPrefix(line, "#"):
			return nil, overlayError(file, n, "# comments are not supported; use ;")
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, overlayError(file, n, "the line is not name = value")
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if err := state.CheckINI(key, value); err != nil {
			e, _ := hteerr.As(err)
			return nil, overlayError(file, n, e.Details)
		}
		if prev, dup := seen[key]; dup {
			return nil, overlayError(file, n, key+" is already set on line "+strconv.Itoa(prev))
		}
		seen[key] = n
		out = append(out, INIValue{Key: key, Value: value})
	}
	return out, nil
}

func overlayError(file string, line int, msg string) error {
	where := file
	if line > 0 {
		where += ":" + strconv.Itoa(line)
	}
	return hteerr.Newf(hteerr.InvalidState, "fix the overlay; it is never rewritten by gash", "%s: %s", where, msg)
}
