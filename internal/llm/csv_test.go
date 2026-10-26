// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const csvFixture = "id,name,note\n" +
	"1,ann,\"hello, world\"\n" +
	"2,bob,\"multi\nline\"\n" +
	"3,cy,plain\n" +
	"4,dee,\"say \"\"hi\"\"\"\n" +
	"5,eve,x\n" +
	"6,fay,y\n" +
	"7\n"

func TestCSVSummary(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "a.csv"), csvFixture)
	mustWrite(t, filepath.Join(d, "semi.csv"), "a;b;c\n1;2,5;3\n4;5;6\n")
	mustWrite(t, filepath.Join(d, "tab.tsv"), "a\tb\n1\t2\n")
	mustWrite(t, filepath.Join(d, "bom.csv"), "\xef\xbb\xbfid,name\r\n1,a\r\n\r\n2,b")
	mustWrite(t, filepath.Join(d, "empty.csv"), "")
	mustWrite(t, filepath.Join(d, "bad.csv"), "a,b\n1,2\n3\n4,5,6\n5,x\"y\n\"6\"z,7\n\"7,8\n")
	m := New()
	env := baseEnv(d, nil)
	sample := `[["1","ann","hello, world"],["2","bob","multi\nline"],["3","cy","plain"],["4","dee","say \"hi\""],["5","eve","x"]]`
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"a.csv"}, `{"cols":3,"header":["id","name","note"],"sample":` + sample + `,"delimiter":",","errors":[]}`},
		// --count reads to the end: the quoted newline is one record, the
		// short last record is listed at its physical line.
		{[]string{"a.csv", "--count", "--head", "1"}, `{"rows":7,"cols":3,"header":["id","name","note"],"sample":[["1","ann","hello, world"]],` +
			`"delimiter":",","errors":[{"line":9,"error":"1 fields, header has 3"}],"error_count":1}`},
		{[]string{"a.csv", "--cols"}, `{"cols":3,"header":["id","name","note"],"delimiter":",","errors":[]}`},
		// The filter reads on until the sample is full: here, to the end.
		{[]string{"a.csv", "--where", "name=bob"}, `{"cols":3,"header":["id","name","note"],"sample":[["2","bob","multi\nline"]],` +
			`"delimiter":",","errors":[{"line":9,"error":"1 fields, header has 3"}]}`},
		{[]string{"a.csv", "--where", "3=y", "--count", "--head", "0"}, `{"rows":7,"matches":1,"cols":3,"header":["id","name","note"],"sample":[],` +
			`"delimiter":",","errors":[{"line":9,"error":"1 fields, header has 3"}],"error_count":1}`},
		{[]string{"semi.csv", "--head", "1"}, `{"cols":3,"header":["a","b","c"],"sample":[["1","2,5","3"]],"delimiter":";","errors":[]}`},
		{[]string{"semi.csv", "--head", "1", "--delim", ","}, `{"cols":1,"header":["a;b;c"],"sample":[["1;2","5;3"]],"delimiter":",","errors":[{"line":2,"error":"2 fields, header has 1"}]}`},
		{[]string{"tab.tsv"}, `{"cols":2,"header":["a","b"],"sample":[["1","2"]],"delimiter":"\t","errors":[]}`},
		{[]string{"bom.csv", "--count"}, `{"rows":2,"cols":2,"header":["id","name"],"sample":[["1","a"],["2","b"]],"delimiter":",","errors":[],"error_count":0}`},
		{[]string{"empty.csv", "--count"}, `{"rows":0,"cols":0,"header":[],"sample":[],"delimiter":",","errors":[],"error_count":0}`},
		{[]string{"bad.csv", "--count", "--head", "0"}, `{"rows":6,"cols":2,"header":["a","b"],"sample":[],"delimiter":",","errors":[` +
			`{"line":3,"error":"1 fields, header has 2"},{"line":4,"error":"3 fields, header has 2"},` +
			`{"line":5,"error":"quote in an unquoted field"},{"line":6,"error":"text after a closing quote"},` +
			`{"line":7,"error":"1 fields, header has 2; unterminated quoted field"}],"error_count":5}`},
	} {
		got := runGo(t, m, d, env, "llm_csv", tc.args...)
		require.Equal(t, result{tc.want + "\n", "", 0}, got, "%q", tc.args)
		var v map[string]any
		require.NoError(t, json.Unmarshal([]byte(got.Stdout), &v), got.Stdout)
	}

	// Row formats: the sample as rows, the rest on one stderr line.
	got := runGo(t, m, d, env, "llm_csv", "a.csv", "--head", "2", "--format", "tsv")
	require.Equal(t, result{"id\tname\tnote\n1\tann\thello, world\n2\tbob\tmulti\\nline\n", `{"delimiter":","}` + "\n", 0}, got)
	got = runGo(t, m, d, env, "llm_csv", "a.csv", "--cols", "--count", "--format", "tsv")
	require.Equal(t, result{"column\nid\nname\nnote\n",
		`{"delimiter":",","error_count":1,"errors":[{"error":"1 fields, header has 3","line":9}],"rows":7}` + "\n", 0}, got)
}

func TestCSVManyErrors(t *testing.T) {
	d := physTemp(t)
	var b strings.Builder
	b.WriteString("a,b\n")
	for i := range 15 {
		fmt.Fprintf(&b, "%d\n%d,ok\n", i, i)
	}
	mustWrite(t, filepath.Join(d, "e.csv"), b.String())
	m := New()
	got := runGo(t, m, d, baseEnv(d, nil), "llm_csv", "e.csv", "--count", "--head", "0")
	require.Equal(t, 0, got.RC)
	var v struct {
		Rows       int
		Errors     []struct{ Line int }
		ErrorCount int `json:"error_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(got.Stdout), &v))
	require.Equal(t, 30, v.Rows)
	require.Equal(t, 15, v.ErrorCount)
	require.Len(t, v.Errors, csvMaxErrors)
	require.Equal(t, 2, v.Errors[0].Line)
	require.Equal(t, 20, v.Errors[9].Line)
}

func TestCSVErrors(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "a.csv"), csvFixture)
	mustWrite(t, filepath.Join(d, ".env"), "A=1\n")
	m := New()
	env := baseEnv(d, nil)
	code := func(r result) (e struct{ Error, Details, Action, Hint string }) {
		t.Helper()
		require.Equal(t, 1, r.RC, r.Stdout)
		require.Empty(t, r.Stdout)
		require.NoError(t, json.Unmarshal([]byte(r.Stderr), &e), r.Stderr)
		return e
	}
	e := code(runGo(t, m, d, env, "llm_csv", "a.csv", "--where", "nope=1"))
	require.Equal(t, "unknown_column", e.Error)
	require.Equal(t, "RETRY", e.Action)
	require.Equal(t, "Columns: id, name, note (or a 1-based number)", e.Hint)
	e = code(runGo(t, m, d, env, "llm_csv", "a.csv", "--where", "4=x"))
	require.Equal(t, "unknown_column", e.Error)
	require.Equal(t, "invalid_where", code(runGo(t, m, d, env, "llm_csv", "a.csv", "--where", "name")).Error)
	require.Equal(t, "invalid_delimiter", code(runGo(t, m, d, env, "llm_csv", "a.csv", "--delim", "ab")).Error)
	require.Equal(t, "invalid_delimiter", code(runGo(t, m, d, env, "llm_csv", "a.csv", "--delim", `"`)).Error)
	require.Equal(t, "missing_file", code(runGo(t, m, d, env, "llm_csv")).Error)
	require.Equal(t, "unknown_option", code(runGo(t, m, d, env, "llm_csv", "a.csv", "--bogus")).Error)
	require.Equal(t, "secret_file_blocked", code(runGo(t, m, d, env, "llm_csv", ".env")).Error)
}

func TestCSVParseDelim(t *testing.T) {
	for in, want := range map[string]byte{",": ',', ";": ';', "|": '|', "tab": '\t', `\t`: '\t', "\t": '\t'} {
		got, ok := parseDelim(in)
		require.True(t, ok, in)
		require.Equal(t, want, got, in)
	}
	for _, in := range []string{"", "ab", `"`, "\n", "\r"} {
		_, ok := parseDelim(in)
		require.False(t, ok, "%q", in)
	}
}

// csvRecords parses text with the streaming parser, fed in chunks of
// sizes drawn from sizes (the whole text at once when sizes is nil).
func csvRecords(text string, delim byte, sizes func() int) ([][]string, []int) {
	var recs [][]string
	var lines []int
	p := newCSVParser(delim, -1, func() bool { return true }, func(r *csvRecord) bool {
		recs = append(recs, append([]string(nil), r.fields...))
		lines = append(lines, r.line)
		return true
	})
	b := []byte(text)
	if sizes == nil {
		p.feed(b)
	} else {
		for len(b) > 0 {
			n := min(sizes(), len(b))
			p.feed(b[:n])
			b = b[n:]
		}
	}
	p.finish()
	return recs, lines
}

// TestCSVParserMatchesEncodingCSV checks the streaming parser, whole and
// cut at every kind of chunk boundary, against encoding/csv on valid
// RFC 4180 text (quoted delimiters, quotes and newlines; LF and CRLF).
func TestCSVParserMatchesEncodingCSV(t *testing.T) {
	r := rand.New(rand.NewSource(34))
	alphabet := []string{"a", "b", " ", ",", ";", "\"", "\n", "é"}
	field := func() string {
		var s strings.Builder
		for range r.Intn(5) {
			s.WriteString(alphabet[r.Intn(len(alphabet))])
		}
		v := s.String()
		if strings.ContainsAny(v, ",\"\n") || (v != "" && r.Intn(4) == 0) {
			return `"` + strings.ReplaceAll(v, `"`, `""`) + `"`
		}
		return v
	}
	for iter := range 400 {
		var text strings.Builder
		nl := "\n"
		if r.Intn(2) == 0 {
			nl = "\r\n"
		}
		for range 1 + r.Intn(6) {
			cols := 1 + r.Intn(4)
			for c := range cols {
				if c > 0 {
					text.WriteByte(',')
				}
				text.WriteString(field())
			}
			text.WriteString(nl)
		}
		in := text.String()
		if r.Intn(3) == 0 {
			in = strings.TrimSuffix(in, nl)
		}
		cr := csv.NewReader(strings.NewReader(in))
		cr.FieldsPerRecord = -1
		want, err := cr.ReadAll()
		require.NoError(t, err, "%q", in)
		got, _ := csvRecords(in, ',', nil)
		require.Equal(t, want, got, "iteration %d: %q", iter, in)
		for _, max := range []int{1, 2, 3, 7} {
			chunked, _ := csvRecords(in, ',', func() int { return 1 + r.Intn(max) })
			require.Equal(t, want, chunked, "iteration %d, chunks up to %d: %q", iter, max, in)
		}
	}
}

func TestCSVParserLinesAndCR(t *testing.T) {
	recs, lines := csvRecords("a,\"x\ny\"\n\nb,c\r\nd\re,f", ',', nil)
	require.Equal(t, [][]string{{"a", "x\ny"}, {"b", "c"}, {"d\re", "f"}}, recs)
	require.Equal(t, []int{1, 4, 5}, lines)
	// A CRLF cut between its two bytes is still one line end.
	recs, lines = csvRecords("a\r\nb\r\n", ',', func() int { return 2 })
	require.Equal(t, [][]string{{"a"}, {"b"}}, recs)
	require.Equal(t, []int{1, 2}, lines)
	recs, _ = csvRecords("a\r", ',', nil)
	require.Equal(t, [][]string{{"a"}}, recs)
}

func TestCSVSniff(t *testing.T) {
	for in, want := range map[string]byte{
		"a,b,c\n1,2,3\n":          ',',
		"a;b\n1;2,5\n":            ';',
		"a\tb\n1\t2\n":            '\t',
		"a|b\n1|2\n":              '|',
		"\"x;y\",b\n1,2\n":        ',',
		"name\nann\n":             ',',
		"a;b;c\n1;2\n3,4;5;6;7\n": ';',
	} {
		require.Equal(t, string(want), string(sniffDelim([]byte(in))), "%q", in)
	}
}
