// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGrep34_DefaultsUnchanged(t *testing.T) {
	d := walkFixture(t)
	env := baseEnv(physTemp(t), nil)
	m := New()
	for _, args := range [][]string{
		{"TODO"}, {"TODO", "src", "--ext", "php"}, {"TODO", "-C", "1"}, {"TODO", "-n", "2"}, {"TODO", "src/sub/c.php"},
	} {
		want := runGo(t, m, d, env, "llm_grep", args...)
		got := runGo(t, m, d, env, "llm_grep", append(args, "--absolute")...)
		require.Equal(t, want, got, args)
		require.Equal(t, 0, want.RC, want.Stderr)
	}
}

func TestGrep34_Text(t *testing.T) {
	d := walkFixture(t)
	env := baseEnv(physTemp(t), nil)
	m := New()
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"TODO", "--relative"}, []string{"src/a.php:1:<?php // TODO one", "src/b.js:2:// TODO js",
			"src/sub/c.php:2:// TODO two", "src/sub/c.php:3:// TODO three TODO", "tests/t.php:1:<?php // TODO test"}},
		{[]string{"TODO", "src", "--relative", "-n", "2"}, []string{"a.php:1:<?php // TODO one", "b.js:2:// TODO js"}},
		{[]string{"TODO", "src", "--relative", "-C", "1", "--ext", "php"}, []string{"a.php:1:<?php // TODO one", "--",
			"sub/c.php-1-<?php", "sub/c.php:2:// TODO two", "sub/c.php:3:// TODO three TODO"}},
		{[]string{"TODO", "-o", "--relative"}, []string{"src/a.php:1:TODO", "src/b.js:2:TODO", "src/sub/c.php:2:TODO",
			"src/sub/c.php:3:TODO", "src/sub/c.php:3:TODO", "tests/t.php:1:TODO"}},
		{[]string{"TODO [a-z]+", "--only-matching", "src/sub/c.php"}, []string{"2:TODO two", "3:TODO three"}},
		{[]string{"TODO", "-l", "--relative"}, []string{"src/a.php", "src/b.js", "src/sub/c.php", "tests/t.php"}},
		{[]string{"TODO", "--files-only", "src/a.php", "--relative"}, []string{"a.php"}},
		{[]string{"TODO", "--files-only", "src/a.php"}, []string{d + "/src/a.php"}},
		{[]string{"TODO", "-l", "--exclude", "tests", "--relative"}, []string{"src/a.php", "src/b.js", "src/sub/c.php"}},
		{[]string{"TODO", "-l", "--exclude", "src/sub", "--exclude", "*.js", "--relative"}, []string{"src/a.php", "tests/t.php"}},
		{[]string{"TODO", "-l", "--exclude", "sub/c.php", "--relative"}, []string{"src/a.php", "src/b.js", "src/sub/c.php", "tests/t.php"}},
		{[]string{"TODO", "src", "-l", "--exclude", "sub/c.php", "--relative"}, []string{"a.php", "b.js"}},
		{[]string{"TODO", "--relative", "--max-lines", "2"}, []string{"src/a.php:1:<?php // TODO one", "src/b.js:2:// TODO js", "[... 3 lines omitted]"}},
		{[]string{"TODO", "--relative", "--tail", "1", "-n", "1"}, []string{"[... 4 lines omitted]", "tests/t.php:1:<?php // TODO test"}},
		{[]string{"TODO", "--relative", "--width", "12", "-n", "2"}, []string{"src/a.php:1…", "src/b.js:2:…"}},
		{[]string{"nomatch", "--relative"}, nil},
	} {
		r := runGo(t, m, d, env, "llm_grep", tc.args...)
		require.Equal(t, 0, r.RC, "%v %s", tc.args, r.Stderr)
		require.Empty(t, r.Stderr, tc.args)
		require.Equal(t, tc.want, lines(r.Stdout), tc.args)
	}
}

func TestGrep34_CountsDistinctRows(t *testing.T) {
	d := walkFixture(t)
	mustWrite(t, filepath.Join(d, "bin.dat"), "TODO\x00x\n")
	env := baseEnv(physTemp(t), nil)
	m := New()
	for _, tc := range []struct {
		args         []string
		want, stderr string
	}{
		{[]string{"TODO", "--count"}, `{"count":5}`, ""},
		{[]string{"TODO", "-o", "-c"}, `{"count":6}`, ""},
		{[]string{"TODO", "-l", "-c"}, `{"count":4}`, ""},
		{[]string{"nomatch", "-c"}, `{"count":0}`, ""},
		{[]string{"TODO", "--by", "file", "--relative"}, `{"count":5,"by":{"src/a.php":1,"src/b.js":1,"src/sub/c.php":2,"tests/t.php":1}}`, ""},
		{[]string{"TODO", "--by", "ext", "-o"}, `{"count":6,"by":{"js":1,"php":5}}`, ""},
		{[]string{"TODO", "--by", "dir", "-l"}, `{"count":4,"by":{"src":3,"tests":1}}`, ""},
		{[]string{"TODO", "--count", "--format", "tsv", "--by", "dir"}, "key\tcount\nsrc\t4\ntests\t1", `{"count":5}`},
		{[]string{"TODO", "--distinct"}, `[{"match":"TODO","count":6}]`, ""},
		{[]string{"TODO [a-z]+", "--distinct", "-o"}, `[{"match":"TODO js","count":1},{"match":"TODO one","count":1},{"match":"TODO test","count":1},{"match":"TODO three","count":1},{"match":"TODO two","count":1}]`, ""},
		{[]string{"TODO|t[a-z]+", "--distinct", "-n", "2"}, `[{"match":"TODO","count":6},{"match":"test","count":1}]`, `{"truncated":2}`},
		{[]string{"TODO [a-z]+", "--distinct", "--format", "tsv", "--tail", "1"}, "match\tcount\nTODO two\t1", `{"truncated":4}`},
		{[]string{"TODO", "src/sub", "--format", "json", "--relative"}, `[{"file":"c.php","line":2,"text":"// TODO two"},{"file":"c.php","line":3,"text":"// TODO three TODO"}]`, ""},
		{[]string{"TODO", "src/sub/c.php", "--format", "jsonl", "--relative", "-o", "-n", "2"}, `{"file":"c.php","line":2,"text":"TODO"}` + "\n" + `{"file":"c.php","line":3,"text":"TODO"}`, `{"truncated":1}`},
		{[]string{"TODO", "--format", "tsv", "--relative", "-l"}, "file\nsrc/a.php\nsrc/b.js\nsrc/sub/c.php\ntests/t.php", ""},
		{[]string{"TODO", "--format", "json", "-n", "0"}, `[]`, ""},
		{[]string{"TODO", "bin.dat", "--format", "jsonl", "--relative"}, `{"file":"bin.dat","line":1,"text":"binary file matches (found \"\\0\" byte around offset 4)"}`, ""},
		{[]string{"TODO", "bin.dat", "--count"}, `{"count":1}`, ""},
	} {
		r := runGo(t, m, d, env, "llm_grep", tc.args...)
		require.Equal(t, 0, r.RC, "%v %s", tc.args, r.Stderr)
		require.Equal(t, tc.want+"\n", r.Stdout, tc.args)
		if tc.stderr != "" {
			tc.stderr += "\n"
		}
		require.Equal(t, tc.stderr, r.Stderr, tc.args)
	}
}

func TestGrep34_Errors(t *testing.T) {
	d := walkFixture(t)
	env := baseEnv(physTemp(t), nil)
	m := New()
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"TODO", "-C", "1", "-o"}, "conflicting_options"},
		{[]string{"TODO", "-C", "1", "--format", "json"}, "conflicting_options"},
		{[]string{"TODO", "-C", "1", "--count"}, "conflicting_options"},
		{[]string{"TODO", "-l", "-o"}, "conflicting_options"},
		{[]string{"TODO", "-l", "--distinct"}, "conflicting_options"},
		{[]string{"TODO", "--distinct", "--by", "file"}, "conflicting_options"},
		{[]string{"TODO", "--by", "type"}, "invalid_by"},
		{[]string{"TODO", "--exclude", ""}, "invalid_filter"},
		{[]string{"TODO", "--exclude", "a\nb"}, "invalid_filter"},
		{[]string{"TODO", "--exclude"}, "missing_value"},
		{[]string{"TODO", "--by"}, "missing_value"},
		{[]string{"TODO", "--format", "csv"}, "invalid_format"},
		{[]string{"(", "--relative"}, "invalid_regex"},
		{[]string{"TODO", "nope", "--relative"}, "path_not_found"},
		{[]string{"", "--count"}, "missing_pattern"},
	} {
		r := runGo(t, m, d, env, "llm_grep", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Empty(t, r.Stdout, tc.args)
		var e struct{ Error, Action string }
		require.NoError(t, json.Unmarshal([]byte(r.Stderr), &e), r.Stderr)
		require.Equal(t, tc.code, e.Error, tc.args)
		require.Equal(t, "RETRY", e.Action, tc.args)
	}
	// Secret files stay out of every mode.
	mustWrite(t, filepath.Join(d, "src", ".env"), "TODO=secret\n")
	mustWrite(t, filepath.Join(d, "src", "id_rsa"), "TODO secret\n")
	r := runGo(t, m, d, env, "llm_grep", "secret", "src", "--count")
	require.Equal(t, result{`{"count":0}` + "\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_grep", "TODO", "src/.env", "--count")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `"error":"secret_file_blocked"`)
}
