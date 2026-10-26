// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package pathguard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// protectTree is DIR with private/ (PROTECT:all), contracts/
// (PROTECT:content), public/, and a link into each protected tree.
func protectTree(t *testing.T) (dir string, g *Guard) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	for _, d := range []string{"private", "contracts", "public"} {
		require.NoError(t, os.Mkdir(filepath.Join(dir, d), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, d, "a.txt"), []byte("x\n"), 0o644))
	}
	require.NoError(t, os.Symlink(filepath.Join(dir, "private"), filepath.Join(dir, "plink")))
	require.NoError(t, os.Symlink(filepath.Join(dir, "contracts", "a.txt"), filepath.Join(dir, "public", "c.txt")))
	g = guardAt(dir, dir).WithRules([]Rule{{Mode: "all", Path: "~/private"}, {Mode: "content", Path: dir + "/contracts/"}})
	return dir, g
}

// TestProtectAll: a PROTECT:all tree is refused like /root, by name and
// through a link, and a walk leaves it out; a walk may start above it.
func TestProtectAll(t *testing.T) {
	dir, g := protectTree(t)
	for _, p := range []string{"private", "private/a.txt", "./private/x", dir + "/private", "plink", "plink/a.txt", "~/private/a.txt"} {
		_, err := g.ValidatePath(p)
		r, ok := err.(*Refusal)
		require.Truef(t, ok, "%q: expected refusal, got %v", p, err)
		require.Equal(t, "forbidden_path", r.Code, p)
		require.True(t, g.Hides(p), p)
	}
	for _, p := range []string{"public/a.txt", "contracts/a.txt", "privateer", "."} {
		_, err := g.ValidatePath(p)
		require.NoError(t, err, p)
		require.False(t, g.Hides(p), p)
	}
	require.True(t, g.UnderForbidden(dir+"/private/a.txt"))
	require.False(t, g.UnderForbidden(dir+"/public/a.txt"))
	require.False(t, g.PathAboveForbidden(dir, "names"), "a walk from above starts, and leaves the tree out")
	require.True(t, g.AboveProtected(dir, "names"))
	require.True(t, g.AboveProtected(dir, "content"))
	require.False(t, g.AboveProtected(dir+"/public", "content"))
}

// TestProtectContent: the files of a PROTECT:content tree are secret
// files, by name and through a link; a names walk keeps them, a content
// walk leaves them out.
func TestProtectContent(t *testing.T) {
	dir, g := protectTree(t)
	for _, p := range []string{"contracts/a.txt", dir + "/contracts", "public/c.txt"} {
		_, err := g.ValidatePath(p)
		require.NoError(t, err, p)
		require.True(t, g.IsSecretFile(p), p)
	}
	require.False(t, g.IsSecretFile("public/a.txt"))
	require.False(t, g.WalkSkip("names")(dir+"/contracts/a.txt"))
	require.True(t, g.WalkSkip("content")(dir+"/contracts/a.txt"))
	require.True(t, g.WalkSkip("names")(dir+"/private/a.txt"))
	require.False(t, g.WalkSkip("content")(dir+"/public/a.txt"))

	only := guardAt(dir, dir).WithRules([]Rule{{Mode: "content", Path: dir + "/contracts"}})
	require.False(t, only.AboveProtected(dir, "names"), "a listing may walk a content tree")
	require.True(t, only.AboveProtected(dir, "content"))
	require.ElementsMatch(t, []string{dir + "/contracts", dir + "/contracts"}, only.Sealed)
	require.Len(t, only.Trees(), len(only.Forbidden)+2)
}

// TestRulesEncoding: the variable carries rules one per line, and a line
// it cannot read is skipped, never turned into a looser rule.
func TestRulesEncoding(t *testing.T) {
	rules := []Rule{{Mode: "all", Path: "~/private"}, {Mode: "content", Path: "/srv/a b/c"}}
	require.Equal(t, rules, DecodeRules(EncodeRules(rules)))
	require.Equal(t, []Rule{{Mode: "all", Path: "/x"}}, DecodeRules("all /x\nnone /y\ncontent rel\nall\n\ncontent ~user/x\n"))
}

// TestRules: the PROTECT: lines of the caller's file that apply here (no
// host, or not this one), plus the variable's; a file change is seen at
// once.
func TestRules(t *testing.T) {
	home := t.TempDir()
	env := filepath.Join(home, "env")
	require.NoError(t, os.WriteFile(env, []byte("PROTECT:all=~/a\nPROTECT:content@web=/srv\nPROTECT:content=/b\n"), 0o600))
	vars := map[string]string{"HOME": home, "GASH_ENV_FILE": env}
	get := func(k string) string { return vars[k] }
	require.Equal(t, []Rule{{Mode: "all", Path: "~/a"}, {Mode: "content", Path: "/b"}}, Rules(get))
	vars[ProtectVar] = "all /c\n"
	require.Equal(t, []Rule{{Mode: "all", Path: "~/a"}, {Mode: "content", Path: "/b"}, {Mode: "all", Path: "/c"}}, Rules(get))
	require.NoError(t, os.WriteFile(env, []byte("PROTECT:all=/d\n# longer than it was\n"), 0o600))
	delete(vars, ProtectVar)
	require.Equal(t, []Rule{{Mode: "all", Path: "/d"}}, Rules(get))
}
