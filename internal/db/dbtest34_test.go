// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// gash_db_test tests only what it can reach from this machine: a +ssh or
// +docker connection gets the query to run instead, before any line that
// announces a test.
func TestDBTestRefusesRemoteDrivers(t *testing.T) {
	home := t.TempDir()
	mustWrite(t, filepath.Join(home, ".gash_env"),
		"HOST:web=deploy@web.example\nDB:live=mysql+ssh://app:pw@web/shop\nDB:files=sqlite+ssh://web/data/app.db\n", 0o600)
	for _, name := range []string{"live", "files"} {
		r := runGo(t, New(), home, baseEnv(home, map[string]string{"GASH_HEADLESS": ""}), "gash_db_test", name)
		require.Equal(t, 1, r.RC, name)
		require.Empty(t, r.Stdout, name)
		require.NotContains(t, r.Stderr, "Testing connection", name)
		require.Contains(t, r.Stderr, "gash_db_test tests mysql, mariadb and pgsql connections from this machine; '"+name+"' is a", name)
		require.Contains(t, r.Stderr, `llm_db_query "SELECT 1" -c `+name+` --scalar`, name)
	}
}
