// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"regexp"
	"strings"
)

// Redaction is __ai_redact ported verbatim (LEDGER AI-06, AI-07
// reproduced): the line-based awk PRIVATE KEY rule, then the eight sed -E
// rules in order, on the whole user text. An unclosed PRIVATE KEY block
// swallows the rest of the text (AI-06); bearer headers, SSH: lines and
// crypt hashes are not caught (AI-07). The sed rules use POSIX
// leftmost-longest matching, as sed -E does.

var (
	reKeyBegin = regexp.MustCompile(`-+BEGIN [A-Z0-9 ]*PRIVATE KEY( BLOCK)?-+`)
	reKeyEnd   = regexp.MustCompile(`-+END [A-Z0-9 ]*PRIVATE KEY( BLOCK)?-+`)

	// The eight sed -E rules, in order. Rules 1-3 keep group 1; rules 4-8
	// replace the whole match.
	redactRules = []struct {
		re   *regexp.Regexp
		repl string
	}{
		{regexp.MustCompilePOSIX(`([A-Za-z][A-Za-z0-9+.-]*://[^/:@[:space:]]*):[^/@[:space:]]+@`), `$1:[REDACTED]@`},
		{regexp.MustCompilePOSIX(`(([Pp][Aa][Ss][Ss]([Ww][Oo][Rr][Dd]|[Ww][Dd])?|[Ss][Ee][Cc][Rr][Ee][Tt]|[Tt][Oo][Kk][Ee][Nn]|[Aa][Pp][Ii][_-]?[Kk][Ee][Yy]|[Aa][Cc][Cc][Ee][Ss][Ss][_-]?[Kk][Ee][Yy]|[Pp][Rr][Ii][Vv][Aa][Tt][Ee][_-]?[Kk][Ee][Yy]|[Cc][Rr][Ee][Dd][Ee][Nn][Tt][Ii][Aa][Ll][Ss]?)[A-Za-z0-9_.-]*["']?[[:space:]]*[=:][[:space:]]*)("[^"]*"|'[^']*'|[^[:space:],;]+)`), `$1[REDACTED]`},
		{regexp.MustCompilePOSIX(`(([Pp][Aa][Ss][Ss]([Ww][Oo][Rr][Dd]|[Ww][Dd])?|[Ss][Ee][Cc][Rr][Ee][Tt]|[Tt][Oo][Kk][Ee][Nn]|[Aa][Pp][Ii][_-]?[Kk][Ee][Yy]|[Aa][Cc][Cc][Ee][Ss][Ss][_-]?[Kk][Ee][Yy]|[Pp][Rr][Ii][Vv][Aa][Tt][Ee][_-]?[Kk][Ee][Yy]|[Cc][Rr][Ee][Dd][Ee][Nn][Tt][Ii][Aa][Ll][Ss]?)[[:space:]]+)[A-Za-z0-9_+/=.-]{16,}`), `$1[REDACTED]`},
		{regexp.MustCompilePOSIX(`AKIA[0-9A-Z]{16}`), `[REDACTED]`},
		{regexp.MustCompilePOSIX(`(gh[pousr]_|github_pat_)[A-Za-z0-9_]{20,}`), `[REDACTED]`},
		{regexp.MustCompilePOSIX(`(sk-ant-|sk-)[A-Za-z0-9_-]{20,}`), `[REDACTED]`},
		{regexp.MustCompilePOSIX(`xox[abposr]-[A-Za-z0-9-]{10,}`), `[REDACTED]`},
		{regexp.MustCompilePOSIX(`AIza[0-9A-Za-z_-]{35}`), `[REDACTED]`},
	}
)

// redact runs the awk block rule and the eight sed rules on text, as the
// 2.x pipeline `printf '%s\n' "$text" | awk ... | sed ...` did, captured in
// $(...) (trailing newlines stripped).
func redact(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	skip := false
	for _, line := range lines {
		switch {
		case !skip && reKeyBegin.MatchString(line):
			out = append(out, "[REDACTED PRIVATE KEY]")
			skip = true
		case skip:
			if reKeyEnd.MatchString(line) {
				skip = false
			}
			// Dropped either way (next in awk).
		default:
			for _, r := range redactRules {
				line = r.re.ReplaceAllString(line, r.repl)
			}
			out = append(out, line)
		}
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}
