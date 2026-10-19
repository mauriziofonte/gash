// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mauriziofonte/gash/internal/ui"
)

const initialAnalysisJSON = `{"hostname":"host1","platform":"Debian 12",` +
	`"sections":[{"title":"Services Detected","findings":[` +
	`{"severity":"ok","text":"nginx running"},{"severity":"warning","text":"redis stopped"}]}]}`

const drilldownReplyJSON = `{"title":"Answer","summary":"A short summary.",` +
	`"components":[{"name":"Redis","status":"running",` +
	`"config_highlights":["maxmemory 256mb"],` +
	`"issues":[{"severity":"warning","text":"Low memory"}]}]}`

// drilldownAPI makes the fake endpoint always answer with the drill-down
// reply (Claude shape).
func drilldownAPI(f *fakeAPI) {
	f.handle = func(_ int, _ []byte, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":`+
			jsonString(drilldownReplyJSON)+`}],"model":"claude-haiku-4-5","stop_reason":"end_turn"}`)
	}
}

// TestDrillMenu drives the drill-down loop through a scripted terminal: a
// free-text question, a save, then quit.
func TestDrillMenu(t *testing.T) {
	f := newFakeAPI(t)
	drilldownAPI(f)
	m := testModule(f)
	saveDir := t.TempDir()
	m.getwd = func() (string, error) { return saveDir, nil }
	m.now = func() time.Time { return time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC) }

	cb, _, _ := newCall(map[string]string{}, nil, false)
	sess := m.si.Begin(cb, false, false)
	defer sess.Release()

	in := strings.NewReader("?\nwhat is my memory usage\ns\nq\n")
	c, out, errb := newCall(map[string]string{}, in, true)
	s := ui.StyleOf(c.Term, false)
	m.drillMenu(c, s, sess, "claude", "test-token", initialAnalysisJSON)

	// One API call (the free-text question); the answer is formatted.
	if f.count() != 1 {
		t.Fatalf("want 1 API call, got %d", f.count())
	}
	o := out.String()
	for _, want := range []string{"Answer", "A short summary.", "Redis (running)", "maxmemory 256mb", "Low memory"} {
		if !strings.Contains(o, want) {
			t.Errorf("menu output missing %q\n---\n%s", want, o)
		}
	}
	// The menu and the "> " prompt were shown.
	if !strings.Contains(o, "Drill down for details:") {
		t.Errorf("menu heading missing")
	}
	if !strings.Contains(errb.String(), "> ") {
		t.Errorf("prompt missing from stderr: %q", errb.String())
	}

	// Save wrote the report (LEDGER AI-09) with the initial header and the
	// free-text question.
	saved := filepath.Join(saveDir, "gash-ai-sysinfo-20200102-030405.md")
	b, err := os.ReadFile(saved)
	if err != nil {
		t.Fatalf("report not saved: %v", err)
	}
	for _, want := range []string{"# System Analysis Report", "## Question: what is my memory usage", "### Redis"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("saved report missing %q\n---\n%s", want, b)
		}
	}
	if !strings.Contains(errb.String(), "Report saved to gash-ai-sysinfo-20200102-030405.md") {
		t.Errorf("no save confirmation: %q", errb.String())
	}
}

// TestDrillMenuInvalidAndEOF: an unknown choice warns and loops; EOF exits.
func TestDrillMenuInvalidAndEOF(t *testing.T) {
	f := newFakeAPI(t)
	drilldownAPI(f)
	m := testModule(f)
	cb, _, _ := newCall(map[string]string{}, nil, false)
	sess := m.si.Begin(cb, false, false)
	defer sess.Release()

	in := strings.NewReader("x\n") // one bad choice, then EOF
	c, _, errb := newCall(map[string]string{}, in, true)
	s := ui.StyleOf(c.Term, false)
	m.drillMenu(c, s, sess, "claude", "test-token", initialAnalysisJSON)

	if f.count() != 0 {
		t.Fatalf("no API call expected, got %d", f.count())
	}
	if !strings.Contains(errb.String(), "Invalid choice. Enter 1-6, ?, s, or q.") {
		t.Fatalf("no invalid-choice warning: %q", errb.String())
	}
}

// TestFormatDrilldown checks the component rendering (plain, no color).
func TestFormatDrilldown(t *testing.T) {
	c, out, errb := newCall(map[string]string{}, nil, false)
	s := ui.StyleOf(c.Term, false)
	formatDrilldown(s, printer(c), out, drilldownReplyJSON)
	o := out.String()
	for _, want := range []string{"Answer", "A short summary.", "Redis (running)", "  maxmemory 256mb", "Low memory"} {
		if !strings.Contains(o, want) {
			t.Errorf("missing %q\n%s", want, o)
		}
	}
	// Invalid JSON prints the error.
	errb.Reset()
	var bad bytes.Buffer
	formatDrilldown(s, ui.NewPrinter(&bad, c.Term, false), &bad, "{not json")
	if !strings.Contains(bad.String(), "Invalid JSON response from AI") {
		t.Fatalf("no invalid-JSON error: %q", bad.String())
	}
}

// TestExtractFindings pulls a section's findings as "[sev] text" lines.
func TestExtractFindings(t *testing.T) {
	got := extractFindings(initialAnalysisJSON, "Services Detected")
	want := "[ok] nginx running\n[warning] redis stopped"
	if got != want {
		t.Fatalf("extractFindings=%q want %q", got, want)
	}
	if extractFindings(initialAnalysisJSON, "No Such") != "" {
		t.Fatal("unknown section should yield empty findings")
	}
	if extractFindings("{bad", "Services Detected") != "" {
		t.Fatal("invalid JSON should yield empty findings")
	}
}

// TestMarkdownAccumulators checks the three Markdown builders.
func TestMarkdownAccumulators(t *testing.T) {
	m := testModule(newFakeAPI(t))
	m.now = func() time.Time { return time.Date(2021, 6, 7, 8, 9, 10, 0, time.UTC) }

	init := mdInitial(m, initialAnalysisJSON, "claude")
	for _, want := range []string{
		"# System Analysis Report",
		"**Generated:** 2021-06-07 08:09:10 | **Provider:** Claude",
		"## Server: host1",
		"*Debian 12*",
		"**Summary:** 0 critical, 1 warnings, 0 info, 1 ok",
		"### Services Detected",
		"- **OK:** nginx running",
		"- **Warning:** redis stopped",
	} {
		if !strings.Contains(init, want) {
			t.Errorf("mdInitial missing %q\n%s", want, init)
		}
	}

	dd := mdDrilldown(drilldownReplyJSON)
	for _, want := range []string{"\n---\n\n", "## Drill-down: Answer", "*A short summary.*", "### Redis (`running`)", "- maxmemory 256mb", "- **Warning:** Low memory"} {
		if !strings.Contains(dd, want) {
			t.Errorf("mdDrilldown missing %q\n%s", want, dd)
		}
	}

	ft := mdFreetext("why slow", drilldownReplyJSON)
	if !strings.Contains(ft, "## Question: why slow") || !strings.Contains(ft, "### Redis (`running`)") {
		t.Errorf("mdFreetext wrong:\n%s", ft)
	}
}

// TestLedger_AI10_md: a reply without a sections array leaves the summary
// counts empty (the 2.x jq produced nothing).
func TestLedger_AI10_md(t *testing.T) {
	m := testModule(newFakeAPI(t))
	m.now = func() time.Time { return time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC) }
	md := mdInitial(m, `{"hostname":"h","platform":"p"}`, "gemini")
	if !strings.Contains(md, "**Summary:**  critical,  warnings,  info,  ok") {
		t.Fatalf("AI-10 empty counts not reproduced:\n%s", md)
	}
}
