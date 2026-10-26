// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"strings"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmexec"
)

// execRules are the llm_exec settings and process context of a call, as
// llm_exec builds them: the settings from ~/.gash_env only, the caller's
// exported environment, its logical working directory. llm_wait --cmd-ok
// and llm_docker_exec check their command with them.
func (m *Module) execRules(c *invoke.Call) (llmexec.Settings, llmexec.Environ) {
	f := config.LoadCaller(c.Env.Get)
	setting := func(name string) string {
		v, _ := f.Setting(name)
		return v
	}
	s := llmexec.Settings{
		Exec:      setting("GASH_LLM_EXEC"),
		ExecAllow: setting("GASH_LLM_EXEC_ALLOW"),
		Sandbox:   setting("GASH_LLM_SANDBOX"),
	}
	vars := map[string]string{}
	for _, kv := range c.Env.Child() {
		k, v, _ := strings.Cut(kv, "=")
		vars[k] = v
	}
	env := llmexec.Environ{
		Cwd:      m.logicalCwd(c),
		Home:     c.Env.Get("HOME"),
		RootHome: m.rootHome(),
		BashPath: m.bashPath(c),
		Vars:     vars,
	}
	return s, env
}

// execPlan checks line under the llm_exec rules and returns what llm_exec
// would run, with the probe that answered; a refusal comes back as the
// *llmexec.Refusal error, for the probe's refusal.
func (m *Module) execPlan(c *invoke.Call, line string) (*llmexec.Plan, *probe, error) {
	s, env := m.execRules(c)
	pr := &probe{m: m, c: c}
	plan, err := llmexec.Build(line, s, env, pr)
	return plan, pr, err
}
