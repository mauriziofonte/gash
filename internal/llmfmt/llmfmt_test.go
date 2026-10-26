// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmfmt

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
)

func call() (*invoke.Call, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	return &invoke.Call{Ctx: context.Background(), Name: "llm_test_fn", Stdout: &out, Stderr: &errb, Env: invoke.EnvMap(nil)}, &out, &errb
}

func envelopeCode(t *testing.T, s string) string {
	t.Helper()
	var e struct{ Error, Action string }
	require.NoError(t, json.Unmarshal([]byte(s), &e), s)
	return e.Error + "/" + e.Action
}

func TestTakeParsesSharedFlags(t *testing.T) {
	c, _, _ := call()
	o := New(FormatFlag | BudgetFlags | JobsFlag | TimeoutFlag | PathFlags)
	args := []string{"--format", "tsv", "--width", "80", "--max-lines", "5", "--tail", "2", "-j", "4", "--timeout", "1.5", "--relative", "x"}
	for len(args) > 0 {
		n, ok := o.Take(c, args)
		require.True(t, ok)
		if n == 0 {
			require.Equal(t, []string{"x"}, args)
			break
		}
		args = args[n:]
	}
	require.Equal(t, TSV, o.Format)
	require.Equal(t, 80, o.Width)
	require.Equal(t, 5, o.Head)
	require.Equal(t, 2, o.Tail)
	require.Equal(t, 4, o.Jobs)
	require.Equal(t, 1500*time.Millisecond, o.Timeout)
	require.True(t, o.Relative)
}

func TestTakeRefusesBadValues(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--format", "xml"}, "invalid_format/RETRY"},
		{[]string{"--format"}, "missing_value/RETRY"},
		{[]string{"--width", "0"}, "invalid_width/RETRY"},
		{[]string{"--max-lines", "-1"}, "invalid_max_lines/RETRY"},
		{[]string{"--head", "x"}, "invalid_head/RETRY"},
		{[]string{"--tail", "1234567890"}, "invalid_tail/RETRY"},
		{[]string{"-j", "0"}, "invalid_jobs/RETRY"},
		{[]string{"--timeout", "0"}, "invalid_timeout/RETRY"},
		{[]string{"--timeout", "2h"}, "invalid_timeout/RETRY"},
	} {
		c, _, errb := call()
		o := New(FormatFlag | BudgetFlags | JobsFlag | TimeoutFlag)
		_, ok := o.Take(c, tc.args)
		require.False(t, ok, tc.args)
		require.Equal(t, tc.want, envelopeCode(t, errb.String()), tc.args)
	}
}

func TestTakeIgnoresFlagsNotAccepted(t *testing.T) {
	c, _, _ := call()
	o := New(BudgetFlags)
	n, ok := o.Take(c, []string{"--format", "tsv"})
	require.True(t, ok)
	require.Zero(t, n)
}

func TestParseSeconds(t *testing.T) {
	for in, want := range map[string]time.Duration{"2": 2 * time.Second, "0.5": 500 * time.Millisecond, "500ms": 500 * time.Millisecond, "1m": time.Minute} {
		d, err := ParseSeconds(in)
		require.NoError(t, err, in)
		require.Equal(t, want, d, in)
	}
	for _, in := range []string{"", "-1", "0", "abc", "61m"} {
		_, err := ParseSeconds(in)
		require.Error(t, err, in)
	}
}

func TestCutIsUTF8Aware(t *testing.T) {
	require.Equal(t, "hello", Cut("hello", 5))
	require.Equal(t, "hel…", Cut("hello", 4))
	require.Equal(t, "hello", Cut("hello", 0))
	require.Equal(t, "àè…", Cut("àèìòù", 3))
	// Wide characters take two columns and are never split.
	require.Equal(t, "日…", Cut("日本語", 4))
	require.Equal(t, 6, StringWidth("日本語"))
	// A combining accent takes no column.
	require.Equal(t, 1, StringWidth("é"))
}

func TestLinesHeadTailMarker(t *testing.T) {
	var b bytes.Buffer
	o := &Options{Head: 2, Tail: 2}
	l := o.Lines(&b, "")
	for i := 1; i <= 7; i++ {
		l.Line(strings.Repeat("x", i))
	}
	require.Equal(t, 3, l.Close())
	require.Equal(t, "x\nxx\n[... 3 lines omitted]\nxxxxxx\nxxxxxxx\n", b.String())

	b.Reset()
	o = &Options{Head: 10, Width: 3}
	l = o.Lines(&b, "")
	l.Line("abcdef")
	require.Equal(t, 0, l.Close())
	require.Equal(t, "ab…\n", b.String())

	b.Reset()
	o = &Options{Tail: 1}
	l = o.Lines(&b, "")
	l.Line("a")
	l.Line("b")
	require.Equal(t, 1, l.Close())
	require.Equal(t, "[... 1 lines omitted]\nb\n", b.String())
}

func TestCap(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	h, tl, k := Cap(&Options{}, items)
	require.Equal(t, items, h)
	require.Nil(t, tl)
	require.Zero(t, k)
	h, tl, k = Cap(&Options{Head: 2}, items)
	require.Equal(t, []int{1, 2}, h)
	require.Nil(t, tl)
	require.Equal(t, 3, k)
	h, tl, k = Cap(&Options{Head: 1, Tail: 1}, items)
	require.Equal(t, []int{1}, h)
	require.Equal(t, []int{5}, tl)
	require.Equal(t, 3, k)
}

var sample = Table{
	Cols: []string{"id", "name", "note"},
	Rows: [][]any{
		{1, "a\tb", nil},
		{int64(2), "c\\d", true},
		{2.5, "line\nbreak", map[string]any{"k": 1}},
	},
}

func TestWriteFormats(t *testing.T) {
	c, out, errb := call()
	o := New(FormatFlag)
	require.Zero(t, o.Write(c, Default, sample, nil))
	require.Equal(t, `[{"id":1,"name":"a\tb","note":null},{"id":2,"name":"c\\d","note":true},{"id":2.5,"name":"line\nbreak","note":{"k":1}}]`+"\n", out.String())
	require.Empty(t, errb.String())

	c, out, _ = call()
	o.Write(c, JSONL, sample, nil)
	require.Equal(t, 3, strings.Count(out.String(), "\n"))
	require.True(t, strings.HasPrefix(out.String(), `{"id":1,"name":"a\tb","note":null}`+"\n"))

	c, out, _ = call()
	o.Write(c, TSV, sample, nil)
	require.Equal(t, "id\tname\tnote\n1\ta\\tb\t\\N\n2\tc\\\\d\ttrue\n2.5\tline\\nbreak\t{\"k\":1}\n", out.String())

	c, out, _ = call()
	o.Write(c, TableF, sample, nil)
	require.Equal(t, "id   name        note\n1    a b         \n2    c\\d         true\n2.5  line break  {\"k\":1}\n", out.String())
}

func TestWriteCapReportsTruncation(t *testing.T) {
	c, out, errb := call()
	o := &Options{allow: FormatFlag | BudgetFlags, Head: 1}
	require.Equal(t, 2, o.Write(c, TSV, sample, map[string]any{"rows": 3}))
	require.Equal(t, "id\tname\tnote\n1\ta\\tb\t\\N\n", out.String())
	require.Equal(t, `{"rows":3,"truncated":2}`+"\n", errb.String())
	require.Equal(t, 2, c.Truncated())

	c, out, _ = call()
	o.Write(c, TableF, sample, nil)
	require.Contains(t, out.String(), "[... 2 rows omitted]\n")
}

func TestCountWrite(t *testing.T) {
	c, out, _ := call()
	o := New(FormatFlag)
	Count{N: 3}.Write(c, o, Default)
	require.Equal(t, `{"count":3}`+"\n", out.String())

	c, out, _ = call()
	Count{N: 3, By: map[string]int{"php": 2, "js": 1}}.Write(c, o, JSON)
	require.Equal(t, `{"count":3,"by":{"js":1,"php":2}}`+"\n", out.String())

	c, out, errb := call()
	Count{N: 3, By: map[string]int{"php": 2, "js": 1}}.Write(c, o, TSV)
	require.Equal(t, "key\tcount\nphp\t2\njs\t1\n", out.String())
	require.Equal(t, `{"count":3}`+"\n", errb.String())
}

func TestMapKeepsOrderAndBoundsEachItem(t *testing.T) {
	items := []int{50, 1, 30, 2, 10}
	got := Map(context.Background(), 3, 0, items, func(_ context.Context, ms int) int {
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return ms * 2
	})
	require.Equal(t, []int{100, 2, 60, 4, 20}, got)

	res := Map(context.Background(), 2, 20*time.Millisecond, []int{1, 500}, func(ctx context.Context, ms int) string {
		select {
		case <-time.After(time.Duration(ms) * time.Millisecond):
			return "ok"
		case <-ctx.Done():
			return "timeout"
		}
	})
	require.Equal(t, []string{"ok", "timeout"}, res)
	require.Empty(t, Map(context.Background(), 4, 0, []int(nil), func(context.Context, int) int { return 0 }))
}

func TestRel(t *testing.T) {
	require.Equal(t, "src/a.php", Rel("/p", "/p/src/a.php"))
	require.Equal(t, ".", Rel("/p", "/p"))
	require.Equal(t, "/q/x", Rel("/p", "/q/x"))
	require.Equal(t, "/pp/x", Rel("/p", "/pp/x"))
	require.Equal(t, "etc/x", Rel("/", "/etc/x"))
	require.Equal(t, "a/b", Rel(".", "./a/b"))
}

func TestMs(t *testing.T) {
	require.Equal(t, 1.5, Ms(1500*time.Microsecond))
	require.Equal(t, 0.0, Ms(10*time.Microsecond))
}
