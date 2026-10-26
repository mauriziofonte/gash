// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestShTierIsClassifyPath: the sh tier of a resolved path is the Go one,
// macOS /private aliases and per-user temporary trees included.
func TestShTierIsClassifyPath(t *testing.T) {
	paths := []string{
		"/", "/etc", "/etc/nginx/nginx.conf", "/etc/ssh/sshd_config", "/etc/passwd", "/var/www/app/x.php",
		"/var/tmp/x", "/tmp/x", "/usr/local/bin/x", "/usr/bin/env", "/root/x", "/home/u/app/x", "/home/u/.ssh/config",
		"/home/u/.SSH/x", "/srv/app/config.php", "/boot", "/bootx", "/lib64/x", "/opt/x",
		"/private/etc/hosts", "/private/etc/ssh/sshd_config", "/private/var/www/x", "/private/tmp/x", "/private",
		"/privateetc/x", "/var/folders/ab/T/x", "/private/var/folders/ab/T/x", "/var/folders/ab/.ssh/x",
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
		require.Equal(t, classifyResolved(p), got[i], p)
	}
}

// TestClassifyLocalUnaliases: on macOS /etc, /var and /tmp resolve into
// /private, which must not take a write out of its tier.
func TestClassifyLocalUnaliases(t *testing.T) {
	for p, want := range map[string]string{
		"/private/etc/hosts":             "ask:/etc",
		"/private/etc/ssh/sshd_config":   "deny:/etc/ssh",
		"/private/var/lib/x":             "ask:/var",
		"/private/tmp/x":                 "allow",
		"/private/var/folders/ab/T/x":    "allow",
		"/private/var/folders/ab/.ssh/x": "ask:.ssh",
		"/privateetc/x":                  "allow",
	} {
		require.Equal(t, want, ClassifyLocal(p), p)
	}
	require.Equal(t, "/etc/hosts", UnaliasResolved("/private/etc/hosts"))
	require.Equal(t, "/private", UnaliasResolved("/private"))
}

func TestShResolveAndFrames(t *testing.T) {
	// A physical directory: on macOS the temporary root and /home are
	// symlinks, which gash_resolve follows.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	out, err := exec.Command("sh", "-c", ShResolve()+`cd "$1" && HOME=$1/u gash_resolve '~/a/./b'; gash_resolve "$1/x/../y"`, "sh", dir).Output()
	require.NoError(t, err)
	want := dir + "/u/a/b\n"
	require.Equal(t, want, string(out[:min(len(want), len(out))]))
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
