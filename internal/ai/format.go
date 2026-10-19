// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"encoding/json"
	"io"

	"github.com/mauriziofonte/gash/internal/invoke"
)

// aiReply is the structured response (the five types of the schema).
type aiReply struct {
	Type       string `json:"type"`
	Text       string `json:"text"`
	Command    string `json:"command"`
	Code       string `json:"code"`
	Lang       string `json:"lang"`
	Issue      string `json:"issue"`
	Suggestion string `json:"suggestion"`
}

// formatResponse is __ai_format_response: print the reply by its type.
// Model text is printed as given (never escape-expanded); labels are KV
// keys. Writes to c.Stdout.
func (m *Module) formatResponse(c *invoke.Call, jsonReply string) {
	var r aiReply
	_ = json.Unmarshal([]byte(jsonReply), &r) // a bad reply yields zero fields
	if r.Type == "" {
		r.Type = "fallback"
	}
	if r.Lang == "" {
		r.Lang = "bash"
	}
	s := style(c)
	w := c.Stdout
	line := func(s string) { _, _ = io.WriteString(w, s+"\n") }
	switch r.Type {
	case "command":
		if r.Command != "" {
			s.KV(w, "--width", "0", "--value-color", "accent", "Command:", "`"+r.Command+"`")
		}
		if r.Text != "" {
			s.KV(w, "--width", "0", "Explanation:", r.Text)
		}
	case "explanation":
		if r.Text != "" {
			s.KV(w, "--width", "0", "Explanation:", r.Text)
		}
	case "troubleshoot":
		if r.Issue != "" {
			s.KV(w, "--width", "0", "Issue:", r.Issue)
		}
		if r.Suggestion != "" {
			s.KV(w, "--width", "0", "Suggestion:", r.Suggestion)
		}
		if r.Text != "" && r.Issue == "" {
			line(r.Text)
		}
	case "code":
		if r.Text != "" {
			line(r.Text)
		}
		s.Print(w, "muted", "# "+r.Lang)
		if r.Code != "" {
			line(r.Code)
		}
	default: // fallback and any other type
		if r.Text != "" {
			line(r.Text)
		}
	}
}
