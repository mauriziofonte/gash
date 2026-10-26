// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// without removes the named tools from the fixture's PATH.
func (fx *fixture) without(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		require.NoError(t, os.Remove(filepath.Join(fx.bin, n)))
	}
}

// fakeNatives are fixed in-process answers.
func fakeNatives() *natives {
	return &natives{
		hostname: func() (string, error) { return "native-host", nil },
		uname:    func() (string, string, string, bool) { return "Linux", "6.9.0", "aarch64", true },
		numCPU:   func() int { return 8 },
		statfs:   func(string) (FS, bool) { return FS{Total: 200 << 30, Free: 50 << 30, Avail: 40 << 30}, true },
		addr:     func() string { return "192.0.2.7" },
	}
}

// TestOverviewNatives: without hostname, uname, getconf, nproc and df,
// and where hostname -I fails (macOS, BusyBox), the overview reads its
// fields in-process (3.4: 2.x left them blank and printed Bash's
// "uname: command not found"); with no natives they stay blank.
func TestOverviewNatives(t *testing.T) {
	fx := newFixture(t, etcTree, procTree)
	fx.without(t, "uname", "getconf", "df")
	fx.stub(t, "hostname", "exit 1")
	r := fx.call(t, nil, "overview", "--llm")
	require.Equal(t, result{"=== OVERVIEW ===\nUptime=3d 4h\nLoad=0.50 0.40 0.30\nCPU=Intel(R) Xeon(R) CPU @ 2.20GHz\n" +
		"Memory KiB=used=12000000 total=16000000\nSwap KiB=used=0 total=2097152\n", "", 0}, r)

	fx.m.native = fakeNatives()
	r = fx.call(t, nil, "overview", "--llm")
	require.Equal(t, result{"=== OVERVIEW ===\nHost=native-host\nUptime=3d 4h\nLoad=0.50 0.40 0.30\nCPU=Intel(R) Xeon(R) CPU @ 2.20GHz x8\n" +
		"Memory KiB=used=12000000 total=16000000\nSwap KiB=used=0 total=2097152\nDisk / KiB=used=157286400 total=199229440\n", "", 0}, r)
	r = fx.call(t, nil, "overview")
	require.Empty(t, r.err)
	require.Contains(t, r.out, "native-host · Debian GNU/Linux 12 (bookworm)")
	require.Contains(t, r.out, "Linux 6.9.0 aarch64 · up 3d 4h")
	require.Contains(t, r.out, "Address  192.0.2.7")

	// The tools win where they answer.
	fx = newFixture(t, etcTree, procTree)
	fx.m.native = fakeNatives()
	require.Equal(t, result{overviewLLM, "", 0}, fx.call(t, nil, "overview", "--llm"))
}

// TestIdentityWithoutTools: without uname, hostname and uptime the
// identity section notes each (3.4: 2.x printed "bash: uname: command
// not found" on stderr and left the rest blank), or reads the kernel and
// the host name in-process.
func TestIdentityWithoutTools(t *testing.T) {
	fx := newFixture(t, etcTree)
	fx.without(t, "uname", "hostname", "uptime")
	r := fx.call(t, nil, "identity", "--llm")
	require.Equal(t, result{"=== IDENTITY ===\nOS=Debian GNU/Linux 12 (bookworm)\nOS ID=debian\nVersion=12\nDebian Version=12.5\n" +
		"Note=uname is not installed\nNote=hostname is not installed\nMachine-ID=0123456789abcdef\nNote=uptime is not installed\n" +
		"Virtualization=kvm\nCloud=Hetzner\n", "", 0}, r)

	fx.m.native = fakeNatives()
	r = fx.call(t, nil, "identity", "--llm")
	require.Equal(t, result{"=== IDENTITY ===\nOS=Debian GNU/Linux 12 (bookworm)\nOS ID=debian\nVersion=12\nDebian Version=12.5\n" +
		"Kernel=6.9.0\nArch=aarch64\nHostname=native-host\nMachine-ID=0123456789abcdef\nNote=uptime is not installed\n" +
		"Virtualization=kvm\nCloud=Hetzner\n", "", 0}, r)
}

// netstatLinux is a net-tools netstat -tlnp / -ulnp.
const netstatLinux = `case "$1" in
-tlnp) printf 'Active Internet connections (only servers)\nProto Recv-Q Send-Q Local Address Foreign Address State PID/Program name\ntcp 0 0 0.0.0.0:22 0.0.0.0:* LISTEN 1/sshd\n' ;;
-ulnp) printf 'Active Internet connections (only servers)\nProto Recv-Q Send-Q Local Address Foreign Address State PID/Program name\nudp 0 0 0.0.0.0:68 0.0.0.0:* 2/dhclient\n' ;;
*) exit 1 ;;
esac`

// netstatBSD is the netstat -an -p tcp|udp of macOS.
const netstatBSD = `case "$*" in
"-an -p tcp") printf 'Active Internet connections (including servers)\nProto Recv-Q Send-Q  Local Address          Foreign Address        (state)    \ntcp4       0      0  *.22                   *.*                    LISTEN     \ntcp4       0      0  10.0.0.5.22            10.0.0.9.5000          ESTABLISHED\n' ;;
"-an -p udp") printf 'Active Internet connections (including servers)\nProto Recv-Q Send-Q  Local Address          Foreign Address        (state)    \nudp4       0      0  *.5353                 *.*                               \nudp4       0      0  10.0.0.5.123           17.253.1.1.123                    \n' ;;
*) exit 1 ;;
esac`

// TestListeningNetstat: without ss the listening ports come from netstat,
// net-tools on Linux or the BSD one of macOS (3.4: 2.x printed nothing),
// and without either the blocks say so.
func TestListeningNetstat(t *testing.T) {
	fx := newFixture(t)
	fx.without(t, "ss")
	fx.stub(t, "netstat", netstatLinux)
	require.Contains(t, fx.call(t, nil, "services", "--llm").out, "--- Listening ports\ntcp 0 0 0.0.0.0:22 0.0.0.0:* LISTEN 1/sshd\n")
	require.Contains(t, fx.deep(t, "network"), "--- Listening TCP\ntcp 0 0 0.0.0.0:22 0.0.0.0:* LISTEN 1/sshd\n"+
		"--- Listening UDP\nudp 0 0 0.0.0.0:68 0.0.0.0:* 2/dhclient\n")

	fx.m.goos = "darwin"
	fx.stub(t, "netstat", netstatBSD)
	require.Contains(t, fx.deep(t, "network"), "--- Listening TCP\ntcp4       0      0  *.22                   *.*                    LISTEN\n"+
		"--- Listening UDP\nudp4       0      0  *.5353                 *.*\n")
	require.Contains(t, fx.call(t, nil, "services", "--llm").out, "--- Listening ports\ntcp4       0      0  *.22                   *.*                    LISTEN\n")

	fx.without(t, "netstat")
	require.Contains(t, fx.call(t, nil, "services", "--llm").out, "--- Listening ports\nNote=neither ss nor netstat is installed\n")
	require.Contains(t, fx.call(t, nil, "services").out, "  Note:                    neither ss nor netstat is installed\n")
}

// TestDeepFallbacks: BSD ps refuses --sort and BSD df -T: ps aux is
// sorted here and df -h read (3.4: 2.x printed nothing); a tool missing
// under sudo is noted (fail2ban-client, ufw).
func TestDeepFallbacks(t *testing.T) {
	fx := deepFixture(t)
	fx.stub(t, "ps", `case "$2" in --sort*) echo "ps: illegal option -- -" >&2; exit 1 ;; esac
printf 'USER PID %%CPU %%MEM COMMAND\na 1 0.5 1.0 one\nb 2 9.0 0.1 two\nc 3 2.0 30.5 three\n'`)
	fx.stub(t, "df", `[ "$1" = -h ] && printf 'Filesystem Size Used Avail Capacity Mounted on\n/dev/disk1s1 100G 50G 50G 50%% /\n' || exit 1`)
	fx.without(t, "fail2ban-client", "ufw")
	perf := fx.deep(t, "performance")
	require.Contains(t, perf, "--- Top by memory\nUSER PID %CPU %MEM COMMAND\nc 3 2.0 30.5 three\na 1 0.5 1.0 one\nb 2 9.0 0.1 two\n")
	require.Contains(t, perf, "--- Top by CPU\nUSER PID %CPU %MEM COMMAND\nb 2 9.0 0.1 two\nc 3 2.0 30.5 three\na 1 0.5 1.0 one\n")
	require.Contains(t, fx.deep(t, "storage"), "--- Disk usage\nFilesystem Size Used Avail Capacity Mounted on\n/dev/disk1s1 100G 50G 50G 50% /\n--- Block devices\n")
	sec := fx.deep(t, "security")
	require.Contains(t, sec, "--- UFW\nNote=ufw is not installed\n")
	require.Contains(t, sec, "--- fail2ban\nNote=fail2ban-client is not installed\n")
}

// TestDeepSudoNote: a drill-down that reads as root says under its header
// why it could not (3.4); the others say nothing.
func TestDeepSudoNote(t *testing.T) {
	fx := deepFixture(t)
	fx.uid = 1000
	for _, n := range DeepSections {
		got := fx.deep(t, n)
		head, _, _ := strings.Cut(got, "\n")
		note := "Note=sudo credentials are not cached (run sudo -v first): files only root can read are left out\n"
		if deepNeedsSudo[n] {
			require.True(t, strings.HasPrefix(got, head+"\n"+note), n)
			require.Equal(t, 1, strings.Count(got, note), n)
		} else {
			require.NotContains(t, got, "Note=sudo", n)
		}
	}
}

// TestNoteAfterHeader: the note goes after the first line, however the
// writes split it, and an error stops the write.
func TestNoteAfterHeader(t *testing.T) {
	var b bytes.Buffer
	w := &noteAfterHeader{w: &b, note: "N\n"}
	for _, p := range []string{"=== X", " ===\n--- a\n", "x\n"} {
		n, err := w.Write([]byte(p))
		require.NoError(t, err)
		require.Equal(t, len(p), n)
	}
	require.Equal(t, "=== X ===\nN\n--- a\nx\n", b.String())
	_, err := (&noteAfterHeader{w: failWriter{}, note: "N\n"}).Write([]byte("a\nb"))
	require.Error(t, err)
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

// TestCollectSudoNote: ai_sysinfo's dossier says once, under the first
// section that reads as root, why it could not.
func TestCollectSudoNote(t *testing.T) {
	fx := newFixture(t)
	fx.uid = 1000
	e := invoke.EnvMap(fx.env)
	var w bytes.Buffer
	c := &invoke.Call{Name: "ai_sysinfo", Env: e, Stdin: strings.NewReader(""), Stdout: &w, Stderr: &w, Term: term.Fixed(e, false, false, false)}
	s := fx.m.Begin(c, false, true)
	defer s.Release()
	for _, n := range []string{"overview", "identity", "storage", "network"} {
		s.Collect(&w, n)
	}
	require.Equal(t, 1, strings.Count(w.String(), "Note=sudo credentials are not cached"))
	require.Contains(t, w.String(), "=== IDENTITY ===\nNote=sudo credentials are not cached")
}

// The disk share is df's Capacity, used of used + available: the 10 GiB
// reserved for root (or outside a container's allowance) are not space a
// user can fill, so a full disk does not show 75% (3.4).
func TestOverviewDiskShare(t *testing.T) {
	fx := newFixture(t, etcTree, procTree)
	fx.stub(t, "df", `printf 'Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/vda1 104857600 52428800 10485760 84%% /\n'`)
	r := fx.call(t, nil, "overview", "--llm")
	require.Contains(t, r.out, "Disk / KiB=used=52428800 total=62914560\n")
}
