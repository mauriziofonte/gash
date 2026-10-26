// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// runStdin runs fn with stdin as its standard input.
func runStdin(t *testing.T, m *Module, cwd string, env map[string]string, stdin, fn string, args ...string) result {
	t.Helper()
	t.Chdir(cwd)
	tbl := invoke.NewTable()
	m.Register(tbl)
	var out, errb bytes.Buffer
	e := invoke.EnvMap(env)
	c := &invoke.Call{Ctx: context.Background(), Name: fn, Args: args, Env: e, Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: &errb,
		Term: term.Fixed(e, false, false, false)}
	rc := tbl.Run(c)
	return result{out.String(), errb.String(), rc}
}

const jsonFixture = `{
  "name": "app",
  "version": "1.0.0",
  "scripts": {"build": "tsc", "test": "vitest run"},
  "paths": {"/api/x": {"post": {"summary": "make é"}}},
  "tags": ["a", "b", "c"],
  "n": 1.50, "e": 1E3, "t": true, "z": null,
  "users": [
    {"id": 1, "name": "ann", "meta": {"k": [1, 2]}},
    {"id": 2, "name": "bob\tx", "role": "admin"},
    {"id": 3, "name": null}
  ],
  "dup": 1, "dup": 2
}
`

func TestJSONPathAndValue(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "f.json"), jsonFixture)
	m := New()
	env := baseEnv(d, nil)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"f.json", "--path", "scripts"}, `{"build":"tsc","test":"vitest run"}`},
		{[]string{"f.json", "--path", "scripts.build"}, `"tsc"`},
		{[]string{"f.json", "--path", `paths["/api/x"].post`}, `{"summary":"make é"}`},
		{[]string{"f.json", "--path", "tags[-1]"}, `"c"`},
		{[]string{"f.json", "--path", "tags[0]"}, `"a"`},
		{[]string{"f.json", "--path", "$.users[1].role"}, `"admin"`},
		{[]string{"f.json", "--path", "n"}, `1.50`},
		{[]string{"f.json", "--path", "e"}, `1E3`},
		{[]string{"f.json", "--path", "z"}, `null`},
		{[]string{"f.json", "--path", "dup"}, `2`},
		{[]string{"f.json", "--path", "."}, `{"name":"app","version":"1.0.0","scripts":{"build":"tsc","test":"vitest run"},` +
			`"paths":{"/api/x":{"post":{"summary":"make é"}}},"tags":["a","b","c"],"n":1.50,"e":1E3,"t":true,"z":null,` +
			`"users":[{"id":1,"name":"ann","meta":{"k":[1,2]}},{"id":2,"name":"bob\tx","role":"admin"},{"id":3,"name":null}],"dup":1,"dup":2}`},
		{[]string{"f.json", "--path", "users[1].name", "--width", "3"}, `"bo…"`},
		{[]string{"f.json", "--keys"}, `["name","version","scripts","paths","tags","n","e","t","z","users","dup"]`},
		{[]string{"f.json", "--path", "users", "--keys"}, `["id","name","meta","role"]`},
	} {
		got := runGo(t, m, d, env, "llm_json", tc.args...)
		require.Equal(t, result{tc.want + "\n", "", 0}, got, "%q", tc.args)
	}
	// The cut array says so on stderr.
	got := runGo(t, m, d, env, "llm_json", "f.json", "--path", "tags", "--tail", "1")
	require.Equal(t, result{`["c"]` + "\n", `{"truncated":2}` + "\n", 0}, got)
	got = runGo(t, m, d, env, "llm_json", "f.json", "--path", "users", "--head", "1")
	require.Equal(t, result{`[{"id":1,"name":"ann","meta":{"k":[1,2]}}]` + "\n", `{"truncated":2}` + "\n", 0}, got)
}

func TestJSONRows(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "f.json"), jsonFixture)
	m := New()
	env := baseEnv(d, nil)
	got := runGo(t, m, d, env, "llm_json", "f.json", "--path", "users", "--format", "tsv")
	require.Equal(t, result{"id\tname\tmeta\trole\n1\tann\t{\"k\":[1,2]}\t\\N\n2\tbob\\tx\t\\N\tadmin\n3\t\\N\t\\N\t\\N\n", "", 0}, got)
	got = runGo(t, m, d, env, "llm_json", "f.json", "--path", "users", "--format", "jsonl", "--head", "2")
	require.Equal(t, result{`{"id":1,"name":"ann","meta":{"k":[1,2]}}` + "\n" + `{"id":2,"name":"bob\tx","role":"admin"}` + "\n", `{"truncated":1}` + "\n", 0}, got)
	got = runGo(t, m, d, env, "llm_json", "f.json", "--path", "scripts", "--format", "tsv")
	require.Equal(t, result{"key\tvalue\nbuild\ttsc\ntest\tvitest run\n", "", 0}, got)
	got = runGo(t, m, d, env, "llm_json", "f.json", "--path", "tags", "--format", "table")
	require.Equal(t, result{"value\na\nb\nc\n", "", 0}, got)
	got = runGo(t, m, d, env, "llm_json", "f.json", "--keys", "--format", "tsv", "--head", "2")
	require.Equal(t, result{"key\nname\nversion\n", `{"truncated":9}` + "\n", 0}, got)
	got = runGo(t, m, d, env, "llm_json", "f.json", "--path", "users", "--format", "json")
	require.Equal(t, `[{"id":1,"name":"ann","meta":{"k":[1,2]}},{"id":2,"name":"bob\tx","role":"admin"},{"id":3,"name":null}]`+"\n", got.Stdout)
}

func TestJSONShape(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "f.json"), jsonFixture)
	m := New()
	env := baseEnv(d, nil)
	got := runGo(t, m, d, env, "llm_json", "f.json", "--shape")
	require.Equal(t, 0, got.RC, got.Stderr)
	require.Equal(t, `{"type":"object{12}","keys":{"name":"string","version":"string",`+
		`"scripts":{"type":"object{2}","keys":{"build":"string","test":"string"}},`+
		`"paths":{"type":"object{1}","keys":{"/api/x":"object{1}"}},`+
		`"tags":{"type":"array[3]","items":"string"},"n":"number","e":"number","t":"boolean","z":"null",`+
		`"users":{"type":"array[3]","items":"object{2..3}"},"dup":"number"}}`+"\n", got.Stdout)
	require.True(t, json.Valid([]byte(got.Stdout)))
	got = runGo(t, m, d, env, "llm_json", "f.json", "--path", "users", "--shape")
	require.Equal(t, result{`{"type":"array[3]","items":{"type":"object{2..3}","keys":{"id":"number","name":"string|null",` +
		`"meta":"object{1}","role":"string"},"optional":["meta","role"]}}` + "\n", "", 0}, got)
	got = runGo(t, m, d, env, "llm_json", "f.json", "--path", "users", "--shape", "--format", "tsv")
	require.Equal(t, result{"path\ttype\n.\tarray[3]\n[]\tobject{2..3}\n[].id\tnumber\n[].name\tstring|null\n[].meta\tobject{1}\n[].role\tstring\n", "", 0}, got)
	got = runGo(t, m, d, env, "llm_json", "f.json", "--path", "name", "--shape")
	require.Equal(t, result{`{"type":"string"}` + "\n", "", 0}, got)
	got = runStdin(t, m, d, env, `[[1,[2]],[],{"a":1}]`, "llm_json", "-", "--shape")
	require.Equal(t, result{`{"type":"array[3]","items":{"type":"array[0..2]|object{1}","keys":{"a":"number"},"items":"number|array[1]"}}` + "\n", "", 0}, got)
	// Keys past the cap are counted, not listed.
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < jsonShapeKeys+5; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"k` + strings.Repeat("x", i) + `":1`)
	}
	b.WriteString("}")
	got = runStdin(t, m, d, env, b.String(), "llm_json", "-", "--shape")
	require.Contains(t, got.Stdout, `,"truncated":5}`)
}

func TestJSONErrors(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "f.json"), jsonFixture)
	mustWrite(t, filepath.Join(d, "bad.json"), "{\n  \"a\": 1,\n  \"b\": [1, 2,]\n}\n")
	mustWrite(t, filepath.Join(d, ".env"), "{}\n")
	m := New()
	env := baseEnv(d, nil)
	code := func(r result) (e struct{ Error, Details, Action, Hint string }) {
		t.Helper()
		require.Equal(t, 1, r.RC, r.Stdout)
		require.Empty(t, r.Stdout)
		require.NoError(t, json.Unmarshal([]byte(r.Stderr), &e), r.Stderr)
		return e
	}
	e := code(runGo(t, m, d, env, "llm_json", "f.json", "--path", "users[1].meta.k"))
	require.Equal(t, "json_path_not_found", e.Error)
	require.Equal(t, "RETRY", e.Action)
	require.Equal(t, "users[1].meta.k", e.Details)
	require.Equal(t, "Nearest existing prefix: users[1] (object, keys: id, name, role)", e.Hint)
	e = code(runGo(t, m, d, env, "llm_json", "f.json", "--path", "tags[9]"))
	require.Equal(t, "Nearest existing prefix: tags (array, length 3)", e.Hint)
	e = code(runGo(t, m, d, env, "llm_json", "f.json", "--path", "name.x"))
	require.Equal(t, "Nearest existing prefix: name (string)", e.Hint)
	e = code(runGo(t, m, d, env, "llm_json", "f.json", "--path", "a..b"))
	require.Equal(t, "invalid_json_path", e.Error)
	e = code(runGo(t, m, d, env, "llm_json", "bad.json"))
	require.Equal(t, "invalid_json", e.Error)
	require.Equal(t, "line 3 column 14: invalid character ']', expected a value", e.Details)
	e = code(runGo(t, m, d, env, "llm_json", "f.json", "--path", "name", "--keys"))
	require.Equal(t, "not_an_object", e.Error)
	e = code(runGo(t, m, d, env, "llm_json", ".env"))
	require.Equal(t, "secret_file_blocked", e.Error)
	require.Equal(t, "FATAL", e.Action)
	e = code(runGo(t, m, d, env, "llm_json", "nope.json"))
	require.Equal(t, "file_not_found", e.Error)
	e = code(runGo(t, m, d, env, "llm_json"))
	require.Equal(t, "missing_file", e.Error)
	e = code(runGo(t, m, d, env, "llm_json", "f.json", "f.json"))
	require.Equal(t, "unexpected_argument", e.Error)
	e = code(runGo(t, m, d, env, "llm_json", "f.json", "--keys", "--shape"))
	require.Equal(t, "unexpected_argument", e.Error)
	e = code(runGo(t, m, d, env, "llm_json", "f.json", "--path"))
	require.Equal(t, "missing_value", e.Error)
	e = code(runGo(t, m, d, env, "llm_json", "f.json", "--format", "csv"))
	require.Equal(t, "invalid_format", e.Error)
	e = code(runGo(t, m, d, env, "llm_json", "/etc/shadow"))
	require.Equal(t, "forbidden_path", e.Error)
	for in, msg := range map[string]string{
		``:              "line 1 column 1: unexpected end of input, expected a value",
		`{"a":1} x`:     "line 1 column 9: extra data after the value",
		`"a` + "\x01\"": "line 1 column 3: control character in string",
		`[01]`:          "line 1 column 3: expected ',' or ']' after an array element",
		`[1.]`:          "line 1 column 4: invalid number: a digit must follow the decimal point",
		`{"a" 1}`:       "line 1 column 6: expected ':' after an object key",
		`"\x"`:          `line 1 column 3: invalid escape in string`,
		`[tru]`:         "line 1 column 2: invalid literal, expected true",
		"\"\xff\"":      "line 1 column 2: not valid UTF-8",
		`NaN`:           "line 1 column 1: invalid character 'N', expected a value",
	} {
		e = code(runStdin(t, m, d, env, in, "llm_json", "-"))
		require.Equal(t, "invalid_json", e.Error, in)
		require.Equal(t, msg, e.Details, "%q", in)
	}
	// Deep nesting is refused, not a crash.
	e = code(runStdin(t, m, d, env, strings.Repeat("[", jsonMaxDepth+5), "llm_json", "-"))
	require.Equal(t, "invalid_json", e.Error)
	require.Contains(t, e.Details, "nested too deep")
}

// TestJSONMatchesEncodingJSON: on valid and invalid texts the scanner
// agrees with encoding/json.Valid, and --path . gives the same value.
func TestJSONMatchesEncodingJSON(t *testing.T) {
	for _, in := range []string{
		`0`, `-0`, `-0.5e-3`, `1e+9`, `"😀"`, `{"a":{"b":[true,false,null]}}`, ` [ ] `, `{"":""}`,
		`[1,]`, `{"a":1,}`, `01`, `+1`, `.5`, `1e`, `"\u12"`, `[`, `{"a"}`, `"a"b`, `tru`, `nul`, `[1 2]`,
		"\xef\xbb\xbf[1]", "\"\t\"", `"\/"`,
	} {
		_, err := validJSON([]byte(in))
		want := json.Valid(bytes.TrimPrefix([]byte(in), []byte("\xef\xbb\xbf")))
		require.Equal(t, want, err == nil, "%q: %v", in, err)
		if err == nil {
			start, _ := validJSON([]byte(in))
			var a, b any
			require.NoError(t, json.Unmarshal(appendCompact(nil, valueAt([]byte(in), start).raw(), 0), &a))
			require.NoError(t, json.Unmarshal(bytes.TrimPrefix([]byte(in), []byte("\xef\xbb\xbf")), &b))
			require.Equal(t, b, a, in)
		}
	}
}
