// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runcmd

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/mauriziofonte/gash/internal/dev"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llm"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/runner"
)

const runUsage = `Usage: gash run [--width N] [--max-lines N] [--head N] [--tail N] [--plain] [--test-summary] [--runtime NAME:VERSION]... [--safe] -- CMD...

Run CMD and return its exit status unchanged.

  --width N               cut each output line at N columns
  --max-lines N, --head N keep the first N lines of stdout and of stderr
  --tail N                keep the last N lines; what is cut becomes one
                          "[... K lines omitted]" line
  --plain                 drop color and other escape sequences, and keep
                          the last state of a line redrawn with carriage
                          returns (progress bars); before the budget
  --test-summary          print the agent_test summary of CMD's output instead
                          of the output (stdout and stderr merged)
  --runtime NAME:VERSION  put that runtime's directory first on PATH:
                          node:24, php:8.4 (repeatable; gash runtime resolves
                          it, php is reachable as php); VERSION project is
                          the version the project pins (.nvmrc,
                          .target-php-version, composer.json, ...), and a
                          project that pins none stops with
                          runtime_not_pinned
  --safe                  run CMD under llm_exec's checked read-only rules
                          (GASH_LLM_EXEC=unrestricted does not apply)

Options come before -- (or before the first word that is not an option);
every word after it belongs to CMD. CMD may be a gash function
(gash run --max-lines 20 -- files_largest .). Without a budget flag or
--plain CMD's output passes through untouched. gash's own errors (a bad option, a
program not found, a --safe refusal, a runtime not installed) are JSON
envelopes on stderr.
`

// testOutputCap bounds the output --test-summary keeps (the newest).
const testOutputCap = 64 << 20

// runCmd is gash run [budget] [--plain] [--test-summary] [--runtime
// NAME:VERSION] [--safe] -- CMD... (plan S11).
func (m *Module) runCmd(c *invoke.Call) int {
	if words, partial, ok := completing(c); ok {
		return runComplete(c, words, partial)
	}
	return invoke.WithTelemetry(c, "gash run", func() int { return m.runMain(c) })
}

func (m *Module) runMain(c *invoke.Call) int {
	o := llmfmt.New(llmfmt.BudgetFlags)
	summary, safe, plain := false, false, false
	var runtimes, cmd []string
	args := c.Args
	for len(args) > 0 {
		a := args[0]
		if a == "--" {
			cmd = args[1:]
			break
		}
		if isHelp(a) {
			_, _ = io.WriteString(c.Stdout, runUsage)
			return 0
		}
		n, ok := o.Take(c, args)
		if !ok {
			return 1
		}
		if n > 0 {
			args = args[n:]
			continue
		}
		switch a {
		case "--test-summary":
			summary = true
			args = args[1:]
			continue
		case "--safe":
			safe = true
			args = args[1:]
			continue
		case "--plain":
			plain = true
			args = args[1:]
			continue
		case "--runtime":
			if len(args) < 2 {
				return fail(c, "missing_value", a, "--runtime needs NAME:VERSION: node:24, php:8.4, php:project")
			}
			runtimes = append(runtimes, args[1])
			args = args[2:]
			continue
		}
		if strings.HasPrefix(a, "-") {
			return fail(c, "unknown_option", a, "Options come before --: --width, --max-lines, --head, --tail, --plain, --test-summary, --runtime, --safe")
		}
		cmd = args
		break
	}
	if len(cmd) == 0 {
		return fail(c, "empty_command", "", "Give the command after --: gash run --max-lines 50 -- make test")
	}
	env, ok := m.runtimeEnv(c, runtimes)
	if !ok {
		return 1
	}

	var stdin io.Reader = c.Stdin
	var out, errw io.Writer = c.Stdout, c.Stderr
	var capture interface {
		Write([]byte) (int, error)
		Bytes() []byte
	}
	var sinks []*lineSink
	switch {
	case summary:
		capture = llm.NewCapWriter(testOutputCap)
		stdin, out, errw = nil, capture, capture
	case o.Width > 0 || o.Capped():
		so, se := &lineSink{lw: o.Lines(c.Stdout, "lines")}, &lineSink{lw: o.Lines(c.Stderr, "lines")}
		sinks = []*lineSink{so, se}
		out, errw = so, se
	}
	// --plain cleans each stream before the budget or the summary sees it.
	var plains []*plainWriter
	if plain {
		po, pe := newPlainWriter(out), newPlainWriter(errw)
		plains = []*plainWriter{po, pe}
		out, errw = po, pe
	}
	closeSinks := func() {
		for _, p := range plains {
			_ = p.Close()
		}
		omitted := 0
		for _, s := range sinks {
			omitted += s.Close()
		}
		c.NoteTruncated(omitted)
	}

	t0 := time.Now()
	var rc int
	sub := &invoke.Call{Ctx: c.Ctx, Name: cmd[0], Args: cmd[1:], Env: env, Stdin: stdin, Stdout: out, Stderr: errw,
		Term: c.Term, Handlers: c.Handlers}
	if sub.Stdin == nil {
		sub.Stdin = bytes.NewReader(nil)
	}
	switch h, inProc := gashFunction(c, cmd[0]); {
	case safe:
		// The refusal goes to the caller's stderr, never shaped.
		guard := *sub
		guard.Stderr = c.Stderr
		code, refused := llm.SafeExec(&guard, cmd, stdin, out, errw)
		if refused {
			closeSinks()
			return code
		}
		rc = code
	case inProc:
		rc = h(sub)
	default:
		res, err := runner.RunUser(c.Ctx, m.run, cmd, runner.Options{Env: env.Child(), Stdin: stdin, Stdout: out, Stderr: errw})
		if code, failed := llm.StartFailure(c, cmd, res, err); failed {
			closeSinks()
			return code
		}
		rc = res.ExitCode
	}
	closeSinks()
	if summary {
		llm.WriteTestSummary(c, o, llm.TestRun{Argv: cmd, Runner: "auto", Output: capture.Bytes(), ExitCode: rc,
			Duration: time.Since(t0), Cwd: llm.LogicalCwd(c)})
	}
	return rc
}

// gashFunction is the handler of a gash function CMD names (one the
// binary runs: the shell-side shims need the caller's shell).
func gashFunction(c *invoke.Call, name string) (invoke.Handler, bool) {
	if c.Handlers == nil {
		return nil, false
	}
	f, ok := registry.Lookup(name)
	if !ok || !f.Delegated() || f.Impl != registry.ImplBinary {
		return nil, false
	}
	return c.Handlers.Lookup(name)
}

// runtimeEnv is the caller's environment with the directory of each
// --runtime NAME:VERSION first on PATH, in the order given.
func (m *Module) runtimeEnv(c *invoke.Call, specs []string) (invoke.Env, bool) {
	if len(specs) == 0 {
		return c.Env, true
	}
	r := dev.NewResolver(c.Ctx, c.Env, m.run, true)
	defer r.Save()
	list, ok := llm.RuntimeSpecs(c, r, specs)
	if !ok {
		return c.Env, false
	}
	return llm.RuntimeEnv(c, r, list)
}

// lineSink splits what a command writes into lines for a LineWriter; a
// last line without a newline is written at Close.
type lineSink struct {
	mu   sync.Mutex
	lw   *llmfmt.LineWriter
	part []byte
}

func (s *lineSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data := p
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			s.part = append(s.part, data...)
			break
		}
		if len(s.part) > 0 {
			s.part = append(s.part, data[:i]...)
			s.lw.Line(string(s.part))
			s.part = s.part[:0]
		} else {
			s.lw.Line(string(data[:i]))
		}
		data = data[i+1:]
	}
	return len(p), nil
}

// Close writes the pending line, the marker and the kept tail, and
// returns the number of lines cut.
func (s *lineSink) Close() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.part) > 0 {
		s.lw.Line(string(s.part))
		s.part = nil
	}
	return s.lw.Close()
}

// runComplete completes gash run: the options before CMD, NAME: after
// --runtime, files and commands from CMD on.
func runComplete(c *invoke.Call, words []string, partial string) int {
	for i, w := range words {
		if w == "--" || (!strings.HasPrefix(w, "-") && (i == 0 || !runValueFlag(words[i-1]))) {
			return complete(c.Stdout, partial, cobra.ShellCompDirectiveDefault)
		}
	}
	if len(words) > 0 {
		switch prev := words[len(words)-1]; {
		case prev == "--runtime":
			return complete(c.Stdout, partial, cobra.ShellCompDirectiveNoSpace|cobra.ShellCompDirectiveNoFileComp,
				"node:", "node:project", "php:", "php:project")
		case runValueFlag(prev):
			return complete(c.Stdout, partial, cobra.ShellCompDirectiveNoFileComp)
		}
	}
	if strings.HasPrefix(partial, "-") {
		return complete(c.Stdout, partial, cobra.ShellCompDirectiveNoFileComp,
			"--", "--width", "--max-lines", "--head", "--tail", "--plain", "--test-summary", "--runtime", "--safe", "--help")
	}
	return complete(c.Stdout, partial, cobra.ShellCompDirectiveDefault)
}

func runValueFlag(w string) bool {
	switch w {
	case "--width", "--max-lines", "--head", "--tail", "--runtime":
		return true
	}
	return false
}
