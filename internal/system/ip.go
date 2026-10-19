// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"io"
	"net/http"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// ipPublic is ip_public: one GET of https://ipinfo.io/ip over net/http
// where 2.x ran wget (SYS-36); the body is printed unvalidated, without a
// client timeout (SYS-13). -h anywhere is help; other words are ignored.
func (m *Module) ipPublic(c *invoke.Call) int {
	noColor := false
	for _, a := range c.Args {
		switch a {
		case "--no-color":
			noColor = true
		case "-h", "--help":
			if help.NeedsOf(c, "--help") {
				return 0
			}
		}
	}
	ip := m.fetchIP(c)
	if ip == "" {
		ui.NewPrinter(c.Stderr, c.Term, false).Error("Failed to retrieve public IP address.")
		return 1
	}
	ui.StyleOf(c.Term, noColor).KV(c.Stdout, "--value-color", "accent", "Public IP:", ip)
	return 0
}

// fetchIP is `ip=$(wget -qO- URL 2>/dev/null) || true`: the body of a 2xx
// answer as $(...) keeps it, "" otherwise.
func (m *Module) fetchIP(c *invoke.Call) string {
	req, err := http.NewRequestWithContext(ctx(c), http.MethodGet, m.ipURL, nil)
	if err != nil {
		return ""
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return ""
	}
	body, _ := io.ReadAll(resp.Body) // a cut transfer keeps what wget had written
	return runner.CommandSubst(body)
}
