// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTree34_DefaultsUnchanged(t *testing.T) {
	d := walkFixture(t)
	env := baseEnv(physTemp(t), nil)
	m := New()
	for _, args := range [][]string{{}, {"--depth", "3"}, {"--stats"}, {"--text"}, {"--text", "src"}, {"--format", "json"}} {
		want := runGo(t, m, d, env, "llm_tree", args...)
		got := runGo(t, m, d, env, "llm_tree", append(args, "--absolute")...)
		require.Equal(t, want, got, args)
		require.Equal(t, 0, want.RC, want.Stderr)
	}
}

func TestTree34_Count(t *testing.T) {
	d := walkFixture(t)
	env := baseEnv(physTemp(t), nil)
	m := New()
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--count"}, `{"count":13}`},
		{[]string{"--count", "--all"}, `{"count":19}`},
		{[]string{"--count", "--type", "f"}, `{"count":7}`},
		{[]string{"--count", "--type", "d"}, `{"count":5}`},
		{[]string{"--count", "--depth", "1"}, `{"count":5}`},
		{[]string{"--count", "--depth", "2", "--type", "f"}, `{"count":5}`},
		{[]string{"--by", "type"}, `{"count":13,"by":{"d":5,"f":7,"l":1}}`},
		{[]string{"--by", "dir"}, `{"count":13,"by":{".":5,".hidden":1,"src":6,"tests":1}}`},
		{[]string{"--count", "--by", "ext", "--type", "f", "src"}, `{"count":4,"by":{"js":1,"php":3}}`},
		{[]string{"--count", "--name", "*.php"}, `{"count":5}`},
		{[]string{"--count", "--type", "f", "--since", "2026-01-06"}, `{"count":3}`},
		{[]string{"--count", "--type", "f", "--newer", "src/sub/c.php"}, `{"count":0}`},
		{[]string{"--count", "--format", "tsv"}, "count\n13"},
		{[]string{"--count", "vendor"}, `{"count":1}`},
	} {
		r := runGo(t, m, d, env, "llm_tree", tc.args...)
		require.Equal(t, 0, r.RC, "%v %s", tc.args, r.Stderr)
		require.Equal(t, tc.want+"\n", r.Stdout, tc.args)
	}
	// The count is the number of entries the JSON tree lists at that depth.
	r := runGo(t, m, d, env, "llm_tree", "--depth", "9")
	require.Equal(t, 13, strings.Count(r.Stdout, `"name":`))
}

func TestTree34_Top(t *testing.T) {
	d := walkFixture(t)
	env := baseEnv(physTemp(t), nil)
	m := New()
	for _, tc := range []struct {
		args         []string
		want, stderr string
	}{
		{[]string{"--top", "10"}, `{"total":168,"files":11,"top":[{"p":"src","size":74,"files":4},{"p":".hidden","size":21,"files":1},{"p":"vendor","size":21,"files":1},{"p":"tests","size":19,"files":1},{"p":"node_modules","size":11,"files":1},{"p":"__pycache__","size":6,"files":1}],"skipped":0}`, ""},
		{[]string{"--top", "2"}, `{"total":168,"files":11,"top":[{"p":"src","size":74,"files":4},{"p":".hidden","size":21,"files":1}],"skipped":0,"truncated":4}`, ""},
		{[]string{"--top", "3", "--depth", "2"}, `{"total":168,"files":11,"top":[{"p":"src","size":74,"files":4},{"p":"src/sub","size":43,"files":2},{"p":".hidden","size":21,"files":1}],"skipped":0,"truncated":4}`, ""},
		{[]string{"--top", "9", "src", "--depth", "5"}, `{"total":74,"files":4,"top":[{"p":"sub","size":43,"files":2},{"p":"sub/deep","size":6,"files":1}],"skipped":0}`, ""},
		{[]string{"--top", "1", "src", "--absolute"}, `{"total":74,"files":4,"top":[{"p":"` + d + `/src/sub","size":43,"files":2}],"skipped":0}`, ""},
		{[]string{"--top", "5", "--by", "ext"}, `{"total":168,"files":11,"top":[{"ext":"php","size":137,"files":8},{"ext":"js","size":24,"files":2},{"ext":"md","size":7,"files":1}],"skipped":0}`, ""},
		{[]string{"--top", "2", "--name", "*.js"}, `{"total":24,"files":2,"top":[{"p":"src","size":13,"files":1},{"p":"node_modules","size":11,"files":1}],"skipped":0,"truncated":4}`, ""},
		{[]string{"--top", "1", "--since", "2026-01-08", "--name", "*.php", "src"}, `{"total":37,"files":1,"top":[{"p":"sub","size":37,"files":1}],"skipped":0}`, ""},
		{[]string{"--top", "2", "--format", "tsv"}, "p\tsize\tfiles\nsrc\t74\t4\n.hidden\t21\t1", `{"files":11,"skipped":0,"total":168,"truncated":4}`},
		{[]string{"--top", "9", "--head", "1", "--tail", "1", "--format", "table"}, "p            size  files\nsrc          74    4\n[... 4 rows omitted]\n__pycache__  6     1", `{"files":11,"skipped":0,"total":168,"truncated":4}`},
		{[]string{"--top", "1", "--width", "3"}, `{"total":168,"files":11,"top":[{"p":"src","size":74,"files":4}],"skipped":0,"truncated":5}`, ""},
		{[]string{"--top", "1", "src/sub/deep"}, `{"total":6,"files":1,"top":[],"skipped":0}`, ""},
	} {
		r := runGo(t, m, d, env, "llm_tree", tc.args...)
		require.Equal(t, 0, r.RC, "%v %s", tc.args, r.Stderr)
		require.Equal(t, tc.want+"\n", r.Stdout, tc.args)
		if tc.stderr != "" {
			tc.stderr += "\n"
		}
		require.Equal(t, tc.stderr, r.Stderr, tc.args)
	}
}

// TestTree34_TopSkipped: an unreadable directory is counted in skipped,
// its row stays (size 0).
func TestTree34_TopSkipped(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every directory")
	}
	d := walkFixture(t)
	locked := filepath.Join(d, "locked")
	mustWrite(t, filepath.Join(locked, "x"), "xxxx")
	require.NoError(t, os.Chmod(locked, 0))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	env := baseEnv(physTemp(t), nil)
	r := runGo(t, New(), d, env, "llm_tree", "--top", "1")
	require.Equal(t, `{"total":168,"files":11,"top":[{"p":"src","size":74,"files":4}],"skipped":1,"truncated":6}`+"\n", r.Stdout)
}

func TestTree34_Listing(t *testing.T) {
	d := walkFixture(t)
	env := baseEnv(physTemp(t), nil)
	m := New()
	r := runGo(t, m, d, env, "llm_tree", "--relative", "src")
	require.Equal(t, result{`{"path":".","type":"directory","children":[{"name":"a.php","type":"file"},{"name":"b.js","type":"file"},{"name":"sub","type":"directory"}]}` + "\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_tree", "--all")
	require.Contains(t, r.Stdout, `{"name":"vendor","type":"directory"}`)
	require.Contains(t, r.Stdout, `{"name":"node_modules","type":"directory"}`)
	r = runGo(t, m, d, env, "llm_tree", "--text", "src", "--max-lines", "2")
	require.Equal(t, result{"src/\na.php\n[... 5 lines omitted]\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_tree", "--text", "src", "--tail", "1")
	require.Equal(t, result{"[... 6 lines omitted]\n    d.PHP\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_tree", "--text", "src", "--width", "6")
	require.Equal(t, result{"src/\na.php\nb.js\nsub/\n  c.p…\n  dee…\n    d…\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_tree", "--text", "--all", "vendor")
	require.Equal(t, result{"vendor/\nv.php\n", "", 0}, r)
}

func TestTree34_Errors(t *testing.T) {
	d := walkFixture(t)
	env := baseEnv(physTemp(t), nil)
	m := New()
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"--top", "3", "--count"}, "conflicting_options"},
		{[]string{"--count", "--text"}, "conflicting_options"},
		{[]string{"--top", "3", "--type", "f"}, "conflicting_options"},
		{[]string{"--name", "*.php"}, "conflicting_options"},
		{[]string{"--since", "1d"}, "conflicting_options"},
		{[]string{"--format", "tsv"}, "conflicting_options"},
		{[]string{"--max-lines", "3"}, "conflicting_options"},
		{[]string{"--top", "0"}, "invalid_top"},
		{[]string{"--top", "x"}, "invalid_top"},
		{[]string{"--top", "3", "--by", "type"}, "invalid_by"},
		{[]string{"--count", "--by", "size"}, "invalid_by"},
		{[]string{"--count", "--type", "l"}, "invalid_type"},
		{[]string{"--count", "--since", "soon"}, "invalid_since"},
		{[]string{"--count", "--newer", "nope"}, "path_not_found"},
		{[]string{"--top"}, "missing_value"},
		{[]string{"--count", "--name"}, "missing_value"},
		{[]string{"--count", "README.md"}, "not_a_directory"},
		{[]string{"--count", "--depth", "0"}, "invalid_depth"},
	} {
		r := runGo(t, m, d, env, "llm_tree", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Empty(t, r.Stdout, tc.args)
		var e struct{ Error, Action string }
		require.NoError(t, json.Unmarshal([]byte(r.Stderr), &e), r.Stderr)
		require.Equal(t, tc.code, e.Error, tc.args)
		require.Equal(t, "RETRY", e.Action, tc.args)
	}
}
