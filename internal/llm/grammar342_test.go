// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDashPattern: a PATTERN that starts with - reaches llm_grep through
// -e/--regexp or after --, and llm_find through -- (3.4.2); a flag value
// may start with - already.
func TestDashPattern(t *testing.T) {
	m := New()
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "src/a.txt"), "x -foo y\nplain\n")
	mustWrite(t, filepath.Join(d, "src/-b.txt"), "--help\n")
	env := baseEnv(physTemp(t), nil)
	hit := d + "/src/a.txt:1:x -foo y\n"
	for _, args := range [][]string{
		{"-e", "-foo", "src"}, {"src", "-e", "-foo"}, {"--regexp", "-foo", "src"}, {"--regexp=-foo", "src"},
		{"--", "-foo", "src"}, {"-n", "5", "--", "-foo", "src"}, {"-e", "-foo", "--", "src"},
	} {
		require.Equal(t, result{hit, "", 0}, runGo(t, m, d, env, "llm_grep", args...), args)
	}
	// After --, --help and -h are data.
	require.Equal(t, result{d + "/src/-b.txt:1:--help\n", "", 0}, runGo(t, m, d, env, "llm_grep", "--", "--help", "src"))

	for _, c := range []struct {
		args       []string
		code, what string
	}{
		{[]string{"-e", "a", "-e", "b", "src"}, "unexpected_argument", "b"},
		{[]string{"-e", "a", "src", "other"}, "unexpected_argument", "other"},
		{[]string{"src", "-e"}, "missing_value", "-e"},
		{[]string{"-foo", "src"}, "unknown_option", "-foo"},
	} {
		r := runGo(t, m, d, env, "llm_grep", c.args...)
		require.Equal(t, 1, r.RC, c.args)
		require.Contains(t, r.Stderr, `"error":"`+c.code+`","details":"`+c.what+`"`, c.args)
	}
	// The hint of an unknown option lists them all, from the help.
	r := runGo(t, m, d, env, "llm_grep", "-foo", "src")
	require.Contains(t, r.Stderr, `"hint":"Options: -e/--regexp, --ext, -C/--context, -n/--limit, --exclude, `)
	require.Contains(t, r.Stderr, `--max-lines, --head, --tail, --host (llm_grep --help)"`)

	require.Equal(t, result{d + "/src/-b.txt\n", "", 0}, runGo(t, m, d, env, "llm_find", "--", "-*", "src"))
	require.Equal(t, result{d + "/src/a.txt\n", "", 0}, runGo(t, m, d, env, "llm_find", "*", "src", "--contains", "-foo"))
	r = runGo(t, m, d, env, "llm_find", "-*", "src")
	require.Contains(t, r.Stderr, `"error":"unknown_option","details":"-*"`)
}

// TestGrepFallbackDashPattern: the POSIX fallback of llm_grep --host reads
// -e and -- as the local parser does.
func TestGrepFallbackDashPattern(t *testing.T) {
	for _, args := range [][]string{{"-e", "-foo", "src"}, {"--", "-foo", "src"}, {"-n", "3", "--", "-foo", "src"}} {
		p, ok := grepFallback(nil, "n0nce", args)
		require.True(t, ok, args)
		require.Nil(t, p.Err, args)
		require.Contains(t, p.Script, "\nP='src'\n", args)
		require.Contains(t, p.Script, "\nPAT='-foo'\n", args)
	}
	// What only the local parser decides goes to the binary.
	_, ok := grepFallback(nil, "n0nce", []string{"src", "-e", "x"})
	require.False(t, ok)
	p, ok := findFallback(nil, "n0nce", []string{"--", "-*", "src"})
	require.True(t, ok)
	require.True(t, strings.Contains(p.Script, "\nPAT='-*'\n") && strings.Contains(p.Script, "\nP='src'\n"))
}
