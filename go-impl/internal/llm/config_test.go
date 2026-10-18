// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// noYQEnv is a PATH without yq, so 2.x prints YAML files raw as rc.A does
// for every file it cannot convert.
func noYQEnv(t *testing.T, home string) map[string]string {
	t.Helper()
	bin := physTemp(t)
	for _, tool := range []string{"cat", "wc", "jq", "realpath", "readlink", "sed", "tr", "grep"} {
		if p, err := lookPathTest(tool); err == nil {
			require.NoError(t, os.Symlink(p, filepath.Join(bin, tool)))
		}
	}
	return baseEnv(home, map[string]string{"PATH": bin})
}

func lookPathTest(name string) (string, error) {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}

func TestConfigMatches2x(t *testing.T) {
	d := physTemp(t)
	home := physTemp(t)
	env := noYQEnv(t, home)
	mustWrite(t, filepath.Join(d, "c.json"), "{\"a\": 1}\n")
	mustWrite(t, filepath.Join(d, "small.json"), "{}")
	mustWrite(t, filepath.Join(d, "big.txt"), strings.Repeat("x", 3000))
	mustWrite(t, filepath.Join(d, "bad.yml"), "a: [\n")
	mustWrite(t, filepath.Join(d, ".env"), "SECRET=1\n")
	mustWrite(t, filepath.Join(d, "conf.json.pem"), "x")
	mustWrite(t, filepath.Join(d, "unreadable.json"), "{}")
	require.NoError(t, os.Chmod(filepath.Join(d, "unreadable.json"), 0))
	mustMkdir(t, filepath.Join(d, "dir.json"))
	require.NoError(t, os.Symlink(".env", filepath.Join(d, "innocent.json")))
	m := New()
	for _, args := range [][]string{
		{"c.json"},
		{d + "/c.json"},
		{"small.json"},
		{"big.txt"},
		{"big.txt", "--max-bytes", "2999"},
		{"big.txt", "--max-bytes", "3000"},
		{"--max-bytes", "007", "small.json"},
		{"small.json", "--max-bytes", "1"},
		{"small.json", "--max-bytes", "x"},
		{"small.json", "--max-bytes", "1234567890"},
		{"small.json", "--max-bytes"},
		{"bad.yml"},
		{".env"},
		{"conf.json.pem"},
		{"innocent.json"},
		{"dir.json"},
		{"missing.json"},
		{"../c.json"},
		{"/etc/shadow"},
		{"/proc/1/environ"},
		{},
		{"--bogus"},
		{"-"},
		{"c.json", "other.json"},
		{"c.json", "--help"},
		{"--max-bytes", "x", "-h"},
	} {
		same(t, m, d, env, "llm_config", args...)
	}
}

func TestConfigYAML(t *testing.T) {
	m := New()
	d := physTemp(t)
	env := baseEnv(physTemp(t), map[string]string{"PATH": ""})
	for _, c := range []struct{ in, out string }{
		{"a: 1\n", "{\n  \"a\": 1\n}\n"},
		{"a: 1\nb: [x, 2.5, true, null, ~]\nc: {}\nd: []\ne: {f: \"<&>\"}\n",
			"{\n  \"a\": 1,\n  \"b\": [\n    \"x\",\n    2.5,\n    true,\n    null,\n    null\n  ],\n  \"c\": {},\n  \"d\": [],\n  \"e\": {\n    \"f\": \"<&>\"\n  }\n}\n"},
		{"- a\n- 0x10\n- 1e3\n- 1.0\n- 'quoted: \"x\"'\n", "[\n  \"a\",\n  16,\n  1000,\n  1,\n  \"quoted: \\\"x\\\"\"\n]\n"},
		{"x: &a [1]\ny: *a\n", "{\n  \"x\": [\n    1\n  ],\n  \"y\": [\n    1\n  ]\n}\n"},
		{"a: 1\n---\nb: 2\n", "{\n  \"a\": 1\n}\n{\n  \"b\": 2\n}\n"},
		{"hello\n", "\"hello\"\n"},
		{"a: .inf\n", "a: .inf\n"},
		{"a: [\n", "a: [\n"},
		{"a: 1\na: 2\n", "{\n  \"a\": 1,\n  \"a\": 2\n}\n"},
		{"", ""},
	} {
		mustWrite(t, filepath.Join(d, "c.yml"), c.in)
		r := runGo(t, m, d, env, "llm_config", "c.yml")
		require.Equal(t, result{c.out, "", 0}, r, c.in)
	}
	mustWrite(t, filepath.Join(d, "c.yaml"), "k: v\n")
	require.Equal(t, "{\n  \"k\": \"v\"\n}\n", runGo(t, m, d, env, "llm_config", "c.yaml").Stdout)
	mustWrite(t, filepath.Join(d, "c.YML"), "k: v\n")
	require.Equal(t, "k: v\n", runGo(t, m, d, env, "llm_config", "c.YML").Stdout)
}
