// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"sort"
	"strings"
)

// childEnvAllow is the fixed name allowlist of __llm_exec_env; LC_* is matched
// by prefix.
var childEnvAllow = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "LANG": true,
	"LANGUAGE": true, "TERM": true, "TZ": true, "TMPDIR": true, "NO_COLOR": true,
	"XDG_RUNTIME_DIR": true, "DOCKER_HOST": true, "DOCKER_CONTEXT": true,
}

func childEnvKeep(name string) bool {
	return childEnvAllow[name] || strings.HasPrefix(name, "LC_")
}

// buildChildEnv is __llm_exec_env: the restricted child environment, a short
// allowlist of the caller's exported variables plus PWD and the two git
// variables. The caller's secrets never reach the child (jq env, gawk
// ENVIRON, ps e would otherwise print them). Names are sorted for a
// deterministic slice; order does not affect execution.
func buildChildEnv(env Environ) []string {
	var names []string
	for name := range env.Vars {
		if childEnvKeep(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]string, 0, len(names)+3)
	for _, name := range names {
		out = append(out, name+"="+env.Vars[name])
	}
	out = append(out, "PWD="+env.Cwd, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	return out
}

// unrestrictedEnv keeps the caller's environment but clears BASH_ENV and ENV,
// as 2.x does for `GASH_LLM_EXEC=unrestricted`.
func unrestrictedEnv(env Environ) []string {
	seen := map[string]bool{}
	var out []string
	var names []string
	for name := range env.Vars {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "BASH_ENV" || name == "ENV" {
			continue
		}
		out = append(out, name+"="+env.Vars[name])
		seen[name] = true
	}
	out = append(out, "BASH_ENV=", "ENV=")
	return out
}
