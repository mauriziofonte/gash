// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/term"
)

const hostsEnv = "HOST:web=deploy@web.example.com\nHOST:db=admin@10.0.0.5\nHOST:a-very-long-alias-name=x@y\n"

func TestHostsMatches2x(t *testing.T) {
	for name, env := range map[string]string{
		"none":          "",
		"listed":        hostsEnv,
		"other entries": "GASH_BANNER=0\nDB:x=mysql://u:p@h/d\n",
		"bad lines":     "HOST:bad=nouser\nHOST:ok=u@h\nnonsense\n",
		"repeated":      "HOST:a=u@h\nHOST:a=v@h\n",
	} {
		t.Run(name, func(t *testing.T) {
			for _, m := range []mode{plain, color, headless} {
				newScene(t, env).same(m, "remote_hosts")
			}
			newScene(t, env).same(plain, "remote_hosts", "extra", "words")
		})
	}
	t.Run("GASH_ENV_FILE", func(t *testing.T) {
		s := newScene(t, "")
		f := filepath.Join(s.base, "elsewhere")
		require.NoError(t, os.WriteFile(f, []byte("HOST:z=u@z\n"), 0o600))
		s.env = []string{"GASH_ENV_FILE=" + f}
		r := s.same(plain, "remote_hosts")
		require.Contains(t, r.Out, "u@z")
	})
	t.Run("help", func(t *testing.T) {
		newScene(t, "").same(plain, "remote_hosts", "-h")
	})
}

// 3.0: REM-02 and REM-07 fixed: ssh gets -- before the target and ~/.ssh
// is created 0700 (TestLedger_REM02, TestLedger_REM07); this 2.x
// comparison skips without the 2.x sources.
func TestExecMatches2x(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		m     mode
		files map[string]string
		stdin string
		rc    int
	}{
		{"no target", nil, plain, nil, "", 1},
		{"empty target", []string{""}, plain, nil, "", 1},
		{"unknown alias", []string{"nohost", "uptime"}, plain, nil, "", 1},
		{"unknown alias headless", []string{"nohost", "uptime"}, headless, nil, "", 1},
		{"raw target headless", []string{"u@h", "uptime"}, headless, nil, "", 1},
		{"no command", []string{"web"}, plain, nil, "", 1},
		{"empty command", []string{"web", ""}, plain, nil, "", 1},
		{"extra args", []string{"web", "ls", "-la"}, plain, nil, "", 1},
		{"ran", []string{"web", "uptime"}, plain, map[string]string{"ssh.out": "hello from the remote host\n"}, "", 0},
		{"ran headless", []string{"db", "uptime -p"}, headless, nil, "", 0},
		{"raw target", []string{"someone@example.com", "id"}, plain, nil, "", 0},
		{"stdin passes", []string{"web", "cat"}, plain, nil, "piped\n", 0},
		{"remote status", []string{"web", "false"}, plain, map[string]string{"ssh.status": "3"}, "", 3},
		{"255 healthy", []string{"web", "kill"}, plain, map[string]string{"ssh.status": "255 0"}, "", 255},
		{"255 unreachable", []string{"web", "uptime"}, plain, map[string]string{"ssh.status": "255 255", "ssh.err": "ssh: connect\n"}, "", 1},
		{"help", []string{"-h"}, plain, nil, "", 0},
		{"a target named --help is help", []string{"--help", "x"}, plain, nil, "", 0},
		{"color", []string{"web", "uptime"}, color, map[string]string{"ssh.status": "255 255"}, "", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newScene(t, hostsEnv)
			s.files, s.stdin = tc.files, tc.stdin
			if s.files == nil {
				s.files = map[string]string{}
			}
			r := s.same(tc.m, "remote_exec", tc.args...)
			require.Equal(t, tc.rc, r.RC)
		})
	}
}

func TestRemoteExecScriptMode(t *testing.T) {
	script := "echo one\n\n  echo two\n\n\n"
	cases := []struct {
		name  string
		file  string // content of work/s.sh ("" for none)
		args  []string
		stdin string
		rc    int
	}{
		{"missing file name", "", []string{"web", "--script"}, "", 1},
		{"missing file", "", []string{"web", "--script", "nope.sh"}, "", 1},
		{"a directory", "", []string{"web", "--script", "."}, "", 1},
		{"empty", " \n\t\n", []string{"web", "--script", "s.sh"}, "", 1},
		{"file", script, []string{"web", "--script", "s.sh"}, "", 0},
		{"stdin", "", []string{"web", "--script", "-"}, script, 0},
		{"empty stdin", "", []string{"web", "--script", "-"}, "", 1},
		{"NUL bytes dropped", "echo a\x00b\n", []string{"web", "--script", "s.sh"}, "", 0},
		{"unknown alias first", "", []string{"nohost", "--script", "nope.sh"}, "", 1},
		{"--script not first is a command", "", []string{"web", "x", "--script"}, "", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newScene(t, hostsEnv)
			s.stdin = tc.stdin
			if tc.file != "" {
				s.prep = func(string) {
					require.NoError(t, os.WriteFile(filepath.Join(s.work(), "s.sh"), []byte(tc.file), 0o644))
				}
			}
			r := s.same(plain, "remote_exec", tc.args...)
			require.Equal(t, tc.rc, r.RC)
		})
	}
}

// Script arguments: after an optional --, each one quoted POSIX-style
// (LEDGER REM-04), so the remote command string is not the 2.x one.
func TestRemoteExecScriptArgs(t *testing.T) {
	for _, args := range [][]string{
		{"--", "a b", "~", "", "$(id)", "it's"},
		{"plain", "--", "x"},
		{"--"},
	} {
		s := newScene(t, hostsEnv)
		s.prep = func(string) {
			require.NoError(t, os.WriteFile(filepath.Join(s.work(), "s.sh"), []byte("echo hi\n"), 0o644))
		}
		call := append([]string{"web", "--script", "s.sh"}, args...)
		want := s.run2x(plain, "remote_exec", call...).noDiag(s.root)
		got := s.runGo(plain, "remote_exec", nil, call...)
		require.Equal(t, want.Out, got.Out)
		require.Equal(t, want.Err, got.Err)
		require.Equal(t, want.RC, got.RC)
		rest := args
		if len(rest) > 0 && rest[0] == "--" {
			rest = rest[1:]
		}
		require.Contains(t, got.Log, "  [unset HISTFILE HISTSIZE; bash -s --"+scriptArgs(rest)+"]\n")
		// What a POSIX shell parses out of the Go string is what 2.x's
		// bash parsed out of its own.
		require.Equal(t, parseArgs(t, "bash", remoteCommand(want.Log)), parseArgs(t, "sh", remoteCommand(got.Log)))
		if len(rest) == 0 {
			require.Empty(t, parseArgs(t, "sh", remoteCommand(got.Log)))
		} else {
			require.Equal(t, rest, parseArgs(t, "sh", remoteCommand(got.Log)))
		}
	}
}

// 3.0: REM-01 fixed: the "unknown ignored" case is now refused
// (TestLedger_REM01); this 2.x comparison skips without the 2.x sources.
func TestRemoteScpFlags(t *testing.T) {
	cases := []struct {
		name string
		fn   string
		args []string
		rc   int
	}{
		{"push", "remote_push", []string{"web", "f", "/tmp/f"}, 0},
		{"push all flags", "remote_push", []string{"web", "d", "/tmp/d", "-r", "-p", "--no-progress"}, 0},
		{"push long flags", "remote_push", []string{"web", "d", "/tmp/d", "--recursive", "--preserve"}, 0},
		{"push flags in any order, unknown ignored", "remote_push", []string{"web", "d", "/tmp/d", "--dry-run", "-p", "x", "-r"}, 0},
		{"pull", "remote_pull", []string{"web", "/tmp/f", "./f"}, 0},
		{"pull all flags", "remote_pull", []string{"db", "/tmp/d", "d", "-r", "-p", "--no-progress", "--force"}, 0},
		{"push missing destination", "remote_push", []string{"web", "f"}, 1},
		{"push missing source", "remote_push", []string{"web"}, 1},
		{"pull missing args", "remote_pull", []string{"web"}, 1},
		{"pull no args", "remote_pull", nil, 1},
		{"push unknown alias", "remote_push", []string{"nohost", "f", "/tmp/f"}, 1},
		{"push denied", "remote_push", []string{"web", "f", "/etc/shadow"}, 1},
		{"push denied with --force", "remote_push", []string{"web", "f", "/etc/shadow", "--force"}, 1},
		{"push ask without a terminal", "remote_push", []string{"web", "f", "/etc/app.conf"}, 1},
		{"push ask --force", "remote_push", []string{"web", "f", "/etc/app.conf", "--force"}, 0},
		{"pull denied", "remote_pull", []string{"web", "/etc/shadow", "./s"}, 1},
		{"pull ask without a terminal", "remote_pull", []string{"web", "/root/x", "./x"}, 1},
		{"push scp status", "remote_push", []string{"web", "f", "/tmp/f"}, 0},
		{"push help", "remote_push", []string{"--help"}, 0},
		{"pull help", "remote_pull", []string{"-h"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newScene(t, hostsEnv).same(plain, tc.fn, tc.args...)
			require.Equal(t, tc.rc, r.RC)
		})
	}
	t.Run("headless ask refused", func(t *testing.T) {
		r := newScene(t, hostsEnv).same(headless, "remote_push", "web", "f", "/srv/git/.ssh/authorized_keys")
		require.Equal(t, 1, r.RC)
		require.Contains(t, r.Err, "(matched: .ssh). push refused in headless mode without --force.")
		r = newScene(t, hostsEnv).same(headless, "remote_pull", "web", "/var/log/x", ".", "--force")
		require.Equal(t, 0, r.RC)
	})
	t.Run("scp status", func(t *testing.T) {
		s := newScene(t, hostsEnv)
		s.files = map[string]string{"scp.status": "4", "scp.err": "scp: failed\n"}
		require.Equal(t, 4, s.same(plain, "remote_push", "web", "f", "/tmp/f").RC)
	})
	t.Run("no scp", func(t *testing.T) {
		s := newScene(t, hostsEnv)
		s.stubs = []string{"ssh", "rsync"}
		s.env = []string{"PATH=" + s.stubD()}
		r := s.same(plain, "remote_push", "web", "f", "/tmp/f")
		require.Equal(t, "Error: scp is not installed.\n", r.Err)
	})
	t.Run("color", func(t *testing.T) {
		newScene(t, hostsEnv).same(color, "remote_push", "web", "f", "/etc/shadow")
	})
}

// TestRemotePullForce: --force passes the ASK tier of the remote source
// without a question; DENY stays refused.
func TestRemotePullForce(t *testing.T) {
	r := newScene(t, hostsEnv).same(headless, "remote_pull", "web", "/etc/app.conf", "./a", "--force")
	require.Equal(t, 0, r.RC)
	require.Contains(t, r.Log, "[deploy@web.example.com:/etc/app.conf]")
	r = newScene(t, hostsEnv).same(headless, "remote_pull", "web", "/proc/1/", "./d", "--force")
	require.Equal(t, 1, r.RC)
	require.Empty(t, r.Log)
}

// 3.0: REM-01, REM-03 and REM-05 fixed: --exclude PAT is two words, rsync
// gets -s and a quoted ControlPath (TestLedger_REM01, TestLedger_REM03,
// TestLedger_REM05); this 2.x comparison skips without the 2.x sources.
func TestRemoteRsyncFlags(t *testing.T) {
	cases := []struct {
		name string
		fn   string
		args []string
		m    mode
		rc   int
	}{
		{"push", "remote_sync_push", []string{"web", "d/", "/tmp/d/"}, plain, 0},
		{"push no progress", "remote_sync_push", []string{"web", "d/", "/tmp/d/", "--no-progress"}, plain, 0},
		{"push all flags", "remote_sync_push", []string{"web", "d/", "/tmp/d/", "--delete", "--dry-run", "--exclude=.env", "--exclude=*.log", "--no-progress"}, plain, 0},
		{"push --exclude PAT ignored", "remote_sync_push", []string{"web", "d/", "/tmp/d/", "--exclude", ".env"}, plain, 0},
		{"push delete in home subdir", "remote_sync_push", []string{"web", "d/", "~/project", "--delete"}, headless, 0},
		{"push delete home refused headless", "remote_sync_push", []string{"web", "d/", "~", "--delete"}, headless, 1},
		{"push delete home --force", "remote_sync_push", []string{"web", "d/", "~", "--delete", "--force"}, headless, 0},
		{"push delete without a terminal", "remote_sync_push", []string{"web", "d/", "/srv/d/", "--delete"}, plain, 1},
		{"push ask headless", "remote_sync_push", []string{"web", "d", "/etc/systemd/system/myapp"}, headless, 1},
		{"push denied", "remote_sync_push", []string{"web", "d", "/usr/lib/x", "--force"}, plain, 1},
		{"push missing args", "remote_sync_push", []string{"web", "d"}, plain, 1},
		{"pull", "remote_sync_pull", []string{"web", "/tmp/d/", "./d/"}, plain, 0},
		{"pull all flags", "remote_sync_pull", []string{"web", "/tmp/d/", "d", "--delete", "--dry-run", "--exclude=x", "--no-progress"}, plain, 0},
		{"pull denied", "remote_sync_pull", []string{"web", "/proc/1/", "./d/"}, plain, 1},
		{"pull ask --force", "remote_sync_pull", []string{"web", "/var/log/", "./d/", "--force"}, headless, 0},
		{"pull missing args", "remote_sync_pull", []string{"web"}, plain, 1},
		{"pull help", "remote_sync_pull", []string{"-h"}, plain, 0},
		{"push help", "remote_sync_push", []string{"--help"}, plain, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newScene(t, hostsEnv).same(tc.m, tc.fn, tc.args...)
			require.Equal(t, tc.rc, r.RC)
		})
	}
	t.Run("no rsync", func(t *testing.T) {
		s := newScene(t, hostsEnv)
		s.stubs = []string{"ssh", "scp"}
		s.env = []string{"PATH=" + s.stubD()}
		require.Equal(t, "Error: rsync is not installed.\n", s.same(plain, "remote_sync_push", "web", "d", "/tmp/d").Err)
	})
	t.Run("rsync status and output", func(t *testing.T) {
		s := newScene(t, hostsEnv)
		s.files = map[string]string{"rsync.status": "23", "rsync.out": "sending incremental file list\n"}
		require.Equal(t, 23, s.same(plain, "remote_sync_pull", "web", "/tmp/d", "d").RC)
	})
}

// 3.0: REM-06 fixed: the local check now speaks of the local destination
// (TestLedger_REM06); this 2.x comparison skips without the 2.x sources.
// TestRemoteSyncPullDeleteLocal: remote_sync_pull --delete checks the
// LOCAL destination with the remote wording (LEDGER REM-06).
func TestRemoteSyncPullDeleteLocal(t *testing.T) {
	r := newScene(t, hostsEnv).same(headless, "remote_sync_pull", "web", "/srv", "./", "--delete")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Err, "rsync --delete targeting './' is outside the safe set")
	require.Empty(t, r.Log)
	r = newScene(t, hostsEnv).same(headless, "remote_sync_pull", "web", "/srv", "./sub", "--delete")
	require.Equal(t, 0, r.RC)
}

// The questions read the controlling terminal: y or Y goes ahead,
// anything else (or none) cancels.
func TestRemoteAsks(t *testing.T) {
	s := newScene(t, hostsEnv)
	declined := s.run2x(plain, "remote_push", "web", "f", "/etc/app.conf").noDiag(s.root)
	require.Equal(t, 1, declined.RC)
	for _, in := range []string{"n\n", "yes\n", "", "\n"} {
		got := s.runGo(plain, "remote_push", func(tt *term.Term) *term.Term { return tt.WithTTY(in) }, "web", "f", "/etc/app.conf")
		require.Equal(t, declined, got, "%q", in)
	}
	for _, in := range []string{"y\n", "Y\n", " y \n"} {
		got := s.runGo(plain, "remote_push", func(tt *term.Term) *term.Term { return tt.WithTTY(in) }, "web", "f", "/etc/app.conf")
		require.Equal(t, 0, got.RC, "%q", in)
		require.Equal(t, "Remote path '/etc/app.conf' is sensitive (matched: /etc). Proceed with push? [y/N] ", got.Err)
		require.Contains(t, got.Log, "[deploy@web.example.com:/etc/app.conf]")
	}

	declined = s.run2x(plain, "remote_sync_push", "web", "d/", "/srv/d/", "--delete").noDiag(s.root)
	require.Contains(t, declined.Err, "Info: Operation cancelled.")
	got := s.runGo(plain, "remote_sync_push", func(tt *term.Term) *term.Term { return tt.WithTTY("n\n") }, "web", "d/", "/srv/d/", "--delete")
	require.Equal(t, declined, got)
	got = s.runGo(plain, "remote_sync_push", func(tt *term.Term) *term.Term { return tt.WithTTY("y\n") }, "web", "d/", "/srv/d/", "--delete")
	require.Equal(t, 0, got.RC)
	require.Contains(t, got.Log, "  [--delete]\n")

	// Both questions, the tier one first (the test terminal types y to
	// each).
	got = s.runGo(plain, "remote_sync_push", func(tt *term.Term) *term.Term { return tt.WithTTY("y\n") }, "web", "d/", "/etc/x/", "--delete")
	require.Equal(t, 0, got.RC)
	require.Equal(t, "Remote path '/etc/x/' is sensitive (matched: /etc). Proceed with sync-push? [y/N] "+
		"rsync --delete targeting '/etc/x/' can remove files on the remote. Proceed? [y/N] ", got.Err)
}

// remoteCommand is the last argument of the first ssh call in a stub log.
func remoteCommand(log string) string {
	var last string
	for _, l := range strings.Split(log, "\n") {
		if l == "ssh" && last != "" {
			break
		}
		if strings.HasPrefix(l, "  [") && strings.HasSuffix(l, "]") {
			last = l[3 : len(l)-1]
		}
	}
	return last
}

// parseArgs is what SHELL hands `bash -s --` in the remote command cmd:
// the words after "bash -s --", as the shell parses them.
func parseArgs(t *testing.T, shell, cmd string) []string {
	t.Helper()
	_, rest, ok := strings.Cut(cmd, "bash -s --")
	require.True(t, ok, cmd)
	out, err := exec.Command(shell, "-c", "set --"+rest+"\nfor a; do printf '[%s]' \"$a\"; done").Output()
	require.NoError(t, err)
	var args []string
	for _, a := range strings.Split(string(out), "][") {
		args = append(args, a)
	}
	if string(out) == "" {
		return nil
	}
	args[0] = strings.TrimPrefix(args[0], "[")
	args[len(args)-1] = strings.TrimSuffix(args[len(args)-1], "]")
	return args
}
