// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runcmd

import (
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mauriziofonte/gash/internal/dev"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llm"
	"github.com/mauriziofonte/gash/internal/llmfmt"
)

const runtimeUsage = `Usage: gash runtime php|node|composer|npm|npx VERSION [--path]

Resolve a runtime version to its binary, without loading nvm.sh.

  VERSION   a prefix: 24, 24.1, 8.4, 8 (the newest installed match)
  --path    print the path alone

Prints {"runtime","request","path","version"} (composer adds "php", the
binary it runs under). node comes from ${NVM_DIR:-~/.nvm}/versions/node,
then PATH; php from /usr/bin/phpX.Y and /usr/local/bin/phpX.Y, then PATH.
A version that is not installed is runtime_not_installed (STOP) with the
installed list. Resolutions are cached in
${XDG_CACHE_HOME:-~/.cache}/gash/runtimes.json until the nvm tree or
/usr/bin/php* change. The JSON form for agents: llm_runtime.
`

// runtime is gash runtime RUNTIME VERSION [--path] (plan S8).
func (m *Module) runtime(c *invoke.Call) int {
	if words, partial, ok := completing(c); ok {
		if strings.HasPrefix(partial, "-") {
			return complete(c.Stdout, partial, cobra.ShellCompDirectiveNoFileComp, "--path", "--help")
		}
		n := 0
		for _, w := range words {
			if !strings.HasPrefix(w, "-") {
				n++
			}
		}
		if n == 0 {
			return complete(c.Stdout, partial, cobra.ShellCompDirectiveNoFileComp, dev.RuntimeNames...)
		}
		return complete(c.Stdout, partial, cobra.ShellCompDirectiveNoFileComp)
	}
	var pos []string
	pathOnly := false
	for _, a := range c.Args {
		switch {
		case isHelp(a):
			_, _ = io.WriteString(c.Stdout, runtimeUsage)
			return 0
		case a == "--path":
			pathOnly = true
		case strings.HasPrefix(a, "-") && a != "-":
			return fail(c, "unknown_option", a, "gash runtime takes RUNTIME VERSION and --path")
		case len(pos) == 2:
			return fail(c, "unexpected_argument", a, "Pass one RUNTIME and one VERSION")
		default:
			pos = append(pos, a)
		}
	}
	pos = append(pos, "", "")
	rt, req, ok := llm.RuntimeRequest(c, pos[0], pos[1])
	if !ok {
		return 1
	}
	r := dev.NewResolver(c.Ctx, c.Env, m.run, true)
	res, err := r.Resolve(rt, req)
	r.Save() // a cache that cannot be written only costs the next call a probe
	if err != nil {
		return llm.RuntimeFail(c, err)
	}
	if pathOnly {
		_, _ = io.WriteString(c.Stdout, res.Path+"\n")
		return 0
	}
	_ = llmfmt.JSONLine(c.Stdout, res)
	return 0
}
