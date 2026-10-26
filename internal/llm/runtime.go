// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"errors"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/dev"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
)

// runtime is llm_runtime RUNTIME VERSION (plan V2): the JSON form of
// `gash runtime` for agents, {"runtime","request","path","version"}
// (composer adds "php", the interpreter it runs under). It reads the
// runtime cache and never writes it.
func (m *Module) runtime(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	var pos []string
	for _, a := range c.Args {
		if strings.HasPrefix(a, "-") && a != "-" {
			return fail(c, "unknown_option", a, "llm_runtime takes RUNTIME VERSION and no options")
		}
		if len(pos) == 2 {
			return fail(c, "unexpected_argument", a, "Pass one RUNTIME and one VERSION")
		}
		pos = append(pos, a)
	}
	pos = append(pos, "", "")
	rt, req, ok := RuntimeRequest(c, pos[0], pos[1])
	if !ok {
		return 1
	}
	r := dev.NewResolver(c.Ctx, c.Env, m.run, false)
	res, err := r.Resolve(rt, req)
	if err != nil {
		return RuntimeFail(c, err)
	}
	_ = llmfmt.JSONLine(c.Stdout, res)
	return 0
}

// RuntimeRequest validates a runtime name and a version request (the
// arguments of gash runtime and llm_runtime, the parts of gash run
// --runtime NAME:VERSION and llm_lint --php/--node); on an error the
// envelope is written and ok is false.
func RuntimeRequest(c *invoke.Call, runtime, request string) (string, string, bool) {
	if dev.RuntimeFamily(runtime) == "" {
		fail(c, "unknown_runtime", runtime, "Name one of: "+strings.Join(dev.RuntimeNames, ", "))
		return "", "", false
	}
	req, ok := dev.NormalizeRequest(request)
	if !ok {
		hint := "Give a node major (24) or a version prefix (24.1)"
		if dev.RuntimeFamily(runtime) == "php" {
			hint = "Give a php major.minor (8.4), a major (8) or a full version (8.4.1)"
		}
		fail(c, "invalid_version", request, hint)
		return "", "", false
	}
	return runtime, req, true
}

// RuntimeFail writes the envelope of a resolution error:
// runtime_not_installed (STOP) with the installed versions.
func RuntimeFail(c *invoke.Call, err error) int {
	var ni *dev.NotInstalledError
	if errors.As(err, &ni) {
		return c.Fail(contract.New("runtime_not_installed", ni.Error(), ni.Hint()))
	}
	return fail(c, "internal_error", err.Error(), "")
}
