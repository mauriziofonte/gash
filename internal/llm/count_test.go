// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCount_Matrix(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "a.log"), "ERROR x ERROR\nWARN y\nINFO z\nERROR w\n")
	mustWrite(t, filepath.Join(d, "b.log"), "nothing\nWARN\n")
	env := baseEnv(physTemp(t), nil)
	r := runGo(t, New(), d, env, "llm_count", "a.log", "b.log", "missing.log", "--pat", "ERROR", "--pat", "WARN", "--pat", "ERROR")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, `{"a.log":{"ERROR":2,"WARN":1},"b.log":{"ERROR":0,"WARN":1},"missing.log":{"error":"file_not_found"}}`+"\n", r.Stdout,
		"files and patterns in input order, repeats dropped, lines counted as grep -c")

	r = runGo(t, New(), d, env, "llm_count", "a.log", "--pat", "ERROR", "--occurrences")
	require.Equal(t, `{"a.log":{"ERROR":3}}`+"\n", r.Stdout)

	r = runGo(t, New(), d, env, "llm_count", "a.log", "--pat", `^(ERROR|WARN)`, "--pat", `(?i)error`, "--regex", "--format", "tsv")
	require.Equal(t, "file\tpat\tcount\na.log\t^(ERROR|WARN)\t3\na.log\t(?i)error\t2\n", r.Stdout)

	r = runGo(t, New(), d, env, "llm_count", "a.log", "nope", "--pat", "x", "--format", "tsv")
	require.Equal(t, "file\tpat\tcount\terror\na.log\tx\t1\t\\N\nnope\tx\t\\N\tfile_not_found\n", r.Stdout)

	r = runGo(t, New(), d, env, "llm_count", "a.log", "--pat", `[a-z]+`, "--regex", "--occurrences")
	require.Equal(t, `{"a.log":{"[a-z]+":4}}`+"\n", r.Stdout)
}

func TestCount_Errors(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "a"), "x\n")
	mustWrite(t, filepath.Join(d, ".env"), "SECRET=1\n")
	env := baseEnv(physTemp(t), nil)
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"a"}, "missing_pattern"},
		{[]string{"--pat", "x"}, "missing_file"},
		{[]string{"a", "--pat", ""}, "missing_pattern"},
		{[]string{"a", "--pat", "a\nb"}, "invalid_filter"},
		{[]string{"a", "--pat", "(", "--regex"}, "invalid_regex"},
		{[]string{"a", ".env", "--pat", "SECRET"}, "secret_file_blocked"},
		{[]string{"a", "--pat"}, "missing_value"},
		{[]string{"a", "--pat", "x", "--head", "1"}, "unknown_option"},
	} {
		r := runGo(t, New(), d, env, "llm_count", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
		require.Empty(t, r.Stdout, tc.args)
	}
}

// naiveCount is grep -c (lines) or grep -o | wc -l (occurrences) of a
// literal, line by line.
func naiveCount(data []byte, pat string, occur bool) int64 {
	var n int64
	for _, l := range bytes.Split(data, []byte{'\n'}) {
		if occur {
			n += int64(bytes.Count(l, []byte(pat)))
		} else if bytes.Contains(l, []byte(pat)) {
			n++
		}
	}
	return n
}

// The parallel parts of a large file and the line-aligned blocks count
// what one pass over the whole file counts, also with lines longer than a
// block.
func TestCount_LargeFileParts(t *testing.T) {
	d := physTemp(t)
	var b bytes.Buffer
	for i := 0; b.Len() < parallelMin+3*blockSize; i++ {
		switch {
		case i%9973 == 0:
			b.WriteString(strings.Repeat("payment ", blockSize/4) + "ERROR\n") // a line of 2 blocks
		case i%7 == 0:
			fmt.Fprintf(&b, "2026-10-25 ERROR id=%d timeout timeout\n", i)
		default:
			fmt.Fprintf(&b, "2026-10-25 INFO id=%d payment\n", i)
		}
	}
	data := b.Bytes()
	path := filepath.Join(d, "big.log")
	require.NoError(t, os.WriteFile(path, data, 0o644))
	pats := []string{"ERROR", "payment", "timeout", "id=999", "absent"}
	for _, occur := range []bool{false, true} {
		k := &counter{occur: occur}
		for _, p := range pats {
			k.lits = append(k.lits, []byte(p))
		}
		got, err := k.countFile(context.Background(), path)
		require.NoError(t, err)
		for i, p := range pats {
			require.Equal(t, naiveCount(data, p, occur), got[i], "%s occur=%v", p, occur)
		}
		// The regex path counts the same.
		kr := &counter{occur: occur}
		for _, p := range pats {
			re, err := compileRg(regexp.QuoteMeta(p))
			require.NoError(t, err)
			kr.res = append(kr.res, re)
		}
		gotRe, err := kr.countFile(context.Background(), path)
		require.NoError(t, err)
		require.Equal(t, got, gotRe, "occur=%v", occur)
	}
}

func TestAssert_PassAndFail(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "a.md"), "# Title\n## Gotchas\n## Sites\nfoo foo\n")
	mustWrite(t, filepath.Join(d, "b.md"), "# Other\n## Sites\n")
	env := baseEnv(physTemp(t), nil)
	r := runGo(t, New(), d, env, "llm_assert", "a.md", "b.md", "--has", "## Sites", "--lacks", "TODO")
	require.Equal(t, result{`{"pass":true,"checked":4}` + "\n", "", 0}, r)
	require.Less(t, len(r.Stdout), 200)

	r = runGo(t, New(), d, env, "llm_assert", "a.md", "b.md", "nope.md", "--has", "Gotchas", "--count", "foo=2", "--occurrences")
	require.Equal(t, 1, r.RC)
	require.Empty(t, r.Stderr, "a failed assertion is an answer, not an error envelope")
	var out struct {
		Pass    bool
		Checked int
		Failed  []map[string]any
	}
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &out))
	require.False(t, out.Pass)
	require.Equal(t, 6, out.Checked)
	require.Equal(t, []map[string]any{
		{"file": "b.md", "check": "has:Gotchas", "found": float64(0)},
		{"file": "b.md", "check": "count:foo=2", "found": float64(0)},
		{"file": "nope.md", "error": "file_not_found"},
	}, out.Failed)

	// --count counts lines by default; the last = splits.
	mustWrite(t, filepath.Join(d, "c"), "a=b a=b\n")
	r = runGo(t, New(), d, env, "llm_assert", "c", "--count", "a=b=1")
	require.Equal(t, 0, r.RC, r.Stdout)
	r = runGo(t, New(), d, env, "llm_assert", "c", "--count", "^a=b=1", "--regex")
	require.Equal(t, 0, r.RC, r.Stdout)

	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"c"}, "missing_pattern"},
		{[]string{"--has", "x"}, "missing_file"},
		{[]string{"c", "--count", "x"}, "invalid_assertion"},
		{[]string{"c", "--count", "x=-1"}, "invalid_assertion"},
		{[]string{"c", "--count", "=1"}, "invalid_assertion"},
	} {
		r := runGo(t, New(), d, env, "llm_assert", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
	}
}

// lineBounds cuts at line starts whatever the number of parts, and the
// parts together are the file.
func TestLineBounds(t *testing.T) {
	d := physTemp(t)
	content := "a\nbb\n\nccc\n" + strings.Repeat("x", 300) + "\nend"
	p := filepath.Join(d, "f")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	f, err := os.Open(p)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	size := int64(len(content))
	for parts := 1; parts <= 40; parts++ {
		b, err := lineBounds(f, size, parts)
		require.NoError(t, err)
		require.Equal(t, int64(0), b[0])
		require.Equal(t, size, b[len(b)-1])
		var joined []byte
		for i := 1; i < len(b); i++ {
			require.Less(t, b[i-1], b[i], "parts=%d", parts)
			if b[i-1] > 0 {
				require.Equal(t, byte('\n'), content[b[i-1]-1], "part %d starts a line (parts=%d)", i-1, parts)
			}
			require.NoError(t, scanBlocks(context.Background(), f, b[i-1], b[i], func(blk []byte) { joined = append(joined, blk...) }))
		}
		require.Equal(t, content, string(joined), "parts=%d", parts)
	}
}
