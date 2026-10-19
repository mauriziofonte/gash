// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"io"
	"strings"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// registerConfig binds the public listing helpers of lib/core/config.sh.
func registerConfig(t *invoke.Table) {
	t.Handle("gash_db_list", dbList)
	t.Handle("gash_ai_list", aiList)
}

// The settings come from config.LoadCaller, silently: the generated stub
// ran the shell's __gash_load_env first, which printed the load warnings
// once for the shell (LEDGER CORE-18).

// listTable prints the rows of a listing as 2.x piped them to __gash_table
// (name TAB value lines, re-read as table rows, so a tab inside a value
// starts a column as it did).
func listTable(s ui.Style, w io.Writer, lines []string, minName string) {
	text := strings.Join(lines, "\n")
	if text != "" {
		text += "\n"
	}
	s.Table(w, ui.ReadRows(strings.NewReader(text)),
		"--no-header", "--indent", "2", "--gap", "1", "--min", minName, "--color", "1:accent")
}

// dbList is gash_db_list: the DB: connections of the settings file, the
// passwords masked by the 2.x rule (LEDGER CORE-03, reproduced), every
// duplicate name listed (CORE-04, reproduced), the messages naming
// ~/.gash_env whatever GASH_ENV_FILE says (CORE-06, reproduced).
func dbList(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	f := config.LoadCaller(c.Env.Get)
	s := ui.StyleOf(c.Term, false)
	p := s.Printer(c.Stderr)
	if len(f.DB) == 0 {
		p.Info("No database connections configured in ~/.gash_env")
		p.Info("Run 'gash_env_init' to create a template configuration")
		return 0
	}
	p.Info("Available database connections:")
	_, _ = io.WriteString(c.Stdout, "\n")
	lines := make([]string, 0, len(f.DB))
	for _, r := range f.DBList() {
		lines = append(lines, r.Name+"\t"+r.URL)
	}
	listTable(s, c.Stdout, lines, "1:15")
	return 0
}

// aiList is gash_ai_list: the AI: providers of the settings file, each
// token shown as its first eight characters and "...".
func aiList(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	f := config.LoadCaller(c.Env.Get)
	s := ui.StyleOf(c.Term, false)
	p := s.Printer(c.Stderr)
	if len(f.AI) == 0 {
		p.Info("No AI providers configured in ~/.gash_env")
		p.Info("Add: AI:claude=YOUR_API_KEY or AI:gemini=YOUR_API_KEY")
		return 0
	}
	p.Info("Available AI providers:")
	_, _ = io.WriteString(c.Stdout, "\n")
	lines := make([]string, 0, len(f.AI))
	for _, e := range f.AI {
		lines = append(lines, e.Name+"\t"+config.MaskAIToken(e.Value))
	}
	listTable(s, c.Stdout, lines, "1:10")
	return 0
}
