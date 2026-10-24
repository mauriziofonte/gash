// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// 3.0: SYS-15 fixed: a normal user on a terminal is asked for sudo only
// for the sections that read as root (and all); overview, network,
// webstack, mail and infra run with no sudo call.
func TestLedger_SYS15(t *testing.T) {
	for _, args := range [][]string{{"overview", "--llm"}, {"network", "--llm"}, {"network"}, {"webstack"}, {"mail", "--llm"}, {"infra"}} {
		fx := newFixture(t, etcTree, procTree)
		fx.uid = 1000
		r := fx.call(t, term.Fixed(invoke.EnvMap(fx.env), true, false, false), args...)
		require.Equal(t, 0, r.rc, args)
		require.Empty(t, fx.sudo(t), args)
		require.Empty(t, r.err, args)
	}
	for _, args := range [][]string{{"auth", "--llm"}, {"identity"}, {"all", "--llm"}} {
		fx := newFixture(t, etcTree, procTree)
		fx.uid = 1000
		r := fx.call(t, term.Fixed(invoke.EnvMap(fx.env), true, false, false), args...)
		require.Equal(t, 0, r.rc, args)
		require.True(t, strings.HasPrefix(fx.sudo(t), "sudo -n true\nsudo -v\n"), args)
		if args[len(args)-1] == "--llm" {
			require.Empty(t, r.err, args)
		} else {
			require.Equal(t, "Info: Elevated privileges needed for full system enumeration.\n"+
				"Warning: Could not obtain sudo. Some data will be limited.\n", r.err, args)
		}
	}
}

// 3.0: SYS-16 fixed: systemd-detect-virt's own none (exit 1 on bare
// metal) is the one value; none also when it prints nothing.
func TestLedger_SYS16(t *testing.T) {
	fx := newFixture(t)
	fx.env["MOCK_VIRT"] = "none"
	fx.env["MOCK_VIRT_RC"] = "1"
	r := fx.call(t, nil, "identity", "--llm")
	require.Contains(t, r.out, "\nVirtualization=none\n")
	require.NotContains(t, r.out, "\nnone\n")
	r = fx.call(t, nil, "identity")
	require.Contains(t, r.out, "  Virtualization:          none\n")
	require.NotContains(t, r.out, "\nnone\n")

	// A tool that prints nothing and fails gives the one none.
	fx.stub(t, "systemd-detect-virt", "exit 1")
	require.Contains(t, fx.call(t, nil, "identity", "--llm").out, "\nVirtualization=none\n")
	// No tool, no row.
	require.NoError(t, os.Remove(filepath.Join(fx.bin, "systemd-detect-virt")))
	require.NotContains(t, fx.call(t, nil, "identity", "--llm").out, "Virtualization")
}

// 3.0: SYS-17 fixed: a sub-header is scoped to its block: one that
// printed nothing is dropped, never inherited by a later line; the
// runtimes print under Containers and the /etc/ssl/private names under
// SSL private keys.
func TestLedger_SYS17(t *testing.T) {
	fx := newFixture(t, tree{"etc/docker/": "", "etc/containerd/": ""})
	require.Equal(t, result{"=== INFRA ===\n--- Containers\ncontainerd=present\n", "", 0}, fx.call(t, nil, "infra", "--llm"))
	// Without a line after it the heading never shows.
	fx = newFixture(t, tree{"etc/docker/": ""})
	require.Equal(t, result{"=== INFRA ===\n", "", 0}, fx.call(t, nil, "infra", "--llm"))
	// An empty fstab and crypttab leave LVM at the section level.
	fx = newFixture(t, tree{"etc/crypttab": "# none\n", "etc/lvm/": ""})
	require.Equal(t, result{"=== STORAGE ===\nLVM=present\n", "", 0}, fx.call(t, nil, "storage", "--llm"))
	// An empty ufw block leaves firewalld at the section level.
	fx = newFixture(t, tree{"etc/ufw/": "", "etc/firewalld/": ""})
	require.Equal(t, result{"=== SECURITY ===\nfirewalld=present\n", "", 0}, fx.call(t, nil, "security", "--llm"))

	fx = newFixture(t, tree{"etc/ssl/private/b.key": "", "etc/ssl/private/a.pem": "", "etc/ssl/private/c.txt": ""})
	r := fx.call(t, nil, "system", "--llm")
	require.True(t, strings.HasPrefix(r.out, "=== SYSTEM ===\n--- Cron\ncron.d:\ncron.daily:\ncron.hourly:\ncron.weekly:\ncron.monthly:\n"+
		"--- SSL private keys\n/etc/ssl/private/a.pem\n/etc/ssl/private/b.key\n"), r.out)
}

// 3.0: SYS-18 fixed: identity reads os-release(5) with the platform
// parser: values quoted or not, escapes undone, never evaluated.
func TestLedger_SYS18(t *testing.T) {
	for name, c := range map[string]struct{ release, want string }{
		"debian": {"PRETTY_NAME=\"Debian GNU/Linux 12 (bookworm)\"\nNAME=\"Debian GNU/Linux\"\nVERSION_ID=\"12\"\nID=debian\n",
			"OS=Debian GNU/Linux 12 (bookworm)\nOS ID=debian\nVersion=12\n"},
		"ubuntu": {"PRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\nVERSION_ID=\"24.04\"\nID=ubuntu\nID_LIKE=debian\n",
			"OS=Ubuntu 24.04.1 LTS\nOS ID=ubuntu\nVersion=24.04\n"},
		"rhel": {"NAME=\"Red Hat Enterprise Linux\"\nID=\"rhel\"\nVERSION_ID=\"9.4\"\nPRETTY_NAME=\"Red Hat Enterprise Linux 9.4 (Plow)\"\n",
			"OS=Red Hat Enterprise Linux 9.4 (Plow)\nOS ID=rhel\nVersion=9.4\n"},
		"fedora": {"NAME=\"Fedora Linux\"\nVERSION_ID=39\nID=fedora\nPRETTY_NAME=\"Fedora Linux 39 (Workstation Edition)\"\n",
			"OS=Fedora Linux 39 (Workstation Edition)\nOS ID=fedora\nVersion=39\n"},
		"tumbleweed": {"NAME=\"openSUSE Tumbleweed\"\nID=opensuse-tumbleweed\nVERSION_ID=\"20241001\"\nPRETTY_NAME=\"openSUSE Tumbleweed\"\n",
			"OS=openSUSE Tumbleweed\nOS ID=opensuse-tumbleweed\nVersion=20241001\n"},
		"leap": {"NAME=\"openSUSE Leap\"\nID=\"opensuse-leap\"\nVERSION_ID=\"15.6\"\nPRETTY_NAME=\"openSUSE Leap 15.6\"\n",
			"OS=openSUSE Leap 15.6\nOS ID=opensuse-leap\nVersion=15.6\n"},
		"alpine": {"NAME=\"Alpine Linux\"\nID=alpine\nVERSION_ID=3.19.1\nPRETTY_NAME=\"Alpine Linux v3.19\"\n",
			"OS=Alpine Linux v3.19\nOS ID=alpine\nVersion=3.19.1\n"},
		"single quotes": {"PRETTY_NAME='Some OS'\nID='x'\nVERSION_ID='1'\n", "OS=Some OS\nOS ID=x\nVersion=1\n"},
		"escapes":       {"PRETTY_NAME=\"A \\\"B\\\" \\$C\"\nID=a\n", "OS=A \"B\" $C\nOS ID=a\n"},
		"command": {"PRETTY_NAME=\"$(echo INJECTED) `id`\"\nID=a$(b)\n",
			"OS=$(echo INJECTED) `id`\nOS ID=a$(b)\n"},
	} {
		fx := newFixture(t, tree{"etc/os-release": c.release})
		r := fx.call(t, nil, "identity", "--llm")
		var got strings.Builder
		for _, l := range strings.SplitAfter(r.out, "\n") {
			if strings.HasPrefix(l, "OS=") || strings.HasPrefix(l, "OS ID=") || strings.HasPrefix(l, "Version=") {
				got.WriteString(l)
			}
		}
		require.Equal(t, c.want, got.String(), name)
		require.Empty(t, r.err, name)
	}
}

// 3.0: SYS-19 fixed: /etc/ssl/private is listed as root with one find
// (a normal user cannot expand a glob in it), the names it prints sorted
// under their own heading.
func TestLedger_SYS19(t *testing.T) {
	fx := newFixture(t, tree{"etc/ssl/private/": ""})
	fx.uid = 1000
	fx.env["SUDO_CACHED"] = "0"
	fx.env["SUDO_ROOT"] = fx.root
	fx.stub(t, "sudo", `printf 'sudo %s\n' "$*" >>"$SUDO_LOG"
case "$1" in
-n) exit 0 ;;
find) [ "$2" = "$SUDO_ROOT/etc/ssl/private" ] && printf '%s\n' "$SUDO_ROOT/etc/ssl/private/z.key" "$SUDO_ROOT/etc/ssl/private/k.pem"; exit 0 ;;
esac
exec "$@"`)
	r := fx.call(t, nil, "system", "--llm")
	require.Contains(t, r.out, "cron.monthly:\n--- SSL private keys\n/etc/ssl/private/k.pem\n/etc/ssl/private/z.key\n--- Profile\n")
	require.Contains(t, fx.sudo(t), "\nsudo find R/etc/ssl/private -maxdepth 1 -name *.pem -o -name *.key -o -name *.crt\n")
	require.Equal(t, 1, strings.Count(fx.sudo(t), "/etc/ssl/private"))
	require.NotContains(t, fx.sudo(t), "sudo ls R/etc/ssl")

	// Root reads the directory itself, sorted, the first ten.
	tr := tree{}
	for i := 0; i < 12; i++ {
		tr[fmt.Sprintf("etc/ssl/private/k%02d.key", 11-i)] = ""
	}
	tr["etc/ssl/private/notes.txt"] = ""
	fx = newFixture(t, tr)
	r = fx.call(t, nil, "system", "--llm")
	want := "--- SSL private keys\n"
	for i := 0; i < 10; i++ {
		want += fmt.Sprintf("/etc/ssl/private/k%02d.key\n", i)
	}
	require.Contains(t, r.out, want+"--- Profile\n")
}

// 3.0: SYS-20 fixed: a second section is an error naming it; -h/--help
// is the help wherever it is.
func TestLedger_SYS20(t *testing.T) {
	fx := newFixture(t, etcTree)
	sections := "Info: Sections: overview identity storage services auth network security webstack mail infra system all\n"
	require.Equal(t, result{"", "Error: Only one section allowed: storage\n" + sections, 1}, fx.call(t, nil, "identity", "storage", "--llm"))
	require.Equal(t, result{"", "Error: Only one section allowed: all\n" + sections, 1}, fx.call(t, nil, "all", "all"))
	unknown := "Info: Sections: overview identity storage services auth network security webstack mail infra system all\nInfo: Flags: --llm, --no-color\n"
	require.Equal(t, result{"", "Error: Unknown argument: bogus\n" + unknown, 1}, fx.call(t, nil, "mail", "bogus", "--llm"))
	for _, args := range [][]string{{"--help", "bogus"}, {"-h"}, {"--llm", "--help"}, {"mail", "-h"}, {"identity", "storage", "--help"}} {
		r := fx.call(t, nil, args...)
		require.Equal(t, 0, r.rc, args)
		require.Contains(t, r.out, "  sysinfo  (module: sysinfo)\n", args)
	}
	require.Equal(t, result{llmWant["storage"], "", 0}, fx.call(t, nil, "storage", "--llm"))
	require.Empty(t, fx.sudo(t))
}

// LEDGER SYS-34: COLUMNS is a width only as a run of digits (a leading 0
// is octal); anything else is 80 and never evaluated.
func TestLedger_SYS34(t *testing.T) {
	fx := newFixture(t)
	marker := filepath.Join(fx.root, "INJECTED")
	rule := func(n int) string { return "\n" + strings.Repeat("─", n) + "\nMAIL\n\n  nothing found\n" }
	for cols, n := range map[string]int{"abc": 60, "a[$(touch " + marker + ")]": 60, "$(touch " + marker + ")": 60, "": 60,
		"040": 32, "50": 50, "200": 60, " 40": 60} {
		fx.env["COLUMNS"] = cols
		require.Equal(t, result{rule(n), "", 0}, fx.call(t, nil, "mail"), cols)
	}
	_, err := os.Stat(marker)
	require.True(t, os.IsNotExist(err))
}

// LEDGER SYS-41: byte order for every listing, whatever the locale and
// the directory order.
func TestLedger_SYS41(t *testing.T) {
	fx := newFixture(t)
	for _, p := range []string{"etc/sysctl.d/a.conf", "etc/sysctl.d/B.conf", "etc/cron.d/a-job", "etc/cron.d/B-job", "etc/cron.d/_x"} {
		require.NoError(t, os.MkdirAll(filepath.Join(fx.root, filepath.Dir(p)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(fx.root, p), []byte("k = "+filepath.Base(p)+"\n"), 0o644))
	}
	want := "--- Cron\ncron.d:\nB-job\n_x\na-job\n"
	tuning := "--- Kernel tuning\n--- /etc/sysctl.d/B.conf:\nk = B.conf\n--- /etc/sysctl.d/a.conf:\nk = a.conf\n"
	for _, lang := range []string{"C.UTF-8", "en_US.UTF-8", "C"} {
		fx.env["LANG"] = lang
		r := fx.call(t, nil, "system", "--llm")
		require.Contains(t, r.out, want, lang)
		require.Contains(t, r.out, tuning, lang)
	}
}

// LEDGER SYS-48 (reproduced): the help keeps the 2.x section
// descriptions, wrong as they are.
func TestLedger_SYS48(t *testing.T) {
	r := newFixture(t).call(t, nil, "--help")
	require.Equal(t, 0, r.rc)
	for _, l := range []string{
		"    storage    NFS exports/config (disk usage lives in files_largest/dirs_largest)\n",
		"    network    Interfaces, DNS, DHCP, hostname\n",
		"    security   Firewall rules, SSH hardening, fail2ban/crowdsec\n",
		"    infra      Docker, containers, monitoring, backup systems\n",
		"    system     Cron jobs, systemd timers, custom hooks\n",
	} {
		require.Contains(t, r.out, l)
	}
}

// wgFixture holds every spelling of the two WireGuard key kinds.
const wgFixture = "[Interface]\nPrivateKey=MARKER-A\nPrivateKey = MARKER-B\n  PrivateKey = MARKER-C\n" +
	"privatekey\t=\tMARKER-D\nAddress = 10.0.0.1/24\n[Peer]\nPresharedKey = MARKER-E\nPublicKey = public\n"

// 3.0: SYS-50 fixed: the WireGuard drill-down masks the value of every
// PrivateKey and PresharedKey line, whatever its case and spacing.
func TestLedger_SYS50_fix(t *testing.T) {
	fx := deepFixture(t, tree{"etc/wireguard/wg9.conf": wgFixture})
	services := fx.deep(t, "services")
	require.NotContains(t, services, "MARKER-")
	require.Contains(t, services, "--- wg9.conf\n[Interface]\nPrivateKey=[REDACTED]\nPrivateKey = [REDACTED]\n  PrivateKey = [REDACTED]\n"+
		"privatekey\t=\t[REDACTED]\nAddress = 10.0.0.1/24\n[Peer]\nPresharedKey = [REDACTED]\nPublicKey = public\n")
}

// SYS-50 (2.x): `sed 's/PrivateKey = .*/PrivateKey = [REDACTED]/'` over
// the same lines leaves three of the five values in clear.
func TestLedger_SYS50_2x(t *testing.T) {
	sed := regexp.MustCompile(`PrivateKey = .*`)
	var clear []string
	for _, l := range strings.Split(wgFixture, "\n") {
		l = sed.ReplaceAllString(l, "PrivateKey = [REDACTED]")
		if i := strings.Index(l, "MARKER-"); i >= 0 {
			clear = append(clear, l[i:])
		}
	}
	require.Equal(t, []string{"MARKER-A", "MARKER-D", "MARKER-E"}, clear)
}

// 3.0: SYS-51 fixed: each I/O scheduler line names its block device.
func TestLedger_SYS51_fix(t *testing.T) {
	out := deepFixture(t).deep(t, "performance")
	require.Contains(t, out, "--- I/O scheduler\nsda: [mq-deadline] none\nvda: none\n")
	require.NotContains(t, out, "queue:")
}

// SYS-51 (2.x): `dirname "$sched" | xargs basename` is the queue
// directory's name for every device.
func TestLedger_SYS51_2x(t *testing.T) {
	for _, p := range []string{"/sys/block/sda/queue/scheduler", "/sys/block/vda/queue/scheduler"} {
		require.Equal(t, "queue", filepath.Base(filepath.Dir(p)))
	}
}

// 3.0: SYS-52 fixed: a file with no final newline is ended by one, so
// the next header starts its own line.
func TestLedger_SYS52_fix(t *testing.T) {
	fx := deepFixture(t, tree{"etc/collectd/collectd.conf.d/cpu.conf": "<Plugin cpu>"})
	require.Contains(t, fx.deep(t, "services"), "<Plugin cpu>\n--- Monit\n")
	var b bytes.Buffer
	d := deepOut{&b}
	d.raw([]byte("a"))
	d.raw([]byte("b\n"))
	d.raw(nil)
	d.echo("--- next")
	require.Equal(t, "a\nb\n--- next\n", b.String())
}

// SYS-52 (2.x): `cat FILE | head -N` copies a final partial line as it
// is, so `echo "--- header"` lands on it.
func TestLedger_SYS52_2x(t *testing.T) {
	require.Equal(t, "<Plugin cpu>--- Monit\n", string(headN([]byte("<Plugin cpu>"), 20))+"--- Monit\n")
}
