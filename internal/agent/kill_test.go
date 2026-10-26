// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/proctab"
	"github.com/mauriziofonte/gash/internal/term"
)

// killTable: the gash process 200 (uid 1000) runs from the shell 100,
// whose subshell 500 has the same command line; 300 sleeps with a zombie
// and a tail; 400 is root's sleep.
const killPSMain = `    1     0     0 Ss   /sbin/init
  100     1  1000 Ss   bash -c x; agent_kill sleep
  200   100  1000 S    /usr/bin/gash agent_kill sleep
  201   200  1000 R    ps -A -o pid=,ppid=,uid=,stat=,args=
  300   100  1000 S    sleep 30
  301   300  1000 Z    [sleep] <defunct>
  302   300  1000 S    tail -f /var/log/app.log
  400     1     0 S    sleep 30
  500   100  1000 S    bash -c x; agent_kill sleep
`

const killPSComm = `    1 init
  100 bash
  200 gash
  201 ps
  300 sleep
  301 sleep
  302 tail
  400 sleep
  500 bash
`

type sent struct {
	pid int
	sig syscall.Signal
}

// fakeKiller is agent_kill over killTable; errs makes a PID's signal fail.
func fakeKiller(errs map[int]error) (*killer, *[]sent) {
	var got []sent
	return &killer{
		table: func(*invoke.Call) (*proctab.Table, error) {
			return proctab.ParsePS([]byte(killPSMain), []byte(killPSComm)), nil
		},
		getpid: func() int { return 200 },
		signal: func(pid int, sig syscall.Signal) error {
			got = append(got, sent{pid, sig})
			return errs[pid]
		},
	}, &got
}

func runKiller(k *killer, args ...string) result {
	var out, errb bytes.Buffer
	env := invoke.EnvMap(map[string]string{"HOME": "/home/u", "GASH_HEADLESS": "1"})
	c := &invoke.Call{Ctx: context.Background(), Name: "agent_kill", Args: args, Env: env,
		Stdout: &out, Stderr: &errb, Term: term.Fixed(env, false, false, false)}
	rc := k.run(c)
	return result{out.String(), errb.String(), rc}
}

func TestKill(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
		sent []sent
	}{
		// A NAME: the caller's shell (100), its subshell (500), the zombie
		// and root's sleep (400) never match.
		{[]string{"sleep"}, `{"killed":1,"pids":[300]}`, []sent{{300, syscall.SIGTERM}}},
		{[]string{"sleep", "--dry-run"}, `{"killed":1,"pids":[300],"cmds":["sleep 30"],"signal":"TERM","dry_run":true,"written":false}`, nil},
		{[]string{"^tail", "sleep 3", "--signal", "KILL"}, `{"killed":2,"pids":[302,300]}`, []sent{{302, syscall.SIGKILL}, {300, syscall.SIGKILL}}},
		// PIDs in the given order, once; gone and zombie ones skipped;
		// the subshell 500 is not an ancestor.
		{[]string{"302", "300", "302", "999", "301", "500", "-s", "hup"}, `{"killed":3,"pids":[302,300,500]}`,
			[]sent{{302, syscall.SIGHUP}, {300, syscall.SIGHUP}, {500, syscall.SIGHUP}}},
		{[]string{"--signal", "9", "--", "302"}, `{"killed":1,"pids":[302]}`, []sent{{302, syscall.SIGKILL}}},
		{[]string{"--signal", "SIGusr1", "302"}, `{"killed":1,"pids":[302]}`, []sent{{302, syscall.SIGUSR1}}},
		// Nothing matched is success.
		{[]string{"no-such-program"}, `{"killed":0,"pids":[]}`, nil},
		{[]string{"999"}, `{"killed":0,"pids":[]}`, nil},
	} {
		k, got := fakeKiller(nil)
		r := runKiller(k, tc.args...)
		require.Equal(t, result{tc.want + "\n", "", 0}, r, tc.args)
		require.Equal(t, tc.sent, *got, tc.args)
	}

	k, _ := fakeKiller(nil)
	r := runKiller(k, "sleep", "tail", "--format", "tsv", "--dry-run")
	require.Equal(t, result{"pid\tname\tcmd\n300\tsleep\tsleep 30\n302\ttail\ttail -f /var/log/app.log\n",
		`{"dry_run":true,"killed":2,"signal":"TERM","written":false}` + "\n", 0}, r)
}

func TestKillRefusals(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"1"}, "kill_refused/FATAL"},
		{[]string{"100"}, "kill_refused/FATAL"},
		{[]string{"200"}, "kill_refused/FATAL"},
		{[]string{"302", "400"}, "kill_refused/FATAL"},
		{nil, "missing_target/RETRY"},
		{[]string{"--dry-run"}, "missing_target/RETRY"},
		{[]string{".*"}, "pattern_too_broad/RETRY"},
		{[]string{"x*"}, "pattern_too_broad/RETRY"},
		{[]string{""}, "pattern_too_broad/RETRY"},
		{[]string{"("}, "invalid_regex/RETRY"},
		{[]string{"0"}, "invalid_pid/RETRY"},
		{[]string{"1234567890"}, "invalid_pid/RETRY"},
		{[]string{"x", "--signal"}, "missing_value/RETRY"},
		{[]string{"x", "--signal", "0"}, "invalid_signal/RETRY"},
		{[]string{"x", "--signal", "65"}, "invalid_signal/RETRY"},
		{[]string{"x", "--signal", "BOGUS"}, "invalid_signal/RETRY"},
		{[]string{"x", "--signal", ""}, "invalid_signal/RETRY"},
		{[]string{"x", "--all"}, "unknown_option/RETRY"},
		{[]string{"x", "--format", "xml"}, "invalid_format/RETRY"},
	} {
		k, got := fakeKiller(nil)
		r := runKiller(k, tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Empty(t, r.Stdout, tc.args)
		require.Equal(t, tc.code, code(t, r), tc.args)
		require.Empty(t, *got, "nothing is signaled after a refusal: %v", tc.args)
	}

	// A process that exits before the signal is not counted; one the
	// kernel refuses is kill_failed after the others were signaled.
	k, got := fakeKiller(map[int]error{300: syscall.ESRCH, 302: syscall.EPERM})
	r := runKiller(k, "300", "302", "500")
	require.Equal(t, `{"killed":1,"pids":[500]}`+"\n", r.Stdout)
	require.Equal(t, "kill_failed/STOP", code(t, r))
	require.Contains(t, r.Stderr, `"details":"302: operation not permitted"`)
	require.Equal(t, 1, r.RC)
	require.Len(t, *got, 3)

	// No process table, or the caller missing from it.
	k, _ = fakeKiller(nil)
	k.table = func(*invoke.Call) (*proctab.Table, error) { return nil, errors.New("no ps") }
	require.Equal(t, "ps_not_found/CONTINUE", code(t, runKiller(k, "x")))
	k, _ = fakeKiller(nil)
	k.getpid = func() int { return 77 }
	require.Equal(t, "ps_not_found/CONTINUE", code(t, runKiller(k, "x")))
}

func TestParseSignal(t *testing.T) {
	for in, want := range map[string]syscall.Signal{
		"TERM": syscall.SIGTERM, "sigterm": syscall.SIGTERM, "15": syscall.SIGTERM, "KILL": syscall.SIGKILL,
		"int": syscall.SIGINT, "2": syscall.SIGINT, "CONT": syscall.SIGCONT, "STOP": syscall.SIGSTOP,
	} {
		s, name, ok := parseSignal(in)
		require.True(t, ok, in)
		require.Equal(t, want, s, in)
		require.NotEmpty(t, name, in)
	}
	_, name, _ := parseSignal("9")
	require.Equal(t, "KILL", name)
	for _, bad := range []string{"0", "-1", "99", "SIG", "TERMX", "1.5"} {
		_, _, ok := parseSignal(bad)
		require.False(t, ok, bad)
	}
}

// TestKillReal signals a real process, a grandchild of this test (the
// child bash is the caller's own chain), by PID and by NAME.
func TestKillReal(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep is not installed")
	}
	for _, byName := range []bool{false, true} {
		secs := "291." + strconv.Itoa(os.Getpid()%97)
		if byName {
			secs += "1"
		}
		cmd := exec.Command("bash", "-c", "sleep "+secs+" & echo $!; wait")
		out, err := cmd.StdoutPipe()
		require.NoError(t, err)
		require.NoError(t, cmd.Start())
		buf := make([]byte, 32)
		n, err := out.Read(buf)
		require.NoError(t, err)
		pid := strings.TrimSpace(string(buf[:n]))
		// $! is known at fork: wait until the child is sleep, not bash.
		for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
			args, _ := exec.Command("ps", "-o", "args=", "-p", pid).Output()
			if strings.HasPrefix(strings.TrimSpace(string(args)), "sleep ") || time.Now().After(deadline) {
				break
			}
		}
		target := pid
		if byName {
			target = "^sleep " + strings.ReplaceAll(secs, ".", `\.`) + "$"
		}
		d := physDir(t)
		r := runIn(t, d, "agent_kill", target, "--dry-run")
		require.Equal(t, `{"killed":1,"pids":[`+pid+`],"cmds":["sleep `+secs+`"],"signal":"TERM","dry_run":true,"written":false}`+"\n", r.Stdout, r.Stderr)
		r = runIn(t, d, "agent_kill", target)
		require.Equal(t, result{`{"killed":1,"pids":[` + pid + `]}` + "\n", "", 0}, r)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			t.Fatal("the victim still runs")
		}
	}
	// The test process itself is the caller.
	r := runIn(t, physDir(t), "agent_kill", strconv.Itoa(os.Getpid()))
	require.Equal(t, "kill_refused/FATAL", code(t, r))
}
