// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"os"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
)

// dockerCheck is llm_docker_check [PATH]: docker_compose_check PATH --json
// with the llm_* grammar and rails (3.0: LEDGER LLM-20 fixed). 2.x passed
// "${1:-.}" on with no path guard, ignored later words, and let the compose
// interpolation read any exported variable, so a secret could come back in
// an image field. Here -h/--help anywhere is help, an option is
// unknown_option, a second PATH unexpected_argument, PATH passes the path
// guard, a PATH/.env that resolves into a protected tree is refused, and
// docker_compose_check runs with a short environment allowlist, so
// ${VAR} in the compose file is resolved from PATH/.env, the compose
// defaults and that allowlist only. The binary cannot change the caller's
// shell variables (LLM-21).
func (m *Module) dockerCheck(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	var p pathTaker
	p.path = "."
	for _, a := range c.Args {
		if strings.HasPrefix(a, "-") {
			return fail(c, "unknown_option", a, "Pass the directory that holds the compose file")
		}
		if !p.take(c, a) {
			return 1
		}
	}
	safe, ok := m.validatePath(c, p.path)
	if !ok {
		return 1
	}
	envFile := strings.TrimSuffix(safe, "/") + "/.env"
	if _, err := os.Lstat(envFile); err == nil && m.guard(c).PathForbidden(envFile, "") {
		return fail(c, "forbidden_path", envFile, "This path is protected and cannot be accessed")
	}
	sub := *c
	sub.Name = "docker_compose_check"
	sub.Args = []string{safe, "--json"}
	sub.Env = dockerEnv(c.Env)
	if c.Handlers == nil {
		return c.Fail(invoke.NotPorted(sub.Name))
	}
	return c.Handlers.Run(&sub)
}

// dockerEnvAllow are the exported variables docker_compose_check keeps
// for llm_docker_check (LC_* by prefix): what docker and the compose
// plugin need to find the daemon, the user's configuration and the
// locale, and nothing that holds a secret (GITHUB_TOKEN included, so
// ghcr.io is asked anonymously).
var dockerEnvAllow = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "LANG": true, "LANGUAGE": true,
	"TERM": true, "TZ": true, "TMPDIR": true, "PWD": true, "NO_COLOR": true,
	"XDG_RUNTIME_DIR": true, "XDG_CONFIG_HOME": true,
	"DOCKER_HOST": true, "DOCKER_CONTEXT": true, "DOCKER_CONFIG": true, "DOCKER_CERT_PATH": true,
	"DOCKER_TLS_VERIFY": true, "DOCKER_API_VERSION": true,
	"GASH_HEADLESS": true, "GASH_NO_COLOR": true,
}

// dockerEnv is env with its exported part cut to dockerEnvAllow: the GASH__
// protocol variables and the shell variables the stub passed for this call
// (GASH__ADDED, which never reach a child) stay as they are.
func dockerEnv(env invoke.Env) invoke.Env {
	added := map[string]bool{}
	for _, n := range strings.Fields(env.Get("GASH__ADDED")) {
		added[n] = true
	}
	var keep []string
	for _, kv := range env.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "GASH__") || added[k] || dockerEnvAllow[k] || strings.HasPrefix(k, "LC_") {
			keep = append(keep, kv)
		}
	}
	return invoke.EnvFrom(keep)
}
