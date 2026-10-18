// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package proc

import (
	"context"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func lookup(t *testing.T, name string) string {
	t.Helper()
	p := Find(name, func(p string) bool { _, err := os.Stat(p); return err == nil })
	if p == "" {
		t.Skipf("no %s", name)
	}
	return p
}

func TestRunEnvironmentAndStatus(t *testing.T) {
	t.Setenv("GASH_SECRET_PROBE", "s3cret")
	res, err := OS{}.Run(context.Background(), Cmd{Path: lookup(t, "env")})
	if err != nil || !res.OK() {
		t.Fatalf("%v %+v", err, res)
	}
	got := strings.Split(strings.TrimSpace(string(res.Stdout)), "\n")
	slices.Sort(got)
	if want := []string{"LANG=C", "LC_ALL=C", "PATH=" + BasePath}; !slices.Equal(got, want) {
		t.Fatalf("child environment %v, want %v", got, want)
	}
	res, err = OS{}.Run(context.Background(), Cmd{Path: lookup(t, "cat"), Stdin: []byte("in\n")})
	if err != nil || string(res.Stdout) != "in\n" {
		t.Fatalf("stdin: %v %q", err, res.Stdout)
	}
	res, err = OS{}.Run(context.Background(), Cmd{Path: lookup(t, "false")})
	if err != nil || res.ExitCode != 1 || res.OK() {
		t.Fatalf("false: %v %+v", err, res)
	}
	if _, err := (OS{}).Run(context.Background(), Cmd{Path: "env"}); err == nil {
		t.Fatal("relative path accepted")
	}
}

func TestRunTimeoutAndDaemon(t *testing.T) {
	start := time.Now()
	_, err := OS{}.Run(context.Background(), Cmd{Path: lookup(t, "sleep"), Args: []string{"5"}, Timeout: 200 * time.Millisecond})
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("timeout: %v after %s", err, time.Since(start))
	}
	// A child that leaves a grandchild holding stdout does not hold Run.
	start = time.Now()
	res, err := OS{}.Run(context.Background(), Cmd{Path: lookup(t, "sh"), Args: []string{"-c", "sleep 3 & echo started"}})
	if err != nil || strings.TrimSpace(string(res.Stdout)) != "started" || time.Since(start) > 2*time.Second {
		t.Fatalf("daemon: %v %q after %s", err, res.Stdout, time.Since(start))
	}
}

func TestStartSession(t *testing.T) {
	s, err := OS{}.Start(context.Background(), Cmd{Path: lookup(t, "cat")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(s.Stdin(), "ping\n"); err != nil {
		t.Fatal(err)
	}
	s.Stdin().Close()
	out, _ := io.ReadAll(s.Stdout())
	res, err := s.Wait()
	if err != nil || !res.OK() || string(out) != "ping\n" {
		t.Fatalf("%v %+v %q", err, res, out)
	}
}

func TestFakeAndString(t *testing.T) {
	f := &Fake{}
	if _, err := f.Run(context.Background(), Cmd{Path: "/usr/sbin/apache2", Args: []string{"-f", "/a b/httpd.conf", "-k", "start"}}); err != nil {
		t.Fatal(err)
	}
	if got := f.Lines(); len(got) != 1 || got[0] != "/usr/sbin/apache2 -f '/a b/httpd.conf' -k start" {
		t.Fatalf("%q", got)
	}
}
