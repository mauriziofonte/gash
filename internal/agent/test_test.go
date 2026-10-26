// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

type testOut struct {
	Runner   *string  `json:"runner"`
	Passed   int      `json:"passed"`
	Failed   int      `json:"failed"`
	Skipped  int      `json:"skipped"`
	Duration *float64 `json:"duration_s"`
	Failures []struct {
		Name, File, Message string
		Line                int
	} `json:"failures"`
	Parsed    bool     `json:"parsed"`
	ExitCode  int      `json:"exit_code"`
	Tail      []string `json:"tail"`
	Truncated int      `json:"truncated"`
}

// runTest runs agent_test ARGS with env, from dir.
func runTest(t *testing.T, dir string, env map[string]string, args ...string) result {
	t.Helper()
	m := New()
	m.getwd = func() (string, error) { return dir, nil }
	tbl := invoke.NewTable()
	m.Register(tbl)
	var out, errb bytes.Buffer
	e := invoke.EnvMap(env)
	rc := tbl.Run(&invoke.Call{Ctx: context.Background(), Name: "agent_test", Args: args, Env: e,
		Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, false, false)})
	return result{out.String(), errb.String(), rc}
}

func parseTest(t *testing.T, r result) testOut {
	t.Helper()
	var o testOut
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &o), r.Stdout+r.Stderr)
	require.NotNil(t, o.Duration)
	return o
}

// TestAgentTest: agent_test runs CMD, summarizes the runner's output and
// returns CMD's exit status; output no parser reads gives its last lines.
func TestAgentTest(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("..", "testparse", "testdata", "pytest_fail.txt"))
	require.NoError(t, err)
	// Each stub is a test runner by name: agent_test runs nothing else.
	bin := physDir(t)
	for name, script := range map[string]string{
		"pytest":         "#!/bin/sh\ncat '" + fixture + "'\necho noise >&2\nexit 1\n",
		"green/pytest":   "#!/bin/sh\necho '============== 4 passed in 0.02s =============='\n",
		"chatter/mocha":  "#!/bin/sh\nfor i in 1 2 3 4 5; do echo line $i; done\nexit 3\n",
		"args/vitest":    "#!/bin/sh\necho \"$1\" >&2\nexit 4\n",
		"noexec/phpunit": "not a program\n",
	} {
		mode := os.FileMode(0o755)
		if name == "noexec/phpunit" {
			mode = 0o644
		}
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(bin, name)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte(script), mode))
	}
	d := physDir(t)
	env := map[string]string{"HOME": physDir(t), "PWD": d, "GASH_HEADLESS": "1", "PATH": bin + ":" + os.Getenv("PATH")}
	green, chatter, args := filepath.Join(bin, "green", "pytest"), filepath.Join(bin, "chatter", "mocha"), filepath.Join(bin, "args", "vitest")

	r := runTest(t, d, env, "--", "pytest", "-x")
	require.Equal(t, 1, r.RC, r.Stderr)
	require.Empty(t, r.Stderr)
	o := parseTest(t, r)
	require.Equal(t, "pytest", *o.Runner)
	require.Equal(t, []int{3, 2, 1, 1}, []int{o.Passed, o.Failed, o.Skipped, o.ExitCode})
	require.True(t, o.Parsed)
	require.Equal(t, "test_div", o.Failures[0].Name)
	require.Equal(t, "tests/test_math.py", o.Failures[0].File)
	require.Equal(t, 12, o.Failures[0].Line)
	require.Equal(t, "ZeroDivisionError: division by zero", o.Failures[0].Message)

	// Budget and width; no -- (CMD starts at the first plain word).
	o = parseTest(t, runTest(t, d, env, "--max-lines", "1", "--width", "10", "pytest"))
	require.Len(t, o.Failures, 1)
	require.Equal(t, 1, o.Truncated)
	require.Equal(t, "ZeroDivis…", o.Failures[0].Message)

	// --runner forces a parser: pytest output read as go test parses nothing.
	o = parseTest(t, runTest(t, d, env, "--runner", "go", "--", "pytest"))
	require.False(t, o.Parsed)
	require.Nil(t, o.Runner)

	// Green with --fail-only prints nothing; without it the counts.
	r = runTest(t, d, env, "--fail-only", "--", green)
	require.Equal(t, result{"", "", 0}, r)
	o = parseTest(t, runTest(t, d, env, "--", green))
	require.Equal(t, 4, o.Passed)
	require.Equal(t, "pytest", *o.Runner)

	// Unparsed output: the last lines, CMD's status.
	r = runTest(t, d, env, "--tail", "2", "--", chatter)
	require.Equal(t, 3, r.RC)
	o = parseTest(t, r)
	require.False(t, o.Parsed)
	require.Equal(t, []string{"line 4", "line 5"}, o.Tail)
	require.Equal(t, 3, o.ExitCode)

	// Options after CMD belong to CMD.
	r = runTest(t, d, env, "--", args, "--runner")
	require.Equal(t, 4, r.RC)
	o = parseTest(t, r)
	require.Equal(t, []string{"--runner"}, o.Tail)

	// Errors before CMD runs.
	for _, tc := range []struct {
		args []string
		code string
		rc   int
	}{
		{[]string{"--"}, "empty_command", 1},
		{[]string{}, "empty_command", 1},
		{[]string{"--runner", "nose", "--", "pytest"}, "unknown_runner", 1},
		{[]string{"--runner"}, "missing_value", 1},
		{[]string{"--verbose", "--", "pytest"}, "unknown_option", 1},
		{[]string{"--", filepath.Join(bin, "none", "jest")}, "command_not_found", 127},
		{[]string{"--", filepath.Join(bin, "noexec", "phpunit")}, "command_not_found", 126},
		{[]string{"--", "bash", "-c", "rm -rf x"}, "not_a_test_command", 1},
		{[]string{"--", "rm", "-rf", "x"}, "not_a_test_command", 1},
	} {
		r := runTest(t, d, env, tc.args...)
		require.Equal(t, tc.rc, r.RC, tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
		require.Empty(t, r.Stdout, tc.args)
	}

	// --help before -- is the help; after it, an argument of CMD.
	r = runTest(t, d, env, "--help")
	require.Contains(t, r.Stdout, "agent_test [--runner")
	for _, f := range []string{"--runner", "--fail-only", "--dry-run", "--max-lines", "--head", "--tail", "--width", "--timeout", "--relative", "--absolute"} {
		require.Contains(t, r.Stdout, f)
	}
	r = runTest(t, d, env, "--", args, "--help")
	require.Equal(t, []string{"--help"}, parseTest(t, r).Tail)
}

// D1: agent_test runs a test suite and nothing else.
func TestTestCommand(t *testing.T) {
	for _, argv := range [][]string{
		{"vendor/bin/pest"}, {"vendor/bin/phpunit", "--filter", "X"}, {"php", "vendor/bin/phpunit"},
		{"php8.3", "artisan", "test"}, {"phpunit.phar"}, {"./node_modules/.bin/vitest", "run"},
		{"node", "node_modules/jest/bin/jest.js"}, {"npx", "playwright", "test"}, {"pytest", "-x"},
		{"python3", "-m", "pytest"}, {"python", "-m", "unittest"}, {"go", "test", "./..."},
		{"cargo", "test"}, {"npm", "test"}, {"npm", "run", "test:unit"}, {"pnpm", "exec", "vitest"},
		{"yarn", "test:e2e"}, {"bun", "test"}, {"composer", "test"}, {"composer", "run-script", "test"},
		{"make", "test"},
	} {
		require.True(t, testCommand(argv), argv)
	}
	for _, argv := range [][]string{
		{"bash", "-c", "pytest"}, {"rm", "-rf", "x"}, {"php", "-r", "unlink('x');"}, {"php", "artisan", "migrate"},
		{"npm", "install"}, {"npm", "run", "build"}, {"go", "run", "."}, {"python3", "x.py"},
		{"node", "x.js"}, {"npx", "rimraf", "x"}, {"make", "install"}, {"sh", "vendor/bin/pest"},
	} {
		require.False(t, testCommand(argv), argv)
	}
}

// --dry-run reports the command, the parser it names and the directory,
// and runs nothing; the gate applies first.
func TestAgentTestDryRun(t *testing.T) {
	bin, d := physDir(t), physDir(t)
	marker := filepath.Join(d, "ran")
	require.NoError(t, os.WriteFile(filepath.Join(bin, "pest"), []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o755))
	env := map[string]string{"HOME": physDir(t), "PWD": d, "GASH_HEADLESS": "1", "PATH": bin + ":" + os.Getenv("PATH")}

	r := runTest(t, d, env, "--dry-run", "--", "vendor/bin/pest", "--filter", "a b")
	require.Equal(t, result{`{"argv":["vendor/bin/pest","--filter","a b"],"runner":"pest","cwd":` +
		string(mustJSON(t, d)) + `,"dry_run":true,"written":false}` + "\n", "", 0}, r)
	r = runTest(t, d, env, "--dry-run", "--runner", "phpunit", "--", "pest")
	require.Contains(t, r.Stdout, `"runner":"phpunit"`)
	r = runTest(t, d, env, "--dry-run", "--", "npm", "test")
	require.Contains(t, r.Stdout, `"runner":null`)
	require.NoFileExists(t, marker)

	r = runTest(t, d, env, "--dry-run", "--", "rm", "-rf", "x")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `"error":"not_a_test_command"`)
	require.Empty(t, r.Stdout)
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// TestAgentTestGo: agent_test on a real go test run. Without -v go test
// names no passing test, so passed and skipped are null (not the number of
// ok packages); with -v a failure is the failing test's own line, not the
// log of the test after it (3.4.2).
func TestAgentTestGo(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is not installed")
	}
	d := physDir(t)
	write(t, filepath.Join(d, "go.mod"), "module example.com/app\n\ngo 1.21\n", 0o644)
	write(t, filepath.Join(d, "app_test.go"), "package app\n\nimport \"testing\"\n\n"+
		"func TestOne(t *testing.T)   {}\nfunc TestTwo(t *testing.T)   { t.Fatalf(\"boom two\") }\nfunc TestThree(t *testing.T) { t.Log(\"three ok\") }\n", 0o644)
	home := physDir(t)
	cache, err := exec.Command(goBin, "env", "GOCACHE").Output()
	require.NoError(t, err)
	env := map[string]string{"HOME": home, "PWD": d, "GASH_HEADLESS": "1", "PATH": filepath.Dir(goBin) + ":/usr/bin:/bin",
		"GOTOOLCHAIN": "local", "GOFLAGS": "-mod=mod", "GOCACHE": strings.TrimSpace(string(cache)), "GOPATH": filepath.Join(home, "go")}
	// The command runs in the working directory of the process.
	t.Chdir(d)

	r := runTest(t, d, env, "--", "go", "test", "./...")
	require.Equal(t, 1, r.RC, r.Stdout+r.Stderr)
	require.Contains(t, r.Stdout, `"runner":"go","passed":null,"failed":1,"skipped":null,`)
	o := parseTest(t, r)
	require.Len(t, o.Failures, 1)
	require.Equal(t, []any{"TestTwo", "app_test.go", 6, "boom two"},
		[]any{o.Failures[0].Name, o.Failures[0].File, o.Failures[0].Line, o.Failures[0].Message})

	r = runTest(t, d, env, "--", "go", "test", "-v", "./...")
	require.Equal(t, 1, r.RC, r.Stdout+r.Stderr)
	o = parseTest(t, r)
	require.Equal(t, []int{2, 1, 0}, []int{o.Passed, o.Failed, o.Skipped})
	require.Len(t, o.Failures, 1)
	require.Equal(t, []any{"TestTwo", "app_test.go", 6, "boom two"},
		[]any{o.Failures[0].Name, o.Failures[0].File, o.Failures[0].Line, o.Failures[0].Message})
}
