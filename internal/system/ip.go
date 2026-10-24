// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"context"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// ipTimeout bounds ip_public's whole request (SYS-13).
const ipTimeout = 10 * time.Second

// ipBodyMax is the most of the answer ip_public reads: an address is far
// shorter, so a longer body is refused anyway.
const ipBodyMax = 256

// ipPublic is ip_public: one GET of https://ipinfo.io/ip over net/http
// where 2.x ran wget (SYS-36), bounded by ipTimeout, and printed only when
// the trimmed body parses as an IP address (SYS-13). --raw prints the bare
// address. -h anywhere is help; other words are ignored.
func (m *Module) ipPublic(c *invoke.Call) int {
	noColor, raw := false, false
	for _, a := range c.Args {
		switch a {
		case "--no-color":
			noColor = true
		case "--raw":
			raw = true
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
	if raw {
		write(c.Stdout, ip+"\n")
		return 0
	}
	ui.StyleOf(c.Term, noColor).KV(c.Stdout, "--value-color", "accent", "Public IP:", ip)
	return 0
}

// fetchIP is the address in the body of a 2xx answer received within
// m.ipTimeout, "" for anything else: an error status, a timeout, or a
// body (a captive portal or rate-limit page) that is not an address.
func (m *Module) fetchIP(c *invoke.Call) string {
	cx, cancel := context.WithTimeout(ctx(c), m.ipTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cx, http.MethodGet, m.ipURL, nil)
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, ipBodyMax+1))
	if err != nil || len(body) > ipBodyMax {
		return ""
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(string(body)))
	if err != nil {
		return ""
	}
	return addr.String()
}
