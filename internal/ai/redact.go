// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"regexp"
	"strings"
)

// Redaction is __ai_redact: the line-based awk PRIVATE KEY rule, then the
// eight 2.x sed -E rules in order (POSIX leftmost-longest matching, as sed
// -E does), then the 3.0 rules of LEDGER AI-07: Authorization headers,
// Gash's own SSH: passphrase lines and crypt password hashes. An unclosed
// PRIVATE KEY block drops the rest of the text it is given; the callers
// redact the piped part and the question separately (AI-06).

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
		// LEDGER AI-07 (3.0): bearer/basic/token Authorization headers, the
		// SSH:KEY=PASSPHRASE lines of ~/.gash_env, crypt(3) hashes.
		{regexp.MustCompile(`(?i)\b((?:proxy-)?authorization[ \t]*:[ \t]*(?:bearer|basic|token)[ \t]+)[^ \t]+`), `${1}[REDACTED]`},
		{regexp.MustCompile(`^(SSH:[^=]*=).*`), `${1}[REDACTED]`},
		{regexp.MustCompile(`\$(?:1|2[abxy]?|5|6|7|y|gy)\$[^:\s]+`), `[REDACTED HASH]`},
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
