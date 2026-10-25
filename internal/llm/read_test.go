// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// numbered is "line 1\n" .. "line n\n".
func numbered(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

func TestRead_Blocks(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "a.txt"), numbered(20))
	mustWrite(t, filepath.Join(d, "b.txt"), "x\ny")
	mustWrite(t, filepath.Join(d, "empty"), "")
	env := baseEnv(physTemp(t), nil)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"b.txt"}, "==> b.txt:1-2 <==\n1:x\n2:y\n"},
		{[]string{"a.txt", "--lines", "3:4"}, "==> a.txt:3-4 <==\n3:line 3\n4:line 4\n"},
		{[]string{"a.txt", "--lines", "19-"}, "==> a.txt:19-20 <==\n19:line 19\n20:line 20\n"},
		{[]string{"a.txt", "--lines", ":2", "--lines", "5,5"}, "==> a.txt:1-2 <==\n1:line 1\n2:line 2\n==> a.txt:5-5 <==\n5:line 5\n"},
		// PATH:A-B is that file's own range; --lines is for the others.
		{[]string{"a.txt:7-8", "b.txt", "--lines", "2:2"}, "==> a.txt:7-8 <==\n7:line 7\n8:line 8\n==> b.txt:2-2 <==\n2:y\n"},
		{[]string{"a.txt:9:9"}, "==> a.txt:9-9 <==\n9:line 9\n"},
		{[]string{"a.txt", "--around", "^line 1[05]$", "-C", "1"},
			"==> a.txt:9-11 <==\n9-line 9\n10:line 10\n11-line 11\n==> a.txt:14-16 <==\n14-line 14\n15:line 15\n16-line 16\n"},
		{[]string{"a.txt", "--around", "line 1$", "--around", "line 3$", "-C", "1"},
			"==> a.txt:1-4 <==\n1:line 1\n2-line 2\n3:line 3\n4-line 4\n"},
		{[]string{"a.txt", "--around", "nomatch"}, "==> a.txt <==\n[no match in 20 lines]\n"},
		{[]string{"a.txt", "--lines", "30:40"}, "==> a.txt <==\n[no line in range: the file has 20 lines]\n"},
		{[]string{"empty"}, "==> empty <==\n[empty file]\n"},
		{[]string{"a.txt", "--head", "2"}, "==> a.txt:1-2 <==\n1:line 1\n2:line 2\n[... 18 lines omitted]\n"},
		{[]string{"a.txt", "--tail", "2"}, "[... 18 lines omitted]\n==> a.txt:19-20 <==\n19:line 19\n20:line 20\n"},
		{[]string{"a.txt", "--head", "1", "--tail", "1"}, "==> a.txt:1-1 <==\n1:line 1\n[... 18 lines omitted]\n==> a.txt:20-20 <==\n20:line 20\n"},
		{[]string{"a.txt", "--max-bytes", "14"}, "==> a.txt:1-2 <==\n1:line 1\n2:line 2\n[... 18 lines omitted]\n"},
		{[]string{"b.txt", "--width", "3"}, "==> b.txt:1-2 <==\n1:x\n2:y\n"},
		{[]string{"a.txt", "--lines", "12:12", "--width", "4"}, "==> a.txt:12-12 <==\n12:…\n"},
	} {
		r := runGo(t, New(), d, env, "llm_read", tc.args...)
		require.Equal(t, result{tc.want, "", 0}, r, tc.args)
	}
}

func TestRead_Rows(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "a.txt"), numbered(5))
	env := baseEnv(physTemp(t), nil)
	r := runGo(t, New(), d, env, "llm_read", "a.txt", "--lines", "2:3", "--format", "json")
	require.Equal(t, result{`[{"file":"a.txt","line":2,"text":"line 2"},{"file":"a.txt","line":3,"text":"line 3"}]` + "\n", "", 0}, r)
	r = runGo(t, New(), d, env, "llm_read", "a.txt", "--tail", "1", "--format", "tsv")
	require.Equal(t, result{"file\tline\ttext\na.txt\t5\tline 5\n", `{"truncated":4}` + "\n", 0}, r)
	r = runGo(t, New(), d, env, "llm_read", "a.txt", "--lines", "1:1", "--absolute", "--format", "jsonl")
	require.Equal(t, `{"file":"`+filepath.Join(d, "a.txt")+`","line":1,"text":"line 1"}`+"\n", r.Stdout)
}

func TestRead_Refusals(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "a.txt"), "x\n")
	mustWrite(t, filepath.Join(d, "bin"), "x\x00y")
	mustWrite(t, filepath.Join(d, ".env"), "K=v\n")
	mustWrite(t, filepath.Join(d, "id_rsa"), "key\n")
	mustMkdir(t, filepath.Join(d, "dir"))
	require.NoError(t, os.Symlink(".env", filepath.Join(d, "innocent.txt")))
	env := baseEnv(physTemp(t), nil)
	for _, tc := range []struct {
		args         []string
		code, action string
	}{
		{[]string{"a.txt", ".env"}, "secret_file_blocked", "FATAL"},
		{[]string{"id_rsa"}, "secret_file_blocked", "FATAL"},
		{[]string{"innocent.txt"}, "secret_file_blocked", "FATAL"},
		{[]string{"a.txt", "bin"}, "binary_file", "RETRY"},
		{[]string{"nope"}, "file_not_found", "RETRY"},
		{[]string{"dir"}, "file_not_found", "RETRY"},
		{[]string{"../a.txt"}, "path_traversal_blocked", "FATAL"},
		{[]string{"/etc/shadow"}, "forbidden_path", "FATAL"},
		{[]string{"a.txt", "--lines", "5:2"}, "invalid_range", "RETRY"},
		{[]string{"a.txt", "--lines", "0:2"}, "invalid_range", "RETRY"},
		{[]string{"a.txt", "--lines", "x"}, "invalid_range", "RETRY"},
		{[]string{"a.txt", "-C", "2"}, "invalid_context", "RETRY"},
		{[]string{"a.txt", "--around", "x", "-C", "-1"}, "invalid_context", "RETRY"},
		{[]string{"a.txt", "--around", "("}, "invalid_regex", "RETRY"},
		{[]string{"a.txt", "--max-bytes", "0"}, "invalid_size", "RETRY"},
		{[]string{}, "missing_file", "RETRY"},
	} {
		r := runGo(t, New(), d, env, "llm_read", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Empty(t, r.Stdout, "nothing is printed before a refusal: %v", tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
		require.Contains(t, r.Stderr, `"action":"`+tc.action+`"`, tc.args)
	}
	// llm_config refuses the same files with its own envelope (unchanged).
	r := runGo(t, New(), d, env, "llm_config", ".env")
	require.Equal(t, `{"error":"secret_file_blocked","details":"Cannot read .env or credential files","action":"FATAL","recoverable":false,"hint":"This file contains secrets and cannot be read"}`+"\n", r.Stderr)
}

func TestRead_PathRangeOnlyWhenNotAFile(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "odd:1-2"), "real\n")
	mustWrite(t, filepath.Join(d, "f"), numbered(3))
	r := runGo(t, New(), d, baseEnv(physTemp(t), nil), "llm_read", "odd:1-2", "f:2-2")
	require.Equal(t, result{"==> odd:1-2:1-1 <==\n1:real\n==> f:2-2 <==\n2:line 2\n", "", 0}, r)
}

func TestRead_LongLinesAndCRLF(t *testing.T) {
	d := physTemp(t)
	long := strings.Repeat("é", 100<<10) // 200 KiB, more than the reader's buffer
	mustWrite(t, filepath.Join(d, "f"), "a\r\n"+long+"\nlast")
	spec := &readSpec{maxBytes: readMaxBytes}
	f, err := os.Open(filepath.Join(d, "f"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	res, err := selectLines(context.Background(), f, spec)
	require.NoError(t, err)
	require.Equal(t, 3, res.total)
	require.Len(t, res.head, 3)
	require.Equal(t, "a\r", string(res.head[0].text), "the line as it is, carriage return kept")
	require.Equal(t, long, string(res.head[1].text))
	require.Equal(t, "last", string(res.head[2].text))

	// One line longer than --max-bytes is cut at a character boundary.
	_, err = f.Seek(0, 0)
	require.NoError(t, err)
	res, err = selectLines(context.Background(), f, &readSpec{maxBytes: 5, ranges: []lineRange{{2, 2}}})
	require.NoError(t, err)
	require.Equal(t, "éé…", string(res.head[0].text))
}
