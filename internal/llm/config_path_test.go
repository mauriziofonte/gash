// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const tomlFixture = `# a comment
[project]
name = "demo"
version = "1.2.0"
dependencies = ["requests>=2", "rich"]
authors = [{ name = "Ann", email = "ann@example.com" }]

[project.optional-dependencies]
dev = ["pytest", "ruff"]

[tool.poetry]
"weird.key" = true
float = 3.14
big = 1e20
neg0 = -0.0
int = 0x1F
when = 1979-05-27T07:32:00Z
when2 = 1979-05-27T00:32:00.5-07:00
day = 1979-05-27
clock = 07:32:00
local = 1979-05-27T07:32:00
nested.a.b = 1
str = """
multi
line"""
lit = 'C:\path'

[[tool.mypy.overrides]]
module = "a.*"
ignore = true

[[tool.mypy.overrides]]
ignore = false
module = "b"
`

// The values Python's configparser gives (checked with CPython 3.12:
// {s: dict(cfg[s]) for s in cfg}, DEFAULT left out when empty, a value
// that does not interpolate read raw).
const iniFixture = `[DEFAULT]
Base = /srv
log = %(base)s/log

[metadata]
name = demo
Version: 1.0
description = a %% sign
long = first line
    second line

    after blank
# comment
; other comment
path = %(base)s/app
url = http://x:8080/y
bad = 100%
missing = %(nope)s
empty =

[options]
install_requires =
    requests
    rich
log = %(path)s/override
path = /opt
`

func TestConfigPathFormats(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "pyproject.toml"), tomlFixture)
	mustWrite(t, filepath.Join(d, "setup.cfg"), iniFixture)
	mustWrite(t, filepath.Join(d, "m.yaml"), "kind: A\nspec:\n  n: 010\n---\nkind: B\n")
	mustWrite(t, filepath.Join(d, "one.yml"), "a: 1\nb: [x, y]\n")
	mustWrite(t, filepath.Join(d, "empty.yaml"), "# nothing\n")
	mustWrite(t, filepath.Join(d, "c.json"), `{"a": {"b": [1, 2.50, "\u00e9"]}}`)
	mustWrite(t, filepath.Join(d, "gitconfig"), "[core]\n\tbare = false\n[remote \"origin\"]\n\turl = git@x:y.git\n")
	mustWrite(t, filepath.Join(d, "noext"), "x = 1\n[t]\ny = \"z\"\n")
	mustWrite(t, filepath.Join(d, "data.rc"), `{"a": 1e2}`)
	mustWrite(t, filepath.Join(d, "list.conf"), "- a\n- b\n")
	m := New()
	env := baseEnv(d, nil)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"pyproject.toml", "--path", "."}, `{"project":{"name":"demo","version":"1.2.0","dependencies":["requests>=2","rich"],` +
			`"authors":[{"name":"Ann","email":"ann@example.com"}],"optional-dependencies":{"dev":["pytest","ruff"]}},` +
			`"tool":{"poetry":{"weird.key":true,"float":3.14,"big":1E+20,"neg0":-0.0,"int":31,"when":"1979-05-27T07:32:00Z",` +
			`"when2":"1979-05-27T00:32:00.5-07:00","day":"1979-05-27","clock":"07:32:00","local":"1979-05-27T07:32:00",` +
			`"nested":{"a":{"b":1}},"str":"multi\nline","lit":"C:\\path"},` +
			`"mypy":{"overrides":[{"module":"a.*","ignore":true},{"ignore":false,"module":"b"}]}}}`},
		{[]string{"pyproject.toml", "--path", "project.dependencies[-1]"}, `"rich"`},
		{[]string{"pyproject.toml", "--path", `tool.poetry["weird.key"]`}, `true`},
		{[]string{"setup.cfg", "--path", "."}, `{"DEFAULT":{"base":"/srv","log":"/srv/log"},` +
			`"metadata":{"name":"demo","version":"1.0","description":"a % sign","long":"first line\nsecond line\n\nafter blank",` +
			`"path":"/srv/app","url":"http://x:8080/y","bad":"100%","missing":"%(nope)s","empty":"","base":"/srv","log":"/srv/log"},` +
			`"options":{"install_requires":"\nrequests\nrich","log":"/opt/override","path":"/opt","base":"/srv"}}`},
		{[]string{"setup.cfg", "--path", "metadata.version"}, `"1.0"`},
		{[]string{"m.yaml", "--path", "."}, `[{"kind":"A","spec":{"n":10}},{"kind":"B"}]`},
		{[]string{"m.yaml", "--path", "[1].kind"}, `"B"`},
		{[]string{"one.yml", "--path", "b"}, `["x","y"]`},
		{[]string{"empty.yaml", "--path", "."}, `null`},
		// JSON keeps its numbers and escapes as written.
		{[]string{"c.json", "--path", "a.b"}, `[1,2.50,"\u00e9"]`},
		// Another extension: the first of JSON, TOML, INI and YAML that parses.
		{[]string{"data.rc", "--path", "a"}, `1e2`},
		{[]string{"noext", "--path", "t.y"}, `"z"`},
		{[]string{"gitconfig", "--path", `["remote \"origin\""].url`}, `"git@x:y.git"`},
		{[]string{"list.conf", "--path", "[0]"}, `"a"`},
		{[]string{"--path", "x", "pyproject.toml", "--max-bytes", "5000", "--path", "project.name"}, `"demo"`},
	} {
		got := runGo(t, m, d, env, "llm_config", tc.args...)
		require.Equal(t, result{tc.want + "\n", "", 0}, got, "%q", tc.args)
		require.True(t, json.Valid([]byte(got.Stdout)), got.Stdout)
	}
	// Without --path nothing changes: TOML and INI print raw.
	got := runGo(t, m, d, env, "llm_config", "pyproject.toml")
	require.Equal(t, result{tomlFixture, "", 0}, got)
}

func TestConfigPathErrors(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "bad.toml"), "a = \n")
	mustWrite(t, filepath.Join(d, "nosec.ini"), "k = v\n")
	mustWrite(t, filepath.Join(d, "nokey.ini"), "[s]\n= v\n")
	mustWrite(t, filepath.Join(d, "nodelim.ini"), "[s]\nflag\n")
	mustWrite(t, filepath.Join(d, "bad.yaml"), "a: [1\n")
	mustWrite(t, filepath.Join(d, "bad.json"), `{"a":1,}`)
	mustWrite(t, filepath.Join(d, "ok.toml"), "[t]\na = [1, 2]\n")
	mustWrite(t, filepath.Join(d, ".env"), "TOKEN=x\n")
	mustWrite(t, filepath.Join(d, "big.toml"), "a = \""+strings.Repeat("x", 100)+"\"\n")
	m := New()
	env := baseEnv(d, nil)
	code := func(r result) (e struct{ Error, Details, Action, Hint string }) {
		t.Helper()
		require.Equal(t, 1, r.RC, r.Stdout)
		require.Empty(t, r.Stdout)
		require.NoError(t, json.Unmarshal([]byte(r.Stderr), &e), r.Stderr)
		return e
	}
	for file, details := range map[string]string{
		"bad.toml":    "TOML line 1: expected value but found '\\n' instead",
		"nosec.ini":   "INI line 1: a key before any [section] header",
		"nokey.ini":   "INI line 2: a value with no key",
		"nodelim.ini": "INI line 2: no '=' or ':' after the key",
		"bad.yaml":    "YAML line 1: did not find expected ',' or ']'",
		"bad.json":    "JSON line 1 column 8: expected a string key",
	} {
		e := code(runGo(t, m, d, env, "llm_config", file, "--path", "."))
		require.Equal(t, "invalid_config", e.Error, file)
		require.Equal(t, "RETRY", e.Action, file)
		require.Equal(t, details, e.Details, file)
	}
	e := code(runGo(t, m, d, env, "llm_config", "ok.toml", "--path", "t.a[2]"))
	require.Equal(t, "json_path_not_found", e.Error)
	require.Equal(t, "Nearest existing prefix: t.a (array, length 2)", e.Hint)
	e = code(runGo(t, m, d, env, "llm_config", "ok.toml", "--path", "u"))
	require.Equal(t, "Nearest existing prefix: . (object, keys: t)", e.Hint)
	e = code(runGo(t, m, d, env, "llm_config", "ok.toml", "--path", "t..a"))
	require.Equal(t, "invalid_json_path", e.Error)
	require.Equal(t, "secret_file_blocked", code(runGo(t, m, d, env, "llm_config", ".env", "--path", ".")).Error)
	require.Equal(t, "file_too_large", code(runGo(t, m, d, env, "llm_config", "big.toml", "--path", ".", "--max-bytes", "50")).Error)
	require.Equal(t, "missing_value", code(runGo(t, m, d, env, "llm_config", "ok.toml", "--path")).Error)
}

func TestINIInterpolate(t *testing.T) {
	vals := map[string]string{"a": "1", "b": "%(a)s%(a)s", "loop": "%(loop)s", "pct": "50%%"}
	lookup := func(k string) (string, bool) { v, ok := vals[k]; return v, ok }
	for raw, want := range map[string]string{
		"plain": "plain", "%%": "%", "x%(a)sy": "x1y", "%(B)s": "11", "%(pct)s": "50%",
	} {
		got, ok := iniInterpolate(raw, lookup, 1)
		require.True(t, ok, raw)
		require.Equal(t, want, got, raw)
	}
	for _, raw := range []string{"100%", "%(nope)s", "%(a)", "%()s", "%(loop)s", "%x"} {
		_, ok := iniInterpolate(raw, lookup, 1)
		require.False(t, ok, raw)
	}
}
