// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLLMRuntimeProject: the version "project" is the one the project in
// the working directory pins (.nvmrc, .target-php-version, ...), resolved
// like any other; a project that pins none is runtime_not_pinned (STOP),
// never the default version.
func TestLLMRuntimeProject(t *testing.T) {
	home := physTemp(t)
	root := fakeNVM(t, home)
	bin := fakePHP(t)
	env := baseEnv(home, map[string]string{"PATH": bin, "FAKE_PHP_VERSION": "8.2.20"})
	m := New()

	d := physTemp(t)
	mustWrite(t, filepath.Join(d, ".nvmrc"), "20\n")
	mustWrite(t, filepath.Join(d, "composer.json"), `{"require":{"php":"^8.2"}}`)
	env["PWD"] = d
	r := runGo(t, m, d, env, "llm_runtime", "node", "project")
	require.Equal(t, result{`{"runtime":"node","request":"20","path":"` + root + `/v20.11.1/bin/node","version":"20.11.1"}` + "\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_runtime", "php", "project")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Contains(t, r.Stdout, `{"runtime":"php","request":"8.2",`)

	// Nothing pinned: a STOP naming the directory and the files that pin.
	empty := physTemp(t)
	env["PWD"] = empty
	for _, fam := range []string{"node", "php"} {
		r = runGo(t, m, empty, env, "llm_runtime", fam, "project")
		require.Equal(t, 1, r.RC)
		require.Empty(t, r.Stdout)
		require.Contains(t, r.Stderr, `"error":"runtime_not_pinned","details":"no `+fam+` version is pinned in `+empty+`","action":"STOP"`)
	}
	r = runGo(t, m, empty, env, "llm_runtime", "npm", "project")
	require.Contains(t, r.Stderr, `"error":"runtime_not_pinned","details":"no node version is pinned in `+empty+`"`)
	require.Contains(t, r.Stderr, ".nvmrc, .node-version or package.json engines.node")

	// llm_lint --node project reads the same pin.
	r = runGo(t, m, empty, env, "llm_lint", "--node", "project", ".")
	require.Contains(t, r.Stderr, `"error":"runtime_not_pinned"`)
}
