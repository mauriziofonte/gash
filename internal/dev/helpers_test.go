// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package dev

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/term"
)

// result is one call's output.
type result struct {
	out, err string
	rc       int
}

// devMod returns a module whose /usr/bin and /usr/local/bin probes read
// the sysroot at root ("" is the real root).
func devMod(t *testing.T, root string) *Module {
	t.Helper()
	sys := platform.Real()
	if root != "" {
		s, err := platform.NewSysroot(root)
		require.NoError(t, err)
		sys = s
	}
	return &Module{sys: sys, open: nil}
}

// call runs NAME on m with env, in the working directory cwd (unchanged
// when ""), the plain terminal.
func call(m *Module, cwd string, env map[string]string, name string, args ...string) result {
	if cwd != "" {
		must(os.Chdir(cwd))
	}
	e := invoke.EnvMap(env)
	var out, errb bytes.Buffer
	c := &invoke.Call{Name: name, Args: args, Env: e, Stdin: strings.NewReader(""),
		Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, false, false)}
	tbl := invoke.NewTable()
	m.Register(tbl)
	rc := tbl.Run(c)
	return result{out.String(), errb.String(), rc}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// baseEnv is the environment of a headless call: an empty PATH (no php,
// composer or node on it), HOME and NVM_DIR under home.
func baseEnv(home string, extra map[string]string) map[string]string {
	env := map[string]string{"PATH": "", "HOME": home, "LANG": "C.UTF-8", "GASH_HEADLESS": "1"}
	for k, v := range extra {
		env[k] = v
	}
	return env
}

// mkfile writes content at path, creating parents.
func mkfile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// mkexe writes an executable at path, creating parents.
func mkexe(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755))
}

// phys is a temp directory by its physical path.
func phys(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return d
}
