// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package winbridge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/wsl"
)

// TestScriptsReadOnly is the guardrail: no registered PowerShell script may
// contain a state-changing verb, so a write to Windows can never reach the
// system through the bridge.
func TestScriptsReadOnly(t *testing.T) {
	if len(Registry) == 0 {
		t.Fatal("no scripts registered")
	}
	for _, s := range Registry {
		low := strings.ToLower(s.Body)
		for _, v := range writeVerbs {
			vl := strings.ToLower(v)
			hit := false
			if strings.HasSuffix(v, "-") || strings.Contains(v, "-") {
				hit = strings.Contains(low, vl)
			} else {
				hit = containsWord(low, vl) // iex, iwr, irm, icm
			}
			if hit {
				t.Errorf("script %q contains write verb %q", s.Name, v)
			}
		}
	}
}

func containsWord(s, w string) bool {
	for i := 0; ; {
		j := strings.Index(s[i:], w)
		if j < 0 {
			return false
		}
		start := i + j
		end := start + len(w)
		before := start == 0 || !isLetter(s[start-1])
		after := end == len(s) || !isLetter(s[end])
		if before && after {
			return true
		}
		i = end
	}
}

func isLetter(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }

func TestScriptTextWrapping(t *testing.T) {
	txt := scriptText(Script{Name: "x", Depth: 6, Body: "$result=Get-Thing"})
	for _, want := range []string{"[Console]::In.ReadToEnd()", "ConvertFrom-Json", "ConvertTo-Json -InputObject $result -Compress -Depth 6", "Get-Thing"} {
		if !strings.Contains(txt, want) {
			t.Errorf("wrapped script missing %q: %s", want, txt)
		}
	}
}

func TestDecode(t *testing.T) {
	// UTF-8 with CRLF.
	if got := Decode([]byte("a\r\nb\r\n")); got != "a\nb\n" {
		t.Errorf("utf8 crlf = %q", got)
	}
	// UTF-16 LE with a BOM (what a PowerShell console emits without a forced
	// UTF-8 encoding).
	u16 := []byte{0xff, 0xfe}
	for _, r := range utf16.Encode([]rune("{\"ok\":true}")) {
		u16 = append(u16, byte(r), byte(r>>8))
	}
	if got := Decode(u16); got != `{"ok":true}` {
		t.Errorf("utf16 = %q", got)
	}
}

// stubRunner returns canned output and status for any Run; it records the
// last argv so a test can check what the bridge launched.
type stubRunner struct {
	out    []byte
	errOut []byte
	status int
	timeod bool
	last   runner.Cmd
}

func (s *stubRunner) Run(_ context.Context, c runner.Cmd) (runner.Result, error) {
	s.last = c
	// Mirror Exec: when the caller gives output writers, write to them and
	// leave Result.Stdout/Stderr empty.
	if c.Stdout != nil {
		_, _ = c.Stdout.Write(s.out)
	}
	if c.Stderr != nil {
		_, _ = c.Stderr.Write(s.errOut)
	}
	return runner.Result{ExitCode: s.status, TimedOut: s.timeod}, nil
}
func (s *stubRunner) RunBash(context.Context, runner.Bash) (runner.Result, error) {
	return runner.Result{}, nil
}
func (s *stubRunner) RunScript(context.Context, runner.Script) (runner.Result, error) {
	return runner.Result{}, nil
}
func (s *stubRunner) Start(context.Context, runner.Cmd) error                  { return nil }
func (s *stubRunner) StartPTY(context.Context, runner.Cmd) (runner.PTY, error) { return nil, nil }

func fixtureSys(t *testing.T) platform.Sysroot {
	t.Helper()
	dir := t.TempDir()
	for _, rel := range []string{
		"mnt/c/Windows/System32/reg.exe",
		"mnt/c/Windows/System32/whoami.exe",
		"mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe",
	} {
		p := filepath.Join(dir, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte("MZ"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sr, err := platform.NewSysroot(dir)
	if err != nil {
		t.Fatal(err)
	}
	return sr
}

func usableFacts() wsl.Facts {
	return wsl.Facts{WSL: true, Version: 2,
		Interop:       wsl.Interop{Registered: true, Enabled: true, Socket: "/run/WSL/1_interop"},
		AutomountRoot: "/mnt/", Drives: []wsl.Drive{{Letter: "C", Mount: "/mnt/c"}}}
}

func TestAvailable(t *testing.T) {
	sr := fixtureSys(t)
	run := &stubRunner{}
	// usable
	if e := New(usableFacts(), sr, run, nil).Available(); e != nil {
		t.Errorf("usable: %v", e)
	}
	// not WSL
	if e := New(wsl.Facts{}, sr, run, nil).Available(); e == nil || e.Code != CodeUnavailable {
		t.Errorf("not wsl: %v", e)
	}
	// stale interop
	f := usableFacts()
	f.Interop = wsl.Interop{Registered: true, Enabled: true, Stale: true}
	if e := New(f, sr, run, nil).Available(); e == nil || e.Code != CodeUnavailable {
		t.Errorf("stale: %v", e)
	}
}

func TestDecodeJSON(t *testing.T) {
	sr := fixtureSys(t)
	run := &stubRunner{out: []byte(`{"caption":"Windows 11 Pro","build":"26100","elevated":false}`)}
	b := New(usableFacts(), sr, run, func(string) string { return "" })
	var info struct {
		Caption  string `json:"caption"`
		Build    string `json:"build"`
		Elevated bool   `json:"elevated"`
	}
	if e := b.DecodeJSON(context.Background(), ScriptInfo, nil, &info); e != nil {
		t.Fatalf("decode: %v", e)
	}
	if info.Caption != "Windows 11 Pro" || info.Build != "26100" || info.Elevated {
		t.Errorf("info = %+v", info)
	}
	// It launched the System32 PowerShell, with the read-only flags.
	if !strings.HasSuffix(run.last.Path, "WindowsPowerShell/v1.0/powershell.exe") {
		t.Errorf("launched %q", run.last.Path)
	}
	if !contains(run.last.Args, "-NonInteractive") || !contains(run.last.Args, "-NoProfile") {
		t.Errorf("args = %v", run.last.Args)
	}
}

func TestErrorMapping(t *testing.T) {
	sr := fixtureSys(t)
	// timeout
	b := New(usableFacts(), sr, &stubRunner{timeod: true}, nil)
	if _, e := b.PowerShell(context.Background(), ScriptInfo, nil); e == nil || e.Code != CodeTimeout {
		t.Errorf("timeout: %v", e)
	}
	// access denied from stderr
	b = New(usableFacts(), sr, &stubRunner{status: 1, errOut: []byte("Get-WinEvent: Access is denied")}, nil)
	if _, e := b.PowerShell(context.Background(), ScriptEvents, nil); e == nil || e.Code != CodeAccessDenied {
		t.Errorf("denied: %v", e)
	}
	// elevation required
	b = New(usableFacts(), sr, &stubRunner{status: 1, errOut: []byte("The requested operation requires elevation")}, nil)
	if _, e := b.PowerShell(context.Background(), ScriptEvents, nil); e == nil || e.Code != CodeElevation {
		t.Errorf("elevation: %v", e)
	}
}

func TestTimeoutEnv(t *testing.T) {
	sr := fixtureSys(t)
	b := New(usableFacts(), sr, &stubRunner{}, func(k string) string {
		if k == "GASH_WIN_TIMEOUT" {
			return "3"
		}
		return ""
	})
	if b.Timeout() != 3*time.Second {
		t.Errorf("timeout = %v", b.Timeout())
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// TestSystemDriveMountedTwice: with C mounted on /mnt/c and again, later,
// on another directory (where Windows is not reachable), the bridge finds
// System32 on /mnt/c, as gash_doctor does (3.4.2; it took the later mount
// for the only one and answered interop_unavailable).
func TestSystemDriveMountedTwice(t *testing.T) {
	sr := fixtureSys(t)
	mi := `82 62 0:52 / /mnt/c rw,noatime - 9p C:\134 rw,aname=drvfs;path=C:\;uid=1000;symlinkroot=/mnt/,trans=fd
91 62 0:61 / /win/c rw,noatime - 9p C:\134 rw,aname=drvfs;path=C:\;uid=1001;umask=077;symlinkroot=/mnt/,trans=fd
`
	if err := os.MkdirAll(sr.Path("/proc/self"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sr.Path("/proc/self/mountinfo"), []byte(mi), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sr.Path("/proc/version"), []byte("Linux version 6.6.87.2-microsoft-standard-WSL2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := wsl.Detect(sr, func(string) string { return "" })
	f.Interop = usableFacts().Interop
	b := New(f, sr, &stubRunner{}, nil)
	if e := b.Available(); e != nil {
		t.Fatalf("available: %v", e)
	}
	if p, ok := b.Exe("WindowsPowerShell/v1.0/powershell.exe"); !ok || p != "/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe" {
		t.Errorf("powershell = %q, %v", p, ok)
	}
	if got, err := f.ToWindows("/win/c/Users", wsl.FormWindows); err != nil || got != `C:\Users` {
		t.Errorf("ToWindows(/win/c/Users) = %q, %v", got, err)
	}
}
