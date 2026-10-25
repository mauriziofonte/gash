// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestShTierIsClassifyPath: the sh tier of a resolved path is the Go one.
func TestShTierIsClassifyPath(t *testing.T) {
	paths := []string{
		"/", "/etc", "/etc/nginx/nginx.conf", "/etc/ssh/sshd_config", "/etc/passwd", "/var/www/app/x.php",
		"/var/tmp/x", "/tmp/x", "/usr/local/bin/x", "/usr/bin/env", "/root/x", "/home/u/app/x", "/home/u/.ssh/config",
		"/home/u/.SSH/x", "/srv/app/config.php", "/boot", "/bootx", "/lib64/x", "/opt/x",
	}
	var script strings.Builder
	script.WriteString(ShTier())
	for _, p := range paths {
		script.WriteString("gash_tier " + Quote(p) + "\n")
	}
	out, err := exec.Command("sh", "-c", script.String()).Output()
	require.NoError(t, err)
	got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	require.Len(t, got, len(paths))
	for i, p := range paths {
		require.Equal(t, classifyPath(p), got[i], p)
	}
}

func TestShResolveAndFrames(t *testing.T) {
	dir := t.TempDir()
	out, err := exec.Command("sh", "-c", ShResolve()+`cd "$1" && HOME=/home/u gash_resolve '~/a/./b'; gash_resolve "$1/x/../y"`, "sh", dir).Output()
	require.NoError(t, err)
	require.Equal(t, "/home/u/a/b\n", string(out[:len("/home/u/a/b\n")]))
	require.True(t, strings.HasSuffix(string(out), "/y\n"), string(out))

	errOut := []byte("noise\nN err write_failed key some details here\nN done 1\n")
	code, key, details, ok := ErrFrame(errOut, "N")
	require.True(t, ok)
	require.Equal(t, []string{"write_failed", "key", "some details here"}, []string{code, key, details})
	rest, ok := Frame(errOut, "N", "done")
	require.True(t, ok)
	require.Equal(t, "1", rest)
	_, _, _, ok = ErrFrame([]byte("x\n"), "N")
	require.False(t, ok)
}
