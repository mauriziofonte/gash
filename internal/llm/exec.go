// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmexec"
	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
)

// exec is llm_exec (exec.sh __llm_exec_impl): the line is checked and
// rendered by internal/llmexec and run as its canonical script (D-1 A).
func (m *Module) exec(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	if len(c.Args) > 1 {
		return fail(c, "unexpected_argument", c.Args[1], "Pass the whole command line as one quoted argument")
	}
	// The line runs as a bash script: without bash 2.x exited 127 with
	// nothing said.
	if _, ok := m.findBash(c); !ok {
		return prereq.Fail(c, "bash", "llm_exec", true)
	}
	// Settings come from the file only, never the environment. The
	// caller's shell printed the file's warnings when it loaded it.
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
	plan, err := llmexec.Build(c.Arg(0), s, env, pr)
	if err != nil {
		return pr.refusal(err)
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

// bashPath is ${BASH:-bash}: the bash findBash finds, else the bare word
// (a container's bash, for llm_docker_exec).
func (m *Module) bashPath(c *invoke.Call) string {
	if p, ok := m.findBash(c); ok {
		return p
	}
	return "bash"
}

// findBash is the bash that runs the canonical script: the caller's
// $BASH when it names a program, else bash from the caller's PATH; false
// when there is none. As in 2.x it is a plain lookup, not the 4.4 check of
// gash_doctor, which would start a bash --version on every call.
func (m *Module) findBash(c *invoke.Call) (string, bool) {
	if b := c.Env.Get("BASH"); b != "" {
		if _, err := runner.LookPath(b, c.Env.Child(), ""); err == nil {
			return b, true
		}
	}
	return m.lookPath(c, "bash")
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

// HasSandboxedSed is GNU sed --sandbox as sed, else as gsed (Homebrew's
// name for GNU sed beside the BSD sed of macOS; 3.4).
func (p *probe) HasSandboxedSed() bool {
	return p.ok("sed", "x\n", "sed", "--sandbox", "-n", "p") || p.ok("gsed", "x\n", "gsed", "--sandbox", "-n", "p")
}

// SedName is the program sed runs as: gsed when only gsed has --sandbox.
func (p *probe) SedName() string {
	if !p.ok("sed", "x\n", "sed", "--sandbox", "-n", "p") && p.ok("gsed", "x\n", "gsed", "--sandbox", "-n", "p") {
		return "gsed"
	}
	return "sed"
}

// refusal is the refusal of a line Build refused with this probe. When a
// missing tool is the cause (sed without GNU sed --sandbox, awk without
// gawk --sandbox, diff -r without GNU diff), the hint also says how to
// install the catalog tool here, and the envelope carries missing and
// install as a prerequisite report does; the code and action stay (3.4;
// before, the hint named no package and no command).
func (p *probe) refusal(err error) int {
	var r *pathguard.Refusal
	if !errors.As(err, &r) {
		return refusal(p.c, err)
	}
	tool := ""
	switch {
	case r.Code == "command_not_allowed" && r.Details == "sed" && !p.HasSandboxedSed():
		tool = "gnu-sed"
	case r.Code == "command_not_allowed" && (r.Details == "awk" || r.Details == "gawk") && !p.HasSandboxedGawk():
		tool = "gawk"
	case r.Code == "option_not_allowed" && strings.HasPrefix(r.Details, "diff ") &&
		strings.HasPrefix(r.Hint, "Recursive diff needs GNU diff") && !p.HasDiffNoDeref():
		tool = "diffutils"
	default:
		return refusal(p.c, err)
	}
	rep := prereq.Current(p.c).Report(tool, "")
	e := envelopeOf(r.Code, r.Details, r.Action, r.Hint+". "+rep.Advice()).With("missing", tool)
	if cmd := rep.Install(); cmd != "" {
		e = e.With("install", cmd)
	}
	return p.c.Fail(e)
}

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
