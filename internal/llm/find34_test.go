// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/runner"
)

// walkFixture is a small tree for the 3.4 walker modes: files of known
// sizes and mtimes, the noise directories, a hidden directory and a
// symlink.
func walkFixture(t *testing.T) string {
	t.Helper()
	d := physTemp(t)
	files := []struct {
		rel, content string
		day          int // mtime: 2026-01-<day>
	}{
		{"README.md", "readme\n", 1},
		{"src/a.php", "<?php // TODO one\n", 5},
		{"src/b.js", "x\n// TODO js\n", 3},
		{"src/sub/c.php", "<?php\n// TODO two\n// TODO three TODO\n", 9},
		{"src/sub/deep/d.PHP", "<?php\n", 7},
		{".hidden/h.php", "<?php // TODO hidden\n", 2},
		{"vendor/v.php", "<?php // TODO vendor\n", 4},
		{"node_modules/n.js", "// TODO nm\n", 4},
		{"__pycache__/p.php", "<?php\n", 4},
		{"tests/t.php", "<?php // TODO test\n", 6},
	}
	for _, f := range files {
		p := filepath.Join(d, f.rel)
		mustWrite(t, p, f.content)
		mt := time.Date(2026, 1, f.day, 12, 0, 0, 0, time.UTC)
		require.NoError(t, os.Chtimes(p, mt, mt))
	}
	require.NoError(t, os.Symlink("src/a.php", filepath.Join(d, "link.php")))
	return d
}

// lines splits an output into its lines (none for "").
func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// TestFind34_DefaultsUnchanged: --absolute alone (today's default) and
// the 3.0 calls print what they printed.
func TestFind34_DefaultsUnchanged(t *testing.T) {
	d := walkFixture(t)
	env := baseEnv(physTemp(t), nil)
	m := New()
	for _, args := range [][]string{
		{"*"}, {"*.php"}, {"*", "--type", "d"}, {"*", "-n", "2"}, {"*", "src"}, {"a.php", "src/a.php"},
		{"*", "--contains", "TODO"}, {"*.php", "--contains", "TODO", "-n", "1"},
	} {
		want := runGo(t, m, d, env, "llm_find", args...)
		got := runGo(t, m, d, env, "llm_find", append(args, "--absolute")...)
		require.Equal(t, want, got, args)
		require.Equal(t, 0, want.RC, want.Stderr)
	}
}

// TestFind34_CountMatchesListing: --count is the number of paths the same
// listing prints without -n, and the --by counts add up to it.
func TestFind34_CountMatchesListing(t *testing.T) {
	d := walkFixture(t)
	env := baseEnv(physTemp(t), nil)
	m := New()
	for _, args := range [][]string{
		{"*"}, {"*.php"}, {"*.PHP"}, {"*", "--type", "d"}, {"*", "--all"}, {"*", "--type", "d", "--all"},
		{"*", "src"}, {"src", ".", "--type", "d"}, {"vendor", ".", "--type", "d"}, {"*", "src/a.php"},
		{"*", "--since", "2026-01-05T00:00"}, {"*", "--newer", "src/b.js"}, {"*", "--contains", "TODO"},
		{"*.php", "--contains", "TODO", "--all"}, {"*", "vendor"},
	} {
		list := runGo(t, m, d, env, "llm_find", append(args, "-n", "999999999")...)
		require.Equal(t, 0, list.RC, "%v %s", args, list.Stderr)
		r := runGo(t, m, d, env, "llm_find", append(args, "--count")...)
		require.Equal(t, `{"count":`+strconv.Itoa(len(lines(list.Stdout)))+"}\n", r.Stdout, args)
		for _, by := range []string{"ext", "dir", "type"} {
			r := runGo(t, m, d, env, "llm_find", append(args, "--by", by)...)
			var v struct {
				Count int
				By    map[string]int
			}
			require.NoError(t, json.Unmarshal([]byte(r.Stdout), &v), r.Stdout)
			sum := 0
			for _, n := range v.By {
				sum += n
			}
			require.Equal(t, v.Count, sum, "%v --by %s", args, by)
			if by != "type" || strings.Contains(strings.Join(args, " "), "--type") {
				require.Equal(t, len(lines(list.Stdout)), v.Count, "%v --by %s", args, by)
			}
		}
	}
}

func TestFind34_CountShapes(t *testing.T) {
	d := walkFixture(t)
	env := baseEnv(physTemp(t), nil)
	m := New()
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"*", "--count"}, `{"count":7}`},
		{[]string{"*", "--by", "ext"}, `{"count":7,"by":{"js":1,"md":1,"php":5}}`},
		{[]string{"*", "--by", "dir"}, `{"count":7,"by":{".":1,".hidden":1,"src":4,"tests":1}}`},
		// Every type with --by type and no --type: the root, the
		// directories, the files and the symlink.
		{[]string{"*", "--by", "type"}, `{"count":14,"by":{"d":6,"f":7,"l":1}}`},
		{[]string{"*", "--by", "type", "--type", "f"}, `{"count":7,"by":{"f":7}}`},
		{[]string{"*", "--count", "--all"}, `{"count":10}`},
		{[]string{"*.php", "src", "--count", "--format", "tsv"}, "count\n2"},
		{[]string{"*", "src", "--by", "ext", "--format", "tsv"}, "key\tcount\nphp\t3\njs\t1"},
	} {
		r := runGo(t, m, d, env, "llm_find", tc.args...)
		require.Equal(t, 0, r.RC, "%v %s", tc.args, r.Stderr)
		require.Equal(t, tc.want+"\n", r.Stdout, tc.args)
	}
}

func TestFind34_RelativeSortFilters(t *testing.T) {
	d := walkFixture(t)
	env := baseEnv(physTemp(t), nil)
	m := New()
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"*.php", "--relative"}, []string{".hidden/h.php", "src/a.php", "src/sub/c.php", "tests/t.php"}},
		{[]string{"*.php", "src", "--relative"}, []string{"a.php", "sub/c.php"}},
		{[]string{"*", "src", "--type", "d", "--relative"}, []string{".", "sub", "sub/deep"}},
		{[]string{"*", "src/a.php", "--relative"}, []string{"a.php"}},
		{[]string{"*", "--relative", "-n", "2"}, []string{".hidden/h.php", "README.md"}},
		{[]string{"*", "--relative", "--sort", "mtime"},
			[]string{"src/sub/c.php", "src/sub/deep/d.PHP", "tests/t.php", "src/a.php", "src/b.js", ".hidden/h.php", "README.md"}},
		{[]string{"*", "--relative", "--sort", "mtime", "-n", "2"}, []string{"src/sub/c.php", "src/sub/deep/d.PHP"}},
		{[]string{"*", "--relative", "--sort", "size", "-n", "3"}, []string{"src/sub/c.php", ".hidden/h.php", "tests/t.php"}},
		{[]string{"*", "--relative", "--sort", "name"},
			[]string{".hidden/h.php", "README.md", "src/a.php", "src/b.js", "src/sub/c.php", "src/sub/deep/d.PHP", "tests/t.php"}},
		{[]string{"*", "--relative", "--since", "2026-01-06"}, []string{"src/sub/c.php", "src/sub/deep/d.PHP", "tests/t.php"}},
		{[]string{"*", "--relative", "--since", "2026-01-06T12:00:00Z"}, []string{"src/sub/c.php", "src/sub/deep/d.PHP", "tests/t.php"}},
		{[]string{"*", "--relative", "--newer", "tests/t.php"}, []string{"src/sub/c.php", "src/sub/deep/d.PHP"}},
		{[]string{"*", "--relative", "--since", "@0", "--newer", "src/sub/deep/d.PHP"}, []string{"src/sub/c.php"}},
		{[]string{"*", "--relative", "--since", "1d"}, nil},
		{[]string{"*", "vendor", "--relative", "--all"}, []string{"v.php"}},
		{[]string{"*", "--contains", "TODO", "--relative"}, []string{".hidden/h.php", "src/a.php", "src/b.js", "src/sub/c.php", "tests/t.php"}},
		{[]string{"*", "--contains", "TODO", "--relative", "--sort", "size", "-n", "1"}, []string{"src/sub/c.php"}},
		{[]string{"*", "--contains", "TODO", "--relative", "--since", "2026-01-04"}, []string{"src/a.php", "src/sub/c.php", "tests/t.php"}},
	} {
		r := runGo(t, m, d, env, "llm_find", tc.args...)
		require.Equal(t, 0, r.RC, "%v %s", tc.args, r.Stderr)
		require.Equal(t, tc.want, lines(r.Stdout), tc.args)
		require.Empty(t, r.Stderr, tc.args)
	}
}

func TestFind34_RowsAndBudget(t *testing.T) {
	d := walkFixture(t)
	env := baseEnv(physTemp(t), nil)
	m := New()
	r := runGo(t, m, d, env, "llm_find", "*.php", "src", "--format", "json", "--relative")
	require.Equal(t, result{`[{"p":"a.php","size":18,"mtime":"2026-01-05T12:00:00Z"},{"p":"sub/c.php","size":37,"mtime":"2026-01-09T12:00:00Z"}]` + "\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_find", "*.php", "src", "--format", "jsonl")
	require.Equal(t, result{`{"p":"` + d + `/src/a.php","size":18,"mtime":"2026-01-05T12:00:00Z"}` + "\n" +
		`{"p":"` + d + `/src/sub/c.php","size":37,"mtime":"2026-01-09T12:00:00Z"}` + "\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_find", "*.php", "src", "--format", "tsv", "--relative", "--sort", "size")
	require.Equal(t, result{"p\tsize\tmtime\nsub/c.php\t37\t2026-01-09T12:00:00Z\na.php\t18\t2026-01-05T12:00:00Z\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_find", "*.php", "src", "--format", "table", "--relative")
	require.Equal(t, result{"p          size  mtime\na.php      18    2026-01-05T12:00:00Z\nsub/c.php  37    2026-01-09T12:00:00Z\n", "", 0}, r)

	// -n cuts the rows and says how many on stderr; --head/--tail replace it.
	r = runGo(t, m, d, env, "llm_find", "*", "--format", "tsv", "--relative", "-n", "2")
	require.Equal(t, result{"p\tsize\tmtime\n.hidden/h.php\t21\t2026-01-02T12:00:00Z\nREADME.md\t7\t2026-01-01T12:00:00Z\n", `{"truncated":5}` + "\n", 0}, r)
	r = runGo(t, m, d, env, "llm_find", "*", "--format", "jsonl", "--relative", "-n", "1", "--tail", "1")
	require.Equal(t, `{"p":"tests/t.php","size":19,"mtime":"2026-01-06T12:00:00Z"}`+"\n", r.Stdout)
	require.Equal(t, `{"truncated":6}`+"\n", r.Stderr)
	r = runGo(t, m, d, env, "llm_find", "*", "--format", "json", "-n", "0")
	require.Equal(t, result{"[]\n", "", 0}, r)

	// Text: the budget marks what it cut, inline.
	r = runGo(t, m, d, env, "llm_find", "*", "--relative", "--head", "1", "--tail", "1")
	require.Equal(t, result{".hidden/h.php\n[... 5 lines omitted]\ntests/t.php\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_find", "*", "--relative", "--max-lines", "6")
	require.Equal(t, []string{".hidden/h.php", "README.md", "src/a.php", "src/b.js", "src/sub/c.php", "src/sub/deep/d.PHP", "[... 1 lines omitted]"}, lines(r.Stdout))
	r = runGo(t, m, d, env, "llm_find", "*", "--relative", "--width", "8", "-n", "2")
	require.Equal(t, result{".hidden…\nREADME.…\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_find", "*", "--relative", "-n", "0")
	require.Equal(t, result{"", "", 0}, r)
}

// TestFind34_Tracked: --tracked keeps what git ls-files lists, run
// through the module's runner with the hardened git.
func TestFind34_Tracked(t *testing.T) {
	d := walkFixture(t)
	env := baseEnv(physTemp(t), map[string]string{"PATH": "/usr/bin"})
	git := func(args ...string) []string {
		return append([]string{"git", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-c", "diff.external=", "-c", "core.pager=cat"}, args...)
	}
	m := New()
	m.lookPathFn = func(name string) (string, bool) { return "/usr/bin/" + name, true }
	m.run = runner.NewFake(t,
		runner.Expect{Argv: git("-C", d, "rev-parse", "--git-dir"), Stdout: ".git\n"},
		runner.Expect{Argv: git("-C", d, "ls-files", "-z", "--cached"), Stdout: "README.md\x00src/sub/c.php\x00vendor/v.php\x00gone.php\x00"},
		runner.Expect{Argv: git("-C", d, "rev-parse", "--git-dir"), Stdout: ".git\n"},
		runner.Expect{Argv: git("-C", d, "ls-files", "-z", "--cached"), Stdout: "README.md\x00src/sub/c.php\x00"},
		runner.Expect{Argv: git("-C", d, "rev-parse", "--git-dir"), Stdout: ".git\n"},
		runner.Expect{Argv: git("-C", d, "ls-files", "-z", "--cached"), Stdout: "src/sub/c.php\x00"},
		runner.Expect{Argv: git("-C", d+"/src", "rev-parse", "--git-dir"), Stdout: ".git\n"},
		runner.Expect{Argv: git("-C", d+"/src", "ls-files", "-z", "--cached", "--", ":(literal)a.php")},
		runner.Expect{Argv: git("-C", d, "rev-parse", "--git-dir"), Status: 128},
	)
	r := runGo(t, m, d, env, "llm_find", "*", "--tracked", "--relative")
	require.Equal(t, result{"README.md\nsrc/sub/c.php\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_find", "*", "--tracked", "--count")
	require.Equal(t, result{`{"count":2}` + "\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_find", "*", "--tracked", "--type", "d", "--relative")
	require.Equal(t, result{".\nsrc\nsrc/sub\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_find", "*", "src/a.php", "--tracked")
	require.Equal(t, result{"", "", 0}, r)
	r = runGo(t, m, d, env, "llm_find", "*", "--tracked")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `"error":"not_a_git_repo"`)
	require.Contains(t, r.Stderr, `"action":"CONTINUE"`)
}

func TestFind34_Errors(t *testing.T) {
	d := walkFixture(t)
	env := baseEnv(physTemp(t), nil)
	m := New()
	for _, tc := range []struct {
		args         []string
		code, action string
	}{
		{[]string{"*", "--sort", "date"}, "invalid_sort", "RETRY"},
		{[]string{"*", "--by", "size"}, "invalid_by", "RETRY"},
		{[]string{"*", "--since", "yesterday"}, "invalid_since", "RETRY"},
		{[]string{"*", "--newer", "nope"}, "path_not_found", "RETRY"},
		{[]string{"*", "--format", "xml"}, "invalid_format", "RETRY"},
		{[]string{"*", "--width", "0"}, "invalid_width", "RETRY"},
		{[]string{"*", "--head", "x"}, "invalid_head", "RETRY"},
		{[]string{"*", "--sort"}, "missing_value", "RETRY"},
		{[]string{"*", "--by"}, "missing_value", "RETRY"},
		{[]string{"*", "--newer"}, "missing_value", "RETRY"},
		{[]string{"*", "--since"}, "missing_value", "RETRY"},
		{[]string{"*", "--tail"}, "missing_value", "RETRY"},
		{[]string{"*", "--count", "--type", "x"}, "invalid_type", "RETRY"},
		{[]string{"*", "--count", "nope"}, "path_not_found", "RETRY"},
		{[]string{"*", "--relative", "--contains", "("}, "invalid_regex", "RETRY"},
	} {
		r := runGo(t, m, d, env, "llm_find", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Empty(t, r.Stdout, tc.args)
		var e struct{ Error, Action string }
		require.NoError(t, json.Unmarshal([]byte(r.Stderr), &e), r.Stderr)
		require.Equal(t, tc.code, e.Error, tc.args)
		require.Equal(t, tc.action, e.Action, tc.args)
	}
	// The secret-file rail holds in the 3.4 modes too.
	mustWrite(t, filepath.Join(d, ".env"), "TODO=x\n")
	r := runGo(t, m, d, env, "llm_find", "*", ".env", "--contains", "TODO", "--count")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `"error":"secret_file_blocked"`)
	r = runGo(t, m, d, env, "llm_find", "*", "--contains", "TODO=", "--count", "--all")
	require.Equal(t, result{`{"count":0}` + "\n", "", 0}, r)
}
