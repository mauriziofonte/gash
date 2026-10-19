// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"io"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// quote is gash_inspiring_quote [--source]: a random quote of
// $GASH_DIR/quotes/list.txt (one per line, under a "# source" comment
// line), word-wrapped after a "Quote:" label; --source adds where it was
// said or written. rc 1 when the file is unreadable or holds no quote. Any
// other first argument is reported as an unknown option, positional words
// included (LEDGER SELF-13, reproduced).
func (m *Module) quote(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	s := ui.StyleOf(c.Term, false)
	showSource := false
	switch a := c.Arg(0); a {
	case "":
	case "--source":
		showSource = true
	default:
		s.Printer(c.Stderr).Error("Unknown option: " + a + " (usage: gash_inspiring_quote [--source])")
		return 1
	}

	b, err := m.readFile(installDir(c) + "/quotes/list.txt")
	if err != nil {
		return 1
	}
	// mapfile -t: one element per line, a last line without a newline
	// kept. Every quote has the same chance, as the random line picks of
	// 2.x gave it.
	text := string(b)
	if strings.HasSuffix(text, "\n") {
		text = text[:len(text)-1]
	}
	var quotes, sources []string
	if len(b) > 0 {
		src := ""
		for _, line := range strings.Split(text, "\n") {
			switch {
			case line == "":
				src = ""
			case strings.HasPrefix(line, "#"):
				src = strings.TrimPrefix(line[1:], " ")
			default:
				quotes = append(quotes, line)
				sources = append(sources, src)
				src = ""
			}
		}
	}
	if len(quotes) == 0 {
		return 1
	}
	idx := m.intn(len(quotes))

	width := 0
	if cols := c.Env.Get("COLUMNS"); cols != "" && strings.Trim(cols, "0123456789") == "" && strings.Trim(cols, "0") != "" {
		width = ui.Int(cols)
	} else {
		width = s.Width
	}
	width -= 7
	if width < 20 {
		width = 80
	}

	on, off := "", ""
	if s.Color {
		on, off = s.P.Primary, s.P.Off
	}
	w := c.Stdout
	for i, l := range ui.Wrap(width, quotes[idx]) {
		if i == 0 {
			_, _ = io.WriteString(w, on+"Quote:"+off+" "+l+"\n")
		} else {
			_, _ = io.WriteString(w, "       "+l+"\n")
		}
	}
	if showSource && sources[idx] != "" {
		for _, l := range ui.Wrap(width, "Source: "+sources[idx]) {
			_, _ = io.WriteString(w, "       "+l+"\n")
		}
	}
	return 0
}
