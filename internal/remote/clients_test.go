// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/prereq"
)

// debian is a pinned Debian 12 without the named programs, so the
// expected reports do not depend on the machine.
func debian(missing ...string) *prereq.System {
	return &prereq.System{OS: platform.OS{GOOS: "linux", ID: "debian", Name: "Debian GNU/Linux 12 (bookworm)"}, Manager: prereq.Apt, Sudo: true,
		LookPath: func(name string, _ []string) (string, bool) {
			return "/usr/bin/" + name, !slices.Contains(missing, name)
		}}
}

// TestRemoteMissingClients: remote_exec without ssh exited 127 with no
// message, remote_push and remote_sync_* printed "scp/rsync is not
// installed." and missed a missing ssh: each is now the prerequisite
// report, and nothing runs or is created.
func TestRemoteMissingClients(t *testing.T) {
	report := func(need, tool, purpose string) string {
		return "Error: " + need + " needs " + tool + ", which is not installed.\n" +
			"  " + tool + " " + purpose + " (package openssh-client on Debian GNU/Linux 12).\n" +
			"  Install it:  sudo apt-get install -y openssh-client\n" +
			"  Or run:      gash_doctor --install " + tool + "\n"
	}
	for _, tc := range []struct {
		missing, fn string
		args        []string
		want        string
	}{
		{"ssh", "remote_exec", []string{"box", "uptime"}, report("remote_exec", "ssh", "connects to remote hosts")},
		{"scp", "remote_push", []string{"box", "f", "/tmp/f"}, report("remote_push", "scp", "copies files to and from remote hosts")},
		{"ssh", "remote_pull", []string{"box", "/tmp/f", "f"}, report("remote_pull", "ssh", "connects to remote hosts")},
		{"rsync", "remote_sync_push", []string{"box", "d", "/tmp/d"}, "Error: remote_sync_push needs rsync, which is not installed.\n" +
			"  rsync syncs directories with remote hosts (package rsync on Debian GNU/Linux 12).\n" +
			"  Install it:  sudo apt-get install -y rsync\n  Or run:      gash_doctor --install rsync\n"},
		{"ssh", "remote_sync_pull", []string{"box", "/tmp/d", "d"}, report("remote_sync_pull", "ssh", "connects to remote hosts")},
	} {
		t.Run(tc.fn+" without "+tc.missing, func(t *testing.T) {
			defer prereq.SetSystem(debian(tc.missing))()
			s := newGoScene(t, boxEnv)
			r := s.runGo(plain, tc.fn, nil, tc.args...)
			require.Equal(t, 1, r.RC)
			require.Equal(t, tc.want, r.Err)
			require.Empty(t, r.Out)
			require.Empty(t, r.Log, "nothing ran")
			require.NotContains(t, r.Tree, "home/.ssh", "nothing was created")
		})
	}
	t.Run("headless", func(t *testing.T) {
		defer prereq.SetSystem(debian("ssh"))()
		r := newGoScene(t, boxEnv).runGo(headless, "remote_exec", nil, "box", "uptime")
		require.Equal(t, 1, r.RC)
		require.Equal(t, `{"error":"prerequisite_missing","details":"remote_exec needs ssh, which is not installed","action":"STOP","recoverable":true,`+
			`"hint":"Install openssh-client: sudo apt-get install -y openssh-client","missing":"ssh","install":"sudo apt-get install -y openssh-client"}`+"\n", r.Err)
	})
	t.Run("usage errors first", func(t *testing.T) {
		defer prereq.SetSystem(debian("ssh", "scp"))()
		r := newGoScene(t, boxEnv).runGo(plain, "remote_exec", nil, "box")
		require.Contains(t, r.Err, "Missing remote command")
		r = newGoScene(t, boxEnv).runGo(plain, "remote_push", nil, "box", "f", "/etc/shadow")
		require.Contains(t, r.Err, "matched: /etc/shadow")
	})
}

// TestRemoteExecScriptWithoutCat: the script of --script was read with
// cat, so without cat it was "Script is empty"; the binary reads it now.
func TestRemoteExecScriptWithoutCat(t *testing.T) {
	for _, mode := range []string{"file", "stdin"} {
		s := newGoScene(t, boxEnv)
		// A cat that is not there: status 127, nothing printed.
		s.stubs = append(s.stubs, "cat")
		s.files["cat.status"] = "127"
		args := []string{"box", "--script", "-"}
		if mode == "file" {
			s.prep = func(string) {
				require.NoError(t, os.WriteFile(filepath.Join(s.work(), "s.sh"), []byte("echo one\necho two\n\n"), 0o644))
			}
			args[2] = "s.sh"
		} else {
			s.stdin = "echo one\necho two\n\n"
		}
		r := s.runGo(plain, "remote_exec", nil, args...)
		require.Equal(t, 0, r.RC, r.Err)
		require.NotContains(t, "\n"+r.Log, "\ncat\n", mode)
		require.Contains(t, r.Log, "stdin: e c h o o n e \\n e c h o t w o \\n\n", mode)
	}
	s := newGoScene(t, boxEnv)
	r := s.runGo(plain, "remote_exec", nil, "box", "--script", "nope.sh")
	require.Equal(t, "Error: Script file not readable: nope.sh\n", r.Err)
}
