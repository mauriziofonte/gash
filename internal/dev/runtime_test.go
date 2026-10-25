// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package dev

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
)

// stub writes an executable script at p that prints out.
func stub(t *testing.T, p, out string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\necho '"+out+"'\n"), 0o755))
}

// runtimeFixture is a HOME with an nvm tree, a sysroot with versioned php
// binaries and a PATH directory, all empty until a test fills them.
type runtimeFixture struct {
	home, sys, bin string
}

func newRuntimeFixture(t *testing.T) runtimeFixture {
	t.Helper()
	f := runtimeFixture{home: phys(t), sys: phys(t), bin: phys(t)}
	return f
}

func (f runtimeFixture) node(t *testing.T, v string) {
	t.Helper()
	dir := filepath.Join(f.home, ".nvm", "versions", "node", "v"+v, "bin")
	stub(t, filepath.Join(dir, "node"), "v"+v)
	stub(t, filepath.Join(dir, "npm"), "10.0.0")
}

func (f runtimeFixture) php(t *testing.T, xy, full string) {
	t.Helper()
	stub(t, filepath.Join(f.sys, "usr", "bin", "php"+xy), "PHP "+full+" (cli) (built: Jan  1 2026 00:00:00) (NTS)")
}

func (f runtimeFixture) resolver(t *testing.T, store bool) *Resolver {
	t.Helper()
	sys, err := platform.NewSysroot(f.sys)
	require.NoError(t, err)
	env := invoke.EnvMap(map[string]string{"HOME": f.home, "PATH": f.bin})
	return NewResolver(context.Background(), env, runner.Exec{}, store).WithSysroot(sys)
}

func TestRuntimeRequestsAndMatching(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{{"24", "24", true}, {"v24", "24", true}, {"8.4", "8.4", true}, {"8.4.1", "8.4.1", true},
		{"", "", false}, {"x", "", false}, {"8.4.1.2", "", false}, {"8.", "", false}, {"lts", "", false}} {
		got, ok := NormalizeRequest(tc.in)
		require.Equal(t, tc.ok, ok, tc.in)
		require.Equal(t, tc.want, got, tc.in)
	}
	require.Equal(t, matchYes, versionMatch("24", "24.1.0"))
	require.Equal(t, matchNo, versionMatch("2", "24.1.0"))
	require.Equal(t, matchYes, versionMatch("8.4", "8.4"))
	require.Equal(t, matchProbe, versionMatch("8.4.1", "8.4"))
	require.Equal(t, matchNo, versionMatch("8.4", ""))
	for rt, fam := range map[string]string{"php": "php", "composer": "php", "node": "node", "npm": "node", "npx": "node", "go": ""} {
		require.Equal(t, fam, RuntimeFamily(rt))
	}
}

func TestResolveNodeFromNVM(t *testing.T) {
	f := newRuntimeFixture(t)
	f.node(t, "20.11.1")
	f.node(t, "24.1.0")
	f.node(t, "24.10.2")
	// A stray name and a prerelease are not installed versions.
	require.NoError(t, os.MkdirAll(filepath.Join(f.home, ".nvm", "versions", "node", "v25.0.0-rc.1", "bin"), 0o755))
	r := f.resolver(t, false)

	res, err := r.Resolve("node", "24")
	require.NoError(t, err)
	nvmBin := filepath.Join(f.home, ".nvm", "versions", "node", "v24.10.2", "bin")
	require.Equal(t, Resolution{Runtime: "node", Request: "24", Path: filepath.Join(nvmBin, "node"),
		Version: "24.10.2", Interp: filepath.Join(nvmBin, "node")}, res)

	res, err = r.Resolve("npm", "24.1")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(f.home, ".nvm", "versions", "node", "v24.1.0", "bin", "npm"), res.Path)
	require.Equal(t, "24.1.0", res.Version)

	// npx is missing from the tree.
	_, err = r.Resolve("npx", "24")
	var ni *NotInstalledError
	require.True(t, errors.As(err, &ni))
	require.Equal(t, "npx", ni.Missing)

	_, err = r.Resolve("node", "18")
	require.True(t, errors.As(err, &ni))
	require.Equal(t, []string{"20.11.1", "24.1.0", "24.10.2"}, ni.Installed)
	require.Equal(t, "node 18 is not installed; installed: 20.11.1, 24.1.0, 24.10.2", ni.Error())
	require.Contains(t, ni.Hint(), "nvm install 18")

	// NVM_DIR wins over $HOME/.nvm.
	other := phys(t)
	stub(t, filepath.Join(other, "versions", "node", "v18.20.0", "bin", "node"), "v18.20.0")
	r = NewResolver(context.Background(), invoke.EnvMap(map[string]string{"HOME": f.home, "NVM_DIR": other, "PATH": f.bin}), runner.Exec{}, false)
	res, err = r.Resolve("node", "18")
	require.NoError(t, err)
	require.Equal(t, "18.20.0", res.Version)
}

func TestResolveNodeFromPATH(t *testing.T) {
	f := newRuntimeFixture(t)
	f.node(t, "20.11.1")
	stub(t, filepath.Join(f.bin, "node"), "v22.3.0")
	r := f.resolver(t, false)
	res, err := r.Resolve("node", "22")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(f.bin, "node"), res.Path)
	require.Equal(t, "22.3.0", res.Version)
	// nvm wins when it has the major.
	res, err = r.Resolve("node", "20")
	require.NoError(t, err)
	require.Equal(t, "20.11.1", res.Version)
	// The PATH node is listed among the installed.
	_, err = r.Resolve("node", "16")
	var ni *NotInstalledError
	require.True(t, errors.As(err, &ni))
	require.Equal(t, []string{"20.11.1", "22.3.0"}, ni.Installed)
	inst := r.Installed("node")
	require.Equal(t, []Installed{{"20.11.1", filepath.Join(f.home, ".nvm/versions/node/v20.11.1/bin/node")}, {"22.3.0", filepath.Join(f.bin, "node")}}, inst)
}

func TestResolvePHPAndComposer(t *testing.T) {
	f := newRuntimeFixture(t)
	f.php(t, "8.3", "8.3.6")
	f.php(t, "8.4", "8.4.12")
	r := f.resolver(t, false)

	res, err := r.Resolve("php", "8.4")
	require.NoError(t, err)
	require.Equal(t, "/usr/bin/php8.4", res.Path)
	require.Equal(t, "8.4.12", res.Version)
	require.Equal(t, filepath.Join(f.sys, "usr/bin/php8.4"), res.Interp)

	res, err = r.Resolve("php", "8")
	require.NoError(t, err)
	require.Equal(t, "/usr/bin/php8.4", res.Path)

	// A request finer than the name probes the binary.
	res, err = r.Resolve("php", "8.3.6")
	require.NoError(t, err)
	require.Equal(t, "/usr/bin/php8.3", res.Path)
	_, err = r.Resolve("php", "8.3.7")
	var ni *NotInstalledError
	require.True(t, errors.As(err, &ni))
	require.Equal(t, []string{"8.3", "8.4"}, ni.Installed)
	require.Contains(t, ni.Hint(), "8.3, 8.4")

	// composer: none yet, then one on PATH, run by the resolved php.
	_, err = r.Resolve("composer", "8.4")
	require.True(t, errors.As(err, &ni))
	require.Equal(t, "composer", ni.Missing)
	stub(t, filepath.Join(f.bin, "composer"), "Composer version 2.8.1")
	res, err = r.Resolve("composer", "8.4")
	require.NoError(t, err)
	require.Equal(t, Resolution{Runtime: "composer", Request: "8.4", Path: filepath.Join(f.bin, "composer"),
		Version: "8.4.12", PHP: "/usr/bin/php8.4", Interp: filepath.Join(f.sys, "usr/bin/php8.4")}, res)

	// A plain php on PATH serves the versions /usr/bin lacks.
	stub(t, filepath.Join(f.bin, "php"), "PHP 7.4.33 (cli)")
	res, err = r.Resolve("php", "7")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(f.bin, "php"), res.Path)
	require.Equal(t, "7.4.33", res.Version)
	require.Equal(t, []Installed{{"7.4.33", filepath.Join(f.bin, "php")}, {"8.3.6", "/usr/bin/php8.3"}, {"8.4.12", "/usr/bin/php8.4"}},
		r.Installed("php"))
}

func TestRuntimeCache(t *testing.T) {
	f := newRuntimeFixture(t)
	f.php(t, "8.4", "8.4.1")
	f.node(t, "24.1.0")

	// The llm_* resolver never writes the cache.
	r := f.resolver(t, false)
	_, err := r.Resolve("php", "8.4")
	require.NoError(t, err)
	r.Save()
	cache := filepath.Join(f.home, ".cache", "gash", "runtimes.json")
	require.NoFileExists(t, cache)

	// The command resolver does, and the next call reads it: a stub that
	// now prints another version is not probed again while its mtime and
	// size are the same.
	r = f.resolver(t, true)
	_, err = r.Resolve("php", "8.4")
	require.NoError(t, err)
	_, err = r.Resolve("node", "24")
	require.NoError(t, err)
	r.Save()
	require.FileExists(t, cache)
	st, err := os.Stat(cache)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), st.Mode().Perm())

	bin := filepath.Join(f.sys, "usr", "bin", "php8.4")
	old, err := os.Stat(bin)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\necho 'PHP 8.4.2 (cli) (built: Jan  1 2026 00:00:00) (NTS)'\n"), 0o755))
	require.NoError(t, os.Chtimes(bin, old.ModTime(), old.ModTime()))
	res, err := f.resolver(t, true).Resolve("php", "8.4")
	require.NoError(t, err)
	require.Equal(t, "8.4.1", res.Version, "cached")

	// A new mtime is a new binary.
	later := old.ModTime().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(bin, later, later))
	res, err = f.resolver(t, true).Resolve("php", "8.4")
	require.NoError(t, err)
	require.Equal(t, "8.4.2", res.Version)

	// A new nvm version changes the directory's mtime: it is seen.
	f.node(t, "24.2.0")
	dir := filepath.Join(f.home, ".nvm", "versions", "node")
	require.NoError(t, os.Chtimes(dir, later, later))
	res, err = f.resolver(t, true).Resolve("node", "24")
	require.NoError(t, err)
	require.Equal(t, "24.2.0", res.Version)

	// A corrupt cache is ignored.
	require.NoError(t, os.WriteFile(cache, []byte("{"), 0o600))
	res, err = f.resolver(t, true).Resolve("node", "24")
	require.NoError(t, err)
	require.Equal(t, "24.2.0", res.Version)
}

func TestRuntimeBinDir(t *testing.T) {
	f := newRuntimeFixture(t)
	f.node(t, "24.1.0")
	f.php(t, "8.4", "8.4.1")
	stub(t, filepath.Join(f.sys, "usr", "bin", "php"), "PHP 8.3.0 (cli)")
	r := f.resolver(t, true)

	res, err := r.Resolve("npm", "24")
	require.NoError(t, err)
	dir, err := r.BinDir(res)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(f.home, ".nvm/versions/node/v24.1.0/bin"), dir)

	// /usr/bin/php is another version: a link in the cache.
	res, err = r.Resolve("php", "8.4")
	require.NoError(t, err)
	dir, err = r.BinDir(res)
	require.NoError(t, err)
	require.True(t, filepath.IsAbs(dir))
	require.Contains(t, dir, filepath.Join(f.home, ".cache", "gash", "runtime-bin"))
	target, err := os.Readlink(filepath.Join(dir, "php"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(f.sys, "usr/bin/php8.4"), target)
	again, err := r.BinDir(res)
	require.NoError(t, err)
	require.Equal(t, dir, again)
}

func TestProbeVersion(t *testing.T) {
	d := phys(t)
	env := invoke.EnvMap(map[string]string{"PATH": d})
	for out, want := range map[string]string{
		"git version 2.43.0":                   "2.43.0",
		"GNU bash, version 5.2.21(1)-release":  "5.2.21",
		"go version go1.27.3 linux/amd64":      "1.27.3",
		"jq-1.7.1":                             "1.7.1",
		"no version here":                      "",
		"Python 3.12.3":                        "3.12.3",
		"Docker version 27.0.3, build 7d4bcd8": "27.0.3",
	} {
		stub(t, filepath.Join(d, "tool"), out)
		require.Equal(t, want, ProbeVersion(context.Background(), runner.Exec{}, env, filepath.Join(d, "tool"), "--version"), out)
	}
	require.Equal(t, "", ProbeVersion(context.Background(), runner.Exec{}, env, filepath.Join(d, "missing"), "--version"))
}
