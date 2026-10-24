// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
)

// psAux is the ps aux table of tests/mocks/bin/ps, plus the binary's own
// row and a grep-named process.
const psAux = `USER       PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
root         1  0.0  0.1  12345  678 ?        Ss   00:00   0:01 /sbin/init
maurizio  2222  0.1  0.2  99999  111 pts/0    Sl   00:00   0:00 my-proc --flag
maurizio  3333  0.0  0.1  88888  222 pts/0    Sl   00:00   0:00 another-proc
maurizio  4242  0.0  0.1  88888  222 pts/0    Sl   00:00   0:00 gash process_find proc
maurizio  5555  0.0  0.1  88888  222 pts/0    Sl   00:00   0:00 ripgrep proc
`

func psExpect() runner.Expect { return runner.Expect{Argv: []string{"ps", "aux"}, Stdout: psAux} }

func TestProcessFind(t *testing.T) {
	m, _, _ := tm(t, runner.NewFake(t, psExpect(), psExpect()))
	r := call(m, "process_find", nil, nil, "", "proc")
	require.Equal(t, result{"   2222     maurizio     0.1  my-proc\n   3333     maurizio     0.0  another-proc\n",
		"Info: Processes matching proc:\n", 0}, r)
	r = call(m, "process_find", nil, nil, "", "INIT")
	require.Equal(t, result{"   1        root         0.0  /sbin/init\n", "Info: Processes matching INIT:\n", 0}, r)

	r = call(m, "process_find", nil, nil, "")
	require.Equal(t, result{"", "Error: Missing process name. Usage: process_find <process_name>\n", 1}, r)
}

// SYS-04: no match is an error with status 0; the whole ps line is
// matched (the header too) and every line with "grep" is dropped.
func TestLedger_SYS04(t *testing.T) {
	m, _, _ := tm(t, runner.NewFake(t, psExpect(), psExpect(), psExpect(), psExpect()))
	r := call(m, "process_find", nil, nil, "", "zz")
	require.Equal(t, result{"", "Error: No process found with name 'zz'.\n", 0}, r)
	r = call(m, "process_find", nil, nil, "", "PID")
	require.Equal(t, result{"   PID      USER         %CPU COMMAND\n", "Info: Processes matching PID:\n", 0}, r)
	r = call(m, "process_find", nil, nil, "", "ripgrep")
	require.Equal(t, 0, r.rc)
	require.Equal(t, "Error: No process found with name 'ripgrep'.\n", r.err)
	// A pattern grep refuses: its message, then the no-match error.
	r = call(m, "process_find", nil, nil, "", "[a")
	require.Equal(t, result{"", "grep: Unmatched [, [^, [:, [., or [=\nError: No process found with name '[a'.\n", 0}, r)
}

func pgrep(name, out string, status int) runner.Expect {
	return runner.Expect{Argv: []string{"pgrep", "-ix", "--", escapeERE(name)}, Stdout: out, Status: status}
}

func TestProcessKill(t *testing.T) {
	m, k, _ := tm(t, runner.NewFake(t,
		pgrep("my-proc", "2222\n", 0),
		pgrep("my-proc", "2222\n3333\n", 0),
		runner.Expect{Argv: []string{"ps", "-o", "args=", "-p", "2222"}, Stdout: "my-proc\t--flag\n"},
		runner.Expect{Argv: []string{"ps", "-o", "args=", "-p", "3333"}, Status: 1},
		pgrep("my-proc", "2222\n", 0),
		runner.Expect{Argv: []string{"ps", "-o", "args=", "-p", "2222"}, Stdout: "my-proc\n"},
		pgrep("nope", "", 1),
	))
	// -y: TERM, the poll, a survivor gets KILL.
	k.alive[2222] = true
	r := call(m, "process_kill", nil, nil, "", "-y", "my-proc")
	require.Equal(t, result{"", "Info: Process 2222 ignored SIGTERM; killed with SIGKILL.\n", 0}, r)
	require.Equal(t, "TERM 2222", k.sent[0])
	require.Equal(t, "KILL 2222", k.sent[len(k.sent)-1])
	require.Len(t, k.sent, 1+30+1+1)

	// The plan and the question on the controlling terminal.
	k.sent = nil
	r = call(m, "process_kill", nil, ttyTerm(nil, "y\n"), "", "my-proc")
	require.Equal(t, 0, r.rc)
	require.Equal(t, "\x1b[38;5;75m•\x1b[0m Matching 'my-proc':\n"+
		"  \x1b[38;5;214m2222\x1b[0m  my-proc --flag\n  \x1b[38;5;214m3333\x1b[0m  ?\n"+
		"Kill all 2? \x1b[1m[y/N]\x1b[0m "+
		"\x1b[38;5;75m•\x1b[0m Process 2222 terminated.\n\x1b[38;5;75m•\x1b[0m Process 3333 terminated.\n", r.err)
	require.Equal(t, []string{"TERM 2222", "TERM 3333", "0 2222", "0 3333", "0 2222", "0 3333"}, k.sent)

	r = call(m, "process_kill", nil, ttyTerm(nil, "n\n"), "", "my-proc")
	require.Equal(t, 1, r.rc)
	require.Contains(t, r.err, "Kill it? \x1b[1m[y/N]\x1b[0m \x1b[38;5;75m•\x1b[0m Aborted.\n")

	r = call(m, "process_kill", nil, nil, "", "-f", "nope")
	require.Equal(t, result{"", "Error: No process found with exact name 'nope'. Use process_find (pf) to inspect candidates.\n", 1}, r)
	r = call(m, "process_kill", nil, nil, "", "--bogus", "x")
	require.Equal(t, result{"", "Error: Unknown option: --bogus\n", 1}, r)
	r = call(m, "process_kill", nil, nil, "", "-y")
	require.Equal(t, result{"", "Error: Missing process name. Usage: process_kill [-y] <process_name>\n", 1}, r)
}

// SYS-01: NAME is a name, not a pattern (3.0: SYS-01 fixed): every ERE
// metacharacter is escaped for `pgrep -ix --`, so `.*` matches only a
// process literally named ".*"; a NAME over the 15-byte comm is looked up
// by its first 15 bytes and confirmed on argv[0]'s base name.
func TestLedger_SYS01(t *testing.T) {
	m, k, _ := tm(t, runner.NewFake(t,
		runner.Expect{Argv: []string{"pgrep", "-ix", "--", `\.\*`}, Status: 1},
		runner.Expect{Argv: []string{"pgrep", "-ix", "--", `node\|npm`}, Status: 1},
		runner.Expect{Argv: []string{"pgrep", "-ix", "--", "webpack-dev-ser"}, Stdout: "700\n701\n702\n"},
	))
	r := call(m, "process_kill", map[string]string{"GASH_HEADLESS": "1"}, nil, "", ".*")
	require.Equal(t, result{"", "Error: No process found with exact name '.*'. Use process_find (pf) to inspect candidates.\n", 1}, r)
	r = call(m, "process_kill", map[string]string{"GASH_HEADLESS": "1"}, nil, "", "node|npm")
	require.Equal(t, 1, r.rc)
	require.Empty(t, k.sent)

	root := t.TempDir()
	for pid, argv := range map[string]string{
		"700": "/usr/bin/node\x00/opt/x/webpack-dev-server\x00",
		"701": "/opt/x/bin/Webpack-Dev-Server\x00--port\x00",
		"702": "webpack-dev-service\x00",
	} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, "proc", pid), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "proc", pid, "cmdline"), []byte(argv), 0o644))
	}
	m.sys, _ = platform.NewSysroot(root)
	r = call(m, "process_kill", map[string]string{"GASH_HEADLESS": "1"}, nil, "", "webpack-dev-server")
	require.Equal(t, result{"", "Error: 1 process(es) match 'webpack-dev-server' (PID 701). Refusing to kill without a terminal to confirm; pass -y.\n", 1}, r)
}

// SYS-02: a process the user may not signal is reported as such and the
// call returns 1 (3.0: SYS-02 fixed; 2.x said "terminated", rc 0); ESRCH
// still means terminated.
func TestLedger_SYS02(t *testing.T) {
	m, k, _ := tm(t, runner.NewFake(t, pgrep("root-proc", "1\n", 0), lsof("8080", "1\n", ""), lsofUDP("8080", ""),
		runner.Expect{Argv: []string{"ps", "-o", "comm=", "-p", "1"}, Stdout: "init\n"}))
	k.refuse = syscall.EPERM
	r := call(m, "process_kill", nil, nil, "", "-y", "root-proc")
	require.Equal(t, result{"", "Error: Process 1: permission denied.\n", 1}, r)
	require.Equal(t, []string{"TERM 1"}, k.sent)

	k.sent = nil
	r = call(m, "port_kill", map[string]string{"PATH": binDir(t, "lsof")}, nil, "", "-y", "8080")
	require.Equal(t, result{"", "Info: Listening on port 8080:\n  1  init\nError: Process 1: permission denied.\n", 1}, r)
}

// SYS-10: the last positional wins in process_kill and port_kill.
func TestLedger_SYS10(t *testing.T) {
	bin := binDir(t, "lsof")
	m, _, _ := tm(t, runner.NewFake(t, pgrep("b", "", 1), lsof("81", "", ""), lsofUDP("81", "")))
	r := call(m, "process_kill", nil, nil, "", "-y", "a", "b")
	require.Contains(t, r.err, "exact name 'b'")
	r = call(m, "port_kill", map[string]string{"PATH": bin}, nil, "", "80", "81")
	require.Equal(t, result{"", "Error: Nothing is listening on port 81.\n", 1}, r)
}

func lsof(port, tcp, udp string) runner.Expect {
	return runner.Expect{Argv: []string{"lsof", "-t", "-nP", "-iTCP:" + port, "-sTCP:LISTEN"}, Stdout: tcp}
}

// lsofUDP answers the UDP query; udp is lsof's -F pn output.
func lsofUDP(port, udp string) runner.Expect {
	return runner.Expect{Argv: []string{"lsof", "-nP", "-iUDP:" + port, "-F", "pn"}, Stdout: udp}
}

func TestPortKill(t *testing.T) {
	bin := binDir(t, "lsof")
	env := map[string]string{"PATH": bin}
	m, k, _ := tm(t, runner.NewFake(t,
		lsof("8080", "300\n100\n", ""), lsofUDP("8080", "p100\nf5\nn*:8080\n"),
		runner.Expect{Argv: []string{"ps", "-o", "comm=", "-p", "100"}, Stdout: "node\n"},
		runner.Expect{Argv: []string{"ps", "-o", "comm=", "-p", "300"}, Stdout: ""},
		lsof("8080", "", ""), lsofUDP("8080", ""),
		lsof("8080", "100\n", ""), lsofUDP("8080", ""),
		runner.Expect{Argv: []string{"ps", "-o", "comm=", "-p", "100"}, Stdout: "node\n"},
	))
	k.alive[300] = true
	r := call(m, "port_kill", env, nil, "", "--force", "08080")
	require.Equal(t, result{"", "Info: Listening on port 8080:\n  100  node\n  300  ?\n" +
		"Info: Process 100 on port 8080 terminated.\nInfo: Process 300 on port 8080 ignored SIGTERM; killed with SIGKILL.\n", 0}, r)

	r = call(m, "port_kill", env, nil, "", "8080")
	require.Equal(t, result{"", "Error: Nothing is listening on port 8080.\n", 1}, r)
	// Headless without -y: the plan, then the refused question.
	r = call(m, "port_kill", map[string]string{"PATH": bin, "GASH_HEADLESS": "1"}, nil, "", "8080")
	require.Equal(t, result{"", "Info: Listening on port 8080:\n  100  node\nInfo: Aborted.\n", 1}, r)

	for _, p := range []string{"0", "70000", "80x", "-"} {
		r = call(m, "port_kill", env, nil, "", "--", p)
		require.Equal(t, 1, r.rc, p)
	}
	r = call(m, "port_kill", env, nil, "", "70000")
	require.Equal(t, "Error: Invalid port: 70000 (expected 1-65535).\n", r.err)
	r = call(m, "port_kill", env, nil, "", "-y")
	require.Equal(t, "Error: Missing port. Usage: port_kill [-y] <port>\n", r.err)
	r = call(m, "port_kill", map[string]string{"PATH": t.TempDir()}, nil, "", "80")
	require.Equal(t, result{"", "Error: This function requires 'lsof', which is not available.\n", 1}, r)
}

// SYS-03: only unconnected UDP sockets bound to PORT are targeted, not a
// client whose remote port is PORT (3.0: SYS-03 fixed).
func TestLedger_SYS03(t *testing.T) {
	bin := binDir(t, "lsof")
	m, k, _ := tm(t, runner.NewFake(t,
		lsof("18053", "", ""), lsofUDP("18053", "p777\nf3\nn127.0.0.1:40000->127.0.0.1:18053\n"),
		lsof("18053", "", ""), lsofUDP("18053", "p777\nf3\nn127.0.0.1:40000->127.0.0.1:18053\np888\nf4\nn*:18053\nf5\nn[::1]:18053\np999\nf3\nn*:180530\n"),
		runner.Expect{Argv: []string{"ps", "-o", "comm=", "-p", "888"}, Stdout: "dnsmasq\n"}))
	r := call(m, "port_kill", map[string]string{"PATH": bin}, nil, "", "-y", "18053")
	require.Equal(t, result{"", "Error: Nothing is listening on port 18053.\n", 1}, r)
	require.Empty(t, k.sent)
	r = call(m, "port_kill", map[string]string{"PATH": bin}, nil, "", "-y", "18053")
	require.Equal(t, result{"", "Info: Listening on port 18053:\n  888  dnsmasq\nInfo: Process 888 on port 18053 terminated.\n", 0}, r)
	require.Equal(t, "TERM 888", k.sent[0])
}

// SYS-33: the port is a decimal number without wraparound.
func TestLedger_SYS33(t *testing.T) {
	bin := binDir(t, "lsof")
	m, _, _ := tm(t, runner.NewFake(t, lsof("18080", "", ""), lsofUDP("18080", "")))
	r := call(m, "port_kill", map[string]string{"PATH": bin}, nil, "", "18446744073709551696")
	require.Equal(t, result{"", "Error: Invalid port: 18446744073709551696 (expected 1-65535).\n", 1}, r)
	r = call(m, "port_kill", map[string]string{"PATH": bin}, nil, "", "018080")
	require.Equal(t, result{"", "Error: Nothing is listening on port 18080.\n", 1}, r)
}

func TestSortUN(t *testing.T) {
	require.Equal(t, []string{"7", "10", "300"}, sortUN("300\n10\n7\n10\n"))
	require.Empty(t, sortUN(""))
}

var _ = io.Discard
