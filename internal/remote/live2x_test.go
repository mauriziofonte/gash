// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

const repoRoot = "../.."

// absRoot is the checkout, resolved before any test changes directory.
var absRoot = func() string {
	p, err := filepath.Abs(repoRoot)
	if err != nil {
		panic(err)
	}
	return p
}()

// stubScript is the logging ssh, scp and rsync of the tests: it appends
// its name, its arguments (one per line, between brackets) and its stdin
// to $STUB_DIR/log, prints $STUB_DIR/NAME.out and exits with the next
// status of $STUB_DIR/NAME.status (0 when none is left).
const stubScript = `#!/bin/bash
n=${0##*/}
{
	printf '%s\n' "$n"
	for a in "$@"; do printf '  [%s]\n' "$a"; done
	printf '  stdin:'
	od -An -c | tr -s ' ' | tr -d '\n'
	printf '\n'
} >>"$STUB_DIR/log"
[ -f "$STUB_DIR/$n.out" ] && cat "$STUB_DIR/$n.out"
[ -f "$STUB_DIR/$n.err" ] && cat "$STUB_DIR/$n.err" >&2
st=0
if [ -s "$STUB_DIR/$n.status" ]; then
	read -r st rest <"$STUB_DIR/$n.status"
	printf '%s\n' "$rest" >"$STUB_DIR/$n.status"
	[ -n "$st" ] || st=0
fi
exit "$st"
`

// scene is a fresh HOME (with .gash_env when given), a stub directory
// first on PATH and a work directory, rebuilt for every run at the same
// paths.
type scene struct {
	t     *testing.T
	root  string
	base  string
	gash  string // .gash_env content, "" for none
	stubs []string
	files map[string]string // STUB_DIR/NAME -> content (NAME.out, NAME.status)
	env   []string
	stdin string
	prep  func(home string)
	// homeName is the HOME directory's name under base ("home").
	homeName string
}

func newScene(t *testing.T, gashEnv string) *scene {
	t.Helper()
	if _, err := os.Stat(filepath.Join(absRoot, "lib/modules/gash.sh")); err != nil {
		t.Skip("the 2.x sources are not in this tree")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	root := absRoot
	return &scene{t: t, root: root, base: t.TempDir(), gash: gashEnv, stubs: []string{"ssh", "scp", "rsync"},
		files: map[string]string{}}
}

func (s *scene) home() string {
	if s.homeName != "" {
		return filepath.Join(s.base, s.homeName)
	}
	return filepath.Join(s.base, "home")
}
func (s *scene) stubD() string { return filepath.Join(s.base, "stub") }
func (s *scene) work() string  { return filepath.Join(s.base, "work") }

// environ is the environment of both runs: the defaults, then s.env,
// then the mode's entries, a later entry replacing an earlier one.
func (s *scene) environ(m mode) []string {
	env := []string{"HOME=" + s.home(), "PATH=" + s.stubD() + ":/usr/bin:/bin", "LC_ALL=C", "TERM=xterm-256color",
		"STUB_DIR=" + s.stubD()}
	vals := map[string]string{}
	for _, kv := range append(append(env, s.env...), m.env...) {
		k, v, _ := strings.Cut(kv, "=")
		vals[k] = v
	}
	out := make([]string, 0, len(vals))
	for k, v := range vals {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

func (s *scene) build() {
	s.t.Helper()
	if wd, err := os.Getwd(); err != nil || strings.HasPrefix(wd, s.base+"/") {
		require.NoError(s.t, os.Chdir(s.base))
	}
	for _, d := range []string{s.home(), s.stubD(), s.work()} {
		require.NoError(s.t, os.RemoveAll(d))
		require.NoError(s.t, os.MkdirAll(d, 0o755))
	}
	if s.gash != "" {
		require.NoError(s.t, os.WriteFile(filepath.Join(s.home(), ".gash_env"), []byte(s.gash), 0o600))
	}
	for _, n := range s.stubs {
		require.NoError(s.t, os.WriteFile(filepath.Join(s.stubD(), n), []byte(stubScript), 0o755))
	}
	for n, c := range s.files {
		require.NoError(s.t, os.WriteFile(filepath.Join(s.stubD(), n), []byte(c), 0o644))
	}
	if s.prep != nil {
		s.prep(s.home())
	}
}

// mode is the terminal of a run (color: a color stdout) and the
// environment entries it adds.
type mode struct {
	color bool
	env   []string
}

var (
	plain    = mode{}
	color    = mode{color: true}
	headless = mode{env: []string{"GASH_HEADLESS=1"}}
)

type result struct {
	Out, Err string
	RC       int
	Log      string // what the stubs recorded
	Tree     string // the HOME tree: names, kinds and modes
}

// noDiag is r without Bash's own diagnostics, which name a 2.x source
// line (the /dev/tty error of a question without a terminal, the NUL
// byte warning of a command substitution): the binary does not print
// them (LEDGER DOCK-19, CORE-01).
func (r result) noDiag(root string) result {
	re := regexp.MustCompile(regexp.QuoteMeta(root) + `/lib/[^:\n]+: line [0-9]+: [^\n]*\n`)
	r.Err = re.ReplaceAllString(r.Err, "")
	return r
}

func (s *scene) after() (string, string) {
	log, _ := os.ReadFile(filepath.Join(s.stubD(), "log"))
	var tree []string
	_ = filepath.Walk(s.home(), func(p string, info os.FileInfo, err error) error {
		if err == nil {
			rel, _ := filepath.Rel(s.base, p)
			tree = append(tree, rel+" "+info.Mode().String())
		}
		return nil
	})
	sort.Strings(tree)
	return string(log), strings.Join(tree, "\n")
}

// run2x runs the 2.x function in the work directory: lib/core and
// lib/modules/remote.sh sourced in a Bash without a controlling terminal,
// ~/.gash_env loaded first (its warnings belong to the shell's load), the
// color gate fixed as m says.
func (s *scene) run2x(m mode, fn string, args ...string) result {
	s.t.Helper()
	s.build()
	gate := "1"
	if m.color {
		gate = "0"
	}
	script := `root=$1; shift
for f in output utils validation config; do source "$root/lib/core/$f.sh"; done
source "$root/lib/modules/remote.sh"
__gash_use_color() { return ` + gate + `; }
__gash_load_env 2>/dev/null
"$@"`
	cmd := exec.Command("bash", append([]string{"--noprofile", "--norc", "-c", script, "_", s.root, fn}, args...)...)
	cmd.Dir, cmd.Env = s.work(), s.environ(m)
	cmd.Stdin = strings.NewReader(s.stdin)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // no /dev/tty
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	rc := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		require.True(s.t, errors.As(err, &ee), err)
		rc = ee.ExitCode()
	}
	log, tree := s.after()
	return result{Out: out.String(), Err: errb.String(), RC: rc, Log: log, Tree: tree}
}

// runGo runs the handler in the work directory; tm, when not nil,
// changes the call's terminal (a controlling terminal for a question).
func (s *scene) runGo(m mode, fn string, tm func(*term.Term) *term.Term, args ...string) result {
	s.t.Helper()
	s.build()
	s.t.Chdir(s.work())
	tbl := invoke.NewTable()
	New().Register(tbl)
	e := invoke.EnvFrom(append(s.environ(m), "GASH__HELP=0"))
	tt := term.Fixed(e, false, m.color, false)
	if tm != nil {
		tt = tm(tt)
	}
	// The binary's stdin is a file, as the stub gives it.
	in := filepath.Join(s.base, "stdin")
	require.NoError(s.t, os.WriteFile(in, []byte(s.stdin), 0o644))
	f, err := os.Open(in)
	require.NoError(s.t, err)
	defer func() { _ = f.Close() }()
	var out, errb bytes.Buffer
	c := &invoke.Call{Ctx: context.Background(), Name: fn, Args: args, Env: e, Stdin: f, Stdout: &out, Stderr: &errb, Term: tt}
	rc := tbl.Run(c)
	log, tree := s.after()
	return result{Out: out.String(), Err: errb.String(), RC: rc, Log: log, Tree: tree}
}

// same runs fn the 2.x way and through the handler on fresh scenes and
// requires the same streams, status, stub calls and HOME tree.
func (s *scene) same(m mode, fn string, args ...string) result {
	s.t.Helper()
	want := s.run2x(m, fn, args...).noDiag(s.root)
	got := s.runGo(m, fn, nil, args...)
	require.Equal(s.t, want, got, "%s %q", fn, args)
	if testing.Verbose() {
		s.t.Logf("rc=%d\nstdout:\n%s\nstderr:\n%s\nlog:\n%s", got.RC, got.Out, got.Err, got.Log)
	}
	return got
}

// bash2x runs script with the 2.x remote module loaded and returns its
// stdout.
func bash2x(t *testing.T, script string) string {
	t.Helper()
	s := newScene(t, "")
	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", `root=$1; shift
for f in output utils validation config; do source "$root/lib/core/$f.sh"; done
source "$root/lib/modules/remote.sh"
`+script, "_", s.root)
	cmd.Env = []string{"HOME=" + s.base, "PATH=/usr/bin:/bin", "LC_ALL=C"}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	require.NoError(t, cmd.Run(), errb.String())
	return out.String()
}
