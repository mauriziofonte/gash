// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"io"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

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
	return runner.Expect{Argv: []string{"pgrep", "-ix", name}, Stdout: out, Status: status}
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

// SYS-01: NAME is pgrep's anchored regex, passed as it is; only the
// binary's own PID is left out of the matches.
func TestLedger_SYS01(t *testing.T) {
	m, k, _ := tm(t, runner.NewFake(t, pgrep(".*", "1\n2\n4242\n", 0), pgrep(".*", "4242\n", 0)))
	r := call(m, "process_kill", map[string]string{"GASH_HEADLESS": "1"}, nil, "", ".*")
	require.Equal(t, result{"", "Error: 2 process(es) match '.*' (PID 1 2). Refusing to kill without a terminal to confirm; pass -y.\n", 1}, r)
	require.Empty(t, k.sent)
	r = call(m, "process_kill", map[string]string{"GASH_HEADLESS": "1"}, nil, "", ".*")
	require.Equal(t, 1, r.rc)
	require.Contains(t, r.err, "No process found with exact name '.*'")
}

// SYS-02: a process the user may not signal is reported as terminated.
func TestLedger_SYS02(t *testing.T) {
	m, k, _ := tm(t, runner.NewFake(t, pgrep("root-proc", "1\n", 0)))
	k.refuse = syscall.EPERM
	r := call(m, "process_kill", nil, nil, "", "-y", "root-proc")
	require.Equal(t, result{"", "Info: Process 1 terminated.\n", 0}, r)
	require.Equal(t, []string{"TERM 1", "0 1", "0 1"}, k.sent)
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

func lsofUDP(port, udp string) runner.Expect {
	return runner.Expect{Argv: []string{"lsof", "-t", "-nP", "-iUDP:" + port}, Stdout: udp}
}

func TestPortKill(t *testing.T) {
	bin := binDir(t, "lsof")
	env := map[string]string{"PATH": bin}
	m, k, _ := tm(t, runner.NewFake(t,
		lsof("8080", "300\n100\n", ""), lsofUDP("8080", "100\n"),
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

// SYS-03: the UDP query also lists a client whose remote port is PORT.
func TestLedger_SYS03(t *testing.T) {
	bin := binDir(t, "lsof")
	m, k, _ := tm(t, runner.NewFake(t, lsof("18053", "", ""), lsofUDP("18053", "777\n"),
		runner.Expect{Argv: []string{"ps", "-o", "comm=", "-p", "777"}, Stdout: "python3\n"}))
	r := call(m, "port_kill", map[string]string{"PATH": bin}, nil, "", "-y", "18053")
	require.Equal(t, result{"", "Info: Listening on port 18053:\n  777  python3\nInfo: Process 777 on port 18053 terminated.\n", 0}, r)
	require.Equal(t, "TERM 777", k.sent[0])
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
