// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

// design/architecture.md 10: SIGINT and SIGTERM cancel the handler's
// context and reach the child processes; the exit status is 128+N, what a
// 2.x function killed by signal N returned.
//
// The binary runs docker_stop_all with a fake docker on PATH that records
// its PID, then waits; the test signals gash once the child runs:
//
//   - SIGTERM to gash alone (kill PID): the runner passes it on to the
//     child (its cancel sends SIGTERM); gash exits 143.
//   - SIGINT to the whole process group (Ctrl-C at a terminal): gash and
//     the child both get it; gash exits 130.
//   - SIGINT to gash alone (kill -INT PID): the child is ended through
//     the canceled context; gash exits 130.
//
// In every case the child has exited when gash has, so no process is left
// behind.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const fakeDocker = `#!/bin/sh
trap 'echo TERM >>"$GASH_SIGNAL_MARK"; kill "$child" 2>/dev/null; exit 143' TERM
trap 'echo INT >>"$GASH_SIGNAL_MARK"; kill "$child" 2>/dev/null; exit 130' INT
sleep 60 &
child=$!
echo $$ >"$GASH_SIGNAL_MARK.pid"
wait "$child"
`

func TestSignalExitStatusAndChildren(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("gash runs on linux and darwin only")
	}
	bin := releaseBinary(t)
	cases := []struct {
		name   string
		sig    syscall.Signal
		group  bool   // signal the process group, as a terminal does
		status int    // gash's exit status
		child  string // the first signal the child recorded
	}{
		{"SIGTERM to gash", syscall.SIGTERM, false, 143, "TERM"},
		{"SIGINT to the group", syscall.SIGINT, true, 130, "INT"},
		{"SIGINT to gash", syscall.SIGINT, false, 130, "TERM"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			fakeBin := filepath.Join(dir, "bin")
			require.NoError(t, os.MkdirAll(fakeBin, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(fakeBin, "docker"), []byte(fakeDocker), 0o755))
			mark := filepath.Join(dir, "mark")

			cmd := exec.Command(bin, "docker_stop_all")
			cmd.Env = []string{
				"PATH=" + fakeBin + string(os.PathListSeparator) + os.Getenv("PATH"),
				"HOME=" + dir, "LANG=C", "GASH_HEADLESS=1",
				"GASH_SIGNAL_MARK=" + mark,
			}
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			require.NoError(t, cmd.Start())
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })

			childPID := waitPID(t, mark+".pid", done)
			if tc.group {
				require.NoError(t, syscall.Kill(-cmd.Process.Pid, tc.sig))
			} else {
				require.NoError(t, cmd.Process.Signal(tc.sig))
			}

			var err error
			select {
			case err = <-done:
			case <-time.After(15 * time.Second):
				t.Fatal("gash did not exit after the signal")
			}
			var exitErr *exec.ExitError
			require.True(t, errors.As(err, &exitErr), "gash exited 0 (%v)", err)
			ws := exitErr.Sys().(syscall.WaitStatus)
			require.False(t, ws.Signaled(), "gash was killed by %v instead of exiting", ws.Signal())
			require.Equal(t, tc.status, ws.ExitStatus())

			got, err := os.ReadFile(mark)
			require.NoError(t, err, "the child got no signal")
			// The first signal the child saw. After a Ctrl-C the canceled
			// context may also send it SIGTERM while its INT trap runs.
			first, _, _ := strings.Cut(string(got), "\n")
			require.Equal(t, tc.child, first)
			// The child is gone, not left running (it is reaped by gash).
			require.Eventually(t, func() bool {
				return syscall.Kill(childPID, 0) != nil
			}, 5*time.Second, 20*time.Millisecond, "the child %d is still running", childPID)
		})
	}
}

// waitPID waits until the fake child wrote its PID to path.
func waitPID(t *testing.T, path string, done <-chan error) int {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("gash exited before its child started: %v", err)
		default:
		}
		if b, err := os.ReadFile(path); err == nil && strings.HasSuffix(string(b), "\n") {
			pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
			require.NoError(t, err)
			return pid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the fake docker never started")
	return 0
}
