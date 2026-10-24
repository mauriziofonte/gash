// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/sysinfo"
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
// dumps the dossier with no API call. -h/--help is help anywhere (LEDGER
// AI-24).
func (m *Module) aiSysinfo(c *invoke.Call) int {
	for _, a := range c.Args {
		if (a == "-h" || a == "--help") && help.NeedsOf(c, a) {
			return 0
		}
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

	// Interactive drill-down loop (terminal stdin only, `[[ ! -t 0 ]]`).
	if !c.Term.StdinTTY() {
		return 0
	}
	m.drillMenu(c, s, sess, provider, token, reply)
	return 0
}

// drillMenu is the ai_sysinfo drill-down loop: a menu that drills into one
// of the six sections (Session.Deep plus a drill-down API call), answers a
// free-text question with targeted context, saves the running Markdown
// report (LEDGER AI-09, write unchecked) or quits. The Markdown buffer is
// seeded with the initial analysis and grows with each drill-down and
// question, as 2.x captured each chunk in `$(...)` (trailing newlines
// dropped on append).
func (m *Module) drillMenu(c *invoke.Call, s ui.Style, sess *sysinfo.Session, provider, token, initialJSON string) {
	p := printer(c)
	md := trimNL(mdInitial(m, initialJSON, provider))
	menu := []string{
		"1) Services\t2) Security\t3) Network",
		"4) Storage\t5) Performance\t6) Maintenance",
		"?) Ask a question\ts) Save report\tq) Quit",
	}
	for {
		_, _ = io.WriteString(c.Stdout, "\n")
		s.Heading(c.Stdout, "Drill down for details:")
		s.Table(c.Stdout, menu, "--no-header", "--indent", "2")
		_, _ = io.WriteString(c.Stdout, "\n")
		if c.Term.StdinTTY() {
			_, _ = io.WriteString(c.Stderr, "> ")
		}
		choice, ok := ui.ReadLine(c.Stdin)
		if !ok {
			return // Ctrl+D exits
		}

		switch choice {
		case "q", "Q", "quit", "exit":
			return
		case "?", "ask":
			md += m.askQuestion(c, s, sess, provider, token, initialJSON)
			continue
		case "s", "S", "save":
			m.saveReport(c, md)
			continue
		}

		sectionTitle, deepName := "", ""
		switch choice {
		case "1":
			sectionTitle, deepName = "Services Detected", "services"
		case "2":
			sectionTitle, deepName = "Security Posture", "security"
		case "3":
			sectionTitle, deepName = "Network", "network"
		case "4":
			sectionTitle, deepName = "Storage", "storage"
		case "5":
			sectionTitle, deepName = "Performance Tuning", "performance"
		case "6":
			sectionTitle, deepName = "Maintenance", "maintenance"
		default:
			p.Warning("Invalid choice. Enter 1-6, ?, s, or q.")
			continue
		}

		// Initial findings for the drill-down context (the lines of the
		// matching section of the initial analysis).
		findings := extractFindings(initialJSON, sectionTitle)

		var deep bytes.Buffer
		sess.Deep(&deep, deepName)

		userMsg := fmt.Sprintf("Section: %s\n\nInitial findings:\n%s\n\nDetailed data:\n%s",
			sectionTitle, findings, deep.String())
		reply, rc := m.request(c, provider, token, sysinfoDrilldownPrompt, userMsg,
			sysinfoDrilldownSchema, sysinfoMaxTokens, sysinfoResponseTimeout)
		if rc != 0 {
			continue
		}
		formatDrilldown(s, p, c.Stdout, reply)
		md += trimNL(mdDrilldown(reply))
	}
}

// askQuestion is the `?` menu action: read a question, gather targeted
// context for the entities it names, call the AI and show the answer. It
// returns the Markdown chunk to append (empty on EOF, an empty question or
// a failed call).
func (m *Module) askQuestion(c *invoke.Call, s ui.Style, sess *sysinfo.Session, provider, token, initialJSON string) string {
	p := printer(c)
	_, _ = io.WriteString(c.Stdout, "\n")
	if c.Term.StdinTTY() {
		_, _ = io.WriteString(c.Stderr, "Question: ")
	}
	question, ok := ui.ReadLine(c.Stdin)
	if !ok {
		return "" // Ctrl+D: back to the menu
	}
	if question == "" {
		p.Warning("Empty question.")
		return ""
	}

	qCtx, kinds := m.questionContext(c, sess.Sudo, question)
	userMsg := freetextMsg(initialJSON, qCtx, question)
	// LEDGER AI-01: data collected for this question is confirmed per
	// question (kind data, so auto mode asks too); a refusal or no terminal
	// sends nothing and goes back to the menu. Protected paths contribute
	// only their "skipped" line, so they alone need no question.
	if len(kinds) > 0 {
		what := "question context (" + strings.Join(kinds, ", ") + ")"
		if !m.confirmSend(c, provider, "data", what, len(userMsg)) {
			return ""
		}
	}
	reply, rc := m.request(c, provider, token, sysinfoDrilldownPrompt, userMsg,
		sysinfoDrilldownSchema, sysinfoMaxTokens, sysinfoResponseTimeout)
	if rc != 0 {
		return ""
	}
	formatDrilldown(s, p, c.Stdout, reply)
	return trimNL(mdFreetext(question, reply))
}

// freetextMsg is the user message of __sysinfo_call_freetext_*: the two
// forms differ by whether targeted context was collected.
func freetextMsg(initialJSON, context, question string) string {
	if context != "" {
		return fmt.Sprintf("Initial server analysis:\n%s\n\nDetailed system data for this question:\n%s\n\nUser question: %s\n\nAnswer the question using all data provided above. Group your answer into relevant components.",
			initialJSON, context, question)
	}
	return fmt.Sprintf("Initial server analysis:\n%s\n\nUser question: %s\n\nAnswer the question based on the server analysis above. Group your answer into relevant components.",
		initialJSON, question)
}

// saveReport writes the running Markdown buffer to a timestamped file in
// the working directory. The write is unchecked (LEDGER AI-09): the success
// line prints whatever the write did.
func (m *Module) saveReport(c *invoke.Call, md string) {
	filename := "gash-ai-sysinfo-" + m.now().Format("20060102-150405") + ".md"
	path := filename
	if cwd, err := m.getwd(); err == nil && cwd != "" {
		path = filepath.Join(cwd, filename)
	}
	_ = os.WriteFile(path, []byte(md), 0o644)
	printer(c).Success("Report saved to " + filename)
}

// extractFindings is the jq that pulls the findings of the section titled
// sectionTitle from the initial analysis as "[severity] text" lines.
func extractFindings(initialJSON, sectionTitle string) string {
	var top struct {
		Sections []siSection `json:"sections"`
	}
	if json.Unmarshal([]byte(initialJSON), &top) != nil {
		return ""
	}
	var lines []string
	for _, sec := range top.Sections {
		if sec.Title != sectionTitle {
			continue
		}
		for _, f := range sec.Findings {
			lines = append(lines, "["+f.Severity+"] "+f.Text)
		}
	}
	return strings.Join(lines, "\n")
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
