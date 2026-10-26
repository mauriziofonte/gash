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
// arguments of gash runtime and llm_runtime, the parts of gash run and
// agent_test --runtime NAME:VERSION and llm_lint --php/--node); on an
// error the envelope is written and ok is false. The request "project"
// is the version the project in the working directory pins (as
// php_detect_version and node_detect_version read it); a project that
// pins none is runtime_not_pinned (STOP), never the default version.
func RuntimeRequest(c *invoke.Call, runtime, request string) (string, string, bool) {
	fam := dev.RuntimeFamily(runtime)
	if fam == "" {
		fail(c, "unknown_runtime", runtime, "Name one of: "+strings.Join(dev.RuntimeNames, ", "))
		return "", "", false
	}
	if request == dev.ProjectRequest {
		pin, _ := dev.ProjectPin(c, fam, ".")
		if pin == "" {
			fail(c, "runtime_not_pinned", "no "+fam+" version is pinned in "+New().logicalCwd(c),
				"Ask the user which "+fam+" to use, or name the version; a project pins one in "+dev.PinFiles(fam))
			return "", "", false
		}
		request = pin
	}
	req, ok := dev.NormalizeRequest(request)
	if !ok {
		hint := "Give a node major (24), a version prefix (24.1) or project"
		if fam == "php" {
			hint = "Give a php major.minor (8.4), a major (8), a full version (8.4.1) or project"
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

// RuntimeSpecs resolves the --runtime NAME:VERSION words of gash run and
// agent_test, in order (VERSION may be project); on an error the envelope
// is written and ok is false.
func RuntimeSpecs(c *invoke.Call, r *dev.Resolver, specs []string) ([]dev.Resolution, bool) {
	out := make([]dev.Resolution, 0, len(specs))
	for _, spec := range specs {
		name, ver, ok := strings.Cut(spec, ":")
		if !ok {
			fail(c, "unknown_runtime", spec, "Give NAME:VERSION: node:24, php:8.4, php:project")
			return nil, false
		}
		rt, req, ok := RuntimeRequest(c, name, ver)
		if !ok {
			return nil, false
		}
		res, err := r.Resolve(rt, req)
		if err != nil {
			RuntimeFail(c, err)
			return nil, false
		}
		out = append(out, res)
	}
	return out, true
}

// RuntimeEnv is c's environment with the directory of each resolution
// first on PATH, in order (dev.Resolver.BinDir); on an error the envelope
// is written and ok is false.
func RuntimeEnv(c *invoke.Call, r *dev.Resolver, list []dev.Resolution) (invoke.Env, bool) {
	if len(list) == 0 {
		return c.Env, true
	}
	dirs := make([]string, 0, len(list))
	for _, res := range list {
		dir, err := r.BinDir(res)
		if err != nil {
			fail(c, "runtime_link_failed", err.Error(), "Check that the gash cache directory is writable")
			return c.Env, false
		}
		dirs = append(dirs, dir)
	}
	path := strings.Join(dirs, ":")
	if old := c.Env.Get("PATH"); old != "" {
		path += ":" + old
	}
	environ := c.Env.Environ()
	set := false
	for i, kv := range environ {
		if strings.HasPrefix(kv, "PATH=") {
			environ[i], set = "PATH="+path, true
		}
	}
	if !set {
		environ = append(environ, "PATH="+path)
	}
	return invoke.EnvFrom(environ), true
}
