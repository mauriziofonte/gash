// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"io"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
)

// hosts is remote_hosts: the HOST: entries of ~/.gash_env as an
// alias/target table on stdout (aliases padded to 15 at least), or two
// Info lines when there is none (entries with an invalid target do not
// count). Extra arguments are ignored.
func (m *Module) hosts(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	x := m.begin(c)
	// An entry whose target fails config.ValidHostTarget is skipped, as
	// the HOST: parser should skip it (LEDGER REM-02, REM-08).
	var entries []config.Entry
	for _, e := range x.loadConfig().Hosts {
		if config.ValidHostTarget(e.Value) {
			entries = append(entries, e)
		}
	}
	if len(entries) == 0 {
		x.p.Info("No remote hosts configured in ~/.gash_env")
		x.p.Info("Add: HOST:alias=user@host")
		return 0
	}
	x.p.Info("Configured remote hosts:")
	_, _ = io.WriteString(c.Stdout, "\n")
	rows := make([]string, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, e.Name+"\t"+e.Value)
	}
	x.s.Table(c.Stdout, rows, "--no-header", "--indent", "2", "--gap", "1", "--min", "1:15", "--color", "1:accent")
	return 0
}
