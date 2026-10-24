// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package pathguard

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// readRepoDoc reads a document relative to the repository root.
func readRepoDoc(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", rel))
	require.NoError(t, err)
	return string(data)
}

// docSection is the text from the heading line to the next heading of the
// same or a higher level.
func docSection(t *testing.T, doc, heading string) string {
	t.Helper()
	i := strings.Index(doc, "\n"+heading+"\n")
	require.GreaterOrEqual(t, i, 0, "heading %q not found", heading)
	level := heading[:strings.IndexByte(heading, ' ')+1]
	rest := doc[i+1+len(heading):]
	re := regexp.MustCompile(`(?m)^#{1,` + string(rune('0'+len(level)-1)) + `} `)
	if loc := re.FindStringIndex(rest); loc != nil {
		rest = rest[:loc[0]]
	}
	return rest
}

var backticked = regexp.MustCompile("`([^`]+)`")

func codeSpans(s string) map[string]bool {
	out := map[string]bool{}
	for _, m := range backticked.FindAllStringSubmatch(s, -1) {
		out[m[1]] = true
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// TestDocsMatchTables (LEDGER LLM-32): AGENTS.md section 6 and
// docs/security.md list exactly the forbidden paths and the secret
// patterns, directories and exceptions of the tables in this package.
func TestDocsMatchTables(t *testing.T) {
	agents := readRepoDoc(t, "AGENTS.md")
	security := readRepoDoc(t, "docs/security.md")

	// 6.1 and the security.md section name every forbidden path.
	paths := docSection(t, agents, "### 6.1 Paths")
	forbidden := docSection(t, security, "## Forbidden paths")
	for _, f := range ForbiddenBasePaths {
		require.Contains(t, paths, "`"+f+"`", "AGENTS.md 6.1")
		require.Contains(t, forbidden, "`"+f+"`", "docs/security.md Forbidden paths")
	}

	// 6.2: the bulleted list is the tables, nothing more, nothing less.
	secret := docSection(t, agents, "### 6.2 Secret files")
	list := secret[:strings.Index(secret, "\n\n`llm_config`")]
	list = list[strings.Index(list, "\n* "):]
	want := map[string]bool{}
	for _, tab := range [][]string{SecretPatterns, SecretDirs, SecretExceptions} {
		for _, p := range tab {
			want[p] = true
		}
	}
	got := codeSpans(list)
	delete(got, "llm_grep")
	delete(got, "llm_find --contains")
	require.Equal(t, sortedKeys(want), sortedKeys(got), "AGENTS.md 6.2 list vs internal/pathguard tables")

	// docs/security.md: the Secret files table holds every entry too.
	table := docSection(t, security, "## Secret files")
	spans := codeSpans(table)
	for _, k := range sortedKeys(want) {
		require.True(t, spans[k], "docs/security.md Secret files lacks %q", k)
	}
}
