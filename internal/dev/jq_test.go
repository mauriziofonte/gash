// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package dev

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestJQPathRaw pins jqPathRaw against the documented jq 1.7 behavior:
// the stream of values, the // empty rule, number text, a rendered
// non-string, and the stop on a type error.
func TestJQPathRaw(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`{"require":{"php":"8.1"}}`, "8.1\n"},
		{`{"require":{"php":8.10}}`, "8.10\n"},
		{`{"require":{"php":100e-2}}`, "1.00\n"},
		{`{"require":{"php":1E-7}}`, "1E-7\n"},
		{`{"require":{"php":1e1000}}`, "1E+1000\n"},
		{`{"require":{"php":null}}`, ""},
		{`{"require":{"php":false}}`, ""},
		{`{"require":{}}`, ""},
		{`{}`, ""},
		{`{"require":{"php":{"v":"7.3"}}}`, "{\n  \"v\": \"7.3\"\n}\n"},
		{"\ufeff{\"require\":{\"php\":\"8.1\"}}", "8.1\n"},
		// A type error stops the stream: nothing already printed here.
		{`{"require":"s"}`, ""},
		{`"x" {"require":{"php":"8.1"}}`, ""},
		// Valid values before an error are kept.
		{`{"require":{"php":"8.1"}} {"require":{"php":"9.1"}}`, "8.1\n9.1\n"},
		// NaN and Infinity as jq decodes them.
		{`{"require":{"php":NaN}}`, "null\n"},
		{`{"require":{"php":Infinity}}`, "1.7976931348623157e+308\n"},
	} {
		require.Equal(t, c.want, jqPathRaw([]byte(c.in), "require", "php"), "%q", c.in)
	}
}

// TestJQMatchesJq checks jqPathRaw against the installed jq 1.7 on random
// and curated inputs; skipped without jq 1.7.
func TestJQMatchesJq(t *testing.T) {
	jqBin, err := exec.LookPath("jq")
	if err != nil {
		t.Skip("no jq")
	}
	if v, _ := exec.Command(jqBin, "--version").Output(); !regexp.MustCompile(`^jq-1\.7`).Match(v) {
		t.Skip("not jq 1.7")
	}
	jqSays := func(in string) string {
		cmd := exec.Command(jqBin, "-r", ".require.php // empty")
		cmd.Stdin = strings.NewReader(in)
		out, _ := cmd.Output() // 2>/dev/null: errors ignored, partial stdout kept
		return string(out)
	}
	inputs := []string{
		`{"require":{"php":"8.1"}}`, `{"require":{"php":8.10}}`, `{"require":{"php":100e-2}}`,
		`{"require":{"php":1E-7}}`, `{"require":{"php":1e1000}}`, `{"require":{"php":0.5}}`,
		`{"require":{"php":{"v":"7.3","w":[1,2.5,null,true]}}}`, `{"require":{"php":[1,"a"]}}`,
		`{"require":{"php":null}}`, `{"require":{}}`, `{}`, `{"require":"s"}`,
		`{"require":{"php":"8.1"}} {"require":{"php":"9.1"}}`,
		`{"require":{"php":-0.0}}`, `{"require":{"php":01}}`, `{"require":{"php":1.}}`,
	}
	for _, in := range inputs {
		require.Equal(t, jqSays(in), jqPathRaw([]byte(in), "require", "php"), "%q", in)
	}
}
