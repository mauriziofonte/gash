// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/llm"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/testparse"
)

// testOutputCap bounds the output a test run keeps in memory: past it
// the oldest half is dropped (the summary and the failures come last).
const testOutputCap = 64 << 20

// test is agent_test [--runner auto|NAME] [--fail-only] [--dry-run]
// [budget] -- CMD... (plan L2): it runs a test suite as given (its stdout
// and stderr merged, stdin /dev/null), parses the runner's output and
// prints one JSON object; the exit status is CMD's. A write tool (D1): the
// suite runs the project's own code, which may change files, caches or a
// test database, so it is no llm_* read.
func (m *Module) test(c *invoke.Call) int {
	o := llmfmt.New(llmfmt.BudgetFlags | llmfmt.TimeoutFlag | llmfmt.PathFlags)
	runnerName, failOnly, dry := "auto", false, false
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
		case "--dry-run":
			dry = true
			args = args[1:]
			continue
		}
		if strings.HasPrefix(a, "-") {
			return fail(c, "unknown_option", a, "Options come before --: --runner, --fail-only, --dry-run, --max-lines, --width, --timeout, --absolute")
		}
		cmd = args
		break
	}
	if len(cmd) == 0 {
		return fail(c, "empty_command", "", "Give the test command after --: agent_test -- vendor/bin/pest")
	}
	if !testCommand(cmd) {
		return fail(c, "not_a_test_command", llm.ShellJoin(cmd),
			"agent_test runs a test suite: a runner (pest, phpunit, vitest, jest, mocha, pytest, playwright, ...), "+
				"php artisan test, go test, cargo test, python -m pytest, npm/pnpm/yarn/bun/composer test or make test. "+
				"Run any other command with gash run")
	}
	if dry {
		return m.testDryRun(c, cmd, runnerName)
	}
	out := llm.NewCapWriter(testOutputCap)
	t0 := time.Now()
	res, err := runner.RunUser(c.Ctx, m.run, cmd, runner.Options{
		Env: c.Env.Child(), Stdout: out, Stderr: out, Timeout: o.Timeout, KillAfter: 5 * time.Second,
	})
	if rc, failed := llm.StartFailure(c, cmd, res, err); failed {
		return rc
	}
	llm.WriteTestSummary(c, o, llm.TestRun{Argv: cmd, Runner: runnerName, Output: out.Bytes(), ExitCode: res.ExitCode,
		Duration: time.Since(t0), FailOnly: failOnly, Cwd: m.logicalCwd(c)})
	return res.ExitCode
}

// testDryRun reports what agent_test would run, and runs nothing:
// {"argv":[...],"runner":R,"cwd":DIR,"dry_run":true,"written":false},
// runner the parser CMD names (null when only its output would tell).
func (m *Module) testDryRun(c *invoke.Call, cmd []string, runnerName string) int {
	if runnerName == "auto" {
		runnerName = testparse.Hint(cmd)
	}
	var b strings.Builder
	b.WriteString(`{"argv":[`)
	for i, a := range cmd {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(llmfmt.Quote(a))
	}
	b.WriteString(`],"runner":`)
	if runnerName == "" {
		b.WriteString("null")
	} else {
		b.WriteString(llmfmt.Quote(runnerName))
	}
	b.WriteString(`,"cwd":` + llmfmt.Quote(m.logicalCwd(c)) + `,"dry_run":true,"written":false}` + "\n")
	_, _ = c.Stdout.Write([]byte(b.String()))
	return 0
}

// logicalCwd is the caller's $PWD when it names the working directory,
// the physical directory otherwise (the directory llm_* report).
func (m *Module) logicalCwd(c *invoke.Call) string {
	pwd := c.Env.Get("PWD")
	cwd, err := m.getwd()
	if err != nil {
		return pwd
	}
	if filepath.IsAbs(pwd) {
		a, err1 := os.Stat(pwd)
		b, err2 := os.Stat(cwd)
		if err1 == nil && err2 == nil && os.SameFile(a, b) {
			return pwd
		}
	}
	return cwd
}

// helpOnly prints the help of the call's function (--help before --;
// after it the word belongs to CMD).
func helpOnly(c *invoke.Call) int {
	saved := c.Args
	c.Args = []string{"--help"}
	helpAnywhere(c)
	c.Args = saved
	return 0
}

// testRunners are the programs agent_test runs by name (a path such as
// vendor/bin/phpunit counts by its last element, .phar dropped).
var testRunners = map[string]bool{
	"pest": true, "phpunit": true, "paratest": true, "phpstan": true, "psalm": true,
	"vitest": true, "jest": true, "mocha": true, "playwright": true, "pytest": true,
}

var (
	phpProgram    = lazyre.MustCompile(`^php[0-9.]*$`)
	pythonProgram = lazyre.MustCompile(`^python[0-9.]*$`)
)

// testCommand reports whether argv runs a test suite, the one kind of
// command agent_test runs (gash run takes any other): a test runner, or a
// test runner through php, node, python -m or a package runner, go test,
// cargo test, php artisan test, the test script of npm, pnpm, yarn, bun or
// composer, make test.
func testCommand(argv []string) bool {
	base := func(i int) string {
		if i >= len(argv) {
			return ""
		}
		a := argv[i]
		return strings.TrimSuffix(a[strings.LastIndexByte(a, '/')+1:], ".phar")
	}
	arg := func(i int) string {
		if i >= len(argv) {
			return ""
		}
		return argv[i]
	}
	prog := base(0)
	switch {
	case testRunners[prog]:
		return true
	case phpProgram.MatchString(prog):
		return testRunners[base(1)] || base(1) == "artisan" && arg(2) == "test"
	case prog == "node":
		return testRunners[strings.TrimSuffix(strings.TrimSuffix(base(1), ".mjs"), ".js")]
	case pythonProgram.MatchString(prog):
		return arg(1) == "-m" && (arg(2) == "pytest" || arg(2) == "unittest")
	case prog == "go" || prog == "cargo":
		return arg(1) == "test"
	case prog == "npx" || prog == "bunx" || prog == "pnpx":
		return testRunners[base(1)]
	case prog == "npm" || prog == "pnpm" || prog == "yarn" || prog == "bun" || prog == "composer":
		switch arg(1) {
		case "test", "t":
			return true
		case "run", "run-script":
			return strings.HasPrefix(arg(2), "test")
		case "exec", "x":
			return testRunners[base(2)]
		}
		return prog == "yarn" && strings.HasPrefix(arg(1), "test")
	case prog == "make":
		return strings.HasPrefix(arg(1), "test")
	}
	return false
}
