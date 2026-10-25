// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"errors"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/testparse"
)

// Defaults of the test summary without budget flags: failures listed,
// columns of a message, lines of an output no parser read.
const (
	testDefaultFailures = 50
	testDefaultWidth    = 200
	testDefaultTail     = 20
)

// testOutputCap bounds the output a test run keeps in memory: past it
// the oldest half is dropped (the summary and the failures come last).
const testOutputCap = 64 << 20

// test is llm_test [--runner auto|NAME] [--fail-only] [budget] -- CMD...
// (plan L2): it runs CMD as given (its stdout and stderr merged, stdin
// /dev/null), parses the runner's output and prints one JSON object; the
// exit status is CMD's.
func (m *Module) test(c *invoke.Call) int {
	o := llmfmt.New(llmfmt.BudgetFlags | llmfmt.TimeoutFlag | llmfmt.PathFlags)
	runnerName, failOnly := "auto", false
	args := c.Args
	var cmd []string
	for len(args) > 0 {
		a := args[0]
		if a == "--" {
			cmd = args[1:]
			break
		}
		if a == "--help" || a == "-h" {
			return helpOnly(c)
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
		case "--runner":
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a value")
			}
			if args[1] != "auto" && !testparse.Known(args[1]) {
				return fail(c, "unknown_runner", args[1], "Use auto or one of: "+strings.Join(testparse.Runners, ", "))
			}
			runnerName = args[1]
			args = args[2:]
			continue
		case "--fail-only":
			failOnly = true
			args = args[1:]
			continue
		}
		if strings.HasPrefix(a, "-") {
			return fail(c, "unknown_option", a, "Options come before --: --runner, --fail-only, --max-lines, --width, --timeout, --absolute")
		}
		cmd = args
		break
	}
	if len(cmd) == 0 {
		return fail(c, "empty_command", "", "Give the test command after --: llm_test -- vendor/bin/pest")
	}
	out := &capWriter{max: testOutputCap}
	t0 := time.Now()
	res, err := runner.RunUser(c.Ctx, m.run, cmd, runner.Options{
		Env: c.Env.Child(), Stdout: out, Stderr: out, Timeout: o.Timeout, KillAfter: 5 * time.Second,
	})
	if rc, failed := StartFailure(c, cmd, res, err); failed {
		return rc
	}
	WriteTestSummary(c, o, TestRun{Argv: cmd, Runner: runnerName, Output: out.Bytes(), ExitCode: res.ExitCode,
		Duration: time.Since(t0), FailOnly: failOnly, Cwd: m.logicalCwd(c)})
	return res.ExitCode
}

// helpOnly prints the help of the call's function (--help before --).
func helpOnly(c *invoke.Call) int {
	saved := c.Args
	c.Args = []string{"--help"}
	helpAnywhere(c)
	c.Args = saved
	return 0
}

// StartFailure writes the envelope of a command that did not start:
// command_not_found (RETRY, status 127) for a program not on PATH, the
// runner's message for one that cannot run (126). failed is false when
// the command ran.
func StartFailure(c *invoke.Call, cmd []string, res runner.Result, err error) (int, bool) {
	var se *runner.StartError
	if !errors.As(err, &se) {
		return 0, false
	}
	code := "command_not_found"
	hint := "Check the program name, or give its path (vendor/bin/pest, ./node_modules/.bin/vitest)"
	if se.Status == 126 {
		hint = "The program exists but cannot be executed: check its mode and its #! line"
	}
	_ = c.Fail(envelopeOf(code, se.Error(), "RETRY", hint))
	if res.ExitCode == 0 {
		res.ExitCode = se.Status
	}
	return res.ExitCode, true
}

// TestRun is one finished test command, for WriteTestSummary.
type TestRun struct {
	Argv     []string
	Runner   string // "auto" or a testparse runner name
	Output   []byte // stdout and stderr, merged
	ExitCode int
	Duration time.Duration
	FailOnly bool
	// Cwd turns the absolute paths a runner prints below it into
	// relative ones (plan R3).
	Cwd string
}

// WriteTestSummary prints the llm_test object of a run on c.Stdout:
//
//	{"runner","passed","failed","skipped","duration_s","failures":[{"name","file","line","message"}],"parsed":true,"exit_code"}
//
// or, when no parser read the output,
//
//	{"runner":null,"duration_s","parsed":false,"exit_code","tail":[LINES]}
//
// With FailOnly nothing is printed for a run that exited 0 with no
// failure. gash run --test-summary prints the same.
func WriteTestSummary(c *invoke.Call, o *llmfmt.Options, run TestRun) {
	sum, parsed := testparse.Parse(run.Output, run.Runner, run.Argv)
	if run.FailOnly && run.ExitCode == 0 && (!parsed || sum.Failed == 0) {
		return
	}
	width := o.Width
	if width == 0 {
		width = testDefaultWidth
	}
	dur := strconv.FormatFloat(math.Round(run.Duration.Seconds()*100)/100, 'f', -1, 64)
	var b bytes.Buffer
	if !parsed {
		n := testDefaultTail
		if o.Tail > 0 {
			n = o.Tail
		} else if o.Head > 0 {
			n = o.Head
		}
		b.WriteString(`{"runner":null,"duration_s":` + dur + `,"parsed":false,"exit_code":` + strconv.Itoa(run.ExitCode) + `,"tail":[`)
		for i, l := range testparse.Tail(run.Output, n) {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(llmfmt.Quote(llmfmt.Cut(l, width)))
		}
		b.WriteString("]}\n")
		_, _ = c.Stdout.Write(b.Bytes())
		return
	}
	failures := sum.Failures
	if !o.Capped() {
		o.Head = testDefaultFailures
	}
	head, tail, omitted := llmfmt.Cap(o, failures)
	b.WriteString(`{"runner":` + llmfmt.Quote(sum.Runner) + `,"passed":` + strconv.Itoa(sum.Passed) +
		`,"failed":` + strconv.Itoa(sum.Failed) + `,"skipped":` + strconv.Itoa(sum.Skipped) +
		`,"duration_s":` + dur + `,"failures":[`)
	first := true
	for _, part := range [][]testparse.Failure{head, tail} {
		for _, f := range part {
			if !first {
				b.WriteByte(',')
			}
			first = false
			file := f.File
			if !o.Absolute {
				file = relTo(run.Cwd, file)
			}
			b.WriteString(`{"name":` + llmfmt.Quote(llmfmt.Cut(f.Name, o.Width)) + `,"file":` + llmfmt.Quote(file) +
				`,"line":` + strconv.Itoa(f.Line) + `,"message":` + llmfmt.Quote(llmfmt.Cut(f.Message, width)) + `}`)
		}
	}
	b.WriteString(`],"parsed":true,"exit_code":` + strconv.Itoa(run.ExitCode))
	if omitted > 0 {
		b.WriteString(`,"truncated":` + strconv.Itoa(omitted))
		c.NoteTruncated(omitted)
	}
	b.WriteString("}\n")
	_, _ = c.Stdout.Write(b.Bytes())
}

// relTo is p relative to dir when p is an absolute path below dir (or
// below its physical form), p otherwise.
func relTo(dir, p string) string {
	if !filepath.IsAbs(p) || dir == "" {
		return p
	}
	for _, d := range []string{dir, physical(dir)} {
		if r := llmfmt.Rel(d, p); r != p {
			return r
		}
	}
	return p
}

func physical(dir string) string {
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		return r
	}
	return dir
}

// capWriter keeps what a command prints, at most max bytes: past it the
// oldest half goes. It is safe for the stdout and the stderr copiers of
// one run.
type capWriter struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (w *capWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	if w.max > 0 && len(w.buf) > w.max {
		w.buf = append([]byte(nil), w.buf[len(w.buf)-w.max/2:]...)
	}
	return len(p), nil
}

// Bytes is what was kept.
func (w *capWriter) Bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf
}

// NewCapWriter returns a writer that keeps at most max bytes of output
// (the newest), for gash run --test-summary.
func NewCapWriter(max int) interface {
	Write([]byte) (int, error)
	Bytes() []byte
} {
	return &capWriter{max: max}
}
