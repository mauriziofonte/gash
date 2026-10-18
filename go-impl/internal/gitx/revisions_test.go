// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const setupHistory = setupRepo + ` &&
mkdir -p sub && printf 'one\n' >sub/f.txt && git add sub && git commit -qm "add f" &&
printf 'two\n' >sub/f.txt && git commit -qam "change f" -m "with a body" &&
printf 'three\n' >sub/f.txt && GIT_AUTHOR_NAME=Other git commit -qam "change f again"`

func TestDumpRevisionsMatches2x(t *testing.T) {
	cases := []struct {
		name, setup, cwd string
		args             []string
		rc               int
	}{
		{"missing argument", setupHistory, "w", nil, 1},
		{"outside a repository", "touch f", ".", []string{"f"}, 1},
		{"missing file", setupHistory, "w", []string{"nope.txt"}, 1},
		{"a directory is not a file", setupHistory, "w", []string{"sub"}, 1},
		{"dumped", setupHistory, "w", []string{"sub/f.txt"}, 0},
		{"dumped from a subdirectory", setupHistory, "w/sub", []string{"f.txt"}, 0},
		{"untracked file", setupHistory + " && touch new", "w", []string{"new"}, 0},
		{"help", "", ".", []string{"-h"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newScene(t, tc.setup).same(plain, tc.cwd, "git_dump_revisions", tc.args...)
			require.Equal(t, tc.rc, r.RC)
		})
	}
	t.Run("color", func(t *testing.T) {
		newScene(t, setupHistory).same(color, "w", "git_dump_revisions", "sub/f.txt")
	})
}

// A redirection that fails skips its command and the loop goes on; the
// Bash diagnostic, which names a 2.x script line, is not printed (LEDGER
// DOCK-19). Here a directory sits where the first version would go.
func TestDumpRevisionsUnwritable(t *testing.T) {
	s := newScene(t, setupHistory)
	want := s.run2x(plain, "w", "git_dump_revisions", "sub/f.txt")
	hash := func() string {
		out, err := os.ReadDir(filepath.Join(s.dir, "w/sub"))
		require.NoError(t, err)
		for _, e := range out {
			if n := e.Name(); len(n) > 10 && n[:10] == "f.txt.001." && filepath.Ext(n) != ".logmsg" {
				return n
			}
		}
		return ""
	}()
	require.NotEmpty(t, hash)
	s.setup += " && mkdir sub/" + hash
	want = s.run2x(plain, "w", "git_dump_revisions", "sub/f.txt")
	got := s.runGo(plain, "w", "git_dump_revisions", nil, "sub/f.txt")
	require.Equal(t, want.RC, got.RC)
	require.Equal(t, want.Out, got.Out)
	require.Equal(t, want.State, got.State)
	require.Contains(t, want.Err, "Is a directory")
	require.NotContains(t, got.Err, "Is a directory")
	require.Equal(t, 0, got.RC)
}
