// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
)

// absentRunner is fakeRunner where the named programs are not installed.
type absentRunner struct {
	fakeRunner
	absent map[string]bool
}

func (a *absentRunner) Run(ctx context.Context, c runner.Cmd) (runner.Result, error) {
	if a.absent[c.Path] {
		return runner.Result{ExitCode: 127}, &runner.StartError{Name: c.Path, Status: 127, Err: errors.New("not found")}
	}
	return a.fakeRunner.Run(ctx, c)
}

func absent(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}

// A section whose program is not installed says so (3.4: it was empty,
// and a missing crontab read "(no user crontab)").
func TestQuestionContextMissingTools(t *testing.T) {
	defer prereq.SetSystem(&prereq.System{OS: platform.OS{GOOS: "linux", ID: "debian"},
		LookPath: func(string, []string) (string, bool) { return "", false }})()
	root := t.TempDir()
	for _, d := range []string{"var/tmp", "etc/postgresql/16/main", "etc/ssh"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, d), 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "etc/ssh/sshd_config"), []byte("Port 22\n"), 0o644))
	sr, err := platform.NewSysroot(root)
	require.NoError(t, err)
	m := New()
	m.sys = sr
	m.run = &absentRunner{absent: absent("ls", "ss", "netstat", "systemctl", "find", "crontab")}
	c, _, _ := newCall(map[string]string{}, nil, false)
	noSudo := func(string, ...string) []byte { return nil }

	got := m.collectQuestionContext(c, noSudo, "why does myapp.service fail, list /var/tmp, port 3306")
	require.Equal(t, "=== SYSTEMD UNIT: myapp.service ===\n"+
		"--- Unit file ---\n(systemctl is not installed)\n"+
		"--- Status ---\n(systemctl is not installed)\n"+
		"--- Journal (last 50 lines) ---\n(journalctl is not installed)\n\n"+
		"=== DIRECTORY: /var/tmp ===\n(ls is not installed)\n\n"+
		"=== PORT: 3306 ===\n(ss and netstat are not installed)\n\n", got)

	got = m.collectQuestionContext(c, noSudo, "cron and postgres and ssh")
	require.Contains(t, got, "=== SERVICE: SSH ===\n(systemctl is not installed)\n"+
		"--- /etc/ssh/sshd_config (key lines) ---\n(grep is not installed)\n")
	require.Contains(t, got, "=== SERVICE: PostgreSQL ===\n(systemctl is not installed)\n"+
		"--- pg_hba.conf ---\n(find is not installed)\n")
	require.Contains(t, got, "=== SERVICE: Cron ===\n(crontab is not installed)\n")
	require.NotContains(t, got, "no user crontab")
}

// Captured `netstat -tlnp` (Linux, net-tools) and `netstat -an -p tcp`
// (macOS 15) output.
const (
	netstatLinux = "Active Internet connections (only servers)\n" +
		"Proto Recv-Q Send-Q Local Address           Foreign Address         State       PID/Program name    \n" +
		"tcp        0      0 0.0.0.0:22              0.0.0.0:*               LISTEN      812/sshd: /usr/sbin \n" +
		"tcp        0      0 127.0.0.1:3306          0.0.0.0:*               LISTEN      901/mariadbd        \n" +
		"tcp        0      0 0.0.0.0:2222            0.0.0.0:*               LISTEN      77/dropbear         \n" +
		"tcp6       0      0 :::22                   :::*                    LISTEN      812/sshd: /usr/sbin \n"
	netstatMac = "Active Internet connections (including servers)\n" +
		"Proto Recv-Q Send-Q  Local Address          Foreign Address        (state)    \n" +
		"tcp4       0      0  127.0.0.1.8080         *.*                    LISTEN     \n" +
		"tcp6       0      0  *.18080                *.*                    LISTEN     \n" +
		"tcp4       0      0  192.168.1.5.8080       17.57.144.10.5223      ESTABLISHED\n" +
		"tcp46      0      0  *.8080                 *.*                    LISTEN     \n"
)

func TestNetstatListening(t *testing.T) {
	require.Equal(t, "tcp        0      0 0.0.0.0:22              0.0.0.0:*               LISTEN      812/sshd: /usr/sbin \n"+
		"tcp6       0      0 :::22                   :::*                    LISTEN      812/sshd: /usr/sbin \n",
		string(netstatListening([]byte(netstatLinux), "22")))
	require.Equal(t, "tcp4       0      0  127.0.0.1.8080         *.*                    LISTEN     \n"+
		"tcp46      0      0  *.8080                 *.*                    LISTEN     \n",
		string(netstatListening([]byte(netstatMac), "8080")))
}

// Without ss the port section comes from netstat, with the arguments of
// the system (3.4: it stayed empty, always on macOS).
func TestQuestionContextNetstatFallback(t *testing.T) {
	sr, err := platform.NewSysroot(t.TempDir())
	require.NoError(t, err)
	for name, want := range map[string]struct{ args, out, row string }{
		"linux":  {"-tlnp", netstatLinux, "tcp        0      0 127.0.0.1:3306          0.0.0.0:*               LISTEN      901/mariadbd"},
		"darwin": {"-an -p tcp", netstatMac, "tcp4       0      0  127.0.0.1.8080         *.*                    LISTEN"},
	} {
		old := goos
		goos = name
		r := &absentRunner{absent: absent("ss")}
		r.handle = func(prog string, args []string) ([]byte, int) {
			if prog == "netstat" {
				require.Equal(t, want.args, strings.Join(args, " "), name)
				return []byte(want.out), 0
			}
			return nil, 1
		}
		m := New()
		m.sys = sr
		m.run = r
		c, _, _ := newCall(map[string]string{}, nil, false)
		got := m.collectQuestionContext(c, func(string, ...string) []byte { return nil }, "what is on port 3306 and port 8080")
		goos = old
		require.Contains(t, got, want.row, name)
	}
}
