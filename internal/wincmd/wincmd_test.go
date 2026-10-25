// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package wincmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
	"github.com/mauriziofonte/gash/internal/winbridge"
	"github.com/mauriziofonte/gash/internal/wsl"
)

// scriptRunner answers each Windows call from a fixture chosen by a marker
// in the command line, so one bridge can serve a whole doctor run. It is
// the stand-in for a real Windows: the JSON is what the read-only scripts
// would produce there (a capture on Maurizio's box pins the real shapes).
type scriptRunner struct {
	fixtures map[string]string // marker (substring of the command) -> stdout
}

func (s scriptRunner) Run(_ context.Context, c runner.Cmd) (runner.Result, error) {
	cmd := strings.Join(c.Args, " ")
	for marker, out := range s.fixtures {
		if strings.Contains(cmd, marker) {
			if c.Stdout != nil {
				_, _ = c.Stdout.Write([]byte(out))
			}
			return runner.Result{}, nil
		}
	}
	// Unmatched: empty success (the command treats it as no data).
	return runner.Result{}, nil
}
func (scriptRunner) RunBash(context.Context, runner.Bash) (runner.Result, error) {
	return runner.Result{}, nil
}
func (scriptRunner) RunScript(context.Context, runner.Script) (runner.Result, error) {
	return runner.Result{}, nil
}
func (scriptRunner) Start(context.Context, runner.Cmd) error                  { return nil }
func (scriptRunner) StartPTY(context.Context, runner.Cmd) (runner.PTY, error) { return nil, nil }

func fixtureSys(t *testing.T) platform.Sysroot {
	t.Helper()
	dir := t.TempDir()
	for _, rel := range []string{
		"mnt/c/Windows/System32/reg.exe",
		"mnt/c/Windows/System32/dsregcmd.exe",
		"mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe",
	} {
		p := filepath.Join(dir, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte("MZ"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// The Windows hosts file on the mounted drive.
	hp := filepath.Join(dir, "mnt/c/Windows/System32/drivers/etc/hosts")
	_ = os.MkdirAll(filepath.Dir(hp), 0o755)
	_ = os.WriteFile(hp, []byte("# comment\n127.0.0.1 localhost\n10.0.0.5 app.test api.test  # dev\n"), 0o644)
	sr, err := platform.NewSysroot(dir)
	if err != nil {
		t.Fatal(err)
	}
	return sr
}

func facts() wsl.Facts {
	return wsl.Facts{WSL: true, Version: 2,
		Interop:       wsl.Interop{Registered: true, Enabled: true, Socket: "/run/WSL/1_interop"},
		AutomountRoot: "/mnt/", Distro: "Ubuntu", Drives: []wsl.Drive{{Letter: "C", Mount: "/mnt/c"}}}
}

func call(t *testing.T, fx map[string]string, args ...string) (string, string, int) {
	t.Helper()
	sr := fixtureSys(t)
	var out, errb bytes.Buffer
	env := invoke.EnvMap(nil)
	opt := Options{Bridge: func(c *invoke.Call) *winbridge.Bridge {
		return winbridge.New(facts(), sr, scriptRunner{fixtures: fx}, c.Env.Get)
	}}
	status := Handler(opt)(&invoke.Call{
		Ctx: context.Background(), Name: "win", Args: args, Env: env,
		Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb, Term: term.Fixed(env, false, false, false),
	})
	return out.String(), errb.String(), status
}

func TestInfo(t *testing.T) {
	fx := map[string]string{"Win32_OperatingSystem": `{"caption":"Windows 11 Pro","version":"10.0.26100","build":"26100","ubr":2033,"hostname":"DESK","user":"DESK\\maurizio","elevated":false,"arch":"AMD64"}`}
	out, _, status := call(t, fx, "info", "--json")
	if status != 0 {
		t.Fatalf("status %d", status)
	}
	for _, want := range []string{`"caption":"Windows 11 Pro"`, `"build":"26100"`, `"elevated":false`} {
		if !strings.Contains(out, want) {
			t.Errorf("info json missing %s: %s", want, out)
		}
	}
	// Human form shows the build with the UBR appended.
	out, _, _ = call(t, fx, "info")
	if !strings.Contains(out, "26100.2033") || !strings.Contains(out, "Windows 11 Pro") {
		t.Errorf("info human = %s", out)
	}
}

func TestServicesSingleElement(t *testing.T) {
	// One service: the wrapper object keeps it a list (the 5.1 unwrap trap).
	fx := map[string]string{"Win32_Service": `{"items":[{"Name":"sshd","DisplayName":"OpenSSH","State":"Running","StartMode":"Auto","pid":1234,"account":"LocalSystem"}]}`}
	out, _, status := call(t, fx, "services", "--json")
	if status != 0 || !strings.Contains(out, `"services":[`) || !strings.Contains(out, `"Name":"sshd"`) {
		t.Fatalf("services json = %s (status %d)", out, status)
	}
	out, _, _ = call(t, fx, "services")
	if !strings.Contains(out, "sshd") || !strings.Contains(out, "Running") {
		t.Errorf("services human = %s", out)
	}
}

func TestReg(t *testing.T) {
	regOut := "\r\nHKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion\r\n" +
		"    ProductName    REG_SZ    Windows 11 Pro\r\n" +
		"    CurrentBuild    REG_SZ    26100\r\n\r\n" +
		"HKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion\\Fonts\r\n"
	fx := map[string]string{"query": regOut}
	out, _, status := call(t, fx, "reg", `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion`, "--json")
	if status != 0 {
		t.Fatalf("status %d out %s", status, out)
	}
	for _, want := range []string{`"name":"ProductName"`, `"type":"REG_SZ"`, `"data":"Windows 11 Pro"`, `CurrentVersion\\Fonts`} {
		if !strings.Contains(out, want) {
			t.Errorf("reg json missing %s: %s", want, out)
		}
	}
}

func TestRegRejectsBadKey(t *testing.T) {
	_, errb, status := call(t, nil, "reg", "SOFTWARE\\X")
	if status != 1 || !strings.Contains(errb, "invalid_windows_argument") {
		t.Errorf("bad key: status %d err %s", status, errb)
	}
}

func TestHosts(t *testing.T) {
	out, _, status := call(t, nil, "hosts", "--json")
	if status != 0 {
		t.Fatalf("status %d", status)
	}
	if !strings.Contains(out, `"ip":"10.0.0.5"`) || !strings.Contains(out, `"app.test"`) || strings.Contains(out, "comment") {
		t.Errorf("hosts json = %s", out)
	}
}

func TestInteropUnavailable(t *testing.T) {
	// A bridge whose interop is off: every subcommand fails with the code.
	sr := fixtureSys(t)
	off := facts()
	off.Interop = wsl.Interop{Registered: false}
	var out, errb bytes.Buffer
	env := invoke.EnvMap(nil)
	opt := Options{Bridge: func(c *invoke.Call) *winbridge.Bridge {
		return winbridge.New(off, sr, scriptRunner{}, c.Env.Get)
	}}
	status := Handler(opt)(&invoke.Call{
		Ctx: context.Background(), Name: "win", Args: []string{"info"}, Env: env,
		Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb, Term: term.Fixed(env, false, false, false),
	})
	if status != 1 || !strings.Contains(errb.String(), "interop_unavailable") {
		t.Errorf("interop off: status %d err %s", status, errb.String())
	}
}

func TestDoctorComposes(t *testing.T) {
	fx := map[string]string{
		"Win32_OperatingSystem": `{"caption":"Windows 11 Pro","build":"26100","elevated":true}`,
		"RebootPending":         `{"hotfixes":[],"reboot_pending":true}`,
		"Get-MpComputerStatus":  `{"available":true,"realtime":true,"antivirus_enabled":true,"signature_age_days":1}`,
		"Win32_LogicalDisk":     `{"items":[{"drive":"C:","size":500000000000,"free":20000000000,"fs":"NTFS"}]}`,
		"Get-WinEvent":          `{"items":[]}`,
	}
	out, _, status := call(t, fx, "doctor", "--json")
	if status != 1 { // reboot pending is a warning -> attention
		t.Fatalf("doctor status %d out %s", status, out)
	}
	for _, want := range []string{`"name":"windows"`, `"name":"reboot"`, `"name":"disk C:"`, `"attention":`} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor json missing %s: %s", want, out)
		}
	}
}
