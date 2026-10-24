// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// quoteShape is `"Text." — Author`, the format CONTRIBUTING.md asks for.
var quoteShape = regexp.MustCompile(`^".+" — [^ ].*[^ ]$`)

// The shipped quotes/list.txt keeps the rules of CONTRIBUTING.md: every
// quote has a "# " source line right above it, the "Text." — Author shape,
// and no duplicate text (case-insensitive). Ported from the 2.x spec
// "Quotes file (quotes/list.txt)", which went with tests/specs.
func TestShippedQuotes(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "quotes", "list.txt"))
	require.NoError(t, err)
	defer f.Close()
	seen := map[string]bool{}
	prev, n := "", 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line != "" && !strings.HasPrefix(line, "#") {
			n++
			if len(prev) < 3 || !strings.HasPrefix(prev, "# ") || strings.HasPrefix(prev, "# Gash") {
				t.Errorf("no source line above: %s", line)
			}
			if !quoteShape.MatchString(line) {
				t.Errorf(`not "Text." — Author: %s`, line)
			}
			key, _, _ := strings.Cut(line, `" — `)
			key = strings.ToLower(key)
			if seen[key] {
				t.Errorf("duplicate: %s", line)
			}
			seen[key] = true
		}
		prev = line
	}
	require.NoError(t, sc.Err())
	require.GreaterOrEqual(t, n, 300, "quotes in the shipped list")
}
