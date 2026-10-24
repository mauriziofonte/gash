// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

const dfTyped = "Filesystem Type 1024-blocks Used Available Capacity Mounted on\n" +
	"/dev/vda1 ext4 104857600 52428800 52428800 50% /\n" +
	"tmpfs tmpfs 1000 0 1000 0% /run\n" +
	"/dev/vdb xfs 5000 4600 400 92% /data dir\n" +
	"C:\\ drvfs 999999999 800000000 199999999 81% /mnt/c\n" +
	"/dev/vda1 ext4 104857600 52428800 52428800 50% /\n"

const dfBSD = "Filesystem 1024-blocks Used Available Capacity Mounted on\n" +
	"/dev/disk1s1 9 5 4 56% /\n" +
	"map auto_home 0 0 0 100% /System/Volumes/Data/home\n"

func df(flags, out string, status int) runner.Expect {
	return runner.Expect{Argv: []string{"df", flags}, Stdout: out, Status: status}
}

// The expectations are the output of 2.x disk_usage with the same df.
func TestDiskUsage(t *testing.T) {
	m, _, _ := tm(t, runner.NewFake(t, df("-PTk", dfTyped, 0)))
	r := call(m, "disk_usage", nil, nil, "")
	require.Equal(t, result{"\n  MOUNT      DEVICE     TYPE      SIZE     USED     FREE  USE\n" +
		"  /          /dev/vda1  ext4   100 GiB   50 GiB   50 GiB  ━━━━━─────  50%\n" +
		"  /data dir  /dev/vdb   xfs    4.9 MiB  4.5 MiB  400 KiB  ━━━━━━━━━─  92%\n" +
		"  /mnt/c     C:\\        drvfs  954 GiB  763 GiB  191 GiB  ━━━━━━━━──  81%\n\n", "", 0}, r)

	// BSD df: no -T, the /dev devices.
	m, _, _ = tm(t, runner.NewFake(t, df("-PTk", "", 1), df("-Pk", dfBSD, 0)))
	r = call(m, "disk_usage", nil, nil, "", "--no-color")
	require.Equal(t, result{"\n  MOUNT  DEVICE        TYPE     SIZE     USED     FREE  USE\n" +
		"  /      /dev/disk1s1  -     9.0 KiB  5.0 KiB  4.0 KiB  ━━━━━━────  56%\n\n", "", 0}, r)

	m, _, _ = tm(t, runner.NewFake(t, df("-PTk", "Filesystem Type 1024-blocks Used Available Capacity Mounted on\ntmpfs tmpfs 1 0 1 0% /run\n", 0)))
	r = call(m, "disk_usage", nil, nil, "")
	require.Equal(t, result{"", "Info: No disk-backed filesystems found (df lists only virtual ones).\n", 0}, r)

	m, _, _ = tm(t, runner.NewFake(t, df("-PTk", "", 1), df("-Pk", "", 1)))
	r = call(m, "disk_usage", nil, nil, "")
	require.Equal(t, result{"", "Error: disk_usage: df failed\n", 1}, r)

	m, _, _ = tm(t, runner.NewFake(t))
	r = call(m, "disk_usage", nil, nil, "", "--no-color", "-x")
	require.Equal(t, result{"", "Error: disk_usage: unknown option '-x'\n", 1}, r)
	r = call(m, "disk_usage", nil, nil, "", "-h")
	require.Equal(t, 0, r.rc)
	require.Contains(t, r.out, "disk_usage")
}

// On a color terminal the bar and the percentage take the fill's color:
// success below 75%, warning from 75%, error from 90% (functions_more_spec
// line 37, whose __gash_use_color override a binary cannot see).
func TestDiskUsageColors(t *testing.T) {
	m, _, _ := tm(t, runner.NewFake(t, df("-PTk", dfTyped, 0), df("-PTk", dfTyped, 0)))
	e := invoke.EnvMap(map[string]string{"LANG": "C.UTF-8"})
	r := call(m, "disk_usage", nil, term.Fixed(e, true, true, true), "")
	require.Equal(t, 0, r.rc)
	for pct, color := range map[string]string{"50%": term.SeqSuccess, "81%": term.SeqWarning, "92%": term.SeqError} {
		var line string
		for _, l := range strings.Split(r.out, "\n") {
			if strings.Contains(l, pct) {
				line = l
			}
		}
		require.Contains(t, line, color+"━", pct)
		require.Contains(t, line, color+pct, pct)
	}
	r = call(m, "disk_usage", nil, term.Fixed(e, true, true, true), "", "--no-color")
	require.NotContains(t, r.out, "\x1b[")
}

// 3.0: SYS-14 fixed: df's rows count whatever its exit status (GNU df
// exits 1 for one unreadable mount); -Pk is the fallback only when -PTk
// printed no data row, and the df failure is an output with none.
func TestLedger_SYS14(t *testing.T) {
	ext4 := "Filesystem Type 1024-blocks Used Available Capacity Mounted on\n" +
		"/dev/vda1 ext4 104857600 52428800 52428800 50% /\n"
	m, _, _ := tm(t, runner.NewFake(t, df("-PTk", ext4, 1)))
	r := call(m, "disk_usage", nil, nil, "")
	require.Equal(t, 0, r.rc)
	require.Empty(t, r.err)
	require.Contains(t, r.out, "/dev/vda1")

	header := "Filesystem Type 1024-blocks Used Available Capacity Mounted on\n"
	m, _, _ = tm(t, runner.NewFake(t, df("-PTk", header, 1), df("-Pk", dfBSD, 1)))
	r = call(m, "disk_usage", nil, nil, "")
	require.Equal(t, "\n  MOUNT  DEVICE        TYPE     SIZE     USED     FREE  USE\n"+
		"  /      /dev/disk1s1  -     9.0 KiB  5.0 KiB  4.0 KiB  ━━━━━━────  56%\n\n", r.out)

	m, _, _ = tm(t, runner.NewFake(t, df("-PTk", "", 1), df("-Pk", header, 1)))
	r = call(m, "disk_usage", nil, nil, "")
	require.Equal(t, result{"", "Error: disk_usage: df failed\n", 1}, r)
}

func TestAwkNumber(t *testing.T) {
	for in, want := range map[string]float64{"": 0, "12": 12, " 12x": 12, "-3.5": -3.5, "1e3": 1000, "1e": 1, ".5": 0.5, "x1": 0, "+.": 0} {
		require.Equal(t, want, awkNumber(in), in)
	}
}

// ipServer answers every request with status and body.
func ipServer(t *testing.T, m *Module, status int, body string) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	m.client = srv.Client()
	m.ipURL = srv.URL + "/ip"
}

func TestIPPublic(t *testing.T) {
	m, _, _ := tm(t, runner.NewFake(t))
	ipServer(t, m, 200, "203.0.113.7\n\n")
	r := call(m, "ip_public", nil, nil, "")
	require.Equal(t, result{"Public IP: 203.0.113.7\n", "", 0}, r)
	r = call(m, "ip_public", nil, ttyTerm(nil, "-"), "", "--no-color")
	require.Equal(t, result{"Public IP: 203.0.113.7\n", "", 0}, r)
	r = call(m, "ip_public", nil, ttyTerm(nil, "-"), "")
	require.Equal(t, "\x1b[1mPublic IP:\x1b[0m \x1b[38;5;214m203.0.113.7\x1b[0m\n", r.out)
	// Other words are ignored; -h anywhere is the help.
	r = call(m, "ip_public", nil, nil, "", "x", "-h")
	require.Equal(t, 0, r.rc)
	require.Contains(t, r.out, "ip_public")

	ipServer(t, m, 200, "")
	r = call(m, "ip_public", nil, nil, "")
	require.Equal(t, result{"", "Error: Failed to retrieve public IP address.\n", 1}, r)
}

// 3.0: SYS-13 fixed: only a body that parses as an address is printed
// (an HTML page is the retrieval error), the request is bounded by
// ipTimeout, and --raw prints the bare address.
func TestLedger_SYS13(t *testing.T) {
	m, _, _ := tm(t, runner.NewFake(t))
	fail := result{"", "Error: Failed to retrieve public IP address.\n", 1}
	ipServer(t, m, 200, "<html>Rate limit</html>\n")
	require.Equal(t, fail, call(m, "ip_public", nil, nil, ""))
	ipServer(t, m, 200, "203.0.113.7 extra\n")
	require.Equal(t, fail, call(m, "ip_public", nil, nil, ""))

	ipServer(t, m, 200, " 2001:db8::7\r\n")
	require.Equal(t, result{"2001:db8::7\n", "", 0}, call(m, "ip_public", nil, ttyTerm(nil, "-"), "", "--raw"))
	require.Equal(t, result{"Public IP: 2001:db8::7\n", "", 0}, call(m, "ip_public", nil, nil, ""))

	release := make(chan struct{})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		_, _ = w.Write([]byte("203.0.113.9"))
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	m.client, m.ipURL, m.ipTimeout = srv.Client(), srv.URL+"/ip", 50*time.Millisecond
	start := time.Now()
	require.Equal(t, fail, call(m, "ip_public", nil, nil, "", "--raw"))
	require.Less(t, time.Since(start), 5*time.Second)
	require.Equal(t, 10*time.Second, New().ipTimeout)
}

// SYS-36: net/http, whatever PATH holds; an error status is the wget
// failure.
func TestLedger_SYS36(t *testing.T) {
	m, _, _ := tm(t, runner.NewFake(t))
	ipServer(t, m, 200, "203.0.113.7")
	r := call(m, "ip_public", map[string]string{"PATH": ""}, nil, "")
	require.Equal(t, result{"Public IP: 203.0.113.7\n", "", 0}, r)
	ipServer(t, m, 503, "203.0.113.8")
	r = call(m, "ip_public", nil, nil, "")
	require.Equal(t, result{"", "Error: Failed to retrieve public IP address.\n", 1}, r)
}

// svcEnv is a PATH with the given managers and the systemd run directory
// (absent when rundir is false).
func svcEnv(t *testing.T, rundir bool, names ...string) map[string]string {
	d := t.TempDir()
	if rundir {
		require.NoError(t, os.Mkdir(filepath.Join(d, "run"), 0o700))
	}
	return map[string]string{"PATH": binDir(t, names...), "__GASH_SYSTEMD_RUNDIR": filepath.Join(d, "run")}
}

// isActive expects the systemctl probe of every known service, active
// ones succeeding.
func isActive(active ...string) []runner.Expect {
	var e []runner.Expect
	for _, s := range knownServices {
		st := 3
		for _, a := range active {
			if a == s {
				st = 0
			}
		}
		e = append(e, runner.Expect{Argv: []string{"systemctl", "is-active", "--quiet", s}, Status: st})
	}
	return e
}

// The expectations are the output of 2.x services_stop with the same
// probes.
func TestServicesStop(t *testing.T) {
	f := runner.NewFake(t, isActive("nginx", "docker")...)
	f.Expect(runner.Expect{Argv: []string{"sudo", "systemctl", "stop", "nginx"}, Stderr: "sudo systemctl stop nginx\n"},
		runner.Expect{Argv: []string{"sudo", "systemctl", "stop", "docker"}, Stderr: "sudo systemctl stop docker\n", Status: 5})
	m, _, _ := tm(t, f)
	r := call(m, "services_stop", svcEnv(t, true, "systemctl", "service"), ctty(), "", "-y")
	require.Equal(t, result{"", "Info: Running: nginx docker\nInfo: Stopping nginx...\nsudo systemctl stop nginx\n" +
		"Info: Stopping docker...\nsudo systemctl stop docker\n", 1}, r)

	// No systemd run directory: the SysV wrapper.
	f = runner.NewFake(t)
	for _, s := range knownServices {
		st := 1
		if s == "mysql" {
			st = 0
		}
		f.Expect(runner.Expect{Argv: []string{"service", s, "status"}, Stdout: "noise\n", Status: st})
	}
	f.Expect(runner.Expect{Argv: []string{"sudo", "service", "mysql", "stop"}})
	m, _, _ = tm(t, f)
	r = call(m, "services_stop", svcEnv(t, false, "systemctl", "service"), ctty(), "", "--yes")
	require.Equal(t, result{"", "Info: Running: mysql\nInfo: Stopping mysql...\n", 0}, r)

	m, _, _ = tm(t, runner.NewFake(t, isActive("nginx")...))
	r = call(m, "services_stop", svcEnv(t, true, "systemctl"), nil, "", "--dry-run")
	require.Equal(t, result{"", "Info: Running: nginx\n", 0}, r)

	m, _, _ = tm(t, runner.NewFake(t, isActive("nginx")...))
	env := svcEnv(t, true, "systemctl")
	env["GASH_HEADLESS"] = "1"
	r = call(m, "services_stop", env, nil, "")
	require.Equal(t, result{"", "Info: Running: nginx\nInfo: Aborted.\n", 1}, r)

	m, _, _ = tm(t, runner.NewFake(t, isActive()...))
	r = call(m, "services_stop", svcEnv(t, true, "systemctl"), nil, "")
	require.Equal(t, result{"", "Info: None of the known services is running.\n", 0}, r)

	m, _, _ = tm(t, runner.NewFake(t))
	r = call(m, "services_stop", svcEnv(t, true), nil, "")
	require.Equal(t, result{"", "Error: No service manager found (systemd or service).\n", 1}, r)
	r = call(m, "services_stop", nil, nil, "", "-x")
	require.Equal(t, result{"", "Error: Unknown option: -x (usage: services_stop [-y] [--dry-run])\n", 1}, r)
}

// 3.0: SYS-25 fixed: an unattended -y (headless, or no controlling
// terminal) checks `sudo -n true` first and stops with sudo -n; a sudo
// that wants a password is one clear error and nothing is stopped.
func TestLedger_SYS25(t *testing.T) {
	f := runner.NewFake(t, isActive("redis")...)
	f.Expect(runner.Expect{Argv: []string{"sudo", "-n", "true"}},
		runner.Expect{Argv: []string{"sudo", "-n", "systemctl", "stop", "redis"}, Stdin: runner.Text("typed")})
	m, _, _ := tm(t, f)
	env := svcEnv(t, true, "systemctl")
	env["GASH_HEADLESS"] = "1"
	r := call(m, "services_stop", env, nil, "typed", "-y")
	require.Equal(t, 0, r.rc)

	f = runner.NewFake(t, isActive("redis")...)
	f.Expect(runner.Expect{Argv: []string{"sudo", "-n", "true"}, Stderr: "sudo: a password is required\n", Status: 1})
	m, _, _ = tm(t, f)
	r = call(m, "services_stop", env, nil, "", "-y")
	require.Equal(t, result{"", "Info: Running: redis\nError: sudo needs a password: run interactively or configure NOPASSWD.\n", 1}, r)

	// No controlling terminal, not headless: the same check.
	f = runner.NewFake(t, isActive("redis")...)
	f.Expect(runner.Expect{Argv: []string{"sudo", "-n", "true"}, Status: 1})
	m, _, _ = tm(t, f)
	delete(env, "GASH_HEADLESS")
	r = call(m, "services_stop", env, nil, "", "-y")
	require.Equal(t, 1, r.rc)

	// A sudo that does not start is the shell's message.
	f = runner.NewFake(t, isActive("redis")...)
	f.Expect(runner.Expect{Argv: []string{"sudo", "-n", "true"}, Err: &runner.StartError{Name: "sudo", Status: 127, Err: errors.New("not found")}},
		runner.Expect{Argv: []string{"sudo", "-n", "systemctl", "stop", "redis"}, Err: &runner.StartError{Name: "sudo", Status: 127, Err: errors.New("not found")}})
	m, _, _ = tm(t, f)
	env["GASH_HEADLESS"] = "1"
	r = call(m, "services_stop", env, nil, "", "-y")
	require.Equal(t, result{"", "Info: Running: redis\nInfo: Stopping redis...\nbash: sudo: command not found\n", 1}, r)
}

func TestSudoLast(t *testing.T) {
	// With a command: sudo runs it on the caller's terminal.
	m, _, _ := tm(t, runner.NewFake(t, runner.Expect{Argv: []string{"sudo", "apt", "update"}, Status: 3}))
	r := call(m, "sudo_last", nil, nil, "", "apt", "update")
	require.Equal(t, result{"", "", 3}, r)

	// The stub's call: the recalled command on stdin, Go on the hand-off.
	m, _, h := tm(t, runner.NewFake(t))
	env := map[string]string{"GASH__HANDOFF": "3"}
	r = call(m, "sudo_last", env, ttyTerm(nil, "y\n"), "ls /root")
	require.Equal(t, 0, r.rc)
	require.Equal(t, "\x1b[38;5;220m! Warning:\x1b[0m About to run as root: ls /root\nProceed? \x1b[1m[y/N]\x1b[0m ", r.err)
	require.Equal(t, "1", h.String())
	require.True(t, h.closed)

	// A direct run has no shell to run the command in.
	m, _, h = tm(t, runner.NewFake(t))
	r = call(m, "sudo_last", nil, ttyTerm(nil, "y\n"), "ls /root")
	require.Equal(t, 1, r.rc)
	require.Contains(t, r.err, "sudo_last runs the previous command of the calling shell: use it from a Gash shell.\n")
	require.Empty(t, h.String())

	r = call(m, "sudo_last", nil, nil, "", "--help")
	require.Equal(t, 0, r.rc)
	require.Contains(t, r.out, "sudo_last")
}

// SYS-12: no recalled command is an error before any warning, and
// declining (or headless) returns 1 (3.0: SYS-12 fixed; 2.x returned 0 and
// asked even with no command).
func TestLedger_SYS12(t *testing.T) {
	m, _, h := tm(t, runner.NewFake(t))
	env := map[string]string{"GASH__HANDOFF": "3", "GASH_HEADLESS": "1"}
	r := call(m, "sudo_last", env, nil, "")
	require.Equal(t, result{"", "Error: No previous command in history.\n", 1}, r)
	require.Empty(t, h.String())
	require.True(t, h.closed)

	r = call(m, "sudo_last", env, nil, "make install")
	require.Equal(t, result{"", "Warning: About to run as root: make install\nInfo: Aborted.\n", 1}, r)
	r = call(m, "sudo_last", map[string]string{"GASH__HANDOFF": "3"}, ttyTerm(nil, "n\n"), "make install")
	require.Equal(t, 1, r.rc)
	require.Contains(t, r.err, "Aborted.")
	require.Empty(t, h.String())
}

func TestMkdirCD(t *testing.T) {
	m, _, _ := tm(t, runner.NewFake(t,
		runner.Expect{Argv: []string{"mkdir", "-p", "a b"}},
		runner.Expect{Argv: []string{"mkdir", "-p", "-p"}, Stderr: "mkdir: missing operand\n", Status: 1},
	))
	r := call(m, "mkdir_cd", nil, nil, "", "a b")
	require.Equal(t, result{"", "", 0}, r)
	r = call(m, "mkdir_cd", nil, nil, "", "-p")
	require.Equal(t, result{"", "mkdir: missing operand\n", 1}, r)
	r = call(m, "mkdir_cd", nil, nil, "")
	require.Equal(t, result{"", "Error: Missing directory. Usage: mkdir_cd <directory>\n", 1}, r)
}

// 3.0: SYS-49 fixed: every flag ip_public and disk_usage accept is in the
// USAGE or OPTIONS of the registered help, and in the fallback usage.
func TestLedger_SYS49(t *testing.T) {
	for name, flags := range map[string][]string{
		"ip_public":  {"--no-color", "--raw"},
		"disk_usage": {"--no-color"},
	} {
		page := registeredHelp(t, name)
		usage := page[strings.Index(page, "USAGE"):]
		if i := strings.Index(usage, "EXAMPLES"); i >= 0 {
			usage = usage[:i]
		}
		f, _ := registry.Lookup(name)
		for _, fl := range flags {
			require.Contains(t, usage, fl, name)
			require.Contains(t, f.Fallback.Usage, fl, name)
		}
	}
}
