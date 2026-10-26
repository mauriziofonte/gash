// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
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

func parseTest(t *testing.T, r result) testOut {
	t.Helper()
	var o testOut
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &o), r.Stdout+r.Stderr)
	require.NotNil(t, o.Duration)
	return o
}

// TestLLMTest: llm_test runs CMD, summarizes the runner's output and
// returns CMD's exit status; output no parser reads gives its last lines.
func TestLLMTest(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("..", "testparse", "testdata", "pytest_fail.txt"))
	require.NoError(t, err)
	bin := physTemp(t)
	for name, script := range map[string]string{
		"pytest":  "#!/bin/sh\ncat '" + fixture + "'\necho noise >&2\nexit 1\n",
		"green":   "#!/bin/sh\necho '============== 4 passed in 0.02s =============='\n",
		"chatter": "#!/bin/sh\nfor i in 1 2 3 4 5; do echo line $i; done\nexit 3\n",
		"noexec":  "not a program\n",
	} {
		mode := os.FileMode(0o755)
		if name == "noexec" {
			mode = 0o644
		}
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte(script), mode))
	}
	env := baseEnv(physTemp(t), map[string]string{"PATH": bin + ":" + os.Getenv("PATH")})
	m := New()
	d := physTemp(t)

	r := runGo(t, m, d, env, "llm_test", "--", "pytest", "-x")
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
	o = parseTest(t, runGo(t, m, d, env, "llm_test", "--max-lines", "1", "--width", "10", "pytest"))
	require.Len(t, o.Failures, 1)
	require.Equal(t, 1, o.Truncated)
	require.Equal(t, "ZeroDivis…", o.Failures[0].Message)

	// --runner forces a parser: pytest output read as go test parses nothing.
	o = parseTest(t, runGo(t, m, d, env, "llm_test", "--runner", "go", "--", "pytest"))
	require.False(t, o.Parsed)
	require.Nil(t, o.Runner)

	// Green with --fail-only prints nothing; without it the counts.
	r = runGo(t, m, d, env, "llm_test", "--fail-only", "--", "green")
	require.Equal(t, result{"", "", 0}, r)
	o = parseTest(t, runGo(t, m, d, env, "llm_test", "--", "green"))
	require.Equal(t, 4, o.Passed)
	require.Equal(t, "pytest", *o.Runner)

	// Unparsed output: the last lines, CMD's status.
	r = runGo(t, m, d, env, "llm_test", "--tail", "2", "--", "chatter")
	require.Equal(t, 3, r.RC)
	o = parseTest(t, r)
	require.False(t, o.Parsed)
	require.Equal(t, []string{"line 4", "line 5"}, o.Tail)
	require.Equal(t, 3, o.ExitCode)

	// A shell line runs through bash; options after CMD belong to CMD.
	r = runGo(t, m, d, env, "llm_test", "--", "bash", "-c", "echo $0 >&2; exit 4", "--runner")
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
		{[]string{"--runner", "nose", "--", "true"}, "unknown_runner", 1},
		{[]string{"--runner"}, "missing_value", 1},
		{[]string{"--verbose", "--", "true"}, "unknown_option", 1},
		{[]string{"--", "no-such-program-zz"}, "command_not_found", 127},
		{[]string{"--", "noexec"}, "command_not_found", 126},
	} {
		r := runGo(t, m, d, env, "llm_test", tc.args...)
		require.Equal(t, tc.rc, r.RC, tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
		require.Empty(t, r.Stdout, tc.args)
	}

	// --help before -- is the help; after it, an argument of CMD.
	r = runGo(t, m, d, env, "llm_test", "--help")
	require.Contains(t, r.Stdout, "llm_test [--runner")
	r = runGo(t, m, d, env, "llm_test", "--", "bash", "-c", "echo $0", "--help")
	require.Equal(t, []string{"--help"}, parseTest(t, r).Tail)
}
