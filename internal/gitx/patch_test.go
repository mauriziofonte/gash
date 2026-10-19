// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// setupPatch: main and its origin at "base" (tag base), a feature branch
// adding f and changing a, checked out on main.
const setupPatch = setupOrigin + ` && git tag base &&
git checkout -q -b feature && echo f >f && git add f && echo c >>a && git commit -qam "feature work" &&
printf 'bin\000ary' >bin.dat && git add bin.dat && git commit -qm "binary" &&
git checkout -q main`

func TestApplyPatchMatches2x(t *testing.T) {
	cases := []struct {
		name, setup, cwd string
		args             []string
		rc               int
	}{
		{"outside a repository", "", ".", []string{"main", "feature", "base"}, 1},
		{"no arguments", setupPatch, "w", nil, 1},
		{"two arguments", setupPatch, "w", []string{"main", "feature"}, 1},
		{"empty commit", setupPatch, "w", []string{"main", "feature", ""}, 1},
		{"main missing", setupPatch, "w", []string{"trunk", "feature", "base"}, 1},
		{"feature missing", setupPatch, "w", []string{"main", "feat", "base"}, 1},
		{"bad commit", setupPatch, "w", []string{"main", "feature", "deadbeef"}, 1},
		{"dirty tree", setupPatch + " && echo x >>a", "w", []string{"main", "feature", "base"}, 1},
		{"untracked file is dirty", setupPatch + " && touch u", "w", []string{"main", "feature", "base"}, 1},
		{"applied", setupPatch, "w", []string{"main", "feature", "base"}, 0},
		{"applied from another branch", setupPatch + " && git checkout -q -b other", "w", []string{"main", "feature", "base"}, 0},
		{"pull fails", setupPatch + " && git remote set-url origin ../none.git", "w", []string{"main", "feature", "base"}, 1},
		{"help", "", ".", []string{"-h"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newScene(t, tc.setup).same(plain, tc.cwd, "git_apply_patch", tc.args...)
			require.Equal(t, tc.rc, r.RC)
		})
	}
	t.Run("color", func(t *testing.T) {
		newScene(t, setupPatch).same(color, "w", "git_apply_patch", "main", "feature", "base")
	})
	t.Run("apply fails and rolls back", func(t *testing.T) {
		// main changes a's line after base: the 3-way apply conflicts.
		setup := setupPatch + ` && printf 'z\nz\n' >a && git commit -qam "main conflict" && git push -q origin main 2>/dev/null`
		r := newScene(t, setup).same(plain, "w", "git_apply_patch", "main", "feature", "base")
		require.Equal(t, 1, r.RC)
		require.Contains(t, r.Err, "Patch file kept for inspection: feature.patch")
		require.Contains(t, r.State, "./w/feature.patch")
	})
}
