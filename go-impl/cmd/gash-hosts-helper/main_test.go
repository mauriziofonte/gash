// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The built binary refuses bad argv and bad input with exit 2. Every case
// here is refused before the hosts file is read, so the test never touches
// /etc/hosts.
func TestBinaryRefusals(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go command to build the helper with")
	}
	bin := filepath.Join(t.TempDir(), "gash-hosts-helper")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cases := []struct {
		args  []string
		stdin string
	}{
		{nil, ""},
		{[]string{"apply", "/tmp/hosts"}, "127.0.0.1 a.test\n"},
		{[]string{"--hosts", "/tmp/hosts", "apply"}, "127.0.0.1 a.test\n"},
		{[]string{"list"}, ""},
		{[]string{"apply"}, "10.0.0.1 a.test\n"},
		{[]string{"apply"}, "127.0.0.1 *.a.test\n"},
		{[]string{"apply"}, "127.0.0.1 a.test\n127.0.0.1 b.test\n0.0.0.0 evil.test\n"},
		{[]string{"apply"}, "127.0.0.1 A.test\n"},
		{[]string{"apply"}, strings.Repeat("127.0.0.1 a.test\n", 4097)},
	}
	for _, c := range cases {
		cmd := exec.Command(bin, c.args...)
		cmd.Stdin = strings.NewReader(c.stdin)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 2 {
			t.Errorf("%q: %v, stderr %q", c.args, err, stderr.String())
		}
	}
}
