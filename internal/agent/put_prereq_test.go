// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/prereq"
)

// putTools are the programs agent_put runs on the host.
var putTools = []string{"sh", "bash", "cat", "wc", "mv", "rm", "cp", "chmod", "date", "realpath", "readlink", "sha256sum"}

// putPath is a remote PATH with putTools but the missing ones.
func putPath(t *testing.T, missing ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range putTools {
		if slices.Contains(missing, name) {
			continue
		}
		p, err := exec.LookPath(name)
		require.NoError(t, err, name)
		require.NoError(t, os.Symlink(p, filepath.Join(dir, name)))
	}
	return dir
}

// TestPutRailsWithoutTr: without tr on the host gash_secret compared an
// empty string, so a symlink to a secret file was written through. Under
// dash and bash --posix, without tr, it is refused, and an ordinary upload
// (whose size check used tr) works.
func TestPutRailsWithoutTr(t *testing.T) {
	for _, shell := range [][]string{{"sh"}, {"bash", "--posix"}} {
		if _, err := exec.LookPath(shell[0]); err != nil {
			continue
		}
		f := newPutFixture(t)
		f.ssh.Shell = shell
		f.ssh.Path = putPath(t, "tr")
		write(t, filepath.Join(f.rhome, ".env"), "SECRET=1\n", 0o600)
		require.NoError(t, os.Symlink("../.env", filepath.Join(f.rhome, "app", "settings")))
		require.NoError(t, os.Symlink("../.ssh/config", filepath.Join(f.rhome, "app", "sshconf")))
		for _, target := range []string{"web:app/settings", "web:app/sshconf"} {
			r := f.run(t, "", "agent_put", target, "--from", "config.php", "--force")
			require.Equal(t, 1, r.RC, "%v %s", shell, target)
			require.Equal(t, "secret_file_blocked/FATAL", code(t, r), "%v %s", shell, target)
		}
		require.Equal(t, "SECRET=1\n", read(t, filepath.Join(f.rhome, ".env")))
		out := putJSON(t, f.run(t, "", "agent_put", "web:app/config.php", "--from", "config.php", "--backup"))
		require.Equal(t, true, out["written"], shell)
		require.Equal(t, "<?php return 2;\n", read(t, filepath.Join(f.rhome, "app", "config.php")))
	}
}

// TestPutMissingHostTools: a missing wc made the upload "cut short", a
// missing cp gave the new file the default mode instead of the old one's;
// the program is named now, before anything is written (a dry run too).
func TestPutMissingHostTools(t *testing.T) {
	for _, tc := range []struct {
		missing string
		args    []string
	}{
		{"wc", []string{"web:app/new.php"}},
		{"cp", []string{"web:app/config.php"}},
		{"chmod", []string{"web:app/new.php", "--mode", "600"}},
		{"date", []string{"web:app/config.php", "--backup"}},
		{"wc", []string{"web:app/new.php", "--dry-run"}},
	} {
		f := newPutFixture(t)
		f.ssh.Path = putPath(t, tc.missing)
		r := f.run(t, "", "agent_put", append(tc.args, "--from", "config.php")...)
		require.Equal(t, 1, r.RC, "%q", tc.args)
		require.Equal(t, `{"error":"prerequisite_missing","details":"agent_put on web needs `+tc.missing+`, which is not installed there","action":"STOP","recoverable":true,`+
			`"hint":"Install `+tc.missing+` on web (usually the coreutils package), then retry","missing":"`+tc.missing+`","host":"web"}`+"\n", r.Stderr, "%q", tc.args)
		require.Equal(t, "<?php return 1;\n", read(t, filepath.Join(f.rhome, "app", "config.php")))
		require.NoFileExists(t, filepath.Join(f.rhome, "app", "new.php"))
	}
	// The programs a call does not use are not asked for.
	f := newPutFixture(t)
	f.ssh.Path = putPath(t, "cp", "chmod", "date")
	putJSON(t, f.run(t, "", "agent_put", "web:app/new.php", "--from", "config.php"))
}

// TestPutWithoutLocalSSH: host_unreachable (RETRY) became the
// prerequisite report, before anything connects.
func TestPutWithoutLocalSSH(t *testing.T) {
	defer prereq.SetSystem(&prereq.System{OS: platform.OS{GOOS: "linux", ID: "debian", Name: "Debian GNU/Linux 12 (bookworm)"}, Manager: prereq.Apt, Sudo: true,
		LookPath: func(name string, _ []string) (string, bool) { return "/usr/bin/" + name, name != "ssh" }})()
	f := newPutFixture(t)
	r := f.run(t, "", "agent_put", "web:app/new.php", "--from", "config.php")
	require.Equal(t, 1, r.RC)
	require.Equal(t, `{"error":"prerequisite_missing","details":"agent_put needs ssh, which is not installed","action":"STOP","recoverable":true,`+
		`"hint":"Install openssh-client: sudo apt-get install -y openssh-client","missing":"ssh","install":"sudo apt-get install -y openssh-client"}`+"\n", r.Stderr)
	require.Zero(t, f.ssh.N())
	r = f.run(t, "", "agent_replace", "app/config.php", "--old", "1", "--new", "3", "--host", "web")
	require.Equal(t, "prerequisite_missing/STOP", code(t, r))
	require.Zero(t, f.ssh.N())
}
