// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runner

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A session's input and output are pipes the caller drives while the
// child runs; standard error is captured unless a writer takes it.
func TestStartSession(t *testing.T) {
	s, err := Exec{}.StartSession(context.Background(), helperCmd(t, "stdin"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(s.Stdin(), "hello\n"); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(s.Stdout()).ReadString('\n')
	if err != nil || line != "hello\n" {
		t.Fatalf("echo: %q %v", line, err)
	}
	s.Stdin().Close()
	if res, err := s.Wait(); err != nil || res.ExitCode != 0 {
		t.Fatalf("wait: %+v %v", res, err)
	}
	if res, err := s.Wait(); err != nil || res.ExitCode != 0 {
		t.Fatalf("second wait: %+v %v", res, err)
	}

	s, err = Exec{}.StartSession(context.Background(), helperCmd(t, "out"))
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(s.Stdout())
	res, err := s.Wait()
	if err != nil || string(out) != "to stdout" || string(res.Stderr) != "to stderr" {
		t.Fatalf("captured stderr: %q %+v %v", out, res, err)
	}
	var errw bytes.Buffer
	c := helperCmd(t, "out")
	c.Stderr = &errw
	s, err = Exec{}.StartSession(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(s.Stdout())
	if res, err := s.Wait(); err != nil || res.Stderr != nil || errw.String() != "to stderr" {
		t.Fatalf("stderr writer: %+v %q %v", res, errw.String(), err)
	}

	// Timeout bounds the whole session.
	c = helperCmd(t, "sleep")
	c.Timeout = 100 * time.Millisecond
	s, err = Exec{}.StartSession(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(s.Stdout())
	if res, err := s.Wait(); err != nil || !res.TimedOut || res.Signal != syscall.SIGTERM {
		t.Fatalf("timeout: %+v %v", res, err)
	}

	// The refusals and start errors of Run.
	if _, err := (Exec{}).StartSession(context.Background(), Cmd{Path: "sh", Args: []string{"-c", "x"}}); !errors.Is(err, ErrShellCommand) {
		t.Errorf("shell string: %v", err)
	}
	var se *StartError
	if _, err := (Exec{}).StartSession(context.Background(), Cmd{Path: "/nonexistent/x"}); !errors.As(err, &se) || se.Status != 127 {
		t.Errorf("missing program: %v", err)
	}
}

// statFields are the fields of /proc/self/stat after the command name, as
// cat prints them for itself: [0] is the state, [3] the session ID.
func statFields(t *testing.T, out []byte) (pid int, fields []string) {
	t.Helper()
	s := string(out)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		t.Fatalf("stat: %q", s)
	}
	pid, _ = strconv.Atoi(strings.Fields(s[:i])[0])
	return pid, strings.Fields(s[i+1:])
}

// Setsid makes the child a session leader; without it the child stays in
// the caller's session, as before the option existed.
func TestSetsid(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}
	cat := "/bin/cat"
	if _, err := os.Stat(cat); err != nil {
		t.Skip("no /bin/cat")
	}
	for _, setsid := range []bool{false, true} {
		res, err := Exec{}.Run(context.Background(), Cmd{Path: cat, Args: []string{"/proc/self/stat"}, Options: Options{Setsid: setsid}})
		if err != nil || res.ExitCode != 0 {
			t.Fatalf("setsid=%t: %+v %v", setsid, res, err)
		}
		pid, f := statFields(t, res.Stdout)
		sid, _ := strconv.Atoi(f[3])
		if (sid == pid) != setsid {
			t.Errorf("setsid=%t: pid %d session %d", setsid, pid, sid)
		}
	}
	s, err := Exec{}.StartSession(context.Background(), Cmd{Path: cat, Args: []string{"/proc/self/stat"}, Options: Options{Setsid: true}})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(s.Stdout())
	if _, err := s.Wait(); err != nil {
		t.Fatal(err)
	}
	if pid, f := statFields(t, out); f[3] != strconv.Itoa(pid) {
		t.Errorf("session: pid %d session %s", pid, f[3])
	}
}

// Credential runs the child as another user (root only).
func TestCredential(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	id := "/usr/bin/id"
	if _, err := os.Stat(id); err != nil {
		t.Skip("no /usr/bin/id")
	}
	cred := &Credential{UID: 65534, GID: 65534, Groups: []uint32{65534}}
	res, err := Exec{}.Run(context.Background(), Cmd{Path: id, Args: []string{"-u"}, Options: Options{Credential: cred, Dir: "/"}})
	if err != nil || strings.TrimSpace(string(res.Stdout)) != "65534" {
		t.Fatalf("id -u: %+v %v", res, err)
	}
	res, err = Exec{}.Run(context.Background(), Cmd{Path: id, Args: []string{"-G"}, Options: Options{Credential: cred, Dir: "/"}})
	if err != nil || strings.TrimSpace(string(res.Stdout)) != "65534" {
		t.Fatalf("id -G: %+v %v", res, err)
	}
}

// FileCapture: a grandchild holding the output does not hold Run, and
// the output is read back from the file.
func TestFileCapture(t *testing.T) {
	c := helperCmd(t, "daemon")
	c.FileCapture = true
	t0 := time.Now()
	res, err := Exec{}.Run(context.Background(), c)
	if err != nil || res.ExitCode != 0 || string(res.Stdout) != "parent done" {
		t.Fatalf("daemon: %+v %v", res, err)
	}
	if d := time.Since(t0); d > 900*time.Millisecond {
		t.Errorf("held for %v", d)
	}
	c = helperCmd(t, "out")
	c.FileCapture = true
	var errw bytes.Buffer
	c.Stderr = &errw
	res, err = Exec{}.Run(context.Background(), c)
	if err != nil || string(res.Stdout) != "to stdout" || res.Stderr != nil || errw.String() != "to stderr" {
		t.Fatalf("out: %+v %q %v", res, errw.String(), err)
	}
}

// The Fake plays a session through Serve and records its options; Respond
// answers starts in any order before the expectations.
func TestFakeSessionAndRespond(t *testing.T) {
	cred := &Credential{UID: 1000, GID: 1000}
	f := NewFake(t, Expect{Session: true, Argv: []string{"/usr/bin/sudo", "x"}, Stderr: "bye", Status: 3,
		Serve: func(in io.Reader, out io.Writer) error {
			line, err := bufio.NewReader(in).ReadString('\n')
			if err != nil {
				return err
			}
			_, err = io.WriteString(out, "got "+line)
			return err
		}})
	s, err := f.StartSession(context.Background(), Cmd{Path: "/usr/bin/sudo", Args: []string{"x"}, Options: Options{Setsid: true, Credential: cred}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(s.Stdin(), "hi\n"); err != nil {
		t.Fatal(err)
	}
	s.Stdin().Close()
	out, _ := io.ReadAll(s.Stdout())
	res, err := s.Wait()
	if err != nil || string(out) != "got hi\n" || res.ExitCode != 3 || string(res.Stderr) != "bye" {
		t.Fatalf("session: %q %+v %v", out, res, err)
	}
	if c := f.Calls()[0]; !c.Session || !c.Setsid || c.Credential != cred {
		t.Errorf("recorded: %+v", c)
	}
	// A Serve error ends the session with status 1.
	f.Expect(Expect{Session: true, Argv: []string{"/bin/x"}, Serve: func(io.Reader, io.Writer) error { return errors.New("refused") }})
	s, err = f.StartSession(context.Background(), Cmd{Path: "/bin/x"})
	if err != nil {
		t.Fatal(err)
	}
	s.Stdin().Close()
	if res, _ := s.Wait(); res.ExitCode != 1 || string(res.Stderr) != "refused" {
		t.Errorf("serve error: %+v", res)
	}

	f.Respond(func(c Call) (Expect, bool) {
		if c.Argv[0] != "/usr/bin/php" {
			return Expect{}, false
		}
		return Expect{Stdout: "PHP " + c.Argv[1]}, true
	})
	f.Expect(Expect{Argv: []string{"/usr/bin/apachectl", "-t"}, Status: 1})
	for _, v := range []string{"-v", "-i"} {
		if res, err := f.Run(context.Background(), Cmd{Path: "/usr/bin/php", Args: []string{v}}); err != nil || string(res.Stdout) != "PHP "+v {
			t.Errorf("respond %s: %+v %v", v, res, err)
		}
	}
	if res, err := f.Run(context.Background(), Cmd{Path: "/usr/bin/apachectl", Args: []string{"-t"}}); err != nil || res.ExitCode != 1 {
		t.Errorf("declined start, expectation: %+v %v", res, err)
	}
	quiet := NewFake(nil).Respond(func(Call) (Expect, bool) { return Expect{}, true })
	if _, err := quiet.Run(context.Background(), Cmd{Path: "sh", Args: []string{"-c", "x"}}); !errors.Is(err, ErrShellCommand) {
		t.Errorf("respond keeps the refusals: %v", err)
	}
	if _, err := quiet.StartSession(context.Background(), Cmd{Path: "bash", Args: []string{"-c", "x"}}); !errors.Is(err, ErrShellCommand) {
		t.Errorf("session refusal: %v", err)
	}
}
