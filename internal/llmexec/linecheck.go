// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"regexp"
	"strings"

	"github.com/mauriziofonte/gash/internal/pathguard"
)

// pword is one parsed word: its literal value (quotes removed, no expansion),
// the glob pattern when the word is an unquoted glob, the unquoted-only
// accumulation uq (for brace detection and the leading-~ test), globby, and
// cont, the line continuations (backslash-newline) the parser dropped inside
// it.
type pword struct {
	lit    string
	glob   string
	uq     string
	globby bool
	cont   int
}

// item is one token of the current simple command in source order: a word
// (an index into words) or an allowed redirection (word < 0), with the gap
// before it.
type item struct {
	gap   string
	word  int
	redir string
}

// lineChecker holds the parser state, the environment, the tool probes and
// the output script. One instance validates and renders a single line.
type lineChecker struct {
	env   Environ
	pr    Probe
	guard *pathguard.Guard
	extra map[string]bool

	// current word being built
	word   string
	pat    string
	uq     string
	inword bool
	globby bool
	cont   int

	// current simple command being built
	words  []pword
	redirs []string
	items  []item

	// gap is the raw text since the last token, kept for the rendering:
	// blanks and line continuations only (comment text is dropped).
	gap string

	// output
	cwd    string
	script strings.Builder

	// captured for the re-parse property test: the validated (expanded,
	// unquoted) words and the redirections of each emitted simple command.
	cmdWords  [][]string
	cmdRedirs [][]string
}

func newLineChecker(env Environ, pr Probe, extra map[string]bool) *lineChecker {
	return &lineChecker{
		env:   env,
		pr:    pr,
		guard: pathguard.NewGuard(env.fs(), env.Cwd, env.Home, env.RootHome),
		extra: extra,
		cwd:   env.Cwd,
	}
}

func (lc *lineChecker) lookupUser(name string) (string, bool) {
	if lc.env.LookupUser != nil {
		return lc.env.LookupUser(name)
	}
	return lookupUserOS(name)
}

// redirRE is the 2.x redirection allowlist: 2>&1, >&2 / 1>&2, >/dev/null with
// an optional 1 or 2 and spaces, and &>/dev/null, each at a word boundary and
// followed by whitespace, ;, |, & or end of line.
var redirRE = regexp.MustCompile(`^(2>&1|1?>&2|[12]?>[[:space:]]*/dev/null|&>[[:space:]]*/dev/null)([[:space:];|&]|$)`)

// checkAndRender parses and checks the whole line, returning the canonical
// single-quoted script to run. It mirrors __llm_exec_check_line: each simple
// command is checked as soon as it is parsed, so refusals keep the 2.x
// left-to-right order.
//
// The script keeps the parsed token structure exactly: every operator
// (|, ||, &&, ; and newline) in place, including a dangling one at the end
// or around an empty command, every redirection in place (also in a
// command that has only redirections), and the blanks and line
// continuations between tokens. Only the words change (each is its
// validated value, single-quoted; a glob is its matches) and comments go.
// Bash therefore groups the commands as it grouped the raw line in 2.x, and a line that bash rejected with a syntax error (and
// status 2) is rejected the same way, on the same line, running no word of
// it. Adjacent operators stay adjacent, so ";;" is still the token bash
// refuses as ";;". A line continuation between the two characters of "||"
// or "&&" is dropped before the operator is read, as bash drops it: 2.x saw
// two pipes around an empty command where bash read "||" (3.0: EXEC-08
// fixed). The operator is rendered whole and the continuations after it,
// so later lines keep their line numbers.
func (lc *lineChecker) checkAndRender(cmd string) (string, error) {
	// A NUL byte cannot reach bash (an argument ends at it, and 2.x read
	// the line into a Bash variable, which cannot hold one): refuse it
	// instead of rendering a word bash would cut (3.1: found by
	// FuzzRenderRoundTrip).
	if strings.IndexByte(cmd, 0) >= 0 {
		return "", refuse("construct_not_allowed", "NUL byte", "RETRY",
			"The command holds a NUL byte; write it without one")
	}
	n := len(cmd)
	trailingBackslash := false
	for i := 0; i < n; i++ {
		c := cmd[i]

		if !lc.inword && (c == '1' || c == '2' || c == '>' || c == '&') {
			if m := redirRE.FindStringSubmatch(cmd[i:]); m != nil {
				lc.redirs = append(lc.redirs, m[1])
				lc.items = append(lc.items, item{gap: lc.gap, word: -1, redir: m[1]})
				lc.gap = ""
				i += len(m[1]) - 1
				continue
			}
		}

		switch c {
		case ' ', '\t':
			if err := lc.flushWord(); err != nil {
				return "", err
			}
			lc.gap += string(c)
		case '\n':
			if err := lc.endSimple("\n"); err != nil {
				return "", err
			}
		case ';':
			if err := lc.endSimple(";"); err != nil {
				return "", err
			}
		case '#':
			if lc.inword {
				lc.word += "#"
				lc.pat += "#"
				lc.uq += "#"
			} else {
				for i+1 < n && cmd[i+1] != '\n' {
					i++
				}
			}
		case '\'':
			lc.inword = true
			j := i + 1
			for j < n && cmd[j] != '\'' {
				ch := cmd[j]
				lc.word += string(ch)
				lc.appendPatEscaped(ch)
				j++
			}
			if j >= n {
				return "", refuse("unterminated_quote", "'", "RETRY", "Close every quote")
			}
			i = j
		case '"':
			lc.inword = true
			j := i + 1
			for ; j < n; j++ {
				ch := cmd[j]
				if ch == '"' {
					break
				} else if ch == '$' || ch == '`' {
					return "", refuse("construct_not_allowed", string(ch)+" inside double quotes", "RETRY",
						"Expansions are not allowed; use single quotes for literal text")
				} else if ch == '\\' && j+1 < n && isDquoteEscapable(cmd[j+1]) {
					j++
					ch = cmd[j]
					if ch == '\n' {
						lc.cont++
						continue
					}
				}
				lc.word += string(ch)
				lc.appendPatEscaped(ch)
			}
			if j >= n {
				return "", refuse("unterminated_quote", `"`, "RETRY", "Close every quote")
			}
			i = j
		case '\\':
			i++
			if i >= n {
				// A backslash at the end of the input: refused below,
				// once the rest of the line has passed (EXEC-07).
				trailingBackslash = true
				continue
			}
			ch := cmd[i]
			if ch == '\n' {
				// A line continuation: inside a word it is counted on the
				// word, between tokens it stays in the gap.
				if lc.inword {
					lc.cont++
				} else {
					lc.gap += "\\\n"
				}
				continue
			}
			lc.inword = true
			lc.word += string(ch)
			lc.appendPatEscaped(ch)
		case '$', '`':
			return "", refuse("construct_not_allowed", string(c), "RETRY",
				"Variable expansion and command substitution are not allowed; write values literally")
		case '|':
			// The next character is read past line continuations, as bash
			// joins them before reading the operator (3.0: EXEC-08 fixed).
			j, conts := skipConts(cmd, i+1)
			if j < n && cmd[j] == '&' {
				return "", refuse("construct_not_allowed", "|&", "RETRY", "Use 2>&1 |")
			}
			if j < n && cmd[j] == '|' {
				i = j
				if err := lc.endSimple("||"); err != nil {
					return "", err
				}
				lc.gap = conts
			} else if err := lc.endSimple("|"); err != nil {
				return "", err
			}
		case '&':
			if j, conts := skipConts(cmd, i+1); j < n && cmd[j] == '&' {
				i = j
				if err := lc.endSimple("&&"); err != nil {
					return "", err
				}
				lc.gap = conts
			} else {
				return "", refuse("construct_not_allowed", "&", "RETRY", "Background jobs are not allowed")
			}
		case '<', '>':
			return "", refuse("construct_not_allowed", string(c), "RETRY",
				"Redirections are not allowed (except to /dev/null, 2>&1 and >&2); llm_exec cannot write files")
		case '(', ')':
			return "", refuse("construct_not_allowed", string(c), "RETRY",
				"Subshells and grouping are not allowed; join plain commands with |, &&, || or ;")
		case '*', '?', '[':
			lc.inword = true
			lc.globby = true
			lc.word += string(c)
			lc.pat += string(c)
			lc.uq += string(c)
		default:
			lc.inword = true
			lc.word += string(c)
			lc.pat += string(c)
			lc.uq += string(c)
		}
	}
	if err := lc.endSimple(""); err != nil {
		return "", err
	}
	// EXEC-07 (forced divergence): 2.x dropped a backslash at the end of the
	// input, but bash keeps it as a literal character, so bash ran a word the
	// checks never saw. The line is refused instead. The refusal comes after
	// the rest of the line passed, so a line 2.x refused for another reason
	// keeps that refusal. A backslash-newline stays a continuation, an
	// escaped backslash (two of them) is a word character, and a backslash in
	// a comment or inside quotes is not affected.
	if trailingBackslash {
		return "", refuse("construct_not_allowed", `\`, "RETRY",
			"Remove the trailing backslash or quote it")
	}
	lc.script.WriteString(lc.gap)
	return lc.script.String(), nil
}

// skipConts returns the index of the first character at or after j that
// is not part of a line continuation (backslash-newline), and the
// continuations it skipped.
func skipConts(cmd string, j int) (int, string) {
	k := j
	for k+1 < len(cmd) && cmd[k] == '\\' && cmd[k+1] == '\n' {
		k += 2
	}
	return k, cmd[j:k]
}

func isDquoteEscapable(c byte) bool {
	return c == '$' || c == '`' || c == '"' || c == '\\' || c == '\n'
}

// appendPatEscaped appends ch to pat, prefixing a backslash before a glob
// metacharacter so the pattern treats a quoted/escaped special char as
// literal (2.x: `[[ "$c" == [\*\?\[\]\\] ]] && pat+="\\"`).
func (lc *lineChecker) appendPatEscaped(ch byte) {
	switch ch {
	case '*', '?', '[', ']', '\\':
		lc.pat += "\\"
	}
	lc.pat += string(ch)
}

// flushWord is __llm_exec_flush_word: close the current word into words,
// refusing brace expansion and expanding a leading ~/ in the glob pattern.
func (lc *lineChecker) flushWord() error {
	if lc.inword {
		if strings.Contains(lc.uq, "{") && (strings.Contains(lc.uq, ",") || strings.Contains(lc.uq, "..")) {
			return refuse("construct_not_allowed", lc.word, "RETRY",
				"Brace expansion is not allowed; list the words explicitly or quote them")
		}
		pat := lc.pat
		if strings.HasPrefix(pat, "~/") {
			pat = lc.env.Home + "/" + pat[len("~/"):]
		}
		w := pword{lit: lc.word, uq: lc.uq, globby: lc.globby, cont: lc.cont}
		if lc.globby {
			w.glob = pat
		}
		lc.words = append(lc.words, w)
		lc.items = append(lc.items, item{gap: lc.gap, word: len(lc.words) - 1})
		lc.gap = ""
	}
	lc.word, lc.pat, lc.uq, lc.inword, lc.globby, lc.cont = "", "", "", false, false, 0
	return nil
}

// endSimple is __llm_exec_end_simple: flush the current word, check the
// simple command when it has words (a command with no words, only
// redirections or nothing, is not checked, as in 2.x), advance the tracked
// cwd, and render it followed by sep, the operator that ended it ("" at the
// end of the line).
func (lc *lineChecker) endSimple(sep string) error {
	if err := lc.flushWord(); err != nil {
		return err
	}
	var perWord [][]string
	if len(lc.words) > 0 {
		var err error
		perWord, err = lc.checkSimple(lc.words)
		if err != nil {
			return err
		}
		lc.cwd = lc.updateCwd(lc.words, lc.cwd)
	}
	for _, it := range lc.items {
		lc.script.WriteString(it.gap)
		if it.word < 0 {
			lc.script.WriteString(it.redir)
			continue
		}
		lc.script.WriteString(renderWords(perWord[it.word]))
		// Continuations dropped inside the word go right after it, so
		// later lines keep their line numbers.
		lc.script.WriteString(strings.Repeat("\\\n", lc.words[it.word].cont))
	}
	if sep != "" {
		lc.script.WriteString(lc.gap)
		lc.script.WriteString(sep)
		lc.gap = ""
	}
	lc.words = nil
	lc.redirs = nil
	lc.items = nil
	return nil
}
