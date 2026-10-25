// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/remote/sshtest"
	"github.com/mauriziofonte/gash/internal/term"
)

// putFixture is a local working directory, a local HOME with two HOST:
// entries and a fake remote home (sshtest).
type putFixture struct {
	dir, home, rhome, stub string
	ssh                    *sshtest.SSH
}

func newPutFixture(t *testing.T) *putFixture {
	t.Helper()
	f := &putFixture{dir: physDir(t), home: physDir(t), rhome: physDir(t), stub: physDir(t)}
	write(t, filepath.Join(f.home, ".gash_env"), "HOST:web=deploy@web.example\n", 0o600)
	write(t, filepath.Join(f.dir, "config.php"), "<?php return 2;\n", 0o644)
	write(t, filepath.Join(f.dir, ".env"), "A=1\n", 0o600)
	write(t, filepath.Join(f.rhome, "app", "config.php"), "<?php return 1;\n", 0o640)
	f.ssh = &sshtest.SSH{Home: f.rhome, Path: f.stub + ":" + os.Getenv("PATH")}
	return f
}

func (f *putFixture) run(t *testing.T, stdin string, name string, args ...string) result {
	t.Helper()
	m := New()
	m.getwd = func() (string, error) { return f.dir, nil }
	m.rootHome = func() string { return "/root" }
	m.run = f.ssh
	m.exe = func() (string, error) { return "", os.ErrNotExist }
	tbl := invoke.NewTable()
	m.Register(tbl)
	var out, errb bytes.Buffer
	env := invoke.EnvMap(map[string]string{"HOME": f.home, "PWD": f.dir, "GASH_HEADLESS": "1", "PATH": os.Getenv("PATH")})
	rc := tbl.Run(&invoke.Call{Ctx: context.Background(), Name: name, Args: args, Env: env, Stdin: strings.NewReader(stdin),
		Stdout: &out, Stderr: &errb, Term: term.Fixed(env, false, false, false)})
	return result{out.String(), errb.String(), rc}
}

func putJSON(t *testing.T, r result) map[string]any {
	t.Helper()
	require.Equal(t, 0, r.RC, r.Stderr)
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &out), r.Stdout)
	return out
}

func TestPutNewFileOneInvocation(t *testing.T) {
	f := newPutFixture(t)
	r := f.run(t, "", "agent_put", "web:app/new.php", "--from", "config.php")
	out := putJSON(t, r)
	abs := filepath.Join(f.rhome, "app", "new.php")
	require.Equal(t, `{"host":"web","path":"`+abs+`","bytes":16,"sha256":"`+out["sha256"].(string)+`","replaced":false,"backup":null,"verified":false,"written":true}`+"\n", r.Stdout)
	require.Len(t, out["sha256"], 64)
	require.Equal(t, "<?php return 2;\n", read(t, abs))
	require.Equal(t, 1, f.ssh.N())
	require.Equal(t, "<?php return 2;\n", string(f.ssh.Stdins[0]), "the content goes on stdin")
	left, _ := filepath.Glob(filepath.Join(f.rhome, "app", ".gash-put.*"))
	require.Empty(t, left)
}

func TestPutReplacesWithBackupAndVerify(t *testing.T) {
	f := newPutFixture(t)
	abs := filepath.Join(f.rhome, "app", "config.php")
	out := putJSON(t, f.run(t, "", "agent_put", "web:app/config.php", "--from", "config.php", "--backup", "--verify"))
	require.Equal(t, true, out["replaced"])
	require.Equal(t, true, out["verified"])
	backup := out["backup"].(string)
	require.Regexp(t, `/app/config\.php\.bak-\d{8}T\d{6}Z$`, backup)
	require.Equal(t, "<?php return 1;\n", read(t, backup))
	require.Equal(t, "<?php return 2;\n", read(t, abs))
	st, err := os.Stat(abs)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o640), st.Mode().Perm(), "the old file's mode is kept")

	out = putJSON(t, f.run(t, "fresh\n", "agent_put", "web:app/config.php", "--from", "-", "--mode", "0600", "--backup"))
	require.NotEqual(t, backup, out["backup"], "a second backup in the same second does not overwrite the first")
	require.Equal(t, "fresh\n", read(t, abs))
	st, err = os.Stat(abs)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), st.Mode().Perm())
}

func TestPutDryRunWritesNothing(t *testing.T) {
	f := newPutFixture(t)
	r := f.run(t, "", "agent_put", "web:app/config.php", "--from", "config.php", "--dry-run", "--backup")
	out := putJSON(t, r)
	require.Equal(t, true, out["exists"])
	require.Equal(t, true, out["dry_run"])
	require.Equal(t, false, out["written"])
	require.Nil(t, out["backup"])
	require.Equal(t, "<?php return 1;\n", read(t, filepath.Join(f.rhome, "app", "config.php")))
	require.Empty(t, f.ssh.Stdins[0], "a dry run sends no content")
	out = putJSON(t, f.run(t, "", "agent_put", "web:app/other.php", "--from", "config.php", "--dry-run"))
	require.Equal(t, false, out["exists"])
	require.NoFileExists(t, filepath.Join(f.rhome, "app", "other.php"))
}

func TestPutRefusesBeforeConnecting(t *testing.T) {
	f := newPutFixture(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"web:/etc/passwd", "--from", "config.php", "--force"}, "forbidden_path/FATAL"},
		{[]string{"web:/usr/bin/env", "--from", "config.php"}, "write_path_denied/FATAL"},
		{[]string{"web:/etc/nginx/site.conf", "--from", "config.php"}, "sensitive_path/STOP"},
		{[]string{"web:/var/www/x.php", "--from", "config.php"}, "sensitive_path/STOP"},
		{[]string{"web:.ssh/authorized_keys", "--from", "config.php", "--force"}, "secret_file_blocked/FATAL"},
		{[]string{"web:app/.env", "--from", "config.php"}, "secret_file_blocked/FATAL"},
		{[]string{"web:app/x", "--from", ".env"}, "secret_file_blocked/FATAL"},
		{[]string{"web:app/../../x", "--from", "config.php"}, "path_traversal_blocked/FATAL"},
		{[]string{"nope:app/x", "--from", "config.php"}, "unknown_host/STOP"},
		{[]string{"deploy@web.example:app/x", "--from", "config.php"}, "unknown_host/STOP"},
		{[]string{"app/x", "--from", "config.php"}, "invalid_target/RETRY"},
		{[]string{"web:app/x"}, "missing_file/RETRY"},
		{[]string{"--from", "config.php"}, "missing_host/RETRY"},
		{[]string{"web:app/x", "--from", "config.php", "--mode", "rw"}, "invalid_mode/RETRY"},
		{[]string{"web:app/x", "--from", "nope.php"}, "file_not_found/RETRY"},
		{[]string{"web:app/x", "web:app/y", "--from", "config.php"}, "unexpected_argument/RETRY"},
		{[]string{"web:app/x", "--from", "config.php", "--bogus"}, "unknown_option/RETRY"},
	} {
		r := f.run(t, "", "agent_put", tc.args...)
		require.Equal(t, 1, r.RC, "%q", tc.args)
		require.Equal(t, tc.want, code(t, r), "%q", tc.args)
	}
	require.Zero(t, f.ssh.N(), "nothing connected")
}

func TestPutRemoteRails(t *testing.T) {
	f := newPutFixture(t)
	require.NoError(t, os.Symlink("/usr/bin", filepath.Join(f.rhome, "bin-link")))
	require.NoError(t, os.Symlink("/var/lib/gash-test-nonexistent", filepath.Join(f.rhome, "var-link")))
	require.NoError(t, os.Symlink("server.key", filepath.Join(f.rhome, "app", "k")))
	for _, tc := range []struct{ target, want string }{
		{"web:bin-link/env", "write_path_denied/FATAL"},
		{"web:var-link/x", "sensitive_path/STOP"},
		{"web:app/k", "secret_file_blocked/FATAL"},
		{"web:nodir/x", "path_not_found/RETRY"},
		{"web:app", "invalid_target/RETRY"},
	} {
		r := f.run(t, "", "agent_put", tc.target, "--from", "config.php")
		require.Equal(t, 1, r.RC, tc.target)
		require.Equal(t, tc.want, code(t, r), tc.target)
	}
	require.Equal(t, 5, f.ssh.N(), "refused on the host, after resolving the links")
}

func TestPutVerifyFailureReplacesNothing(t *testing.T) {
	f := newPutFixture(t)
	write(t, filepath.Join(f.stub, "sha256sum"), "#!/bin/sh\necho 0000000000000000000000000000000000000000000000000000000000000000  -\n", 0o755)
	r := f.run(t, "", "agent_put", "web:app/config.php", "--from", "config.php", "--verify", "--backup")
	require.Equal(t, 1, r.RC)
	require.Equal(t, "put_verify_failed/STOP", code(t, r))
	require.Equal(t, "<?php return 1;\n", read(t, filepath.Join(f.rhome, "app", "config.php")))
	left, _ := filepath.Glob(filepath.Join(f.rhome, "app", "*"))
	require.Len(t, left, 1, "no temporary file and no backup left behind")

	f.ssh.Unreachable = true
	r = f.run(t, "", "agent_put", "web:app/config.php", "--from", "config.php")
	require.Equal(t, "host_unreachable/RETRY", code(t, r))
}

func TestReplaceOnAHost(t *testing.T) {
	f := newPutFixture(t)
	write(t, filepath.Join(f.stub, "uname"), "#!/bin/sh\necho SunOS\n", 0o755)
	r := f.run(t, "", "agent_replace", "app/config.php", "--old", "1", "--new", "3", "--host", "web")
	require.Equal(t, 1, r.RC)
	require.Equal(t, "remote_binary_unavailable/STOP", code(t, r), "a write has no POSIX fallback")
	require.Equal(t, 1, f.ssh.N())
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"/etc/passwd", "--old", "a", "--new", "b", "--host", "web"}, "forbidden_path/FATAL"},
		{[]string{"/usr/bin/env", "--old", "a", "--new", "b", "--host", "web"}, "write_path_denied/FATAL"},
		{[]string{"/etc/hosts", "--old", "a", "--new", "b", "--host", "web"}, "sensitive_path/STOP"},
		{[]string{"app/.env", "--old", "--host", "--new", "b", "--host", "web"}, "secret_file_blocked/FATAL"},
		{[]string{"app/x", "--old", "a", "--new", "b", "--host", "nope"}, "unknown_host/STOP"},
	} {
		r := f.run(t, "", "agent_replace", tc.args...)
		require.Equal(t, tc.want, code(t, r), "%q", tc.args)
	}
	require.Equal(t, 1, f.ssh.N(), "refused before connecting")
}
