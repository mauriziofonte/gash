// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package cli

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/spf13/cobra"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/registry"
)

// agentsHintCmd is `gash agents-hint [--format md|json]` (plan S9): the
// routing table of the agent functions, for an agent's instructions.
//
//	md:   a short preamble, then "| Instead of | Use |" rows
//	json: [{"fn":"llm_read","use":"llm_read","replaces":"grep -n X f; ...","write":false},...]
func agentsHintCmd(p *Process, status *int) *cobra.Command {
	var format string
	c := &cobra.Command{
		Use:   "agents-hint",
		Short: "Print the raw-command to Gash-function table for agent instructions",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			switch format {
			case "md":
				_, _ = io.WriteString(p.Stdout, registry.AgentsHint())
			case "json":
				writeRoutesJSON(p.Stdout, registry.Routes())
			default:
				_ = contract.New("invalid_format", "Unknown format: "+format, "Use --format md or --format json").Write(p.Stderr)
				*status = 1
				return nil
			}
			*status = 0
			return nil
		},
	}
	c.Flags().StringVar(&format, "format", "md", "md (Markdown for instructions) or json")
	return c
}

func writeRoutesJSON(w io.Writer, routes []registry.Route) {
	type row struct {
		Fn       string `json:"fn"`
		Use      string `json:"use"`
		Replaces string `json:"replaces"`
		Write    bool   `json:"write"`
	}
	rows := make([]row, len(routes))
	for i, r := range routes {
		rows[i] = row{r.Fn, r.Use, r.Shape, r.Write}
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(rows)
	_, _ = w.Write(b.Bytes())
}
