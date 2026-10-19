// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package child

import (
	"bytes"
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mauriziofonte/gash/internal/runner"
)

func lookup(t *testing.T, name string) string {
	t.Helper()
	p := Find(name, func(p string) bool { _, err := os.Stat(p); return err == nil })
	if p == "" {
		t.Skipf("no %s", name)
	}
	return p
}

// A child gets BaseEnv only: nothing of the caller's environment.
func TestEnvironmentAndStatus(t *testing.T) {
	t.Setenv("GASH_SECRET_PROBE", "s3cret")
	res, err := Run(context.Background(), runner.Exec{}, Cmd(lookup(t, "env"), nil))
	if err != nil || !res.Success() {
		t.Fatalf("%v %+v", err, res)
	}
	got := strings.Split(strings.TrimSpace(string(res.Stdout)), "\n")
	slices.Sort(got)
	if want := []string{"LANG=C", "LC_ALL=C", "PATH=" + BasePath}; !slices.Equal(got, want) {
		t.Fatalf("child environment %v, want %v", got, want)
	}
	c := Cmd(lookup(t, "cat"), nil)
	c.Stdin = bytes.NewReader([]byte("in\n"))
	if res, err := Run(context.Background(), runner.Exec{}, c); err != nil || string(res.Stdout) != "in\n" {
		t.Fatalf("stdin: %v %q", err, res.Stdout)
	}
	if res, err := Run(context.Background(), runner.Exec{}, Cmd(lookup(t, "false"), nil)); err != nil || res.ExitCode != 1 || Output(res) != "" {
		t.Fatalf("false: %v %+v", err, res)
	}
	if _, err := Run(context.Background(), runner.Exec{}, Cmd("env", nil)); err == nil {
		t.Fatal("relative path accepted")
	}
	if res, err := Run(context.Background(), runner.Exec{}, Cmd(lookup(t, "pwd"), nil)); err != nil || strings.TrimSpace(string(res.Stdout)) != "/" {
		t.Fatalf("working directory: %v %q", err, res.Stdout)
	}
}

// A timeout is an error; a daemon's grandchild holding stdout does not
// hold the caller.
func TestTimeoutAndDaemon(t *testing.T) {
	start := time.Now()
	c := Cmd(lookup(t, "sleep"), nil, "5")
	c.Timeout = 200 * time.Millisecond
	if _, err := Run(context.Background(), runner.Exec{}, c); err == nil || !strings.Contains(err.Error(), "timed out") || time.Since(start) > 3*time.Second {
		t.Fatalf("timeout: %v after %s", err, time.Since(start))
	}
	setsid := Find("setsid", func(p string) bool { _, err := os.Stat(p); return err == nil })
	if setsid == "" {
		t.Skip("no setsid")
	}
	start = time.Now()
	res, err := Run(context.Background(), runner.Exec{}, Cmd(setsid, nil, "-f", lookup(t, "sleep"), "3"))
	if err != nil || !res.Success() || time.Since(start) > 2*time.Second {
		t.Fatalf("daemon: %v %+v after %s", err, res, time.Since(start))
	}
}

func TestFakeAndLine(t *testing.T) {
	f := Fake(t, func(c runner.Call) runner.Expect { return runner.Expect{Stdout: "ok"} })
	res, err := Run(context.Background(), f, Cmd("/usr/sbin/apache2", nil, "-f", "/a b/httpd.conf", "-k", "start"))
	if err != nil || string(res.Stdout) != "ok" {
		t.Fatalf("%+v %v", res, err)
	}
	if got := Lines(f.Calls()); len(got) != 1 || got[0] != "/usr/sbin/apache2 -f '/a b/httpd.conf' -k start" {
		t.Fatalf("%q", got)
	}
	if c := f.Calls()[0]; !c.Setsid || c.Dir != "/" || c.Timeout != DefaultTimeout {
		t.Fatalf("defaults: %+v", c)
	}
}
