// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAgentTestRuntime: --runtime NAME:VERSION puts that runtime's
// directory first on the suite's PATH, as gash run --runtime does;
// project is the version the project pins, and a project that pins none
// stops before anything runs. --dry-run lists the resolutions and writes
// no cache.
func TestAgentTestRuntime(t *testing.T) {
	home, d := physDir(t), physDir(t)
	root := filepath.Join(home, ".nvm", "versions", "node")
	for _, v := range []string{"20.11.1", "24.1.0"} {
		bin := filepath.Join(root, "v"+v, "bin")
		require.NoError(t, os.MkdirAll(bin, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(bin, "node"), []byte("#!/bin/sh\necho v"+v+"\n"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(bin, "npm"), []byte("#!/bin/sh\necho npm under "+v+"\n"), 0o755))
	}
	cache := filepath.Join(home, ".cache")
	env := map[string]string{"HOME": home, "PWD": d, "GASH_HEADLESS": "1", "XDG_CACHE_HOME": cache, "PATH": os.Getenv("PATH")}
	t.Chdir(d)
	tail := func(r result) []string {
		t.Helper()
		require.Empty(t, r.Stderr)
		return parseTest(t, r).Tail
	}

	r := runTest(t, d, env, "--runtime", "node:24", "--", "npm", "test")
	require.Equal(t, []string{"npm under 24.1.0"}, tail(r))
	r = runTest(t, d, env, "--runtime", "node:20", "--runtime", "node:24", "--", "npm", "test")
	require.Equal(t, []string{"npm under 20.11.1"}, tail(r))

	// project: nothing pinned is a STOP and nothing runs.
	r = runTest(t, d, env, "--runtime", "node:project", "--", "npm", "test")
	require.Equal(t, 1, r.RC)
	require.Empty(t, r.Stdout)
	require.Contains(t, r.Stderr, `"error":"runtime_not_pinned","details":"no node version is pinned in `+d+`","action":"STOP"`)
	require.NoError(t, os.WriteFile(filepath.Join(d, ".nvmrc"), []byte("20\n"), 0o644))
	r = runTest(t, d, env, "--runtime", "node:project", "--", "npm", "test")
	require.Equal(t, []string{"npm under 20.11.1"}, tail(r))

	// --dry-run: the resolutions, in PATH order; no cache written.
	require.NoError(t, os.RemoveAll(cache))
	r = runTest(t, d, env, "--dry-run", "--runtime", "node:project", "--runtime", "npm:24", "--", "npm", "test")
	require.Equal(t, result{`{"argv":["npm","test"],"runner":null,"cwd":` + string(mustJSON(t, d)) +
		`,"runtimes":[{"runtime":"node","request":"20","path":"` + root + `/v20.11.1/bin/node","version":"20.11.1"},` +
		`{"runtime":"npm","request":"24","path":"` + root + `/v24.1.0/bin/npm","version":"24.1.0"}]` +
		`,"dry_run":true,"written":false}` + "\n", "", 0}, r)
	require.NoDirExists(t, cache)

	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"--runtime"}, "missing_value"},
		{[]string{"--runtime", "node24", "--", "npm", "test"}, "unknown_runtime"},
		{[]string{"--runtime", "ruby:3", "--", "npm", "test"}, "unknown_runtime"},
		{[]string{"--runtime", "node:x", "--", "npm", "test"}, "invalid_version"},
		{[]string{"--runtime", "node:18", "--", "npm", "test"}, "runtime_not_installed"},
		{[]string{"--dry-run", "--runtime", "node:18", "--", "npm", "test"}, "runtime_not_installed"},
	} {
		r := runTest(t, d, env, tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Empty(t, r.Stdout, tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
	}
}
