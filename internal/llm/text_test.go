// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTextstats(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "a.txt"), "hello world\ncafé  au\tlait\nlast line, no newline")
	mustWrite(t, filepath.Join(d, "doc.md"), "---\ntitle: Front matter words\n---\n# Title here\n\n```go\nfunc main() {}\n```\nTwo words\n~~~\nskipped too\n~~~\n")
	mustWrite(t, filepath.Join(d, "bin"), "a\x00b")
	mustWrite(t, filepath.Join(d, ".env"), "K=v\n")
	mustWrite(t, filepath.Join(d, "empty"), "")
	env := baseEnv(physTemp(t), nil)
	r := runGo(t, New(), d, env, "llm_textstats", "a.txt", "doc.md", "bin", ".env", "nope", "empty")
	require.Equal(t, 0, r.RC, r.Stderr)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &rows))
	// wc -l -w -m -c of a.txt (UTF-8): 2 newlines (3 lines here), 9 words, 47 characters, 48 bytes.
	require.Equal(t, map[string]any{"p": "a.txt", "lines": 3.0, "words": 9.0, "chars": 47.0, "bytes": 48.0,
		"gzip": rows[0]["gzip"], "max_line": 21.0, "error": nil}, rows[0])
	var gz bytes.Buffer
	w, _ := gzip.NewWriterLevel(&gz, gzip.DefaultCompression)
	w.Name = "a.txt"
	_, _ = w.Write([]byte("hello world\ncafé  au\tlait\nlast line, no newline"))
	_ = w.Close()
	require.EqualValues(t, gz.Len(), rows[0]["gzip"])
	// Markdown: the words of the heading and of "Two words" only.
	require.EqualValues(t, 5, rows[1]["words"])
	require.EqualValues(t, 12, rows[1]["lines"])
	for i, e := range []string{"binary_file", "secret_file_blocked", "file_not_found"} {
		require.Equal(t, e, rows[2+i]["error"])
		require.Nil(t, rows[2+i]["lines"])
	}
	require.Equal(t, map[string]any{"p": "empty", "lines": 0.0, "words": 0.0, "chars": 0.0, "bytes": 0.0, "gzip": rows[5]["gzip"], "max_line": 0.0, "error": nil}, rows[5])

	// One file: under 200 bytes, no error column.
	r = runGo(t, New(), d, env, "llm_textstats", "a.txt")
	require.Less(t, len(r.Stdout), 200)
	require.NotContains(t, r.Stdout, "error")
	r = runGo(t, New(), d, env, "llm_textstats", "a.txt", "--format", "tsv")
	require.True(t, strings.HasPrefix(r.Stdout, "p\tlines\twords\tchars\tbytes\tgzip\tmax_line\na.txt\t3\t9\t47\t48\t"), r.Stdout)
	r = runGo(t, New(), d, env, "llm_textstats")
	require.Contains(t, r.Stderr, `"error":"missing_file"`)
}

func TestCountWords(t *testing.T) {
	for s, n := range map[string]int64{"": 0, "a": 1, " a  b ": 2, "a\tb\vc\fd\re": 5, "é—x y": 2, "a b": 1, "a b": 2} {
		require.Equal(t, n, countWords([]byte(s)), "%q", s)
	}
}

type issueOut struct {
	Files     int
	Issues    []map[string]any
	Counts    map[string]int
	Truncated int
	Skipped   []map[string]string
}

func textcheck(t *testing.T, d string, args ...string) (issueOut, string) {
	t.Helper()
	r := runGo(t, New(), d, baseEnv(physTemp(t), nil), "llm_textcheck", args...)
	require.Equal(t, 0, r.RC, r.Stderr)
	var out issueOut
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &out), r.Stdout)
	return out, r.Stdout
}

func TestTextcheck(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "clean.txt"), "all good\n")
	mustWrite(t, filepath.Join(d, "bad.txt"), "\xef\xbb\xbfbom line\r\n\tcafé \r\n    zero​width\r\nlast")
	mustWrite(t, filepath.Join(d, "bin"), "\x00")
	_, raw := textcheck(t, d, "clean.txt")
	require.Equal(t, `{"files":1,"issues":[]}`+"\n", raw)

	out, _ := textcheck(t, d, "clean.txt", "bad.txt", "bin", "nope")
	require.Equal(t, 2, out.Files)
	require.Equal(t, []map[string]string{{"file": "bin", "reason": "binary_file"}, {"file": "nope", "reason": "file_not_found"}}, out.Skipped)
	type is struct {
		line         float64
		rule, sample string
	}
	var got []is
	for _, i := range out.Issues {
		require.Equal(t, "bad.txt", i["file"])
		got = append(got, is{i["line"].(float64), i["rule"].(string), i["sample"].(string)})
	}
	require.Equal(t, []is{
		{1, "bom", "U+FEFF"},
		{1, "crlf", "3 of 4 lines"},
		{2, "non-ascii", "U+00E9 é"},
		{2, "trailing-ws", "\tcafé "},
		{3, "non-ascii", "U+200B"},
		{3, "mixed-indent", "    zero​width"},
		{4, "final-newline", "last"},
	}, got)
	require.Equal(t, map[string]int{"bom": 1, "crlf": 1, "non-ascii": 2, "trailing-ws": 1, "mixed-indent": 1, "final-newline": 1}, out.Counts)

	out, _ = textcheck(t, d, "bad.txt", "--rules", "tabs,long-lines=10")
	require.Len(t, out.Issues, 2)
	require.Equal(t, "tabs", out.Issues[0]["rule"])
	require.Equal(t, "long-lines", out.Issues[1]["rule"])
	require.Equal(t, "14 chars:     zero\u200bwidth", out.Issues[1]["sample"])

	// The issue list is capped, with the count of what was cut.
	var many strings.Builder
	for range 150 {
		many.WriteString("x \n")
	}
	mustWrite(t, filepath.Join(d, "many.txt"), many.String())
	out, _ = textcheck(t, d, "many.txt")
	require.Len(t, out.Issues, 100)
	require.Equal(t, 50, out.Truncated)
	require.Equal(t, 150, out.Counts["trailing-ws"])
	out, _ = textcheck(t, d, "many.txt", "--max-lines", "2")
	require.Len(t, out.Issues, 2)
	require.Equal(t, 148, out.Truncated)

	r := runGo(t, New(), d, baseEnv(physTemp(t), nil), "llm_textcheck", "many.txt", "--format", "tsv", "--head", "1")
	require.Equal(t, result{"file\tline\trule\tsample\nmany.txt\t1\ttrailing-ws\tx \n", `{"files":1,"truncated":149}` + "\n", 0}, r)

	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"bad.txt", "--rules", "tabs,emoji"}, "invalid_rule"},
		{[]string{"bad.txt", "--rules", "long-lines=0"}, "invalid_rule"},
		{[]string{}, "missing_file"},
	} {
		r := runGo(t, New(), d, baseEnv(physTemp(t), nil), "llm_textcheck", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
	}
	mustWrite(t, filepath.Join(d, "credentials.json"), "{}\n")
	r = runGo(t, New(), d, baseEnv(physTemp(t), nil), "llm_textcheck", "credentials.json")
	require.Contains(t, r.Stderr, `"error":"secret_file_blocked"`)
}

func TestNonASCIISample(t *testing.T) {
	require.Equal(t, "U+00E9 é, U+2014 —, U+00A0, U+200B +1 more", nonASCIISample([]byte("é—é ​è")))
	require.Equal(t, "invalid UTF-8 byte 0xFF", nonASCIISample([]byte("a\xffb")))
}
