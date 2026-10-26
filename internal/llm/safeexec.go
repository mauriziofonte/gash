// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"io"
	"strings"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/llmexec"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
)

// SafeExec runs argv under llm_exec's rails (AGENTS.md 6.3), for gash run
// --safe: the words are joined into one command line (ShellJoin), checked
// and rendered by internal/llmexec exactly as an llm_exec line is, with
// the settings of the caller's file. A refusal is its envelope on
// c.Stderr (status 1, refused true, nothing run); otherwise the command's
// output goes to stdout and stderr and its status is returned.
func SafeExec(c *invoke.Call, argv []string, stdin io.Reader, stdout, stderr io.Writer) (int, bool) {
	m := New()
	if _, ok := m.findBash(c); !ok {
		return prereq.Fail(c, "bash", "gash run --safe", true), true
	}
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
	pr := &probe{m: m, c: c}
	plan, err := llmexec.Build(ShellJoin(argv), s, env, pr)
	if err != nil {
		return pr.refusal(err), true
	}
	res, err := m.run.RunBash(c.Ctx, runner.Bash{
		Use:    runner.BashLLMExec,
		Wrap:   plan.Wrap,
		Bash:   plan.Bash,
		Script: plan.Script,
		Options: runner.Options{
			Env:    plan.Env,
			Stdin:  stdin,
			Stdout: stdout,
			Stderr: stderr,
		},
	})
	if err != nil && res.ExitCode == 0 {
		return 1, false
	}
	return res.ExitCode, false
}

var plainWord = lazyre.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// ShellJoin is argv as one Bash command line: each word that is not
// plain is put in single quotes (a quote inside it closes, escapes and
// reopens them), an empty word is a pair of quotes.
func ShellJoin(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		if plainWord.MatchString(a) {
			parts[i] = a
			continue
		}
		parts[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(parts, " ")
}

// LogicalCwd is the caller's working directory as its shell names it
// ($PWD when it is the process's directory), for the commands that turn
// absolute paths below it into relative ones.
func LogicalCwd(c *invoke.Call) string { return New().logicalCwd(c) }
