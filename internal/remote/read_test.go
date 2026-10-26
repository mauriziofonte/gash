// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/remote/sshtest"
)

func TestReadFilesOneInvocation(t *testing.T) {
	rhome, err := filepath.EvalSymlinks(t.TempDir()) // the host reports resolved paths
	require.NoError(t, err)
	files := map[string]string{
		"a.txt":     "alpha\n",
		"nonl.txt":  "no newline",
		"bin.dat":   "x\x00y\n\n",
		"empty":     "",
		"big.txt":   "0123456789A",
		"conf/.env": "SECRET=1\n",
		"end.txt":   "looks like\n gash end 1\nbut is not\n",
	}
	for p, c := range files {
		require.NoError(t, os.MkdirAll(filepath.Join(rhome, filepath.Dir(p)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(rhome, p), []byte(c), 0o644))
	}
	require.NoError(t, os.Symlink(forbiddenLinkTarget(), filepath.Join(rhome, "env-link")))
	require.NoError(t, os.Symlink("conf/.env", filepath.Join(rhome, "secret-link")))
	ssh := &sshtest.SSH{Home: rhome}
	tr := Transport{Run: ssh, Env: invoke.EnvMap(map[string]string{"HOME": t.TempDir()})}
	h := Host{Alias: "web", Target: "deploy@web.example"}
	paths := []string{"a.txt", "nonl.txt", "bin.dat", "empty", "big.txt", "conf/.env", "missing", "env-link", "secret-link", "end.txt", "~/a.txt"}
	got, e := tr.ReadFiles(context.Background(), h, paths, 10, 0)
	require.Nil(t, e)
	require.Equal(t, 1, ssh.N(), "N files, one invocation")
	require.Len(t, got, len(paths))
	want := map[string]string{"a.txt": "alpha\n", "nonl.txt": "no newline", "bin.dat": "x\x00y\n\n", "empty": "", "~/a.txt": "alpha\n"}
	errs := map[string]string{"big.txt": "file_too_large", "conf/.env": "secret_file_blocked", "missing": "file_not_found",
		"env-link": "forbidden_path", "secret-link": "secret_file_blocked", "end.txt": "file_too_large"}
	for _, f := range got {
		if c, ok := want[f.Path]; ok {
			require.Nil(t, f.Err, f.Path)
			require.Equal(t, c, string(f.Data), f.Path)
			continue
		}
		require.NotNil(t, f.Err, f.Path)
		require.Equal(t, errs[f.Path], f.Err.Code, f.Path)
		require.Nil(t, f.Data, f.Path)
	}
	require.Equal(t, filepath.Join(rhome, "a.txt"), got[0].Abs)

	got, e = tr.ReadFiles(context.Background(), h, []string{"end.txt"}, 1<<20, 0)
	require.Nil(t, e)
	require.Equal(t, files["end.txt"], string(got[0].Data))

	_, e = tr.ReadFiles(context.Background(), h, []string{"a.txt", "../x"}, 1<<20, 0)
	require.Equal(t, "path_traversal_blocked", e.Code)
	ssh.Unreachable = true
	_, e = tr.ReadFiles(context.Background(), h, []string{"a.txt"}, 1<<20, 0)
	require.Equal(t, "host_unreachable", e.Code)
	require.Equal(t, 3, ssh.N(), "a refused path never connects")
}

func TestParseReadIncomplete(t *testing.T) {
	got := ParseRead("N", []string{"a", "b"}, 10, []byte("N file 1 ok /a\nhello\nN end 1\nN file 2 ok /b\npartial"))
	require.Equal(t, "hello", string(got[0].Data))
	require.Equal(t, "remote_protocol_error", got[1].Err.Code)
}

// forbiddenLinkTarget is a protected path that exists here: /proc/self/environ
// on Linux, /dev/null where there is no /proc (a link to a missing path
// resolves to the link itself, which is no protected path).
func forbiddenLinkTarget() string {
	if _, err := os.Stat("/proc/self/environ"); err == nil {
		return "/proc/self/environ"
	}
	return "/dev/null"
}
