// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// sysinfoSections are the collectors ai_sysinfo runs into the dossier, in
// the 2.x order (no overview).
var sysinfoSections = []string{
	"identity", "storage", "services", "auth", "network",
	"security", "webstack", "mail", "infra", "system",
}

// aiSysinfo is ai_sysinfo [provider] [--raw] [--no-color]: collect the
// system dossier, ask the provider for an analysis and print it. --raw
// dumps the dossier with no API call. Help is the first argument only
// (LEDGER AI-24); --no-color is accepted though the help omits it (AI-25).
func (m *Module) aiSysinfo(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	p := printer(c)

	provider, raw, noColor := "", false, false
	for _, a := range c.Args {
		switch a {
		case "--raw":
			raw = true
		case "--no-color":
			noColor = true
		case "claude", "gemini":
			provider = a
		default:
			p.Error("Unknown argument: " + a)
			p.Usage("ai_sysinfo [claude|gemini] [--raw] [--no-color]")
			return 1
		}
	}
	// --raw implies plain text (no ANSI in the LLM-consumed dump).
	if raw {
		noColor = true
	}
	s := ui.StyleOf(c.Term, noColor)

	// Collect the dossier (sudo once for the whole run).
	sess := m.si.Begin(c, noColor, true)
	defer sess.Release()
	var dossier bytes.Buffer
	for _, name := range sysinfoSections {
		sess.Collect(&dossier, name)
	}
	data := dossier.String()

	if raw {
		_, _ = io.WriteString(c.Stdout, data)
		_, _ = io.WriteString(c.Stdout, "\n")
		return 0
	}

	provider, token, ok := m.resolveProvider(c, provider)
	if !ok {
		return 1
	}

	// One confirmation covers the report and its drill-downs.
	if !m.confirmSend(c, provider, "data",
		"system data from this host, and drill-down details on request", len(data)) {
		return 1
	}

	reply, rc := m.request(c, provider, token, sysinfoSystemPrompt,
		"Analyze this system enumeration:\n\n"+data, sysinfoResponseSchema,
		sysinfoMaxTokens, sysinfoResponseTimeout)
	if rc != 0 {
		return 1
	}
	formatAnalysis(s, printer(c), c.Stdout, reply)
	return 0
}

// siFinding and siSection decode the analysis schema.
type siFinding struct {
	Severity string `json:"severity"`
	Text     string `json:"text"`
}

type siSection struct {
	Title    string      `json:"title"`
	Findings []siFinding `json:"findings"`
}

// formatAnalysis is __sysinfo_format_ai_response: the server heading, the
// severity counts and each section's findings. A reply without a sections
// array keeps the empty counts of the 2.x defect (LEDGER AI-10), without
// the jq error lines (there is no jq).
func formatAnalysis(s ui.Style, p ui.Printer, w io.Writer, jsonReply string) {
	var top map[string]json.RawMessage
	if json.Unmarshal([]byte(jsonReply), &top) != nil {
		p.Error("Invalid JSON response from AI")
		return
	}
	s.Section(w, "Server: "+jqStr(top["hostname"], "unknown"))
	s.Print(w, "muted", jqStr(top["platform"], "unknown"))

	crit, warn, info, ok := severityCounts(top["sections"])
	e, wn, in, su, r := "", "", "", "", ""
	if s.Color {
		e, wn, in, su, r = s.P.Style("error"), s.P.Style("warning"), s.P.Style("info"), s.P.Style("success"), s.P.Off
	}
	fmt.Fprintf(w, "%s%s critical%s  %s%s warnings%s  %s%s info%s  %s%s ok%s\n",
		e, crit, r, wn, warn, r, in, info, r, su, ok, r)
	_, _ = io.WriteString(w, "\n")

	var sections []siSection
	_ = json.Unmarshal(top["sections"], &sections)
	for _, sec := range sections {
		s.Heading(w, sec.Title)
		printFindings(s, w, sec.Findings)
		_, _ = io.WriteString(w, "\n")
	}
}

// printFindings is __sysinfo_print_findings: a Mark line per finding,
// indented two.
func printFindings(s ui.Style, w io.Writer, findings []siFinding) {
	for _, f := range findings {
		s.Mark(w, "--indent", "2", severityStatus(f.Severity), f.Text)
	}
}

// severityStatus is __sysinfo_severity_status.
func severityStatus(sev string) string {
	switch sev {
	case "critical":
		return "error"
	case "warning":
		return "warning"
	case "ok":
		return "ok"
	}
	return "info"
}

// severityCounts counts findings by severity across the sections. When
// sections is absent or not an array, every count is "" (the 2.x empty
// jq output), reproducing the empty counts line of AI-10.
func severityCounts(sectionsRaw json.RawMessage) (crit, warn, info, ok string) {
	var sections []siSection
	if len(sectionsRaw) == 0 || json.Unmarshal(sectionsRaw, &sections) != nil {
		return "", "", "", ""
	}
	var c, wn, in, o int
	for _, sec := range sections {
		for _, f := range sec.Findings {
			switch f.Severity {
			case "critical":
				c++
			case "warning":
				wn++
			case "info":
				in++
			case "ok":
				o++
			}
		}
	}
	return strconv.Itoa(c), strconv.Itoa(wn), strconv.Itoa(in), strconv.Itoa(o)
}

// jqStr is `.KEY // DEFAULT` for a string field: the default only when the
// value is missing, null or false (jq's // keeps an empty string).
func jqStr(raw json.RawMessage, def string) string {
	if len(raw) == 0 {
		return def
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return def
}
