// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import "strings"

// singleQuote renders s as one single-quoted shell word, so bash performs no
// expansion on it (D-1 A: every validated word is single-quoted). An embedded
// single quote is closed, escaped and reopened in the usual POSIX way.
func singleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// renderWords renders the validated words of one parsed word (one word, or
// the matches of a glob) as single-quoted words separated by one space.
func renderWords(words []string) string {
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = singleQuote(w)
	}
	return strings.Join(parts, " ")
}
