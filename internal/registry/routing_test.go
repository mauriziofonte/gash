// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Plan S9: every agent function names the raw command shape it replaces,
// in its own entry or, for the functions that predate 3.4, in
// baseReplaces (never both).
func TestRouting_EveryAgentFunctionReplacesAShape(t *testing.T) {
	for _, f := range Functions() {
		if agentFamily(f.Name) {
			require.NotEmpty(t, f.Replaces, "%s has no Replaces (plan S9)", f.Name)
		}
	}
	for name := range baseReplaces {
		_, ok := Lookup(name)
		require.True(t, ok, "baseReplaces names %s, which is no function", name)
		for _, f := range baseFunctions {
			if f.Name == name {
				require.Empty(t, f.Replaces, "%s sets Replaces in its entry and in baseReplaces", name)
			}
		}
	}
}

// A routing row never names a flag the function's help does not document.
func TestRouting_FlagsAreDocumented(t *testing.T) {
	for _, r := range Routes() {
		f, _ := Lookup(r.Fn)
		for _, w := range strings.Fields(strings.TrimPrefix(r.Use, r.Fn)) {
			if strings.HasPrefix(w, "-") {
				flag, _, _ := strings.Cut(w, "=")
				require.True(t, regexp.MustCompile(`(^|[\s,\[(])`+regexp.QuoteMeta(flag)+`\b`).MatchString(f.Help.Body),
					"%s: routing row %q names %s, which its help does not document", r.Fn, r.Shape, flag)
			}
		}
		require.NotContains(t, r.Shape, " => ", r.Fn)
	}
}

// Plan S9 acceptance: the hint fits in an agent's instructions.
func TestRouting_HintIsSmall(t *testing.T) {
	require.Less(t, len(AgentsHint()), 6*1024)
	require.True(t, strings.HasSuffix(AgentsHint(), RoutingTable()))
}

func TestRouting_MarkdownEscapes(t *testing.T) {
	require.Equal(t, "`a \\| b`", mdCode("a | b"))
	require.Equal(t, "`` echo `x` ``", mdCode("echo `x`"))
}

// AGENTS.md section 7 holds the routing table; -update-docs rewrites it.
func TestRouting_AgentsMDHoldsTheTable(t *testing.T) {
	b, err := os.ReadFile("../../AGENTS.md")
	require.NoError(t, err)
	doc := string(b)
	start := strings.Index(doc, "\n## 7. ")
	end := strings.Index(doc, "\n## 8. ")
	require.True(t, start > 0 && end > start, "AGENTS.md has sections 7 and 8")
	section := doc[start:end]
	if *updateDocs {
		re := regexp.MustCompile(`(?m)^\| Instead of \| Use \|\n(\|.*\|\n)*`)
		if re.MatchString(section) {
			section = re.ReplaceAllLiteralString(section, RoutingTable())
		} else {
			section = strings.TrimRight(section, "\n") + "\n\n" + RoutingTable() + "\n"
		}
		doc = doc[:start] + section + doc[end:]
		require.NoError(t, os.WriteFile("../../AGENTS.md", []byte(doc), 0o644))
	}
	require.Contains(t, section, "\n"+RoutingTable(), "AGENTS.md section 7 is not the routing table (go test ./internal/registry -update-docs)")
}
