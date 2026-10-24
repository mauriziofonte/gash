// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// setupDirty: setupOrigin with a modified, a new and a deleted file.
const setupDirty = setupOrigin + ` && echo n >new && echo m >>a && git rm -q --cached a && git add a && touch gone && git add gone && git commit -qm g && git push -q 2>/dev/null && rm gone && mkdir -p d/e && echo x >d/e/deep`

// 3.0: GIT-01 and GIT-07 fixed: --yes is a flag (the "unknown option"
// case), the cases that commit or push need it, and the push names its
// upstream (TestLedger_GIT01, TestLedger_GIT07); this 2.x comparison
// skips without the 2.x sources.
func TestSyncMatches2x(t *testing.T) {
	cases := []struct {
		name, setup, cwd string
		args             []string
		rc               int
	}{
		{"unknown option", setupDirty, "w", []string{"--yes"}, 1},
		{"-m without a message", setupDirty, "w", []string{"-m"}, 1},
		{"-m with an empty message", setupDirty, "w", []string{"--message", ""}, 1},
		{"outside a repository", "", ".", nil, 1},
		{"detached HEAD", setupOrigin + " && git checkout -q --detach", "w", nil, 1},
		{"merge in progress", setupOrigin + " && git rev-parse HEAD >.git/MERGE_HEAD", "w", nil, 1},
		{"cherry-pick in progress", setupOrigin + " && git rev-parse HEAD >.git/CHERRY_PICK_HEAD", "w", nil, 1},
		{"revert in progress", setupOrigin + " && git rev-parse HEAD >.git/REVERT_HEAD", "w", nil, 1},
		{"rebase in progress", setupOrigin + " && mkdir .git/rebase-merge", "w", nil, 1},
		{"am in progress", setupOrigin + " && mkdir .git/rebase-apply", "w", nil, 1},
		{"no upstream and no origin", setupRepo + " && echo x >x", "w", nil, 1},
		{"no upstream and no origin, --no-push", setupRepo + " && echo x >x", "w", []string{"--no-push"}, 0},
		{"dry run with changes", setupDirty, "w", []string{"--dry-run", "-m", "my message"}, 0},
		{"dry run with nothing", setupOrigin, "w", []string{"--dry-run"}, 0},
		{"dry run, no push", setupDirty, "w", []string{"--dry-run", "--no-push"}, 0},
		{"dry run, new branch", setupOrigin + " && git checkout -q -b topic", "w", []string{"--dry-run"}, 0},
		{"commit and push", setupDirty, "w", nil, 0},
		{"commit and push from a subdirectory", setupDirty, "w/d", []string{"-m", "from d"}, 0},
		{"commit only", setupDirty, "w", []string{"--no-push"}, 0},
		{"nothing to commit, already pushed", setupOrigin, "w", nil, 0},
		{"nothing to commit, commits to push", setupOrigin + " && echo z >z && git add z && git commit -qm z", "w", nil, 0},
		{"new branch sets its upstream", setupOrigin + " && git checkout -q -b topic && echo t >t", "w", nil, 0},
		{"last -m wins", setupDirty, "w", []string{"-m", "one", "-m", "two"}, 0},
		{"push rejected", setupDirty + ` && printf '#!/bin/sh\nexit 1\n' >../origin.git/hooks/pre-receive && chmod +x ../origin.git/hooks/pre-receive`, "w", nil, 1},
		{"help", "", ".", []string{"-h"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newScene(t, tc.setup).same(plain, tc.cwd, "git_sync", tc.args...)
			require.Equal(t, tc.rc, r.RC)
		})
	}
	t.Run("color", func(t *testing.T) {
		newScene(t, setupDirty).same(color, "w", "git_sync", "--dry-run")
		newScene(t, setupDirty).same(color, "w", "git_sync")
	})
}
