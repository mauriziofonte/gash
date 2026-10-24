// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

import (
	"flag"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// updateDocs rewrites the reference pages this test checks:
//
//	go test ./internal/registry -run TestReferenceDocs -update-docs
var updateDocs = flag.Bool("update-docs", false, "rewrite docs/reference pages from the help registry")

// docTopics are the reference pages rendered from the registry and checked
// here (LEDGER LLM-30, LLM-31: docs/reference/llm.md is the registry help).
var docTopics = []string{"llm"}

// renderTopicDoc renders docs/reference/<topic>.md from the registered help:
// a table of the topic's functions, then per function its short
// description and each help section (USAGE, OPTIONS, EXAMPLES, NOTES, ...)
// in body order, dedented, then its See also line.
func renderTopicDoc(t *testing.T, topic string) string {
	t.Helper()
	var desc string
	for _, tp := range Topics() {
		if tp.Name == topic {
			desc = tp.Description
		}
	}
	require.NotEmpty(t, desc, "topic %q", topic)
	var fns []Function
	for _, f := range Functions() {
		if f.Topic == topic && f.HasHelp {
			fns = append(fns, f)
		}
	}
	sort.Slice(fns, func(i, j int) bool { return fns[i].Name < fns[j].Name })

	var b strings.Builder
	b.WriteString("# " + topic + "\n\n" + desc + ".\n\n")
	b.WriteString("_Generated from the help registry by `go test ./internal/registry -run TestReferenceDocs -update-docs`; do not edit by hand._\n\n")
	b.WriteString("| Function | Alias | Description |\n|---|---|---|\n")
	for _, f := range fns {
		alias := ""
		if len(f.Aliases) > 0 {
			alias = "`" + strings.Join(f.Aliases, "`, `") + "`"
		}
		b.WriteString("| [`" + f.Name + "`](#" + strings.ReplaceAll(f.Name, "_", "-") + ") | " + alias + " | " + f.Help.Short + " |\n")
	}
	for _, f := range fns {
		b.WriteString("\n## " + f.Name + "\n\n" + f.Help.Short + "\n")
		for _, sec := range helpSections(f.Help.Body) {
			fence := "text"
			if sec.title == "EXAMPLES" {
				fence = "bash"
			}
			b.WriteString("\n#### " + titleCase(sec.title) + "\n\n```" + fence + "\n" + sec.text + "\n```\n")
		}
		if f.Help.SeeAlso != "" {
			b.WriteString("\nSee also: `" + strings.Join(strings.Fields(f.Help.SeeAlso), "`, `") + "`\n")
		}
	}
	return b.String()
}

type helpSection struct{ title, text string }

// helpSections splits a help body at its unindented upper-case headings;
// the section text loses its two-space indent and its trailing blank lines.
func helpSections(body string) []helpSection {
	var out []helpSection
	for _, line := range strings.Split(body, "\n") {
		if line != "" && line == strings.ToUpper(line) && !strings.HasPrefix(line, " ") {
			out = append(out, helpSection{title: line})
			continue
		}
		if len(out) == 0 {
			continue
		}
		s := &out[len(out)-1]
		if s.text != "" || line != "" {
			s.text += strings.TrimPrefix(line, "  ") + "\n"
		}
	}
	for i := range out {
		out[i].text = strings.TrimRight(out[i].text, "\n")
	}
	return out
}

func titleCase(s string) string {
	s = strings.ToLower(s)
	return strings.ToUpper(s[:1]) + s[1:]
}

// TestReferenceDocs: each page in docTopics is exactly what the registry
// help renders, so a flag or note added to a help body reaches the page.
func TestReferenceDocs(t *testing.T) {
	for _, topic := range docTopics {
		path := "../../docs/reference/" + topic + ".md"
		want := renderTopicDoc(t, topic)
		if *updateDocs {
			require.NoError(t, os.WriteFile(path, []byte(want), 0o644))
			continue
		}
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, want, string(got), "docs/reference/%s.md is stale: run go test ./internal/registry -run TestReferenceDocs -update-docs", topic)
	}
}
