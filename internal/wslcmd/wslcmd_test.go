// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package wslcmd

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/term"
)

const mountinfoWSL2 = `62 1 8:32 / / rw,relatime - ext4 /dev/sdc rw
82 62 0:52 / /mnt/c rw,noatime - 9p C:\134 rw,aname=drvfs;path=C:\;metadata;symlinkroot=/mnt/
83 62 0:53 / /mnt/d rw,noatime - 9p D:\134 rw,aname=drvfs;path=D:\;symlinkroot=/mnt/
`

func sysroot(t *testing.T, files map[string]string) platform.Sysroot {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if body == "<dir>" {
			_ = os.MkdirAll(p, 0o755)
			continue
		}
		if body == "<socket>" {
			l, err := net.Listen("unix", p)
			if err != nil {
				t.Skip("unix socket:", err)
			}
			t.Cleanup(func() { l.Close() })
			continue
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sr, err := platform.NewSysroot(dir)
	if err != nil {
		t.Fatal(err)
	}
	return sr
}

func wsl2(t *testing.T) platform.Sysroot {
	return sysroot(t, map[string]string{
		"proc/sys/kernel/osrelease":          "6.6.87.2-microsoft-standard-WSL2\n",
		"proc/sys/fs/binfmt_misc/WSLInterop": "enabled\ninterpreter /init\n",
		"proc/self/mountinfo":                mountinfoWSL2,
		"proc/1/comm":                        "systemd\n",
		"proc/412":                           "<dir>",
		"run/WSL/412_interop":                "<socket>",
		"sys/class/net/eth0":                 "<dir>",
		"etc/wsl.conf":                       "[boot]\nsystemd=true\n[interop]\nappendWindowsPath=false\n",
	})
}

func call(t *testing.T, sr platform.Sysroot, env map[string]string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	e := invoke.EnvMap(env)
	status := Handler(Options{Sysroot: sr, Getenv: e.Get})(&invoke.Call{
		Ctx: context.Background(), Name: "wsl", Args: args, Env: e,
		Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, false, false),
	})
	return out.String(), errb.String(), status
}

func TestInfoJSON(t *testing.T) {
	sr := wsl2(t)
	out, _, status := call(t, sr, map[string]string{"WSL_DISTRO_NAME": "Ubuntu-24.04", "WSL_INTEROP": "/run/WSL/412_interop"}, "info", "--json")
	if status != 0 {
		t.Fatalf("status %d", status)
	}
	for _, want := range []string{`"wsl":true`, `"version":2`, `"distro":"Ubuntu-24.04"`, `"letter":"C"`, `"metadata":true`} {
		if !strings.Contains(out, want) {
			t.Errorf("info json missing %s in %s", want, out)
		}
	}
}

func TestDoctorStaleInterop(t *testing.T) {
	sr := wsl2(t)
	// WSL_INTEROP points at a socket that is gone: doctor fails.
	out, _, status := call(t, sr, map[string]string{"WSL_DISTRO_NAME": "Ubuntu", "WSL_INTEROP": "/run/WSL/999_interop"}, "doctor", "--json")
	if status != 1 {
		t.Fatalf("stale interop should fail doctor; status %d out %s", status, out)
	}
	if !strings.Contains(out, `"status":"fail"`) || !strings.Contains(out, "stale") && !strings.Contains(out, "gone") {
		t.Errorf("doctor json = %s", out)
	}
}

func TestNotWSL(t *testing.T) {
	sr := sysroot(t, map[string]string{"proc/sys/kernel/osrelease": "6.8.0-generic\n", "proc/version": "Linux\n"})
	// info is a report: off WSL it answers and exits 0.
	out, errb, status := call(t, sr, nil, "info")
	if status != 0 || !strings.Contains(errb, "Not running under WSL") || out != "" {
		t.Errorf("info not WSL: status %d out %q err %q", status, out, errb)
	}
	// doctor is a check: off WSL it cannot run and exits 1.
	_, errb, status = call(t, sr, nil, "doctor")
	if status != 1 || !strings.Contains(errb, "Not running under WSL") {
		t.Errorf("doctor not WSL: status %d err %q", status, errb)
	}
}

func TestPath(t *testing.T) {
	sr := wsl2(t)
	env := map[string]string{"WSL_DISTRO_NAME": "Ubuntu-24.04"}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"path", "/mnt/c/Users/me"}, `C:\Users\me`},
		{[]string{"path", `C:\Users\me`}, "/mnt/c/Users/me"},
		{[]string{"path", "/home/me", "--mixed"}, "//wsl.localhost/Ubuntu-24.04/home/me"},
		{[]string{"path", "/mnt/d/x", "--windows"}, `D:\x`},
	} {
		out, errb, status := call(t, sr, env, c.args...)
		if status != 0 || strings.TrimSpace(out) != c.want {
			t.Errorf("%v = %q (err %q, status %d); want %q", c.args, out, errb, status, c.want)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	sr := wsl2(t)
	out, _, status := call(t, sr, nil, "config", "--json")
	if status != 0 {
		t.Fatalf("status %d", status)
	}
	// appendWindowsPath is set to false in the fixture wsl.conf.
	if !strings.Contains(out, `"key":"appendWindowsPath"`) || !strings.Contains(out, `"value":"false"`) || !strings.Contains(out, `"source":"file"`) {
		t.Errorf("config json = %s", out)
	}
}

func TestInteropExport(t *testing.T) {
	sr := wsl2(t)
	out, _, status := call(t, sr, map[string]string{"WSL_INTEROP": "/run/WSL/999_interop"}, "interop", "--export")
	if status != 0 {
		t.Fatalf("status %d", status)
	}
	// The stale env is replaced by the live socket found in /run/WSL.
	if strings.TrimSpace(out) != "export WSL_INTEROP=/run/WSL/412_interop" {
		t.Errorf("export = %q", out)
	}
}
