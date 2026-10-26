// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// lintTree writes a small project: good and broken files of every
// language, a broken file under node_modules (pruned) and a secret file
// (skipped).
func lintTree(t *testing.T) string {
	t.Helper()
	d := physTemp(t)
	for name, content := range map[string]string{
		"src/ok.php":            "<?php\necho 1;\n",
		"src/bad.php":           "<?php\necho 1\necho 2;\n",
		"ok.sh":                 "echo ok\n",
		"bad.sh":                "if true; then\n",
		"ok.js":                 "const a = 1;\n",
		"bad.js":                "const a = ;\n",
		"esm.mjs":               "export const x = 1;\n",
		"ok.json":               "{\"a\":1}\n",
		"bad.json":              "{\n\"a\":1,\n}\n",
		"tsconfig.json":         "{\n  // comment\n  \"a\": 1,\n}\n",
		"node_modules/x/b.json": "{\"a\":",
		"bad.yaml":              "a: 1\nb: [\n",
		"ok.yml":                "a: 1\n---\nb: 2\n",
		"bad.toml":              "a = 1\nb = \n",
		"ok.py":                 "x = 1\n",
		"bad.py":                "def f(:\n  pass\n",
		"credentials.json":      "{\"pw\":1",
		"README":                "text\n",
	} {
		mustWrite(t, filepath.Join(d, name), content)
	}
	return d
}

type lintOut struct {
	Checked  int `json:"checked"`
	Failed   int `json:"failed"`
	Failures []struct {
		File  string `json:"file"`
		Line  int    `json:"line"`
		Error string `json:"error"`
	} `json:"failures"`
	Truncated int `json:"truncated"`
}

func parseLint(t *testing.T, r result) lintOut {
	t.Helper()
	require.Equal(t, 0, r.RC, r.Stderr)
	var o lintOut
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &o), r.Stdout)
	return o
}

// TestLLMLintDataAndShell: JSON, YAML, TOML (parsed by gash) and shell
// (bash -n) give exact lines and messages; node_modules is pruned, the
// secret file and other extensions are skipped, tsconfig.json may hold
// comments.
func TestLLMLintDataAndShell(t *testing.T) {
	d := lintTree(t)
	m := New()
	env := baseEnv(physTemp(t), nil)
	r := runGo(t, m, d, env, "llm_lint", ".", "--ext", "json,yaml,toml,sh")
	require.Equal(t, `{"checked":8,"failed":4,"failures":[`+
		`{"file":"bad.json","line":3,"error":"invalid character '}' looking for beginning of object key string"},`+
		`{"file":"bad.sh","line":2,"error":"syntax error: unexpected end of file"},`+
		`{"file":"bad.toml","line":2,"error":"expected value but found '\\n' instead"},`+
		`{"file":"bad.yaml","line":2,"error":"did not find expected node content"}]}`+"\n", r.Stdout)
	require.Equal(t, 0, r.RC)

	// One file, by name: shown as given; --absolute shows the full path.
	r = runGo(t, m, d, env, "llm_lint", "./bad.sh")
	require.Equal(t, `{"checked":1,"failed":1,"failures":[{"file":"bad.sh","line":2,"error":"syntax error: unexpected end of file"}]}`+"\n", r.Stdout)
	o := parseLint(t, runGo(t, m, d, env, "llm_lint", "bad.json", "--absolute"))
	require.Equal(t, filepath.Join(d, "bad.json"), o.Failures[0].File)

	// A budget keeps the first failure and counts the rest.
	o = parseLint(t, runGo(t, m, d, env, "llm_lint", "--ext", "json,yaml,toml,sh", "--max-lines", "1"))
	require.Len(t, o.Failures, 1)
	require.Equal(t, 3, o.Truncated)

	// Nothing to check is an empty result, not an error.
	r = runGo(t, m, filepath.Join(d, "src"), env, "llm_lint", "--ext", "py")
	require.Equal(t, result{`{"checked":0,"failed":0,"failures":[]}` + "\n", "", 0}, r)
}

// TestLLMLintInterpreters: php, node and python, where installed, report
// the broken file of each with its line.
func TestLLMLintInterpreters(t *testing.T) {
	d := lintTree(t)
	m := New()
	env := baseEnv(physTemp(t), nil)
	for _, tc := range []struct{ tool, ext, file string }{
		{"php", "php", "src/bad.php"}, {"node", "js", "bad.js"}, {"python3", "py", "bad.py"},
	} {
		t.Run(tc.ext, func(t *testing.T) {
			if _, err := exec.LookPath(tc.tool); err != nil {
				t.Skip(tc.tool + " not installed")
			}
			o := parseLint(t, runGo(t, m, d, env, "llm_lint", "--ext", tc.ext))
			require.Equal(t, 1, o.Failed, "%+v", o)
			require.Equal(t, tc.file, o.Failures[0].File)
			require.Greater(t, o.Failures[0].Line, 0)
			require.NotEmpty(t, o.Failures[0].Error)
			if tc.ext == "js" {
				require.Equal(t, 3, o.Checked) // ok.js, bad.js, esm.mjs
			}
		})
	}
	_, err := os.Stat(filepath.Join(d, "__pycache__"))
	require.True(t, os.IsNotExist(err), "llm_lint wrote .pyc files")
}

// TestLLMLintErrors: the envelopes of bad arguments and missing tools.
func TestLLMLintErrors(t *testing.T) {
	d := lintTree(t)
	m := New()
	home := physTemp(t)
	env := baseEnv(home, nil)
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"credentials.json"}, "secret_file_blocked"},
		{[]string{"README"}, "unsupported_file"},
		{[]string{"nope"}, "path_not_found"},
		{[]string{"--ext", "rb"}, "invalid_ext"},
		{[]string{"--php", "eight"}, "invalid_version"},
		{[]string{"--node"}, "missing_value"},
		{[]string{"--fix"}, "unknown_option"},
	} {
		r := runGo(t, m, d, env, "llm_lint", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
		require.Empty(t, r.Stdout, tc.args)
	}

	// A requested runtime that is not installed is a STOP naming the
	// installed ones; with no node anywhere, .js files are a STOP too.
	r := runGo(t, m, d, env, "llm_lint", "--ext", "js", "--node", "3")
	require.Contains(t, r.Stderr, `"error":"runtime_not_installed","details":"node 3 is not installed`)
	require.Contains(t, r.Stderr, `"action":"STOP"`)
	noNode := baseEnv(home, map[string]string{"PATH": physTemp(t)})
	r = runGo(t, m, d, noNode, "llm_lint", "bad.js")
	require.Contains(t, r.Stderr, `"error":"runtime_not_installed","details":"no node found for 1 .js files"`)
	require.Equal(t, 1, r.RC)
}
