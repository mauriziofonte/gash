// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// elapsedRe matches the footer scan time (__gash_fs_format_elapsed): "Nms",
// "N.NNs" or "NmNs". It is the only non-deterministic token in the rich
// size list, so both sides are normalized to T before comparison.
var elapsedRe = regexp.MustCompile(`[0-9]+ms|[0-9]+\.[0-9]{2}s|[0-9]+m[0-9]+s`)

func normRich(s, fx string) string {
	s = strings.ReplaceAll(s, fx, "FX")
	return elapsedRe.ReplaceAllString(s, "T")
}

// shq single-quotes a word for a POSIX shell command line.
func shq(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// runGoRich invokes a scanner in-process with a color terminal forced
// (stdout a TTY, a 120-column UTF-8 locale), so the size-list rich layout
// runs. It returns stdout and the exit status.
func runGoRich(fn, home, pwd string, args ...string) (string, int) {
	e := invoke.EnvMap(map[string]string{
		"HOME": home, "PWD": pwd, "GASH__HELP": "0",
		"COLUMNS": "120", "LC_ALL": "C.UTF-8",
	})
	var out, errb bytes.Buffer
	c := &invoke.Call{
		Ctx: context.Background(), Name: fn, Args: args, Env: e,
		Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, true, false),
	}
	rc := scanHandlers[fn](c)
	return out.String(), rc
}

// run2xRich runs a 2.x scanner under a pseudo-terminal (via script(1)) so
// __gash_use_color sees a TTY on stdout and takes the rich size-list
// layout, with the same 120-column UTF-8 locale. ok is false (the test
// skips) when bash, script or the 2.x sources are unavailable. script
// merges the child's stderr into the pty stream, so only stdout and the
// exit status are returned.
func run2xRich(t *testing.T, fn, pwd string, args ...string) (string, int, bool) {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		return "", 0, false
	}
	scriptBin, err := exec.LookPath("script")
	if err != nil {
		return "", 0, false
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		return "", 0, false
	}
	for _, f := range []string{"lib/core/output.sh", "lib/core/utils.sh", "lib/core/validation.sh", "lib/modules/files.sh"} {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			return "", 0, false
		}
	}
	wrapper := filepath.Join(t.TempDir(), "rich2x.sh")
	body := `set +e
source "$R/lib/core/output.sh"
source "$R/lib/core/utils.sh"
source "$R/lib/core/validation.sh"
source "$R/lib/modules/files.sh"
"$@"
`
	if err := os.WriteFile(wrapper, []byte(body), 0o700); err != nil {
		return "", 0, false
	}
	parts := []string{shq(bash), shq(wrapper), shq(fn)}
	for _, a := range args {
		parts = append(parts, shq(a))
	}
	// script -q: no header/trailer; -e: exit with the command's status;
	// -c CMD runs CMD under a pty; the typescript goes to /dev/null.
	cmd := exec.Command(scriptBin, "-qec", strings.Join(parts, " "), "/dev/null")
	cmd.Dir = pwd
	cmd.Env = []string{
		"R=" + root, "COLUMNS=120", "LC_ALL=C.UTF-8", "TERM=xterm",
		"PATH=" + os.Getenv("PATH"), "HOME=" + pwd, "PWD=" + pwd,
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	rc := 0
	if ee, ok := err.(*exec.ExitError); ok {
		rc = ee.ExitCode()
	} else if err != nil {
		return "", 0, false
	}
	// The pty translates LF to CRLF; strip the CRs for comparison.
	return strings.ReplaceAll(out.String(), "\r", ""), rc, true
}

// TestDifferential2xRich compares the Go scanners to the live 2.x sources
// on the RICH (color terminal) size-list layout, which TestDifferential2x
// does not reach (it drives the plain/JSON path). It pins the header row
// and the per-rank share bars byte for byte: the rich renderer had no
// differential coverage, which let the missing SHARE header row (and the
// rank-1 bar printed as a raw "bytes/max") slip through.
func TestDifferential2xRich(t *testing.T) {
	fx := mkfix(t)
	home := t.TempDir()
	cases := [][]string{
		{"files_largest", fx},
		{"files_largest", "-n", "2", fx},
		{"dirs_largest", "--depth", "3", fx},
		{"dirs_find_large", "--size", "1K", fx},
		{"dirs_find_large", "--size", "1K", "--with-mtime", fx},
	}
	for _, args := range cases {
		fn, rest := args[0], args[1:]
		o2, rc2, ok := run2xRich(t, fn, home, rest...)
		if !ok {
			t.Skip("2.x sources, bash or script(1) unavailable")
		}
		og, rcg := runGoRich(fn, home, fx, rest...)
		if normRich(og, fx) != normRich(o2, fx) {
			t.Errorf("%v stdout:\n go: %q\n 2x: %q", args, normRich(og, fx), normRich(o2, fx))
		}
		if rcg != rc2 {
			t.Errorf("%v rc: go=%d 2x=%d", args, rcg, rc2)
		}
	}
}
