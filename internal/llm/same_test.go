// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/remote/sshtest"
)

// sameFixture is a local tree and its deployed copy in the fake remote
// home, with a docker stub on the remote PATH that runs `docker exec CTR
// sh -c S` on the host itself (container "gone" does not exist).
type sameFixture struct {
	local, rhome string
	env          map[string]string
	ssh          *sshtest.SSH
	m            *Module
}

func newSameFixture(t *testing.T) *sameFixture {
	t.Helper()
	local, rhome, home, stub := physTemp(t), physTemp(t), physTemp(t), physTemp(t)
	mustWrite(t, filepath.Join(home, ".gash_env"), "HOST:web=deploy@web.example\nHOST:db=deploy@db.example\n")
	for _, d := range []string{filepath.Join(local, "app"), filepath.Join(rhome, "srv", "app")} {
		for p, c := range map[string]string{
			"index.php": "<?php echo 1;\n", "src/a.php": "a\n", "src/b.php": "b\n", "public/x.css": "x{}\n",
			".git/HEAD": "ref\n", "node_modules/m.js": "m\n",
		} {
			mustWrite(t, filepath.Join(d, p), c)
		}
	}
	mustWrite(t, filepath.Join(local, "app", ".env"), "A=1\n")
	mustWrite(t, filepath.Join(rhome, "srv", "app", ".env"), "A=2\n")
	mustWrite(t, filepath.Join(stub, "docker"), "#!/bin/sh\n[ \"$1\" = exec ] || exit 2\n[ \"$2\" = gone ] && { echo 'Error response from daemon: No such container: gone' >&2; exit 1; }\nshift 2\nexec \"$@\"\n")
	require.NoError(t, os.Chmod(filepath.Join(stub, "docker"), 0o755))
	ssh := &sshtest.SSH{Home: rhome, Path: stub + ":" + os.Getenv("PATH")}
	m := New()
	m.run = ssh
	return &sameFixture{local: local, rhome: rhome, env: baseEnv(home, nil), ssh: ssh, m: m}
}

func (f *sameFixture) run(t *testing.T, args ...string) (sameOut, result) {
	t.Helper()
	r := runGo(t, f.m, f.local, f.env, "llm_same", args...)
	var out sameOut
	if r.RC == 0 {
		require.NoError(t, json.Unmarshal([]byte(r.Stdout), &out), r.Stdout)
	}
	return out, r
}

func TestSameTreeOneRoundTrip(t *testing.T) {
	f := newSameFixture(t)
	for i := 0; i < 300; i++ {
		name := fmt.Sprintf("many/f%03d.txt", i)
		mustWrite(t, filepath.Join(f.local, "app", name), name)
		mustWrite(t, filepath.Join(f.rhome, "srv", "app", name), name)
	}
	out, r := f.run(t, "app", "web:srv/app")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.True(t, out.Same)
	require.Equal(t, 305, out.Files, ".git and the secret .env left out")
	require.Equal(t, 1, out.SecretsSkipped)
	require.Equal(t, `{"same":true,"files":305,"diff":[],"secrets_skipped":1}`+"\n", r.Stdout)
	require.Equal(t, 1, f.ssh.N(), "one round trip for 305 files")
}

func TestSameListsTheDifferences(t *testing.T) {
	f := newSameFixture(t)
	mustWrite(t, filepath.Join(f.rhome, "srv", "app", "src", "a.php"), "changed\n")
	require.NoError(t, os.Remove(filepath.Join(f.rhome, "srv", "app", "public", "x.css")))
	mustWrite(t, filepath.Join(f.rhome, "srv", "app", "extra.txt"), "e\n")
	mustWrite(t, filepath.Join(f.rhome, "srv", "app", ".git", "HEAD"), "other\n")
	out, r := f.run(t, "app", "web:"+filepath.Join(f.rhome, "srv", "app"), "db:srv/app", "--exclude", "node_modules")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.False(t, out.Same)
	require.Equal(t, 4, out.Files)
	ps := []string{}
	for _, d := range out.Diff {
		ps = append(ps, d.Host+":"+d.P)
	}
	require.Equal(t, []string{"web:extra.txt", "web:public/x.css", "web:src/a.php", "db:extra.txt", "db:public/x.css", "db:src/a.php"}, ps)
	require.Nil(t, out.Diff[0].Local)
	require.Equal(t, "a2bbdb2de535", *out.Diff[0].Remote, "sha256 of e\\n, 12 hex")
	require.Nil(t, out.Diff[1].Remote)
	require.Len(t, *out.Diff[2].Local, 12)
	require.Equal(t, 2, f.ssh.N())

	out, r = f.run(t, "app", "web:srv/app", "--max-lines", "1", "--width", "4")
	require.Equal(t, 0, r.RC)
	require.Len(t, out.Diff, 1)
	require.Equal(t, 2, out.Truncated, "3 differences, 1 kept")
	require.Equal(t, "ext…", out.Diff[0].P)
}

func TestSameFiles(t *testing.T) {
	f := newSameFixture(t)
	_, r := f.run(t, "app/index.php", "web:srv/app/index.php")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, `{"same":true,"files":1,"diff":[]}`+"\n", r.Stdout)
	out, _ := f.run(t, "app/src/a.php", "web:srv/app/src/b.php")
	require.False(t, out.Same)
	require.Equal(t, "app/src/a.php", out.Diff[0].P)
	out, _ = f.run(t, "app/index.php", "web:srv/app/nope.php")
	require.Equal(t, ".", out.Diff[0].P)
	require.Nil(t, out.Diff[0].Remote)
	out, _ = f.run(t, "app", "web:srv/app/index.php")
	require.Equal(t, "directory", *out.Diff[0].Local)
	require.Equal(t, "file", *out.Diff[0].Remote)
	out, _ = f.run(t, "app/index.php", "web:~/srv/app/index.php")
	require.True(t, out.Same, "~/ is the remote home")
}

func TestSameInAContainer(t *testing.T) {
	f := newSameFixture(t)
	out, r := f.run(t, "app/index.php", "web:srv/app/index.php", "--container", "api")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.True(t, out.Same)
	require.Contains(t, f.ssh.Scripts[0], "docker exec 'api' sh -c")
	_, r = f.run(t, "app/index.php", "web:srv/app/index.php", "--container", "gone")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `"error":"container_unavailable","details":"web: Error response from daemon: No such container: gone"`)
	_, r = f.run(t, "app/index.php", "web:x", "--container", "a;b")
	require.Contains(t, r.Stderr, `"error":"invalid_container_name"`)
}

func TestSameRefusesBeforeConnecting(t *testing.T) {
	f := newSameFixture(t)
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"app", "web"}, "invalid_target"},
		{[]string{"app", "web:"}, "invalid_target"},
		{[]string{"app", "nope:/srv"}, "unknown_host"},
		{[]string{"app", "web:/etc/shadow"}, "forbidden_path"},
		{[]string{"app", "web:srv/../../etc"}, "path_traversal_blocked"},
		{[]string{"app"}, "missing_host"},
		{[]string{}, "missing_file"},
		{[]string{"missing", "web:srv"}, "path_not_found"},
		{[]string{"app/.env", "web:srv/app/.env"}, "secret_file_blocked"},
		{[]string{"/proc", "web:srv"}, "forbidden_path"},
		{[]string{"app", "web:srv", "--exclude", "a/b"}, "invalid_exclude"},
		{[]string{"app", "web:srv", "--bogus"}, "unknown_option"},
		{[]string{"app", "web:srv", "--container"}, "missing_value"},
	} {
		_, r := f.run(t, tc.args...)
		require.Equal(t, 1, r.RC, "%q", tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, "%q", tc.args)
	}
	require.Zero(t, f.ssh.N())
	f.ssh.Unreachable = true
	_, r := f.run(t, "app", "web:srv/app")
	require.Contains(t, r.Stderr, `"error":"host_unreachable"`)
}

func TestSameRemoteRails(t *testing.T) {
	f := newSameFixture(t)
	require.NoError(t, os.Symlink("/proc/self", filepath.Join(f.rhome, "p")))
	_, r := f.run(t, "app", "web:p")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `"error":"forbidden_path"`)
	require.Contains(t, r.Stderr, `"action":"FATAL"`)
}

func TestParseHashes(t *testing.T) {
	h := "0263829989b6fd954f72baaf2fc64bc2e2f01d692d4de72986ea808f6e99813f"
	got := parseHashes(h+"  ./a b\n"+h+" *./c\n"+h+" ./d\nshort\n", false)
	require.Equal(t, map[string]string{"a b": h, "c": h, "d": h}, got)
	require.Equal(t, map[string]string{".": h}, parseHashes(h+"  /srv/x\n", true))
}
