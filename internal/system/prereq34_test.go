// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
)

// debian is Debian 12 for root (so an install command has no sudo) with
// every program but missing.
func debian(missing ...string) *prereq.System {
	return &prereq.System{OS: platform.OS{GOOS: "linux", ID: "debian", Name: "Debian GNU/Linux 12 (bookworm)"},
		Manager: prereq.Apt, Root: true, Sudo: !slices.Contains(missing, "sudo"),
		LookPath: func(name string, _ []string) (string, bool) {
			return "/usr/bin/" + name, !slices.Contains(missing, name)
		}}
}

// notFound is the start error of a program that is not on PATH.
func notFound(name string) error {
	return &runner.StartError{Name: name, Status: 127, Err: errors.New("not found")}
}

// A missing ps or pgrep is the prerequisite report, not "No process
// found" (3.4).
func TestProcessToolsMissing(t *testing.T) {
	defer prereq.SetSystem(debian("ps", "pgrep"))()
	m, _, _ := tm(t, runner.NewFake(t,
		runner.Expect{Argv: []string{"ps", "-eo", "pid=,user=,pcpu=,args="}, Err: notFound("ps")},
		runner.Expect{Argv: []string{"pgrep", "-ix", "--", "nginx"}, Err: notFound("pgrep")},
	))
	r := call(m, "process_find", nil, nil, "", "nginx")
	require.Equal(t, result{"", "Error: process_find needs ps, which is not installed.\n" +
		"  ps lists the running processes (package procps on Debian GNU/Linux 12).\n" +
		"  Install it:  apt-get install -y procps\n" +
		"  Or run:      gash_doctor --install ps\n", 1}, r)
	r = call(m, "process_kill", nil, nil, "", "-y", "nginx")
	require.Equal(t, result{"", "Error: process_kill needs pgrep, which is not installed.\n" +
		"  pgrep finds processes by name (package procps on Debian GNU/Linux 12).\n" +
		"  Install it:  apt-get install -y procps\n" +
		"  Or run:      gash_doctor --install pgrep\n", 1}, r)
}

// On a terminal sudo is not probed first: the first stop that finds no
// sudo reports it once and stops (3.4: one "command not found" per
// service).
func TestServicesStopWithoutSudo(t *testing.T) {
	defer prereq.SetSystem(debian("sudo"))()
	f := runner.NewFake(t, isActive("nginx", "redis")...)
	f.Expect(runner.Expect{Argv: []string{"sudo", "systemctl", "stop", "nginx"}, Err: notFound("sudo")})
	m, _, _ := tm(t, f)
	r := call(m, "services_stop", svcEnv(t, true, "systemctl"), ctty(), "", "-y")
	require.Equal(t, result{"", "Info: Running: nginx redis\nInfo: Stopping nginx...\n" +
		"Error: services_stop needs sudo, which is not installed.\n" +
		"  sudo runs the privileged steps of some functions (package sudo on Debian GNU/Linux 12).\n" +
		"  Install it:  apt-get install -y sudo\n" +
		"  Or run:      gash_doctor --install sudo\n", 1}, r)
}

// wsl_compact: a missing dmesg is said as such (3.4: "dmesg not
// readable"), and a missing sudo is the prerequisite report.
func TestWSLCompactMissingTools(t *testing.T) {
	defer prereq.SetSystem(debian("dmesg", "sudo"))()
	m, _, _ := tm(t, runner.NewFake(t,
		runner.Expect{Argv: []string{"dmesg"}, Err: notFound("dmesg")},
		runner.Expect{Argv: []string{"dmesg", "-T"}, Err: notFound("dmesg")},
		runner.Expect{Script: runner.ScriptWSLCompact,
			Argv: []string{"sudo", "sh", "-c", "sync && echo 3 > /proc/sys/vm/drop_caches && echo 1 > /proc/sys/vm/compact_memory"},
			Err:  notFound("sudo")},
	))
	wslRoot(t, m, true, false)
	r := call(m, "wsl_compact", nil, ctty(), "")
	require.Equal(t, result{"order-7 allocation failures since the VM started: unknown (dmesg is not installed)\n",
		"Info: Dropping the page cache and slab, then compacting memory (sudo)...\n" +
			"Error: wsl_compact needs sudo, which is not installed.\n" +
			"  sudo runs the privileged steps of some functions (package sudo on Debian GNU/Linux 12).\n" +
			"  Install it:  apt-get install -y sudo\n" +
			"  Or run:      gash_doctor --install sudo\n", 1}, r)
}
