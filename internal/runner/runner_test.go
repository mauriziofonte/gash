// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The test binary doubles as the child process: with RUNNER_HELPER in its
// environment it runs a helper instead of the tests, so no system tool is
// needed.
func TestMain(m *testing.M) {
	if mode := os.Getenv("RUNNER_HELPER"); mode != "" {
		os.Exit(helper(mode, os.Args[1:]))
	}
	os.Exit(m.Run())
}

func helper(mode string, args []string) int {
	switch mode {
	case "argv":
		fmt.Print(strings.Join(os.Args, "\x00"))
	case "env":
		fmt.Print(strings.Join(os.Environ(), "\n"))
	case "stdin":
		_, _ = io.Copy(os.Stdout, os.Stdin)
	case "pwd":
		wd, _ := os.Getwd()
		fmt.Print(wd)
	case "out":
		fmt.Fprint(os.Stdout, "to stdout")
		fmt.Fprint(os.Stderr, "to stderr")
	case "exit":
		n, _ := strconv.Atoi(args[0])
		return n
	case "signal":
		n, _ := strconv.Atoi(args[0])
		_ = syscall.Kill(os.Getpid(), syscall.Signal(n))
		time.Sleep(10 * time.Second)
	case "sleep":
		time.Sleep(10 * time.Second)
	case "hold-output":
		// Keeps the inherited output descriptor open well past the race
		// runtime's fixed fork/exec overhead, so TestFileCapture can tell a
		// capture that wrongly blocked on the grandchild from that overhead.
		time.Sleep(3 * time.Second)
	case "ignore-term":
		signal.Ignore(syscall.SIGTERM)
		fmt.Print("ready")
		time.Sleep(10 * time.Second)
	case "touch":
		time.Sleep(200 * time.Millisecond)
		_ = os.WriteFile(args[0], []byte("started"), 0o600)
	case "prompt":
		fmt.Print("Passphrase: ")
		var line string
		_, _ = fmt.Scanln(&line)
		fmt.Printf("got %s\n", line)
		return 3
	case "daemon":
		// A grandchild keeps stdout open after this process exits.
		cmd := exec.Command(os.Args[0])
		cmd.Env = []string{"RUNNER_HELPER=hold-output"}
		cmd.Stdout = os.Stdout
		_ = cmd.Start()
		fmt.Print("parent done")
	}
	return 0
}

func self(t *testing.T) string {
	t.Helper()
	p, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func helperCmd(t *testing.T, mode string, args ...string) Cmd {
	return Cmd{Path: self(t), Args: args, Options: Options{Env: []string{"RUNNER_HELPER=" + mode}}}
}

// Arguments reach the child as they are: no shell parses them.
func TestExplicitArgv(t *testing.T) {
	args := []string{"a b", `"q"`, "$HOME", "x; rm -rf /", "*", "", "'s'", "\\n", "é"}
	res, err := Exec{}.Run(context.Background(), helperCmd(t, "argv", args...))
	if err != nil || !res.Success() {
		t.Fatalf("Run: %+v %v", res, err)
	}
	got := strings.Split(string(res.Stdout), "\x00")
	if want := append([]string{self(t)}, args...); !slices.Equal(got, want) {
		t.Errorf("argv = %q, want %q", got, want)
	}
}

// The child gets exactly Options.Env: nothing of the parent leaks.
func TestExplicitEnvironment(t *testing.T) {
	t.Setenv("RUNNER_PARENT_SECRET", "s3cret")
	c := helperCmd(t, "env")
	c.Env = append(c.Env, "A=1", "B=two words", "A=dup", "MALFORMED")
	res, err := Exec{}.Run(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(string(res.Stdout), "\n")
	if want := []string{"RUNNER_HELPER=env", "A=1", "B=two words"}; !slices.Equal(got, want) {
		t.Errorf("child environment = %q, want %q", got, want)
	}
}

// Secrets and SQL travel on stdin; nil stdin is /dev/null.
func TestStdin(t *testing.T) {
	c := helperCmd(t, "stdin")
	c.Stdin = strings.NewReader("password\nSELECT 1;\n")
	res, err := Exec{}.Run(context.Background(), c)
	if err != nil || string(res.Stdout) != "password\nSELECT 1;\n" {
		t.Errorf("stdin copy = %q, %v", res.Stdout, err)
	}
	res, err = Exec{}.Run(context.Background(), helperCmd(t, "stdin"))
	if err != nil || len(res.Stdout) != 0 {
		t.Errorf("nil stdin gave %q, %v", res.Stdout, err)
	}
}

func TestOutputCaptureAndWriters(t *testing.T) {
	res, err := Exec{}.Run(context.Background(), helperCmd(t, "out"))
	if err != nil || string(res.Stdout) != "to stdout" || string(res.Stderr) != "to stderr" {
		t.Errorf("captured %q %q %v", res.Stdout, res.Stderr, err)
	}
	var out, errb strings.Builder
	c := helperCmd(t, "out")
	c.Stdout, c.Stderr = &out, &errb
	res, err = Exec{}.Run(context.Background(), c)
	if err != nil || out.String() != "to stdout" || errb.String() != "to stderr" || res.Stdout != nil || res.Stderr != nil {
		t.Errorf("writers %q %q, result %q %q, %v", out.String(), errb.String(), res.Stdout, res.Stderr, err)
	}
}

func TestDir(t *testing.T) {
	dir := t.TempDir()
	c := helperCmd(t, "pwd")
	c.Dir = dir
	res, err := Exec{}.Run(context.Background(), c)
	want, _ := filepath.EvalSymlinks(dir)
	if got, _ := filepath.EvalSymlinks(string(res.Stdout)); err != nil || got != want {
		t.Errorf("pwd = %q, want %q (%v)", res.Stdout, want, err)
	}
}

// Exit statuses come back as Bash's $?: the status, or 128+N for signal N.
// A non-zero status is no error.
func TestExitStatus(t *testing.T) {
	res, err := Exec{}.Run(context.Background(), helperCmd(t, "exit", "3"))
	if err != nil || res.ExitCode != 3 || res.Signal != 0 || res.Success() || res.TimedOut {
		t.Errorf("exit 3: %+v %v", res, err)
	}
	res, err = Exec{}.Run(context.Background(), helperCmd(t, "signal", strconv.Itoa(int(syscall.SIGTERM))))
	if err != nil || res.ExitCode != 143 || res.Signal != syscall.SIGTERM {
		t.Errorf("SIGTERM: %+v %v", res, err)
	}
	res, err = Exec{}.Run(context.Background(), helperCmd(t, "signal", strconv.Itoa(int(syscall.SIGINT))))
	if err != nil || res.ExitCode != 130 || res.Signal != syscall.SIGINT {
		t.Errorf("SIGINT: %+v %v", res, err)
	}
}

func TestTimeout(t *testing.T) {
	c := helperCmd(t, "sleep")
	c.Timeout = 100 * time.Millisecond
	t0 := time.Now()
	res, err := Exec{}.Run(context.Background(), c)
	if err != nil || !res.TimedOut || res.Signal != syscall.SIGTERM || res.ExitCode != 143 || res.TimeoutStatus() != 124 {
		t.Errorf("timeout: %+v %v", res, err)
	}
	if d := time.Since(t0); d > 5*time.Second {
		t.Errorf("took %v", d)
	}

	// A child that ignores the signal gets SIGKILL after KillAfter.
	c = helperCmd(t, "ignore-term")
	c.Timeout = 300 * time.Millisecond
	c.KillAfter = 200 * time.Millisecond
	res, err = Exec{}.Run(context.Background(), c)
	if err != nil || !res.TimedOut || res.Signal != syscall.SIGKILL || res.ExitCode != 137 || res.TimeoutStatus() != 137 {
		t.Errorf("ignored SIGTERM: %+v %v", res, err)
	}

	// Another signal on timeout.
	c = helperCmd(t, "sleep")
	c.Timeout = 100 * time.Millisecond
	c.Signal = syscall.SIGINT
	res, err = Exec{}.Run(context.Background(), c)
	if err != nil || !res.TimedOut || res.Signal != syscall.SIGINT || res.ExitCode != 130 {
		t.Errorf("SIGINT on timeout: %+v %v", res, err)
	}

	// Done in time: not timed out.
	c = helperCmd(t, "exit", "0")
	c.Timeout = 10 * time.Second
	if res, err = (Exec{}).Run(context.Background(), c); err != nil || res.TimedOut || res.TimeoutStatus() != 0 {
		t.Errorf("in time: %+v %v", res, err)
	}
}

func TestContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	res, err := Exec{}.Run(ctx, helperCmd(t, "sleep"))
	if !errors.Is(err, context.Canceled) || res.Signal != syscall.SIGTERM || res.TimedOut {
		t.Errorf("cancel: %+v %v", res, err)
	}
	// A context already done starts nothing.
	res, err = Exec{}.Run(ctx, helperCmd(t, "exit", "0"))
	if !errors.Is(err, context.Canceled) || res.ExitCode == 0 {
		t.Errorf("done context: %+v %v", res, err)
	}
}

// A grandchild that keeps the output pipe open does not hang the caller.
func TestOpenPipesAfterExit(t *testing.T) {
	c := helperCmd(t, "daemon")
	c.KillAfter = 200 * time.Millisecond
	t0 := time.Now()
	res, err := Exec{}.Run(context.Background(), c)
	if !errors.Is(err, exec.ErrWaitDelay) || res.ExitCode != 0 || !strings.HasPrefix(string(res.Stdout), "parent done") {
		t.Errorf("open pipes: %+v %v", res, err)
	}
	if d := time.Since(t0); d > 5*time.Second {
		t.Errorf("took %v", d)
	}
}

// Programs are looked up in the PATH of the child environment, as Bash
// does; start failures give Bash's 127 and 126.
func TestLookPath(t *testing.T) {
	bin := t.TempDir()
	if err := os.Symlink(self(t), filepath.Join(bin, "gash-helper")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "noexec"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(bin, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{"RUNNER_HELPER=argv", "PATH=/nonexistent:" + bin}
	res, err := Exec{}.Run(context.Background(), Cmd{Path: "gash-helper", Args: []string{"x"}, Options: Options{Env: env}})
	if err != nil || string(res.Stdout) != "gash-helper\x00x" {
		t.Errorf("PATH lookup: %q %v", res.Stdout, err)
	}

	// An empty PATH entry is the working directory.
	res, err = Exec{}.Run(context.Background(), Cmd{Path: "gash-helper", Options: Options{Env: []string{"RUNNER_HELPER=argv", "PATH=/nonexistent:"}, Dir: bin}})
	if err != nil || string(res.Stdout) != "gash-helper" {
		t.Errorf("empty PATH entry: %q %v", res.Stdout, err)
	}

	tests := []struct {
		name, path string
		env        []string
		status     int
		text       string
	}{
		{"not in PATH", "gash-helper", []string{"PATH=/nonexistent"}, 127, "gash-helper: command not found"},
		{"no PATH in the child environment", "gash-helper", nil, 127, "gash-helper: command not found"},
		{"parent PATH not used", "gash-helper", []string{"PATH="}, 127, "gash-helper: command not found"},
		{"not executable in PATH", "noexec", []string{"PATH=" + bin}, 126, "noexec: Permission denied"},
		{"directory in PATH is skipped", "adir", []string{"PATH=" + bin}, 127, "adir: command not found"},
		{"missing path", bin + "/missing", nil, 127, bin + "/missing: No such file or directory"},
		{"directory path", bin + "/adir", nil, 126, bin + "/adir: Is a directory"},
		{"not executable path", bin + "/noexec", nil, 126, bin + "/noexec: Permission denied"},
		{"empty name", "", []string{"PATH=" + bin}, 127, ": command not found"},
	}
	for _, tt := range tests {
		res, err := Exec{}.Run(context.Background(), Cmd{Path: tt.path, Options: Options{Env: tt.env}})
		var se *StartError
		if !errors.As(err, &se) || se.Status != tt.status || res.ExitCode != tt.status || err.Error() != tt.text {
			t.Errorf("%s: %+v %v, want %d %q", tt.name, res, err, tt.status, tt.text)
		}
	}
}

func TestShellCommandRefused(t *testing.T) {
	refused := [][]string{
		{"sh", "-c", "echo hi"},
		{"/bin/bash", "-c", "x"},
		{"bash", "-ec", "x"},
		{"bash", "--noprofile", "--norc", "-c", "x"},
		{"bash", "-o", "pipefail", "-c", "x"},
		{"bash", "-O", "extglob", "-xc", "x"},
		{"bash", "--rcfile", "f", "-c", "x"},
		{"sudo", "sh", "-c", "sync"},
		{"sudo", "-u", "root", "/usr/bin/dash", "-c", "x"},
		{"env", "-i", "zsh", "-c", "x"},
		{"busybox", "sh", "-c", "x"},
		{"nice", "ksh", "-c", "x"},
		{"fish", "--command", "x"},
		{"fish", "--command=x"},
		{"su", "root", "-c", "x"},
		{"su", "-s", "/bin/sh", "-", "root", "-c", "x"},
		{"su", "--command=x"},
		{"sudo", "-E", "HOME=/root", "bash", "-c", "x"},
		{"sudo", "--user=root", "--", "sh", "-c", "x"},
		{"env", "-u", "PATH", "-C", "/tmp", "A=1", "bash", "-c", "x"},
		{"env", "-S", "sh -c x"},
		{"env", "--split-string=bash -ec", "x"},
		{"nice", "-n", "10", "sh", "-c", "x"},
		{"nice", "-10", "sh", "-c", "x"},
		{"timeout", "-s", "KILL", "5", "bash", "-c", "x"},
		{"stdbuf", "-oL", "nohup", "setsid", "dash", "-c", "x"},
		{"ionice", "-c", "3", "bash", "-c", "x"},
		{"xargs", "-0", "-n", "1", "sh", "-c", "x"},
		{"chroot", "/srv", "/bin/sh", "-c", "x"},
		{"sudo", "su", "-c", "x"},
	}
	for _, argv := range refused {
		_, err := Exec{}.Run(context.Background(), Cmd{Path: argv[0], Args: argv[1:]})
		if !errors.Is(err, ErrShellCommand) {
			t.Errorf("%q: %v, want ErrShellCommand", argv, err)
		}
	}
	allowed := [][]string{
		{"bash", "script.sh", "-c"},
		{"bash", "--", "-c"},
		{"bash", "--version"},
		{"grep", "-c", "bash", "file"},
		{"ssh", "host", "unset HISTFILE; bash -s --"},
		{"sh", "+c"},
		{"pgrep", "-c", "sh"},
		{"su", "-", "root"},
		{"git", "-c", "core.pager=cat", "log"},
		{"git", "tag", "-a", "bash", "-m", "-c"},
		{"git", "commit", "-m", "sh", "-c"},
		{"ssh", "host", "bash", "-c", "x"},
		{"rsync", "-e", "ssh", "sh", "-c"},
		{"sudo", "-u", "bash", "true", "-c"},
		{"env", "A=1", "git", "tag", "-a", "sh", "-m", "-c"},
		{"busybox", "--list"},
		{"timeout", "5"},
		{"env"},
	}
	for _, argv := range allowed {
		if err := checkNoShell(argv); err != nil {
			t.Errorf("%q refused: %v", argv, err)
		}
	}
}

func TestBashArgv(t *testing.T) {
	argv, err := Bash{Use: BashLLMExec, Script: "prelude\n'ls' '-la'"}.argv()
	if want := []string{"bash", "--noprofile", "--norc", "-c", "prelude\n'ls' '-la'"}; err != nil || !slices.Equal(argv, want) {
		t.Errorf("llm_exec argv = %q %v", argv, err)
	}
	wrap := []string{"bwrap", "--ro-bind", "/", "/", "--unshare-all", "--die-with-parent", "--chdir", "/w"}
	argv, err = Bash{Use: BashLLMExec, Wrap: wrap, Bash: "/usr/bin/bash", Script: "s"}.argv()
	if want := append(slices.Clone(wrap), "/usr/bin/bash", "--noprofile", "--norc", "-c", "s"); err != nil || !slices.Equal(argv, want) {
		t.Errorf("sandboxed argv = %q %v", argv, err)
	}
	argv, err = Bash{Use: BashNVMBridge, Args: []string{"ls", "--no-colors", "$(x)"}}.argv()
	if want := []string{"bash", "--noprofile", "--norc", "-c", NVMBridgeScript, "nvm", "ls", "--no-colors", "$(x)"}; err != nil || !slices.Equal(argv, want) {
		t.Errorf("nvm argv = %q %v", argv, err)
	}
	argv, err = Bash{Use: BashLLMExec, Script: "s", Args: []string{"a"}}.argv()
	if want := []string{"bash", "--noprofile", "--norc", "-c", "s", "bash", "a"}; err != nil || !slices.Equal(argv, want) {
		t.Errorf("positional argv = %q %v", argv, err)
	}

	argv, err = Bash{Use: BashDoctorLoad, Bash: "/bin/bash", Args: []string{"/home/u/.gash"}}.argv()
	if want := []string{"/bin/bash", "--noprofile", "--norc", "-i", "-c", DoctorLoadScript, "gash-timing", "/home/u/.gash"}; err != nil || !slices.Equal(argv, want) {
		t.Errorf("doctor load argv = %q %v", argv, err)
	}
	argv, err = Bash{Use: BashDoctorShell}.argv()
	if want := []string{"bash", "-i", "-c", "exit"}; err != nil || !slices.Equal(argv, want) {
		t.Errorf("doctor shell argv = %q %v", argv, err)
	}
	argv, err = Bash{Use: BashDoctorBare, Script: "exit"}.argv()
	if want := []string{"bash", "--noprofile", "--norc", "-i", "-c", "exit"}; err != nil || !slices.Equal(argv, want) {
		t.Errorf("doctor bare argv = %q %v", argv, err)
	}

	for _, b := range []Bash{
		{},
		{Use: BashLLMExec},
		{Use: BashDoctorLoad},
		{Use: BashDoctorLoad, Args: []string{"/d", "x"}},
		{Use: BashDoctorLoad, Args: []string{"/d"}, Name: "other"},
		{Use: BashDoctorLoad, Args: []string{"/d"}, Script: "rm -rf ~"},
		{Use: BashDoctorLoad, Args: []string{"/d"}, Wrap: []string{"nice"}},
		{Use: BashDoctorShell, Script: "rm -rf ~"},
		{Use: BashDoctorShell, Args: []string{"x"}},
		{Use: BashDoctorBare, Name: "x"},
		{Use: BashNVMBridge, Script: "nvm ls; rm -rf ~"},
		{Use: BashNVMBridge, Wrap: []string{"bwrap"}},
		{Use: BashLLMExec, Script: "s", Wrap: []string{"sudo", "sh", "-c", "x"}},
		{Use: 99, Script: "s"},
	} {
		if _, err := (Exec{}).RunBash(context.Background(), b); !errors.Is(err, ErrBashUse) && !errors.Is(err, ErrShellCommand) {
			t.Errorf("%+v: %v, want a refusal", b, err)
		}
	}
	if BashLLMExec.String() != "llm_exec" || BashNVMBridge.String() != "nvm bridge" || BashDoctorLoad.String() != "doctor load" ||
		BashDoctorShell.String() != "doctor shell" || BashDoctorBare.String() != "doctor bare" || BashUse(7).String() != "BashUse(7)" {
		t.Error("BashUse.String")
	}
}

// RunBash runs bash with the script and positional parameters (skipped
// without a bash on the test host).
func TestRunBash(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	b := Bash{Use: BashLLMExec, Bash: bash, Script: `printf '%s|' "$0" "$@"; printf '%s' "$X"; read -r l; printf '|%s' "$l"; exit 4`,
		Name: "n", Args: []string{"a b", "$HOME"}}
	b.Env = []string{"X=x"}
	b.Stdin = strings.NewReader("line\n")
	res, err := Exec{}.RunBash(context.Background(), b)
	if err != nil || string(res.Stdout) != "n|a b|$HOME|x|line" || res.ExitCode != 4 {
		t.Errorf("RunBash: %q %+v %v", res.Stdout, res, err)
	}

	// The nvm bridge loads $NVM_DIR/nvm.sh and passes the arguments to nvm.
	nvmDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(nvmDir, "nvm.sh"), []byte(`nvm() { printf 'nvm[%s]' "$@"; }`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b = Bash{Use: BashNVMBridge, Bash: bash, Args: []string{"ls", "$(touch x)"}}
	b.Env = []string{"NVM_DIR=" + nvmDir}
	b.Dir = nvmDir
	res, err = Exec{}.RunBash(context.Background(), b)
	if err != nil || string(res.Stdout) != "nvm[ls]nvm[$(touch x)]" || res.ExitCode != 0 {
		t.Errorf("nvm bridge: %q %+v %v", res.Stdout, res, err)
	}
	if _, err := os.Stat(filepath.Join(nvmDir, "x")); err == nil {
		t.Error("an nvm argument was run as code")
	}
	b.Env = []string{"NVM_DIR=" + filepath.Join(nvmDir, "missing")}
	b.Stderr = io.Discard
	if res, err = (Exec{}).RunBash(context.Background(), b); err != nil || res.ExitCode != 1 {
		t.Errorf("nvm bridge without nvm.sh: %+v %v", res, err)
	}
}

func TestCommandSubst(t *testing.T) {
	for in, want := range map[string]string{"a\n\n": "a", "a\nb\n": "a\nb", "a\x00b\n": "ab", "\n": "", "a\r\n": "a\r", "": ""} {
		if got := CommandSubst([]byte(in)); got != want {
			t.Errorf("CommandSubst(%q) = %q, want %q", in, got, want)
		}
	}
}
