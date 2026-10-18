// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmexec"
	"github.com/mauriziofonte/gash/internal/runner"
)

// loadConfig reads ~/.gash_env as __gash_load_env did.
// TODO(core): use the shared loader once it is in a leaf package
// (handlers.LoadConfig cannot be imported from here).
func loadConfig(c *invoke.Call) *config.File {
	home := c.Env.Get("HOME")
	return config.Load(config.Options{
		Path:     config.FilePath(home, c.Env.Get("GASH_ENV_FILE")),
		Home:     home,
		Headless: c.Env.Get("GASH_HEADLESS") == "1",
		Debug:    c.Env.Get("GASH_DEBUG") == "1",
		EUID:     os.Geteuid(),
	})
}

// exec is llm_exec (exec.sh __llm_exec_impl): the line is checked and
// rendered by internal/llmexec and run as its canonical script (D-1 A).
func (m *Module) exec(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	if len(c.Args) > 1 {
		return fail(c, "unexpected_argument", c.Args[1], "Pass the whole command line as one quoted argument")
	}
	// Settings come from the file only, never the environment. The stub
	// override parsed the file in the caller's shell (warnings, cache).
	f := loadConfig(c)
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
	plan, err := llmexec.Build(c.Arg(0), s, env, &probe{m: m, c: c})
	if err != nil {
		return refusal(c, err)
	}
	res, err := m.run.RunBash(c.Ctx, runner.Bash{
		Use:    runner.BashLLMExec,
		Wrap:   plan.Wrap,
		Bash:   plan.Bash,
		Script: plan.Script,
		Options: runner.Options{
			Env:    plan.Env,
			Stdin:  c.Stdin,
			Stdout: c.Stdout,
			Stderr: c.Stderr,
		},
	})
	if err != nil && res.ExitCode == 0 {
		return 1
	}
	return res.ExitCode
}

// bashPath is ${BASH:-bash}: the caller's $BASH (the stub passes it), or
// bash from the caller's PATH.
func (m *Module) bashPath(c *invoke.Call) string {
	if b := c.Env.Get("BASH"); b != "" {
		return b
	}
	if p, ok := m.lookPath(c, "bash"); ok {
		return p
	}
	return "bash"
}

// logicalCwd is the caller's $PWD when it names the working directory,
// the physical directory otherwise.
func (m *Module) logicalCwd(c *invoke.Call) string {
	pwd := c.Env.Get("PWD")
	if filepath.IsAbs(pwd) {
		a, err1 := os.Stat(pwd)
		b, err2 := os.Stat(".")
		if err1 == nil && err2 == nil && os.SameFile(a, b) {
			return pwd
		}
	}
	cwd, err := m.getwd()
	if err != nil {
		return pwd
	}
	return cwd
}

// probe answers llmexec's tool probes by running the tools from the
// caller's PATH, as 2.x probed them in the caller's shell.
type probe struct {
	m     *Module
	c     *invoke.Call
	cache map[string]bool
}

func (p *probe) ok(key string, stdin string, name string, args ...string) bool {
	if v, done := p.cache[key]; done {
		return v
	}
	if p.cache == nil {
		p.cache = map[string]bool{}
	}
	path, found := p.m.lookPath(p.c, name)
	v := false
	if found {
		res, err := p.m.run.Run(p.c.Ctx, runner.Cmd{Path: path, Args: args, Options: runner.Options{
			Env: p.c.Env.Child(), Stdin: strings.NewReader(stdin),
		}})
		v = err == nil && res.ExitCode == 0
		if key == "grep" {
			v = v && strings.Contains(string(res.Stdout), "GNU")
		}
	}
	p.cache[key] = v
	return v
}

func (p *probe) HasSandboxedSed() bool { return p.ok("sed", "x\n", "sed", "--sandbox", "-n", "p") }
func (p *probe) HasSandboxedGawk() bool {
	return p.ok("gawk", "", "gawk", "--sandbox", "BEGIN{}")
}
func (p *probe) HasDiffNoDeref() bool {
	return p.ok("diff", "", "diff", "--no-dereference", "/dev/null", "/dev/null")
}
func (p *probe) IsGNUGrep() bool { return p.ok("grep", "", "grep", "--version") }
func (p *probe) HasBwrap() bool {
	return p.ok("bwrap", "", "bwrap", "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc",
		"--unshare-all", "--die-with-parent", "true")
}
