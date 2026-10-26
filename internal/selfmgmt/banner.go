// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"bytes"
	"io"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// Banner renders what lib/shell/banner.sh prints when GASH_BANNER=1: the
// "Gash VERSION — USER" line (product in the primary color, user as the
// accent, colors only where __gash_use_color allows) followed by the
// inspiring quote. gash init bash emits the result as one quoted printf, so
// the banner needs no fork in the shell. Colors follow the shell's stdout
// (GASH__TTY_OUT), which term.New already folded into c.Term.
func Banner(c *invoke.Call) string {
	return New().banner(c)
}

func (m *Module) banner(c *invoke.Call) string {
	s := ui.StyleOf(c.Term, false)
	var P, M, A, R string
	if s.Color {
		P, M, A, R = s.P.Primary, s.P.Muted, s.P.Accent, s.P.Off
	}
	user := c.Env.Get("USER")
	if user == "" {
		user = m.bannerUsername(c)
	}
	var b bytes.Buffer
	// printf '%sGash %s%s %s—%s %s%s%s\n'
	b.WriteString(P + "Gash " + m.version + R + " " + M + "—" + R + " " + A + user + R + "\n")

	// gash_inspiring_quote || true: the quote output, nothing on failure.
	var q bytes.Buffer
	qc := *c
	qc.Args = nil
	qc.Stdout = &q
	m.quote(&qc)
	b.Write(q.Bytes())
	return b.String()
}

// bannerUsername is ${USER:-$(gash_username)}: the gash_username result with
// its trailing newline removed.
func (m *Module) bannerUsername(c *invoke.Call) string {
	var out bytes.Buffer
	uc := *c
	uc.Args = nil
	uc.Stdout = &out
	uc.Stderr = io.Discard // the banner shows UNKNOWN_USERNAME; no warning at shell start
	m.username(&uc)
	return string(bytes.TrimRight(out.Bytes(), "\n"))
}
