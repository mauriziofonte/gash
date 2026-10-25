// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeNVM builds an nvm tree under home: node 20.11.1 and 24.1.0, npm
// beside 24 only. Each node prints its version.
func fakeNVM(t *testing.T, home string) string {
	t.Helper()
	root := filepath.Join(home, ".nvm", "versions", "node")
	for _, v := range []string{"20.11.1", "24.1.0"} {
		bin := filepath.Join(root, "v"+v, "bin")
		mustMkdir(t, bin)
		require.NoError(t, os.WriteFile(filepath.Join(bin, "node"), []byte("#!/bin/sh\necho v"+v+"\n"), 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "v24.1.0", "bin", "npm"), []byte("#!/bin/sh\necho 10.0.0\n"), 0o755))
	return root
}

// TestLLMRuntime: node majors resolve from the nvm tree without nvm.sh,
// npm beside its node; a version that is not installed is a STOP with
// the installed list; nothing is written to the cache.
func TestLLMRuntime(t *testing.T) {
	home := physTemp(t)
	root := fakeNVM(t, home)
	cache := filepath.Join(home, "cache")
	env := baseEnv(home, map[string]string{"PATH": physTemp(t), "XDG_CACHE_HOME": cache})
	m := New()
	d := physTemp(t)

	r := runGo(t, m, d, env, "llm_runtime", "node", "24")
	require.Equal(t, result{`{"runtime":"node","request":"24","path":"` + root + `/v24.1.0/bin/node","version":"24.1.0"}` + "\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_runtime", "node", "v20.11")
	require.Equal(t, result{`{"runtime":"node","request":"20.11","path":"` + root + `/v20.11.1/bin/node","version":"20.11.1"}` + "\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_runtime", "npm", "24")
	require.Equal(t, result{`{"runtime":"npm","request":"24","path":"` + root + `/v24.1.0/bin/npm","version":"24.1.0"}` + "\n", "", 0}, r)

	r = runGo(t, m, d, env, "llm_runtime", "node", "18")
	require.Equal(t, 1, r.RC)
	require.Empty(t, r.Stdout)
	require.Contains(t, r.Stderr, `"error":"runtime_not_installed","details":"node 18 is not installed; installed: 20.11.1, 24.1.0","action":"STOP"`)
	r = runGo(t, m, d, env, "llm_runtime", "npm", "20")
	require.Contains(t, r.Stderr, `"error":"runtime_not_installed"`)

	// NVM_DIR moves the tree.
	moved := physTemp(t)
	require.NoError(t, os.Rename(filepath.Join(home, ".nvm"), filepath.Join(moved, "nvm")))
	env["NVM_DIR"] = filepath.Join(moved, "nvm")
	r = runGo(t, m, d, env, "llm_runtime", "node", "24")
	require.Contains(t, r.Stdout, `"path":"`+moved+`/nvm/versions/node/v24.1.0/bin/node"`)

	_, err := os.Stat(cache)
	require.True(t, os.IsNotExist(err), "llm_runtime wrote the runtime cache")

	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"ruby", "3"}, "unknown_runtime"},
		{[]string{"node", "24.x"}, "invalid_version"},
		{[]string{"node"}, "invalid_version"},
		{[]string{}, "unknown_runtime"},
		{[]string{"node", "24", "x"}, "unexpected_argument"},
		{[]string{"node", "--path"}, "unknown_option"},
	} {
		r := runGo(t, m, d, env, "llm_runtime", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
	}
}

// TestLLMTools: path and version per program in the order asked, null
// for a missing one, "version":null for one that prints none; node lists
// every installed version.
func TestLLMTools(t *testing.T) {
	home := physTemp(t)
	root := fakeNVM(t, home)
	bin := physTemp(t)
	for name, script := range map[string]string{
		"foo":  "#!/bin/sh\n[ \"$1\" = --version ] && echo 'foo version 1.2.3 (build 7)'\n",
		"bar":  "#!/bin/sh\necho no version here\n",
		"go":   "#!/bin/sh\n[ \"$1\" = version ] && echo 'go version go1.27.1 linux/amd64'\n",
		"java": "#!/bin/sh\n[ \"$1\" = -version ] && echo 'openjdk version \"21.0.2\" 2024-01-16' >&2\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755))
	}
	env := baseEnv(home, map[string]string{"PATH": bin})
	m := New()
	d := physTemp(t)
	r := runGo(t, m, d, env, "llm_tools", "foo", "bar", "nope", "go", "java", "foo", "node", "php9")
	require.Equal(t, result{`{"foo":{"version":"1.2.3","path":"` + bin + `/foo"},"bar":{"version":null,"path":"` + bin + `/bar"},` +
		`"nope":null,"go":{"version":"1.27.1","path":"` + bin + `/go"},"java":{"version":"21.0.2","path":"` + bin + `/java"},` +
		`"node":[{"version":"20.11.1","path":"` + root + `/v20.11.1/bin/node"},{"version":"24.1.0","path":"` + root + `/v24.1.0/bin/node"}],` +
		`"php9":null}` + "\n", "", 0}, r)

	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"/usr/bin/git"}, "invalid_filter"},
		{[]string{"--all"}, "unknown_option"},
		{[]string{"-j", "0"}, "invalid_jobs"},
	} {
		r := runGo(t, m, d, env, "llm_tools", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
	}
}
