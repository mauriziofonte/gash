// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/agent"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llm"
	"github.com/mauriziofonte/gash/internal/term"
)

type result struct {
	Stdout, Stderr string
	RC             int
}

func phys(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return d
}

func stub(t *testing.T, dir, name, script string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script), 0o755))
}

// fakeNVM builds node 20.11.1 and 24.1.0 under home's nvm tree.
func fakeNVM(t *testing.T, home string) string {
	t.Helper()
	root := filepath.Join(home, ".nvm", "versions", "node")
	for _, v := range []string{"20.11.1", "24.1.0"} {
		stub(t, filepath.Join(root, "v"+v, "bin"), "node", "echo v"+v+"\n")
	}
	return root
}

// command runs `gash WORD ARGS...` against a table with the commands and
// the llm and agent functions, in cwd.
func command(t *testing.T, cwd string, env map[string]string, word string, args ...string) result {
	t.Helper()
	t.Chdir(cwd)
	tbl := invoke.NewTable()
	Register(tbl)
	llm.Register(tbl)
	agent.Register(tbl)
	h, ok := tbl.LookupCommand(word)
	require.True(t, ok, word)
	var out, errb bytes.Buffer
	e := invoke.EnvMap(env)
	c := &invoke.Call{Ctx: context.Background(), Name: word, Args: args, Env: e, Stdin: bytes.NewReader(nil),
		Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, false, false), Handlers: tbl}
	rc := h(c)
	return result{out.String(), errb.String(), rc}
}

func baseEnv(t *testing.T, extra map[string]string) map[string]string {
	t.Helper()
	home := phys(t)
	env := map[string]string{"PATH": os.Getenv("PATH"), "HOME": home, "XDG_CACHE_HOME": filepath.Join(home, ".cache"),
		"LC_ALL": "C", "GASH_HEADLESS": "1"}
	for k, v := range extra {
		env[k] = v
	}
	return env
}

func errCode(t *testing.T, stderr string) string {
	t.Helper()
	var e struct{ Error string }
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(stderr)), &e), stderr)
	return e.Error
}

// TestRuntimeCommand: gash runtime prints the resolution (or the path),
// writes the cache, and refuses what is not installed with a STOP.
func TestRuntimeCommand(t *testing.T) {
	env := baseEnv(t, nil)
	env["PATH"] = phys(t)
	root := fakeNVM(t, env["HOME"])
	d := phys(t)

	r := command(t, d, env, "runtime", "node", "24")
	require.Equal(t, result{`{"runtime":"node","request":"24","path":"` + root + `/v24.1.0/bin/node","version":"24.1.0"}` + "\n", "", 0}, r)
	_, err := os.Stat(filepath.Join(env["XDG_CACHE_HOME"], "gash", "runtimes.json"))
	require.NoError(t, err, "the cache was not written")
	r = command(t, d, env, "runtime", "--path", "node", "20")
	require.Equal(t, result{root + "/v20.11.1/bin/node\n", "", 0}, r)

	r = command(t, d, env, "runtime", "node", "18")
	require.Equal(t, 1, r.RC)
	require.Equal(t, "runtime_not_installed", errCode(t, r.Stderr))
	require.Contains(t, r.Stderr, "installed: 20.11.1, 24.1.0")

	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"ruby", "3"}, "unknown_runtime"},
		{[]string{"node", "x"}, "invalid_version"},
		{[]string{"node", "24", "25"}, "unexpected_argument"},
		{[]string{"node", "24", "--json"}, "unknown_option"},
	} {
		r := command(t, d, env, "runtime", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Equal(t, tc.code, errCode(t, r.Stderr), tc.args)
	}
	r = command(t, d, env, "runtime", "--help")
	require.Contains(t, r.Stdout, "Usage: gash runtime")
	r = command(t, d, env, "runtime", "__complete", "n")
	require.Equal(t, "node\nnpm\nnpx\n:4\n", r.Stdout)
	r = command(t, d, env, "runtime", "__complete", "node", "--p")
	require.Equal(t, "--path\n:4\n", r.Stdout)
}

// TestRunCommand: gash run keeps CMD's status, shapes its output only
// when asked, summarizes tests, puts runtimes first on PATH, applies the
// llm_exec rails with --safe, runs gash functions in process, and its
// own errors are envelopes.
func TestRunCommand(t *testing.T) {
	env := baseEnv(t, nil)
	bin := phys(t)
	root := fakeNVM(t, env["HOME"])
	fixture, err := filepath.Abs(filepath.Join("..", "testparse", "testdata", "pytest_fail.txt"))
	require.NoError(t, err)
	stub(t, bin, "lines", "i=1; while [ $i -le 10 ]; do echo out$i; echo err$i >&2; i=$((i+1)); done; printf tail; exit 3\n")
	stub(t, bin, "pytest", "cat '"+fixture+"'; exit 1\n")
	env["PATH"] = bin + ":" + env["PATH"]
	d := phys(t)

	// Passthrough: every byte, the status.
	r := command(t, d, env, "run", "--", "lines")
	require.Equal(t, 3, r.RC)
	require.Equal(t, "out1\nout2\nout3\nout4\nout5\nout6\nout7\nout8\nout9\nout10\ntail", r.Stdout)
	require.True(t, strings.HasPrefix(r.Stderr, "err1\n"))
	r = command(t, d, env, "run", "bash", "-c", "exit 42")
	require.Equal(t, result{"", "", 42}, r)

	// Shaped: each stream on its own budget; the last line without a
	// newline is a line.
	r = command(t, d, env, "run", "--max-lines", "2", "--", "lines")
	require.Equal(t, result{"out1\nout2\n[... 9 lines omitted]\n", "err1\nerr2\n[... 8 lines omitted]\n", 3}, r)
	r = command(t, d, env, "run", "--tail", "1", "--width", "3", "lines")
	require.Equal(t, result{"[... 10 lines omitted]\nta…\n", "[... 9 lines omitted]\ner…\n", 3}, r)

	// --test-summary: the llm_test object, CMD's status.
	r = command(t, d, env, "run", "--test-summary", "--", "pytest")
	require.Equal(t, 1, r.RC)
	var sum struct {
		Runner   string
		Failed   int
		ExitCode int `json:"exit_code"`
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &sum), r.Stdout)
	require.Equal(t, "pytest", sum.Runner)
	require.Equal(t, 2, sum.Failed)
	require.Equal(t, 1, sum.ExitCode)

	// --runtime: the nvm node first on PATH.
	r = command(t, d, env, "run", "--runtime", "node:24", "--", "node")
	require.Equal(t, result{"v24.1.0\n", "", 0}, r)
	r = command(t, d, env, "run", "--runtime", "node:20", "--runtime", "node:24", "--", "sh", "-c", "echo $PATH")
	require.True(t, strings.HasPrefix(r.Stdout, root+"/v20.11.1/bin:"+root+"/v24.1.0/bin:"+bin+":"), r.Stdout)
	r = command(t, d, env, "run", "--runtime", "node:18", "--", "node")
	require.Equal(t, "runtime_not_installed", errCode(t, r.Stderr))
	require.Empty(t, r.Stdout)

	// --safe: llm_exec's rails; a refusal runs nothing.
	mustWrite := func(p, s string) { require.NoError(t, os.WriteFile(p, []byte(s), 0o644)) }
	mustWrite(filepath.Join(d, "a.txt"), "alpha\nbeta\n")
	r = command(t, d, env, "run", "--safe", "--", "cp", "a.txt", "b.txt")
	require.Equal(t, 1, r.RC)
	require.Equal(t, "command_not_allowed", errCode(t, r.Stderr))
	_, err = os.Stat(filepath.Join(d, "b.txt"))
	require.True(t, os.IsNotExist(err))
	r = command(t, d, env, "run", "--safe", "--max-lines", "1", "--", "grep", "-n", "a", "a.txt")
	require.Equal(t, result{"1:alpha\n[... 1 lines omitted]\n", "", 0}, r)

	// A gash function runs in process, under the budget.
	r = command(t, d, env, "run", "--max-lines", "1", "--", "llm_find", "*.txt", ".")
	require.Equal(t, result{d + "/a.txt\n", "", 0}, r)

	// gash's own errors.
	for _, tc := range []struct {
		args []string
		code string
		rc   int
	}{
		{[]string{}, "empty_command", 1},
		{[]string{"--"}, "empty_command", 1},
		{[]string{"--verbose", "--", "true"}, "unknown_option", 1},
		{[]string{"--width", "0", "--", "true"}, "invalid_width", 1},
		{[]string{"--runtime"}, "missing_value", 1},
		{[]string{"--runtime", "node24", "--", "true"}, "unknown_runtime", 1},
		{[]string{"--", "no-such-program-zz"}, "command_not_found", 127},
	} {
		r := command(t, d, env, "run", tc.args...)
		require.Equal(t, tc.rc, r.RC, tc.args)
		require.Equal(t, tc.code, errCode(t, r.Stderr), tc.args)
	}
	r = command(t, d, env, "run", "--help")
	require.Contains(t, r.Stdout, "Usage: gash run")
	r = command(t, d, env, "run", "--", "sh", "-c", "echo $0", "--help")
	require.Equal(t, result{"--help\n", "", 0}, r)

	// Telemetry: one "gash run" line.
	tel := filepath.Join(d, "tel.jsonl")
	env["GASH_TELEMETRY_FILE"] = tel
	r = command(t, d, env, "run", "--max-lines", "1", "--", "lines")
	require.Equal(t, 3, r.RC)
	data, err := os.ReadFile(tel)
	require.NoError(t, err)
	require.Regexp(t, `^\{"ts":.*"fn":"gash run".*"rc":3.*"truncated":19\}\n$`, string(data))

	// Completion.
	r = command(t, d, env, "run", "__complete", "--runtime", "")
	require.Equal(t, "node:\nphp:\n:6\n", r.Stdout)
	r = command(t, d, env, "run", "__complete", "--te")
	require.Equal(t, "--test-summary\n:4\n", r.Stdout)
	r = command(t, d, env, "run", "__complete", "--", "")
	require.Equal(t, ":0\n", r.Stdout)
}

// TestAgentFunctions: every llm_* and agent_* function with a handler,
// its family, usage, flags and replaces.
func TestAgentFunctions(t *testing.T) {
	env := baseEnv(t, nil)
	d := phys(t)
	r := command(t, d, env, "agent-functions", "--json")
	require.Equal(t, 0, r.RC, r.Stderr)
	var rows []agentFunction
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &rows))
	by := map[string]agentFunction{}
	for _, row := range rows {
		by[row.Name] = row
		require.True(t, strings.HasPrefix(row.Name, "llm_") || strings.HasPrefix(row.Name, "agent_"), row.Name)
		require.NotEmpty(t, row.Short, row.Name)
		require.NotEmpty(t, row.Usage, row.Name)
	}
	lint := by["llm_lint"]
	require.Equal(t, "read", lint.Family)
	require.Equal(t, []string{"--ext", "--php", "--node", "-j", "--timeout", "--max-lines", "--width", "--absolute"}, lint.Flags)
	require.NotNil(t, lint.Replaces)
	require.Contains(t, *lint.Replaces, "php -l")
	require.Equal(t, []string{}, by["llm_runtime"].Flags)
	require.Contains(t, by, "llm_find")
	for name, row := range by {
		if strings.HasPrefix(name, "agent_") {
			require.Equal(t, "write", row.Family)
		}
	}
	require.Contains(t, r.Stdout, `"replaces":null`)

	r = command(t, d, env, "agent-functions")
	require.Regexp(t, `(?m)^llm_lint +read +Syntax-check files`, r.Stdout)
	require.Equal(t, len(rows), strings.Count(r.Stdout, "\n"))
	r = command(t, d, env, "agent-functions", "--yaml")
	require.Equal(t, "unknown_option", errCode(t, r.Stderr))
	r = command(t, d, env, "agent-functions", "llm")
	require.Equal(t, "unexpected_argument", errCode(t, r.Stderr))
	r = command(t, d, env, "agent-functions", "__complete", "--j")
	require.Equal(t, "--json\n:4\n", r.Stdout)
}

// TestShellJoin: the --safe line quotes what is not a plain word.
func TestShellJoin(t *testing.T) {
	require.Equal(t, `grep -n 'a b' '' 'it'\''s' x.txt`, llm.ShellJoin([]string{"grep", "-n", "a b", "", "it's", "x.txt"}))
}
