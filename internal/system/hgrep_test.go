// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/runner"
)

// histFixture is a history file with a duplicate, equal epochs, a record
// whose command is another #EPOCH line, a self-reference, a tab and a
// chain.
const histFixture = "#1700000000\ngit push origin main\n#1700000100\n  git status  \n#1700000200\ngit push origin main\n" +
	"#1700000200\nzeta tie git\n#1700000200\nalpha tie git\n#99\nGIT upper\n#abc\ngit not-a-record\n" +
	"#1700000300\n#1700000400\ngit after double\n#1700000500\nhgrep git\n#1700000600\necho a\tb git\n" +
	"#1700000700\ncmd a.b git\n#1700000800\ncd /x && git log\n#1700000900\ncd /x && git log\n#1700001000\ncd /x || ls git\n"

func histFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "h")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

func hgrep(t *testing.T, content string, env map[string]string, args ...string) result {
	t.Helper()
	m, _, _ := tm(t, runner.NewFake(t))
	if env == nil {
		env = map[string]string{}
	}
	env["HISTFILE"] = histFile(t, content)
	return call(m, "hgrep", env, nil, "", args...)
}

func TestHgrepPlain(t *testing.T) {
	r := hgrep(t, histFixture, nil, "git")
	require.Equal(t, 0, r.rc)
	require.Empty(t, r.err)
	require.Equal(t, "[1970-01-01 00:01:39] GIT upper\n[2023-11-14 22:15:00] git status\n"+
		"[2023-11-14 22:16:40] git push origin main\n[2023-11-14 22:16:40] zeta tie git\n"+
		"[2023-11-14 22:16:40] alpha tie git\n[2023-11-14 22:23:20] echo a\tb git\n"+
		"[2023-11-14 22:25:00] cmd a.b git\n[2023-11-14 22:28:20] cd /x && git log\n"+
		"[2023-11-14 22:30:00] cd /x || ls git\n", r.out)

	r = hgrep(t, histFixture, nil, "git", "-n", "2")
	require.Equal(t, "[2023-11-14 22:28:20] cd /x && git log\n[2023-11-14 22:30:00] cd /x || ls git\n", r.out)

	r = hgrep(t, histFixture, nil, "-a", "hgrep")
	require.Equal(t, "[2023-11-14 22:21:40] hgrep git\n", r.out)
	r = hgrep(t, histFixture, nil, "hgrep")
	require.Empty(t, r.out)
}

func TestHgrepJSONCountEmpty(t *testing.T) {
	r := hgrep(t, histFixture, nil, "push", "-j")
	require.Equal(t, `[{"timestamp":"2023-11-14 22:16:40","epoch":1700000200,"count":2,"command":"git push origin main"}]`+"\n", r.out)
	r = hgrep(t, histFixture, nil, "git", "-c")
	require.Equal(t, "9\n", r.out)
	// -c wins over -j.
	r = hgrep(t, histFixture, nil, "git", "-c", "-j")
	require.Equal(t, "9\n", r.out)
	for _, tc := range []struct{ args, out string }{{"zz -j", "[]\n"}, {"zz -c", "0\n"}, {"zz", ""}} {
		r = hgrep(t, histFixture, nil, strings.Fields(tc.args)...)
		require.Equal(t, 0, r.rc)
		require.Equal(t, tc.out, r.out, tc.args)
	}
	r = hgrep(t, "#1700000000\necho \"q\" \\x\n", nil, "echo", "-j")
	require.Equal(t, `[{"timestamp":"2023-11-14 22:13:20","epoch":1700000000,"count":1,"command":"echo \"q\" \\x"}]`+"\n", r.out)
}

func TestHgrepArguments(t *testing.T) {
	r := hgrep(t, histFixture, nil)
	require.Equal(t, 1, r.rc)
	require.Equal(t, "Error: Missing required argument: pattern\nUsage: hgrep <pattern> [options]. Use -h for help.\n", r.err)
	r = hgrep(t, histFixture, nil, "a", "b")
	require.Equal(t, "Error: Multiple patterns not supported: b\n", r.err)
	r = hgrep(t, histFixture, nil, "a", "-x")
	require.Equal(t, "Error: Unknown option: -x. Use -h for help.\n", r.err)
	for _, args := range [][]string{{"a", "-n"}, {"a", "-n", "x"}, {"a", "--limit", ""}} {
		r = hgrep(t, histFixture, nil, args...)
		require.Equal(t, 1, r.rc)
		require.Equal(t, "Error: Option -n requires a numeric argument\n", r.err)
	}
	// An empty word leaves the pattern unset, as 2.x did.
	r = hgrep(t, histFixture, nil, "", "push", "-c")
	require.Equal(t, "1\n", r.out)

	r = hgrep(t, histFixture, nil, "-h")
	require.Equal(t, 0, r.rc)
	require.Equal(t, registeredHelp(t, "hgrep"), r.out)

	m, _, _ := tm(t, runner.NewFake(t))
	r = call(m, "hgrep", map[string]string{"HISTFILE": "/nonexistent/h"}, nil, "", "git")
	require.Equal(t, 1, r.rc)
	require.Equal(t, "Error: Cannot read history file: /nonexistent/h\n", r.err)
	r = call(m, "hgrep", map[string]string{"HOME": "/nonexistent"}, nil, "", "git")
	require.Equal(t, "Error: Cannot read history file: /nonexistent/.bash_history\n", r.err)
}

// The override runs `history -a` between the checks and the search: the
// first call stops there with Go on the hand-off.
func TestHgrepHandoff(t *testing.T) {
	m, _, h := tm(t, runner.NewFake(t))
	r := call(m, "hgrep", map[string]string{"HISTFILE": histFile(t, histFixture), "GASH__HANDOFF": "3"}, nil, "", "push")
	require.Equal(t, result{"", "", 0}, r)
	require.Equal(t, "1", h.String())
	require.True(t, h.closed)

	// Refused arguments and help write nothing on the hand-off.
	h.Reset()
	r = call(m, "hgrep", map[string]string{"HISTFILE": "/nonexistent", "GASH__HANDOFF": "3"}, nil, "", "push")
	require.Equal(t, 1, r.rc)
	require.Empty(t, h.String())

	r = call(m, "hgrep", map[string]string{"HISTFILE": histFile(t, histFixture), "GASH__RESUME": "1"}, nil, "", "push")
	require.Equal(t, "[2023-11-14 22:16:40] git push origin main\n", r.out)
	require.Empty(t, h.String())
}

func TestHgrepInteractive(t *testing.T) {
	m, _, _ := tm(t, runner.NewFake(t))
	env := map[string]string{"HISTFILE": histFile(t, histFixture)}
	r := call(m, "hgrep", env, ttyTerm(env, ""), "", "push")
	require.Equal(t, "\x1b[38;5;245m[2023-11-14 22:16:40]\x1b[0m git \x1b[38;5;214mpush\x1b[0m origin main \x1b[38;5;245m(x2)\x1b[0m\n", r.out)

	r = call(m, "hgrep", env, ttyTerm(env, ""), "", "cd", "-g")
	require.Equal(t, "\x1b[1mcd /x\x1b[0m \x1b[38;5;245m(2 variants)\x1b[0m\n"+
		"  \x1b[38;5;39m├─\x1b[0m \x1b[38;5;245m22:28:20\x1b[0m && git log \x1b[38;5;245m(x2)\x1b[0m\n"+
		"  \x1b[38;5;39m└─\x1b[0m \x1b[38;5;245m22:30:00\x1b[0m || ls git\n", r.out)

	// --no-color keeps the interactive layout without ANSI.
	r = call(m, "hgrep", env, ttyTerm(env, ""), "", "cd", "-g", "--no-color")
	require.Equal(t, "cd /x (2 variants)\n  ├─ 22:28:20 && git log (x2)\n  └─ 22:30:00 || ls git\n", r.out)
	// -H is the plain data-plane rendering.
	r = call(m, "hgrep", env, ttyTerm(env, ""), "", "cd", "-g", "-H")
	require.Equal(t, "[2023-11-14 22:28:20] cd /x && git log\n[2023-11-14 22:30:00] cd /x || ls git\n", r.out)
	// Not a terminal: grouping is silently flat.
	r = call(m, "hgrep", env, nil, "", "cd", "-g")
	require.Equal(t, "[2023-11-14 22:28:20] cd /x && git log\n[2023-11-14 22:30:00] cd /x || ls git\n", r.out)
	// ASCII glyphs outside UTF-8.
	env["LANG"] = "C"
	r = call(m, "hgrep", env, ttyTerm(env, ""), "", "cd", "-g", "--no-color")
	require.Equal(t, "cd /x (2 variants)\n  |- 22:28:20 && git log (x2)\n  `- 22:30:00 || ls git\n", r.out)
}

func TestHgrepPick(t *testing.T) {
	bin := binDir(t, "fzf")
	f := runner.NewFake(t, runner.Expect{
		Argv:   []string{"fzf", "--height=40%", "--reverse", "--no-sort", "--prompt=hgrep> ", "--tiebreak=index"},
		Stdin:  runner.Text("[2023-11-14 22:28:20] cd /x && git log\n[2023-11-14 22:30:00] cd /x || ls git\n"),
		Stdout: "[2023-11-14 22:30:00] cd /x || ls git\n",
	}, runner.Expect{
		Argv:   []string{"fzf", "--height=40%", "--reverse", "--no-sort", "--prompt=hgrep> ", "--tiebreak=index"},
		Status: 130,
	})
	m, _, _ := tm(t, f)
	env := map[string]string{"HISTFILE": histFile(t, histFixture), "PATH": bin}
	r := call(m, "hgrep", env, ttyTerm(env, ""), "", "cd", "--pick")
	require.Equal(t, result{"cd /x || ls git\n", "", 0}, r)
	r = call(m, "hgrep", env, ttyTerm(env, ""), "", "cd", "--pick")
	require.Equal(t, result{"", "", 0}, r)

	r = call(m, "hgrep", env, nil, "", "cd", "--pick")
	require.Equal(t, result{"", "Error: --pick requires an interactive terminal\n", 1}, r)
	env["GASH_HEADLESS"] = "1"
	r = call(m, "hgrep", env, nil, "", "cd", "--pick")
	require.Equal(t, 1, r.rc)
	delete(env, "GASH_HEADLESS")
	env["PATH"] = t.TempDir()
	r = call(m, "hgrep", env, ttyTerm(env, ""), "", "cd", "--pick")
	require.Equal(t, result{"", "\x1b[38;5;196m✗ Error:\x1b[0m --pick requires fzf; install it or drop --pick\n", 1}, r)
	// No match: nothing to pick, status 0 before any check.
	r = call(m, "hgrep", env, nil, "", "zz", "--pick")
	require.Equal(t, result{"", "", 0}, r)
}

// SYS-05: an invalid -E regex is an error with status 1 and no stdout in
// every output mode, also for an empty history (3.0: SYS-05 fixed; 2.x
// printed the empty result with status 0).
func TestLedger_SYS05(t *testing.T) {
	for _, tc := range []string{"-j", "-c", "-H"} {
		r := hgrep(t, histFixture, nil, "-E", "(", tc)
		require.Equal(t, result{"", "Error: Invalid regex: (\n", 1}, r, tc)
	}
	r := hgrep(t, "", nil, "-E", "(", "-j")
	require.Equal(t, result{"", "Error: Invalid regex: (\n", 1}, r)
}

// SYS-06: --json uses the jq-compatible encoder (control bytes as \u00xx,
// so the array parses and round-trips), and one trailing CR of a CRLF
// history file is dropped from both lines (3.0: SYS-06 fixed).
func TestLedger_SYS06(t *testing.T) {
	r := hgrep(t, "#1700000000\necho \x1b[31mred\n#1700000001\necho cr\r\n", nil, "echo", "-j")
	require.Equal(t, "[{\"timestamp\":\"2023-11-14 22:13:20\",\"epoch\":1700000000,\"count\":1,\"command\":\"echo \\u001b[31mred\"},"+
		"{\"timestamp\":\"2023-11-14 22:13:21\",\"epoch\":1700000001,\"count\":1,\"command\":\"echo cr\"}]\n", r.out)
	var got []struct{ Command string }
	require.NoError(t, json.Unmarshal([]byte(r.out), &got))
	require.Equal(t, "echo \x1b[31mred", got[0].Command)
	r = hgrep(t, "#1700000000\r\necho crlf\r\n#1700000001\r\necho two\r\n", nil, "echo", "-j")
	require.Equal(t, "[{\"timestamp\":\"2023-11-14 22:13:20\",\"epoch\":1700000000,\"count\":1,\"command\":\"echo crlf\"},"+
		"{\"timestamp\":\"2023-11-14 22:13:21\",\"epoch\":1700000001,\"count\":1,\"command\":\"echo two\"}]\n", r.out)
}

// SYS-07: `--` ends the options, so a pattern may start with "-"; -h is
// help anywhere before `--` (3.0: SYS-07 fixed).
func TestLedger_SYS07(t *testing.T) {
	h := "#1700000000\ngit push --force\n#1700000001\ngit push\n"
	r := hgrep(t, h, nil, "--", "--force")
	require.Equal(t, result{"[2023-11-14 22:13:20] git push --force\n", "", 0}, r)
	r = hgrep(t, h, nil, "-c", "--", "-h")
	require.Equal(t, result{"0\n", "", 0}, r)
	r = hgrep(t, h, nil, "git", "-h")
	require.Equal(t, result{registeredHelp(t, "hgrep"), "", 0}, r)
	r = hgrep(t, h, nil, "--", "a", "b")
	require.Equal(t, result{"", "Error: Multiple patterns not supported: b\n", 1}, r)
}

// SYS-28: the pattern reaches the matcher verbatim; -E is a Go regexp,
// case-insensitive, on the command as written.
func TestLedger_SYS28(t *testing.T) {
	h := "#1700000000\ncmd C:\\new\n#1700000001\ncmd a.b\n#1700000002\ncmd axb\n#1700000003\nNOSPACE\n"
	r := hgrep(t, h, nil, `C:\new`, "-c")
	require.Equal(t, result{"1\n", "", 0}, r)
	r = hgrep(t, h, nil, "-E", `a\.b`)
	require.Equal(t, result{"[2023-11-14 22:13:21] cmd a.b\n", "", 0}, r)
	r = hgrep(t, h, nil, "-E", `^\S+$`)
	require.Equal(t, result{"[2023-11-14 22:13:23] NOSPACE\n", "", 0}, r)
	// Syntax Go regexp lacks takes the invalid-regex path (3.0: SYS-05
	// fixed, so that is status 1 with no stdout).
	for _, p := range []string{`\<cmd`, `\ycmd`, `(a)\1`} {
		r = hgrep(t, h, nil, "-E", p, "-c")
		require.Equal(t, result{"", "Error: Invalid regex: " + p + "\n", 1}, r, p)
	}
}

// SYS-29: no awk, no warning for a fixed pattern with metacharacters.
func TestLedger_SYS29(t *testing.T) {
	r := hgrep(t, histFixture, nil, "a.b")
	require.Equal(t, result{"[2023-11-14 22:25:00] cmd a.b git\n", "", 0}, r)
}

// SYS-30: -n is a decimal number, never evaluated, at most 9 digits.
func TestLedger_SYS30(t *testing.T) {
	r := hgrep(t, histFixture, nil, "git", "-n", "08", "-c")
	require.Equal(t, "8\n", r.out)
	r = hgrep(t, histFixture, nil, "git", "-n", "0", "-c")
	require.Equal(t, "9\n", r.out)
	r = hgrep(t, histFixture, nil, "git", "-n", "999999999", "-c")
	require.Equal(t, "9\n", r.out)
	for _, v := range []string{"1234567890", "9223372036854775808"} {
		r = hgrep(t, histFixture, nil, "git", "-n", v)
		require.Equal(t, result{"", "Error: Option -n requires a numeric argument\n", 1}, r, v)
	}
}

// SYS-31: equal epochs keep the order of the last executions in the file,
// reversed with -r.
func TestLedger_SYS31(t *testing.T) {
	h := "#1705330400\nzeta tie\n#1705330400\nmid tie\n#1705330400\nalpha tie\n#1705330400\nzeta tie\n"
	r := hgrep(t, h, nil, "tie")
	require.Equal(t, "[2024-01-15 14:53:20] mid tie\n[2024-01-15 14:53:20] alpha tie\n[2024-01-15 14:53:20] zeta tie\n", r.out)
	r = hgrep(t, h, nil, "tie", "-r")
	require.Equal(t, "[2024-01-15 14:53:20] zeta tie\n[2024-01-15 14:53:20] alpha tie\n[2024-01-15 14:53:20] mid tie\n", r.out)
}

// SYS-32: the gawk UTF-8 path in every locale: Unicode simple folding for
// matching and highlighting.
func TestLedger_SYS32(t *testing.T) {
	h := "#1700000000\nÉCOLE upper\n#1700000001\nGIT log\n"
	for _, loc := range []string{"C.UTF-8", "C"} {
		r := hgrep(t, h, map[string]string{"LC_ALL": loc}, "école")
		require.Equal(t, "[2023-11-14 22:13:20] ÉCOLE upper\n", r.out, loc)
	}
	m, _, _ := tm(t, runner.NewFake(t))
	env := map[string]string{"HISTFILE": histFile(t, h), "LC_ALL": "C"}
	r := call(m, "hgrep", env, ttyTerm(env, ""), "", "git")
	require.Equal(t, "\x1b[38;5;245m[2023-11-14 22:13:21]\x1b[0m \x1b[38;5;214mGIT\x1b[0m log\n", r.out)
	r = call(m, "hgrep", env, ttyTerm(env, ""), "", "-E", "g.t|LOG")
	require.Equal(t, "\x1b[38;5;245m[2023-11-14 22:13:21]\x1b[0m \x1b[38;5;214mGIT\x1b[0m \x1b[38;5;214mlog\x1b[0m\n", r.out)
}

// SYS-43: -n N is always the N most recent matches, with -r too (3.0:
// SYS-43 option b; 2.x kept the N oldest with -r).
func TestLedger_SYS43(t *testing.T) {
	h := "#1700000000\necho one\n#1700000100\necho two\n#1700000200\necho three\n"
	r := hgrep(t, h, nil, "echo", "-r", "-n", "1")
	require.Equal(t, "[2023-11-14 22:16:40] echo three\n", r.out)
	r = hgrep(t, h, nil, "echo", "-r", "-n", "2")
	require.Equal(t, "[2023-11-14 22:16:40] echo three\n[2023-11-14 22:15:00] echo two\n", r.out)
	r = hgrep(t, h, nil, "echo", "-n", "2")
	require.Equal(t, "[2023-11-14 22:15:00] echo two\n[2023-11-14 22:16:40] echo three\n", r.out)
}

// SYS-47: one help body for `hgrep -h` and gash_help, listing every option
// of the grammar, `-r` as newest first, and examples whose timestamps are
// their epochs in UTC.
func TestLedger_SYS47(t *testing.T) {
	body := registeredHelp(t, "hgrep")
	for _, flag := range []string{"-n, --limit", "-a, --all", "-r, --reverse", "-j, --json", "-H, --no-highlight",
		"-E, --regex", "-c, --count", "-g, --group", "--pick", "--no-color", "-h, --help", "--  "} {
		require.Contains(t, body, flag)
	}
	require.Contains(t, body, "-r, --reverse       Newest first (default: oldest first)")
	require.Contains(t, body, `"timestamp":"`+time.Unix(1705330321, 0).UTC().Format("2006-01-02 15:04:05")+`","epoch":1705330321`)
	require.Contains(t, body, "["+time.Unix(1705330321, 0).UTC().Format("2006-01-02 15:04:05")+"] git push origin main")
}

// registeredHelp is the registered help page of name, as gash_help prints
// it without colors.
func registeredHelp(t *testing.T, name string) string {
	t.Helper()
	f, ok := registry.Lookup(name)
	require.True(t, ok)
	var b strings.Builder
	require.True(t, help.Display(&b, false, f))
	return b.String()
}
