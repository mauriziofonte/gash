// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/remote/sshtest"
)

// runFixture is a fake remote (sshtest) with three registered hosts, a
// remote home holding a few files and a remote PATH whose uptime sleeps
// (the timeout cases).
type runFixture struct {
	home string
	env  map[string]string
	ssh  *sshtest.SSH
	m    *Module
}

func newRunFixture(t *testing.T) *runFixture {
	t.Helper()
	rhome := physTemp(t)
	home := physTemp(t)
	mustWrite(t, filepath.Join(home, ".gash_env"), "HOST:web=deploy@web.example\nHOST:db=deploy@db.example\nHOST:cache=ops@cache.example\n")
	mustWrite(t, filepath.Join(rhome, "app.log"), "one\ntwo\nthree\n")
	var many strings.Builder
	for i := 1; i <= 300; i++ {
		fmt.Fprintf(&many, "line %d\n", i)
	}
	mustWrite(t, filepath.Join(rhome, "many.txt"), many.String())
	stub := physTemp(t)
	mustWrite(t, filepath.Join(stub, "uptime"), "#!/bin/sh\nexec sleep 5\n")
	require.NoError(t, os.Chmod(filepath.Join(stub, "uptime"), 0o755))
	ssh := &sshtest.SSH{Home: rhome, Path: stub + ":" + os.Getenv("PATH")}
	m := New()
	m.run = ssh
	return &runFixture{home: rhome, env: baseEnv(home, nil), ssh: ssh, m: m}
}

func (f *runFixture) run(t *testing.T, args ...string) result {
	t.Helper()
	return runGo(t, f.m, f.home, f.env, "llm_remote_run", args...)
}

func rows(t *testing.T, s string) []map[string]any {
	t.Helper()
	var out []map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &out), s)
	return out
}

func TestRemoteRunFansOut(t *testing.T) {
	f := newRunFixture(t)
	r := f.run(t, "web", "db", "cache", "--", "wc -l app.log; cat nope")
	require.Equal(t, 0, r.RC, r.Stderr)
	got := rows(t, r.Stdout)
	require.Len(t, got, 3)
	for i, h := range []string{"web", "db", "cache"} {
		require.Equal(t, h, got[i]["host"])
		require.Equal(t, float64(1), got[i]["rc"], "cat nope fails")
		require.Equal(t, "3 app.log", got[i]["out"])
		require.Contains(t, got[i]["err"], "nope")
		require.Nil(t, got[i]["error"])
		require.Contains(t, got[i], "ms")
	}
	require.Regexp(t, `^\[\{"host":"web","rc":1,"ms":[0-9.]+,"out":"3 app.log","err":"[^"]*nope[^"]*","error":null\},`, r.Stdout)
	require.Equal(t, 3, f.ssh.N(), "one invocation per host")
	for i, s := range f.ssh.Scripts {
		require.Equal(t, "unset HISTFILE HISTSIZE; sh -s", s, "the script goes on stdin")
		require.Contains(t, string(f.ssh.Stdins[i]), "__llm_cx_grep", "the llm_exec prelude runs on the host")
	}
}

func TestRemoteRunSeveralWordsAreOneCommand(t *testing.T) {
	f := newRunFixture(t)
	r := f.run(t, "web", "--", "grep", "-c", "t", "app.log")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, "2", rows(t, r.Stdout)[0]["out"])
	require.Equal(t, `grep -c t app.log`, joinWords([]string{"grep", "-c", "t", "app.log"}))
	require.Equal(t, `grep 'a b' '' f`, joinWords([]string{"grep", "a b", "", "f"}))
	require.Equal(t, `ls -la | wc -l`, joinWords([]string{"ls -la | wc -l"}))
}

func TestRemoteRunStepsInOneInvocation(t *testing.T) {
	f := newRunFixture(t)
	r := f.run(t, "web", "--step", "cat app.log", "--step", "cat nope", "--step", "echo done")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, 1, f.ssh.N(), "N steps, one ssh invocation")
	got := rows(t, r.Stdout)
	require.Len(t, got, 3)
	require.Equal(t, float64(1), got[0]["step"])
	require.Equal(t, float64(0), got[0]["rc"])
	require.Equal(t, "one\ntwo\nthree", got[0]["out"])
	require.Equal(t, float64(1), got[1]["rc"])
	require.Contains(t, got[1]["out"], "nope", "stderr is in out")
	require.Equal(t, "done", got[2]["out"])
	require.IsType(t, float64(0), got[2]["ms"], "GNU date gives ms")
	require.Empty(t, r.Stderr)

	r = f.run(t, "web", "--step", "cat app.log", "--step", "cat nope", "--step", "echo done", "--stop-on-error")
	require.Equal(t, 0, r.RC)
	require.Len(t, rows(t, r.Stdout), 2)
	require.Equal(t, `{"skipped":1,"stopped":true}`+"\n", r.Stderr)
}

func TestRemoteRunBudget(t *testing.T) {
	f := newRunFixture(t)
	r := f.run(t, "web", "--", "cat many.txt")
	out := rows(t, r.Stdout)[0]["out"].(string)
	lines := strings.Split(out, "\n")
	require.Len(t, lines, 201)
	require.Equal(t, "line 100", lines[99])
	require.Equal(t, "[... 100 lines omitted]", lines[100])
	require.Equal(t, "line 300", lines[200])

	r = f.run(t, "web", "--tail", "2", "--width", "5", "--", "cat many.txt")
	require.Equal(t, "[... 298 lines omitted]\nline…\nline…", rows(t, r.Stdout)[0]["out"])

	r = f.run(t, "web", "--format", "tsv", "--step", "cat app.log")
	require.Equal(t, "step\trc\tms\tout\n", r.Stdout[:len("step\trc\tms\tout\n")])
	require.Contains(t, r.Stdout, "\t0\t")
	require.Contains(t, r.Stdout, "\tone\\ntwo\\nthree\n")
}

func TestRemoteRunRefusesBeforeConnecting(t *testing.T) {
	f := newRunFixture(t)
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"web", "--", "rm -rf /tmp/x"}, "dangerous_command_blocked"},
		{[]string{"web", "--", "touch x"}, "command_not_allowed"},
		{[]string{"web", "--", "cat .env"}, "secret_file_blocked"},
		{[]string{"web", "--", "cat /proc/1/environ"}, "forbidden_path"},
		{[]string{"web", "--", "cat ../../root/x"}, "forbidden_path"},
		{[]string{"web", "--", "ls /var/log/*.log"}, "remote_glob_unsupported"},
		{[]string{"web", "--", "cat ~/notes"}, "remote_glob_unsupported"},
		{[]string{"web", "--step", "uptime", "--step", "tail -n 5 app.log > x"}, ""},
		{[]string{"nope", "--", "uptime"}, "unknown_host"},
		{[]string{"deploy@web.example", "--", "uptime"}, "unknown_host"},
		{[]string{"--", "uptime"}, "missing_host"},
		{[]string{"web"}, "empty_command"},
		{[]string{"web", "--", ""}, "empty_command"},
		{[]string{"web", "--step", "uptime", "--", "uptime"}, "conflicting_options"},
		{[]string{"web", "db", "--step", "uptime"}, "unexpected_argument"},
		{[]string{"web", "--stop-on-error", "--", "uptime"}, "conflicting_options"},
		{[]string{"web", "--bogus", "--", "uptime"}, "unknown_option"},
		{[]string{"web", "--step"}, "missing_value"},
	} {
		r := f.run(t, tc.args...)
		require.Equal(t, 1, r.RC, "%q", tc.args)
		if tc.code != "" {
			require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, "%q", tc.args)
		}
	}
	require.Zero(t, f.ssh.N(), "nothing connected")
}

func TestRemoteRunQuotedGlobsAndHelp(t *testing.T) {
	f := newRunFixture(t)
	r := f.run(t, "web", "--", `grep -c '^o*n' app.log`)
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, "1", rows(t, r.Stdout)[0]["out"])
	r = f.run(t, "web", "--", "echo", "-h")
	require.Equal(t, "-h", rows(t, r.Stdout)[0]["out"], "-h after -- is the command's")
	r = f.run(t, "web", "--step", "-h")
	require.Contains(t, r.Stdout, "llm_remote_run HOST...")
	for line, want := range map[string]string{
		"ls *.log": "*.log", "cat a?b": "a?b", "ls [ab]x": "[ab]x", "cat ~/x": "~/x", "echo a~b": "", "[ -f x ]": "",
		"echo '*'": "", `echo "*"`: "", `echo \*`: "", "grep -e x|wc -l": "", "x;ls *": "*", `echo "a\"*"`: "",
	} {
		got, bad := localExpansion(line)
		require.Equal(t, want, got, line)
		require.Equal(t, want != "", bad, line)
	}
}

func TestRemoteRunUnreachableAndTimeout(t *testing.T) {
	f := newRunFixture(t)
	r := f.run(t, "web", "db", "--timeout", "0.5", "--", "uptime")
	require.Equal(t, 0, r.RC, r.Stderr)
	got := rows(t, r.Stdout)
	require.Nil(t, got[0]["rc"])
	require.Equal(t, "timeout", got[0]["error"])

	r = f.run(t, "web", "--timeout", "0.5", "--step", "echo first", "--step", "uptime", "--step", "echo never")
	require.Equal(t, 0, r.RC, r.Stderr)
	got = rows(t, r.Stdout)
	require.Len(t, got, 2)
	require.Equal(t, "first", got[0]["out"])
	require.Equal(t, float64(2), got[1]["step"])
	require.Nil(t, got[1]["rc"])
	require.Equal(t, `{"timed_out":true}`+"\n", r.Stderr)

	f.ssh.Unreachable = true
	r = f.run(t, "web", "db", "--", "uptime")
	require.Equal(t, 0, r.RC)
	for _, row := range rows(t, r.Stdout) {
		require.Equal(t, "host_unreachable", row["error"])
		require.Contains(t, row["err"], "Connection refused")
	}
	r = f.run(t, "web", "--step", "uptime")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `"error":"host_unreachable"`)
	require.Contains(t, r.Stderr, `"action":"RETRY"`)
}

func TestRemoteRunDropsLoginNoise(t *testing.T) {
	require.Equal(t, "out\n", afterBegin([]byte("motd\ngash-x begin\nout\n"), "gash-x"))
	require.Equal(t, "out\n", afterBegin([]byte("out\n"), "gash-x"))
	steps, rest := parseSteps("a\n\nN step 1 0 1700000000000000000 1700000000250000000\nb\nN step 2 3  \npartial", "N")
	require.Len(t, steps, 2)
	require.Equal(t, "a\n", steps[0].out)
	require.Equal(t, 250.0, steps[0].ms)
	require.Equal(t, 3, steps[1].rc)
	require.Nil(t, steps[1].ms)
	require.Equal(t, "partial", rest)
}
