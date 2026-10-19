// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mauriziofonte/gash/internal/ui"
)

// siComponent decodes one entry of the drill-down/free-text schema's
// components array.
type siComponent struct {
	Name             string      `json:"name"`
	Status           string      `json:"status"`
	ConfigHighlights []string    `json:"config_highlights"`
	Issues           []siFinding `json:"issues"`
}

// formatDrilldown is __sysinfo_format_drilldown_response: a title, an
// optional summary, then each component as "name (status)" with its config
// highlights and issues.
func formatDrilldown(s ui.Style, p ui.Printer, w io.Writer, jsonReply string) {
	var top map[string]json.RawMessage
	if json.Unmarshal([]byte(jsonReply), &top) != nil {
		p.Error("Invalid JSON response from AI")
		return
	}
	title := jqStr(top["title"], "Analysis")
	summary := jqStr(top["summary"], "")

	s.Section(w, title)
	if summary != "" {
		s.Print(w, "muted", summary)
		_, _ = io.WriteString(w, "\n")
	}

	bold, off, muted := "", "", ""
	if s.Color {
		bold, off, muted = s.P.Bold, s.P.Off, s.P.Muted
	}

	var comps []siComponent
	_ = json.Unmarshal(top["components"], &comps)
	for _, comp := range comps {
		sc := ""
		scoff := ""
		if s.Color {
			sc = statusColor(s, comp.Status)
			scoff = off
		}
		fmt.Fprintf(w, "%s%s%s (%s%s%s)\n", bold, comp.Name, off, sc, comp.Status, scoff)
		for _, h := range comp.ConfigHighlights {
			moff := ""
			if muted != "" {
				moff = off
			}
			fmt.Fprintf(w, "  %s%s%s\n", muted, h, moff)
		}
		printFindings(s, w, comp.Issues)
		_, _ = io.WriteString(w, "\n")
	}
}

// statusColor is the per-status color of __sysinfo_format_drilldown_response.
func statusColor(s ui.Style, status string) string {
	switch status {
	case "running":
		return s.P.Success
	case "installed":
		return s.P.Info
	case "stopped":
		return s.P.Warning
	case "eol", "error":
		return s.P.Error
	}
	return s.P.Muted
}

// mdLabel is __sysinfo_md_label.
func mdLabel(sev string) string {
	switch sev {
	case "critical":
		return "Critical:"
	case "warning":
		return "Warning:"
	case "ok":
		return "OK:"
	}
	return "Info:"
}

// trimNL is the trailing-newline drop of an `x=$(...)` / `x+=$(...)` capture.
func trimNL(s string) string { return strings.TrimRight(s, "\n") }

// capitalize is Bash's `${s^}`: the first byte uppercased.
func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// mdInitial is __sysinfo_md_initial: the Markdown of the initial analysis.
// A reply without a sections array leaves the summary counts empty (LEDGER
// AI-10), as the 2.x jq produced nothing.
func mdInitial(m *Module, jsonReply, provider string) string {
	var top map[string]json.RawMessage
	_ = json.Unmarshal([]byte(jsonReply), &top)
	hostname := jqStr(top["hostname"], "unknown")
	platform := jqStr(top["platform"], "unknown")
	crit, warn, info, ok := severityCounts(top["sections"])

	var b strings.Builder
	b.WriteString("# System Analysis Report\n\n")
	b.WriteString("**Generated:** " + m.now().Format("2006-01-02 15:04:05") +
		" | **Provider:** " + capitalize(provider) + "\n\n")
	b.WriteString("## Server: " + hostname + "\n")
	b.WriteString("*" + platform + "*\n\n")
	b.WriteString("**Summary:** " + crit + " critical, " + warn + " warnings, " +
		info + " info, " + ok + " ok\n\n")

	var sections []siSection
	_ = json.Unmarshal(top["sections"], &sections)
	for _, sec := range sections {
		b.WriteString("### " + sec.Title + "\n")
		for _, f := range sec.Findings {
			b.WriteString("- **" + mdLabel(f.Severity) + "** " + f.Text + "\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// mdDrilldown is __sysinfo_md_drilldown.
func mdDrilldown(jsonReply string) string {
	var top map[string]json.RawMessage
	_ = json.Unmarshal([]byte(jsonReply), &top)
	title := jqStr(top["title"], "Analysis")
	summary := jqStr(top["summary"], "")

	var b strings.Builder
	b.WriteString("\n---\n\n")
	b.WriteString("## Drill-down: " + title + "\n")
	if summary != "" {
		b.WriteString("*" + summary + "*\n")
	}
	b.WriteString("\n")
	mdComponents(&b, top["components"])
	return b.String()
}

// mdFreetext is __sysinfo_md_freetext.
func mdFreetext(question, jsonReply string) string {
	var top map[string]json.RawMessage
	_ = json.Unmarshal([]byte(jsonReply), &top)
	summary := jqStr(top["summary"], "")

	var b strings.Builder
	b.WriteString("\n---\n\n")
	b.WriteString("## Question: " + question + "\n")
	if summary != "" {
		b.WriteString("*" + summary + "*\n")
	}
	b.WriteString("\n")
	mdComponents(&b, top["components"])
	return b.String()
}

// mdComponents writes the component list shared by the drill-down and
// free-text Markdown.
func mdComponents(b *strings.Builder, raw json.RawMessage) {
	var comps []siComponent
	_ = json.Unmarshal(raw, &comps)
	for _, comp := range comps {
		b.WriteString("### " + comp.Name + " (`" + comp.Status + "`)\n")
		for _, h := range comp.ConfigHighlights {
			b.WriteString("- " + h + "\n")
		}
		for _, iss := range comp.Issues {
			b.WriteString("- **" + mdLabel(iss.Severity) + "** " + iss.Text + "\n")
		}
		b.WriteString("\n")
	}
}
