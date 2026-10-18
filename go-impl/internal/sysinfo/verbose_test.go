// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
	"github.com/mauriziofonte/gash/internal/ui"
)

// procTree holds the /proc files of the overview.
var procTree = tree{
	"proc/uptime":  "273600.55 1000.00\n",
	"proc/loadavg": "0.50 0.40 0.30 1/200 1234\n",
	"proc/cpuinfo": "processor\t: 0\nmodel name\t: Intel(R)  Xeon(R)   CPU @ 2.20GHz\nflags\t: x\n",
	"proc/meminfo": "MemTotal:       16000000 kB\nMemFree:         1000000 kB\nMemAvailable:    4000000 kB\nSwapTotal:       2097152 kB\nSwapFree:        2097152 kB\n",
}

// tty is a color terminal on stdin, stdout and stderr.
func (fx *fixture) tty() *term.Term {
	return term.Fixed(invoke.EnvMap(fx.env), true, true, true)
}

// The 2.x bytes of `sysinfo overview` over etcTree and procTree.
const (
	overviewPlain = "\n╭───────────────────────────────────────────────────────╮\n│  web1 · Debian GNU/Linux 12 (bookworm)                │\n│  Linux 6.1.0 x86_64 · up 3d 4h · load 0.50 0.40 0.30  │\n│                                                       │\n│  CPU      Intel(R) Xeon(R) CPU @ 2.20GHz · 4 cores    │\n│  Memory   ━━━━━━━━──  11 GiB / 15 GiB    75%          │\n│  Swap     ──────────  0.0 KiB / 2.0 GiB  0%           │\n│  Disk /   ━━━━━─────  50 GiB / 100 GiB   50%          │\n│  Address  10.0.0.5                                    │\n╰───────────────────────────────────────────────────────╯\n\n"
	overviewTTY   = "\n\u001b[38;5;39m╭───────────────────────────────────────────────────────╮\u001b[0m\n\u001b[38;5;39m│\u001b[0m  \u001b[1mweb1\u001b[0m · Debian GNU/Linux 12 (bookworm)                \u001b[38;5;39m│\u001b[0m\n\u001b[38;5;39m│\u001b[0m  \u001b[38;5;245mLinux 6.1.0 x86_64 · up 3d 4h · load 0.50 0.40 0.30\u001b[0m  \u001b[38;5;39m│\u001b[0m\n\u001b[38;5;39m│\u001b[0m                                                       \u001b[38;5;39m│\u001b[0m\n\u001b[38;5;39m│\u001b[0m  \u001b[38;5;245mCPU    \u001b[0m  Intel(R) Xeon(R) CPU @ 2.20GHz · 4 cores    \u001b[38;5;39m│\u001b[0m\n\u001b[38;5;39m│\u001b[0m  \u001b[38;5;245mMemory \u001b[0m  \u001b[38;5;220m━━━━━━━━\u001b[0m\u001b[38;5;245m──\u001b[0m  11 GiB / 15 GiB    \u001b[38;5;220m75%\u001b[0m          \u001b[38;5;39m│\u001b[0m\n\u001b[38;5;39m│\u001b[0m  \u001b[38;5;245mSwap   \u001b[0m  \u001b[38;5;82m\u001b[0m\u001b[38;5;245m──────────\u001b[0m  0.0 KiB / 2.0 GiB  \u001b[38;5;82m0%\u001b[0m           \u001b[38;5;39m│\u001b[0m\n\u001b[38;5;39m│\u001b[0m  \u001b[38;5;245mDisk / \u001b[0m  \u001b[38;5;82m━━━━━\u001b[0m\u001b[38;5;245m─────\u001b[0m  50 GiB / 100 GiB   \u001b[38;5;82m50%\u001b[0m          \u001b[38;5;39m│\u001b[0m\n\u001b[38;5;39m│\u001b[0m  \u001b[38;5;245mAddress\u001b[0m  10.0.0.5                                    \u001b[38;5;39m│\u001b[0m\n\u001b[38;5;39m╰───────────────────────────────────────────────────────╯\u001b[0m\n\n"
	overviewLLM   = "=== OVERVIEW ===\nHost=web1\nUptime=3d 4h\nLoad=0.50 0.40 0.30\nCPU=Intel(R) Xeon(R) CPU @ 2.20GHz x4\nMemory KiB=used=12000000 total=16000000\nSwap KiB=used=0 total=2097152\nDisk / KiB=used=52428800 total=104857600\n"
)

func TestOverview(t *testing.T) {
	fx := newFixture(t, etcTree, procTree)
	require.Equal(t, result{overviewPlain, "", 0}, fx.call(t, nil, "overview"))
	require.Equal(t, result{overviewTTY, "", 0}, fx.call(t, fx.tty(), "overview"))
	require.Equal(t, result{overviewPlain, "", 0}, fx.call(t, fx.tty(), "overview", "--no-color"))
	require.Equal(t, result{overviewLLM, "", 0}, fx.call(t, fx.tty(), "overview", "--llm"))
	// all opens with it.
	r := fx.call(t, nil, "--llm")
	require.Equal(t, overviewLLM+llmWant["identity"], r.out[:len(overviewLLM)+len(llmWant["identity"])])
}

// Up to a day: hours and minutes; nothing measured: no row.
func TestOverviewBare(t *testing.T) {
	fx := newFixture(t, tree{"proc/uptime": "4000.2 1\n", "proc/meminfo": "MemTotal: 0 kB\n"})
	for _, n := range []string{"df", "getconf", "hostname"} {
		fx.stub(t, n, "exit 1")
	}
	fx.stub(t, "nproc", "exit 1")
	r := fx.call(t, nil, "overview", "--llm")
	require.Equal(t, result{"=== OVERVIEW ===\nHost=web1\nUptime=1h 6m\n", "", 0}, r)
	r = fx.call(t, nil, "overview")
	second := "Linux 6.1.0 x86_64 · up 1h 6m"
	bar := strings.Repeat("─", ui.Len(second)+4)
	require.Equal(t, "\n╭"+bar+"╮\n│  web1"+ui.Spaces(ui.Len(second)-4)+"  │\n│  "+second+"  │\n│  "+ui.Spaces(ui.Len(second))+"  │\n╰"+bar+"╯\n\n", r.out)
}

func TestHumanKiB(t *testing.T) {
	for k, want := range map[int64]string{0: "0.0 KiB", 9: "9.0 KiB", 1023: "1023 KiB", 1024: "1.0 MiB", 16479424: "16 GiB", 15872: "16 MiB", 16384: "16 MiB", 10239: "10.0 MiB",
		1 << 40: "1.0 PiB", 1 << 52: "4096 PiB", -5: "-5.0 KiB"} {
		require.Equal(t, want, humanKiB(k), k)
	}
	require.Equal(t, "Memory\t75/100\t75 KiB / 100 KiB\t75%\twarning", usageRow("Memory", 75, 100))
	require.Equal(t, "Disk /\t9/10\t9.0 KiB / 10 KiB\t90%\terror", usageRow("Disk /", 9, 10))
	require.Equal(t, "Swap\t0/0\t0.0 KiB / 0.0 KiB\t0%\tsuccess", usageRow("Swap", 0, 0))
}

// The 2.x bytes of verbose sections.
const (
	infraTTY     = "\n\u001b[38;5;39m────────────────────────────────────────────────────────────\u001b[0m\n\u001b[1mINFRA\u001b[0m\n\n  \u001b[1mSamba\u001b[0m\n    [global]\n      workgroup = WORKGROUP\n  \u001b[1mOpenMediaVault\u001b[0m\n  \u001b[1mOMV:                    \u001b[0m installed\n  \u001b[1mconfig.xml:             \u001b[0m 10 bytes\n  \u001b[1mOpenVPN\u001b[0m\n    /etc/openvpn/client.conf\n    /etc/openvpn/sub/site.ovpn\n  \u001b[1mDocker\u001b[0m\n  \u001b[1mcontainerd:             \u001b[0m present\n  \u001b[1mCollectd\u001b[0m\n    cpu.conf\n  \u001b[1mWebmin\u001b[0m\n    miniserv.conf\n    virtual-server\n  \u001b[38;5;39m•\u001b[0m Virtualmin: installed\n"
	networkPlain = "\n────────────────────────────────────────────────────────────\nNETWORK\n\n  resolv.conf\n    nameserver 1.1.1.1\n    \n    search example.com\n  hosts\n    127.0.0.1 localhost\n    ::1 localhost\n  netplan\n    network:\n      version: 2\n"
	mailEmptyTTY = "\n\u001b[38;5;39m────────────────────────────────────────────────────────────\u001b[0m\n\u001b[1mMAIL\u001b[0m\n\n\u001b[38;5;245m  nothing found\u001b[0m\n"
)

func TestVerbose(t *testing.T) {
	fx := newFixture(t, etcTree)
	fx.env["COLUMNS"] = "80"
	require.Equal(t, result{infraTTY, "", 0}, fx.call(t, fx.tty(), "infra"))
	require.Equal(t, result{networkPlain, "", 0}, fx.call(t, nil, "network"))
	require.Equal(t, result{ui.StripSGR(infraTTY), "", 0}, fx.call(t, fx.tty(), "infra", "--no-color"))

	empty := newFixture(t)
	require.Equal(t, result{mailEmptyTTY, "", 0}, empty.call(t, empty.tty(), "mail"))
	// No "nothing found" in the compact form.
	require.Equal(t, result{"=== MAIL ===\n", "", 0}, empty.call(t, empty.tty(), "mail", "--llm"))
}

// The formatter of the compact form without a state directory (the
// collectors called directly): every header at once, empty values
// skipped (spec 11, 25, 39, 53, 67).
func TestFormatter(t *testing.T) {
	var b bytes.Buffer
	o := &out{w: &b, llm: true}
	o.section("TEST")
	o.kv("Key", "value")
	o.kv("Empty", "")
	o.item("plain text")
	o.subhead("Sub")
	o.subhead("Empty sub")
	o.raw([]string{"a", ""})
	o.close()
	require.Equal(t, "=== TEST ===\nKey=value\nplain text\n--- Sub\n--- Empty sub\na\n\n", b.String())
}

// The lazy headers of sysinfo (spec 394): a sub-header only when a line
// follows, once; an empty section says so in verbose mode only.
func TestLazyHeaders(t *testing.T) {
	var b bytes.Buffer
	o := &out{w: &b, llm: true, lazy: true}
	o.section("STORAGE")
	o.subhead("Empty one")
	o.raw(nil)
	o.subhead("Filled")
	o.item("a")
	o.item("b")
	o.subhead("Raw")
	o.raw([]string{"line"})
	o.kv("Key", "value")
	o.section("MAIL")
	o.subhead("Postfix")
	o.close()
	require.Equal(t, "=== STORAGE ===\n--- Filled\na\nb\n--- Raw\nline\nKey=value\n=== MAIL ===\n", b.String())

	b.Reset()
	o = &out{w: &b, lazy: true, s: ui.Style{G: term.UnicodeGlyphs, Columns: 80}}
	o.section("MAIL")
	o.subhead("Postfix")
	o.close()
	require.Equal(t, "\n"+strings.Repeat("─", 60)+"\nMAIL\n\n  nothing found\n", b.String())
}
