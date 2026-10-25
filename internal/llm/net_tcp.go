// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"context"
	"net"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
)

// tcpTimeout is the default --timeout of one llm_tcp target.
const tcpTimeout = 5 * time.Second

// tcp is llm_tcp (plan N4): can a TCP connection be opened to each
// HOST:PORT, and how long did it take. Nothing is sent; the connection is
// closed at once.
func (m *Module) tcp(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(netFlags)
	targets, ok := netArgs(c, o, "Use -j N, --timeout S, --format F or HOST:PORT targets", nil)
	if !ok {
		return 1
	}
	if len(targets) == 0 {
		return fail(c, "missing_target", "llm_tcp needs a HOST:PORT", "Pass one or more HOST:PORT targets, e.g. db.local:5432 [::1]:22")
	}
	for _, t := range targets {
		if _, _, ok := splitHostPort(t, ""); !ok {
			return fail(c, "invalid_target", t, "Give HOST:PORT with a port from 1 to 65535 (an IPv6 address in brackets: [::1]:22)")
		}
	}
	rows := llmfmt.Map(c.Ctx, o.JobsOr(llmfmt.NetworkJobs), o.TimeoutOr(tcpTimeout), targets, func(ctx context.Context, t string) []any {
		start := time.Now()
		var d net.Dialer
		conn, err := d.DialContext(ctx, "tcp", t)
		ms := llmfmt.Ms(time.Since(start))
		if err != nil {
			return []any{t, false, ms, netError(ctx, err)}
		}
		_ = conn.Close()
		return []any{t, true, ms, nil}
	})
	o.Write(c, o.FormatOr(llmfmt.JSON), llmfmt.Table{Cols: []string{"target", "open", "ms", "error"}, Rows: rows}, nil)
	return 0
}
