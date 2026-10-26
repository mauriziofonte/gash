// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/remote"
	"github.com/mauriziofonte/gash/internal/remote/sshtest"
)

// protectHost is the fallback fixture (bsd: without GNU realpath) with
// PROTECT: lines for the host: lines are added to the local ~/.gash_env
// after the HOST: entry.
func protectHost(t *testing.T, bsd bool, lines string) *hostFixture {
	t.Helper()
	f := newHostFixtureOn(t, bsd)
	mustWrite(t, filepath.Join(f.env["HOME"], ".gash_env"), "HOST:web=deploy@web.example\n"+lines)
	mustWrite(t, filepath.Join(f.dir, "private", "a.txt"), "TODO private\n")
	mustWrite(t, filepath.Join(f.dir, "contracts", "app.yaml"), "a: TODO\n")
	mustWrite(t, filepath.Join(f.dir, "public", "app.yaml"), "a: 1\n")
	require.NoError(t, os.Symlink(filepath.Join(f.dir, "private"), filepath.Join(f.dir, "plink")))
	require.NoError(t, os.Symlink(filepath.Join(f.dir, "contracts", "app.yaml"), filepath.Join(f.dir, "public", "c.yaml")))
	return f
}

// TestHostProtectedFallback: the host's PROTECT: paths hold on the POSIX
// fallback: as written before anything connects, and on the path the host
// resolves (a link into a tree); a line for another host does not apply.
func TestHostProtectedFallback(t *testing.T) {
	for _, bsd := range []bool{false, true} {
		testHostProtectedFallback(t, bsd)
	}
}

func testHostProtectedFallback(t *testing.T, bsd bool) {
	f := protectHost(t, bsd, "PROTECT:all@web=~/private\nPROTECT:content@web=~/contracts\nPROTECT:all@db=~/public\n")
	call := func(fn string, args ...string) result {
		t.Helper()
		return runGo(t, f.m, f.dir, f.env, fn, append(args, "--host", "web")...)
	}

	n := f.ssh.N()
	r := call("llm_tree", "private")
	require.Equal(t, "forbidden_path/FATAL", gitCode(t, r))
	r = call("llm_config", "~/private/a.txt")
	require.Equal(t, "forbidden_path/FATAL", gitCode(t, r))
	require.Equal(t, n, f.ssh.N(), "refused before ssh")

	r = call("llm_tree", "plink")
	require.Equal(t, "forbidden_path/FATAL", gitCode(t, r))
	r = call("llm_tree", ".")
	require.Equal(t, "forbidden_path/FATAL", gitCode(t, r), "a fallback walk may not start above a PROTECT:all tree")
	r = call("llm_config", "contracts/app.yaml")
	require.Equal(t, "secret_file_blocked/FATAL", gitCode(t, r))
	r = call("llm_config", "public/c.yaml")
	require.Equal(t, "secret_file_blocked/FATAL", gitCode(t, r), "through a link")
	r = call("llm_grep", "TODO", "contracts")
	require.Equal(t, "secret_file_blocked/FATAL", gitCode(t, r))
	r = call("llm_config", "public/app.yaml")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Contains(t, r.Stdout, `"a": 1`)
	require.Greater(t, f.ssh.N(), n)
	for _, s := range f.ssh.Scripts {
		if strings.Contains(s, "GASH__PROTECT") {
			require.Contains(t, s, "GASH__PROTECT='all ~/private\ncontent ~/contracts\n'; export GASH__PROTECT; ")
			require.NotContains(t, s, "~/public")
		}
	}

	// A link the host resolves into a PROTECT:all path lists no target; a
	// link into a PROTECT:content path does (3.4.1).
	require.NoError(t, os.Symlink(filepath.Join(f.dir, "plink", "a.txt"), filepath.Join(f.dir, "public", "pa")))
	r = call("llm_tree", "public", "--text")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, "public/\napp.yaml\nc.yaml -> "+f.dir+"/contracts/app.yaml\npa -> [protected]\n", r.Stdout)

	// A content tree alone: a listing walks it, a search does not.
	f = protectHost(t, bsd, "PROTECT:content=~/contracts\n")
	r = call("llm_find", "*.yaml", ".")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Contains(t, r.Stdout, "contracts/app.yaml")
	r = call("llm_grep", "TODO", ".")
	require.Equal(t, "forbidden_path/FATAL", gitCode(t, r))
}

// TestHostNoBinary: GASH_REMOTE_NO_BINARY keeps the binary off the hosts
// it names ("*": every host): no cache test, no upload; a function with a
// fallback answers through it, one without is remote_binary_unavailable.
func TestHostNoBinary(t *testing.T) {
	if b, ok := remote.LocalBinary(nil); !ok || b.Platform == "" {
		t.Skip("no binary is shipped from this platform")
	}
	exe := filepath.Join(physTemp(t), "gash")
	mustWrite(t, exe, "#!/bin/sh\ncase $1 in version) echo 3.4.0 ;; *) printf '{\"protect\":\"%s\"}\\n' \"$(printf %s \"$GASH__PROTECT\" | tr '\\n' ';')\" ;; esac\n")
	require.NoError(t, os.Chmod(exe, 0o755))
	for _, tc := range []struct {
		setting string
		off     bool
	}{{"", false}, {"GASH_REMOTE_NO_BINARY=db\n", false}, {"GASH_REMOTE_NO_BINARY=db, web\n", true}, {"GASH_REMOTE_NO_BINARY=*\n", true}} {
		home, rhome := physTemp(t), physTemp(t)
		mustWrite(t, filepath.Join(home, ".gash_env"), "HOST:web=deploy@web.example\nPROTECT:all=~/private\n"+tc.setting)
		mustWrite(t, filepath.Join(rhome, "conf.yaml"), "a: 1\n")
		ssh := &sshtest.SSH{Home: rhome}
		m := New()
		m.run = ssh
		m.exe = func() (string, error) { return exe, nil }
		env := baseEnv(home, map[string]string{"PATH": sshtest.ClientPath(t, os.Getenv("PATH"))})
		d := physTemp(t)

		r := runGo(t, m, d, env, "llm_stat", "conf.yaml", "--host", "web")
		if !tc.off {
			require.Equal(t, 0, r.RC, r.Stderr)
			require.Equal(t, `{"host":"web","protect":"all ~/private;"}`+"\n", r.Stdout, "the binary gets the rules")
			require.NotEmpty(t, ssh.Stdins[len(ssh.Stdins)-1], "shipped")
			continue
		}
		require.Equal(t, "remote_binary_unavailable/STOP", gitCode(t, r), tc.setting)
		require.Contains(t, r.Stderr, "GASH_REMOTE_NO_BINARY keeps the gash binary off this host")
		r = runGo(t, m, d, env, "llm_config", "conf.yaml", "--host", "web")
		require.Equal(t, 0, r.RC, r.Stderr)
		require.Contains(t, r.Stdout, `"a": 1`, "the fallback answers")
		for i, s := range ssh.Scripts {
			require.NotContains(t, s, ".cache/gash/remote", tc.setting)
			require.Empty(t, ssh.Stdins[i], "nothing uploaded")
		}
		_, err := os.Stat(filepath.Join(rhome, ".cache"))
		require.True(t, os.IsNotExist(err), "no cache on the host")
	}
}

// TestRemoteRunProtected: llm_remote_run checks the line against the
// PROTECT: paths of every host it names, as written (a relative path is
// below the remote home), before anything connects.
func TestRemoteRunProtected(t *testing.T) {
	f := newRunFixture(t)
	f.env["GASH_ENV_FILE"] = envFile(t, "HOST:web=deploy@web.example\nHOST:db=deploy@db.example\n"+
		"PROTECT:all@db=~/private\nPROTECT:content=/srv/app/storage\n")
	for line, want := range map[string]string{
		"cat private/a.txt":             "forbidden_path/FATAL",
		"ls -la private":                "forbidden_path/FATAL",
		"find . -name x":                "forbidden_path/FATAL",
		"cat /srv/app/storage/keys.txt": "secret_file_blocked/FATAL",
		"grep -r token /srv/app":        "forbidden_path/FATAL",
	} {
		r := f.run(t, "web", "db", "--", line)
		require.Equal(t, 1, r.RC, "%s: %+v", line, r)
		require.Equal(t, want, gitCode(t, r), line)
	}
	require.Zero(t, f.ssh.N(), "nothing connected")
	r := f.run(t, "web", "--", "cat private/a.txt")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, 1, f.ssh.N(), "web has no PROTECT:all line")
}
