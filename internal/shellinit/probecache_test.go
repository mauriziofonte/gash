// Gash 3.1. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package shellinit

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// The probes' answers are reused, and the layer is the same, while the key
// and the PATH directories stay; a newer PATH directory, another TERM or a
// day's age runs the probes again.
func TestProbeCache(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell stubs")
	}
	home, bin := t.TempDir(), t.TempDir()
	log := filepath.Join(t.TempDir(), "log")
	stub := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(bin, name),
			[]byte("#!/bin/sh\necho "+name+" >>"+log+"\n"+body+"\n"), 0o755))
	}
	stub("dircolors", `echo "LS_COLORS='di=01;34:';"; echo "export LS_COLORS"`)
	stub("grep", "exit 0")
	stub("ls", "exit 0")
	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(bin, old, old))

	now := time.Now().Unix()
	emit := func(termName string) string {
		t.Helper()
		e := New()
		e.now = func() int64 { return now }
		env := invoke.EnvMap(map[string]string{"HOME": home, "PATH": bin, "TERM": termName, "LANG": "C.UTF-8"})
		c := &invoke.Call{Name: "init", Env: env, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Term: term.Fixed(env, false, false, false)}
		out, err := e.Emit(c)
		require.NoError(t, err)
		return out
	}
	runs := func() int {
		b, _ := os.ReadFile(log)
		return strings.Count(string(b), "\n")
	}

	first := emit("xterm")
	require.Equal(t, 3, runs())
	require.Contains(t, first, "LS_COLORS='di=01;34:'")
	fi, err := os.Stat(filepath.Join(home, ".cache/gash/init-probes"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), fi.Mode().Perm())

	require.Equal(t, first, emit("xterm"))
	require.Equal(t, 3, runs(), "answered by the cache")

	emit("vt100")
	require.Equal(t, 6, runs(), "another TERM")

	now += probeCacheMaxAge
	emit("vt100")
	require.Equal(t, 9, runs(), "a day old")

	newer := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(bin, newer, newer))
	emit("vt100")
	require.Equal(t, 12, runs(), "a PATH directory changed")
}
