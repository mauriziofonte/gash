// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package logs

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// gashLogs runs `gash logs ARGS...` for a caller whose HOME is home.
func gashLogs(home string, args ...string) (string, string, int) {
	e := invoke.EnvMap(map[string]string{"HOME": home, "PATH": os.Getenv("PATH")})
	var out, errb bytes.Buffer
	c := &invoke.Call{Ctx: context.Background(), Name: "gash", Args: args, Env: e,
		Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, false, false)}
	rc := Handler(Options{})(c)
	return out.String(), errb.String(), rc
}

// TestProtectedPaths: with PROTECT: lines in ~/.gash_env, gash logs never
// lists a log in a PROTECT:all path, refuses one as a PATH (forbidden_path)
// and drops it from the default roots; a log in a PROTECT:content path is
// listed as protected and never deleted or truncated (3.4.1).
func TestProtectedPaths(t *testing.T) {
	d, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	now := time.Now()
	for _, f := range []string{"vault/app.log", "vault/app.log.1", "vault/app.log.2.gz", "ledger/old.log.1", "ledger/live.log", "pub/web.log", "pub/web.log.1"} {
		writeFile(t, filepath.Join(d, f), 1000, 0, now)
	}
	require.NoError(t, os.Symlink(filepath.Join(d, "vault"), filepath.Join(d, "pub", "vlink")))
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gash_env"),
		[]byte("PROTECT:all="+d+"/vault\nPROTECT:content="+d+"/ledger\n"), 0o600))

	out, errb, rc := gashLogs(home, "scan", "--json", d)
	require.Equal(t, 0, rc, errb)
	require.NotContains(t, out, "vault")
	require.Contains(t, out, `"path":"`+d+`/ledger/old.log.1","size":1000,`)
	require.Contains(t, out, `"reclaim":0,"protected":true`)
	require.Contains(t, out, `"total_files":4,"total_size":4000,"total_reclaim":2000,`)
	require.Contains(t, out, `"protected":2}`)

	for _, p := range []string{d + "/vault", d + "/vault/app.log", d + "/pub/vlink", d + "/pub/../vault"} {
		for _, args := range [][]string{{"scan", p}, {"clean", p, "--truncate", "--yes"}} {
			out, errb, rc = gashLogs(home, args...)
			require.Equal(t, 1, rc, "%q", args)
			require.Empty(t, out)
			require.Contains(t, errb, `"error":"forbidden_path"`, "%q", args)
		}
	}

	out, errb, rc = gashLogs(home, "clean", d, "--truncate", "--dry-run", "--json")
	require.Equal(t, 0, rc, errb)
	require.Contains(t, out, `"deleted":1,"truncated":1,`, "pub/ only")

	_, errb, rc = gashLogs(home, "clean", d, "--truncate", "--yes")
	require.Equal(t, 0, rc, errb)
	for f, size := range map[string]int64{"vault/app.log": 1000, "vault/app.log.1": 1000, "vault/app.log.2.gz": 1000,
		"ledger/old.log.1": 1000, "ledger/live.log": 1000, "pub/web.log": 0} {
		fi, err := os.Stat(filepath.Join(d, f))
		require.NoError(t, err, f)
		require.Equal(t, size, fi.Size(), f)
	}
	_, err = os.Stat(filepath.Join(d, "pub", "web.log.1"))
	require.True(t, os.IsNotExist(err), "the rotated log outside the trees is deleted")

	// A default root inside a PROTECT:all path is dropped.
	t.Chdir(filepath.Join(d, "vault"))
	out, errb, rc = gashLogs(home, "scan", "--json")
	require.Equal(t, 0, rc, errb)
	require.NotContains(t, out, "vault")
}
