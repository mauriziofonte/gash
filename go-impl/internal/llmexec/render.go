// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import "strings"

// singleQuote renders s as one single-quoted shell word, so bash performs no
// expansion on it (D-1 A: every validated word is single-quoted). An embedded
// single quote is closed, escaped and reopened in the usual POSIX way.
func singleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// renderCmd renders one simple command: the single-quoted words followed by
// the recorded redirection tokens, which stay unquoted so bash applies them.
func renderCmd(words, redirs []string) string {
	parts := make([]string, 0, len(words)+len(redirs))
	parts = append(parts, words...)
	parts = append(parts, redirs...)
	return strings.Join(parts, " ")
}
