// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var headless = mode{env: []string{"GASH_HEADLESS=1"}}

// LEDGER GIT-10 (kept): the git module functions run the user's hooks
// (testdata/script/git10_hooks.txtar is the shim-level proof). 3.0:
// GIT-01 fixed: headless, git_sync needs --yes.
func TestLedger_GIT10(t *testing.T) {
	setup := setupRepo + ` && printf '#!/bin/sh\n: >"$HOME/../hook-ran"\n' >.git/hooks/pre-commit &&
chmod +x .git/hooks/pre-commit && echo x >x`
	s := newGoScene(t, setup)
	r := s.run(headless, "w", nil, "git_sync", "--no-push", "--yes")
	require.Equal(t, 0, r.RC, r.Err)
	_, err := os.Stat(filepath.Join(s.base, "hook-ran"))
	require.NoError(t, err)
}
