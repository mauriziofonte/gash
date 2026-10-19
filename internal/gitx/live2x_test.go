// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
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

// scene is a git scenario rebuilt from its setup script for every run, at
// the same path, so the 2.x function and the handler see the same tree
// and the same hashes (fixed identities and dates).
type scene struct {
	t     *testing.T
	root  string // the checkout with the 2.x sources
	setup string
	base  string
	dir   string
	env   []string
}

// gitConfig is the HOME/.gitconfig of every scene.
const gitConfig = `[user]
	name = Tess Ter
	email = tess@example.com
[init]
	defaultBranch = main
[advice]
	detachedHead = false
`

// newScene returns a scene whose tree setup (Bash, run in the scene
// directory) builds. extra entries are added to the environment of the
// setup and of both runs.
func newScene(t *testing.T, setup string, extra ...string) *scene {
	t.Helper()
	need2x(t)
	base := t.TempDir()
	home := filepath.Join(base, "home")
	tmp := filepath.Join(base, "tmp")
	require.NoError(t, os.MkdirAll(home, 0o755))
	require.NoError(t, os.MkdirAll(tmp, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(gitConfig), 0o644))
	env := []string{
		"HOME=" + home, "PATH=/usr/local/bin:/usr/bin:/bin", "LC_ALL=C", "TERM=xterm-256color",
		"TMPDIR=" + tmp, "GIT_CONFIG_NOSYSTEM=1", "GIT_CEILING_DIRECTORIES=" + base,
		"GIT_AUTHOR_DATE=1700000000 +0000", "GIT_COMMITTER_DATE=1700000000 +0000",
	}
	root := absRoot
	return &scene{t: t, root: root, setup: setup, base: base, dir: filepath.Join(base, "s"), env: append(env, extra...)}
}

func need2x(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(absRoot, "lib/modules/git.sh")); err != nil {
		t.Skip("the 2.x sources are not in this tree")
	}
	for _, p := range []string{"bash", "git"} {
		if _, err := exec.LookPath(p); err != nil {
			t.Skip(p + " not available")
		}
	}
}

// build recreates the scene directory and runs the setup in it.
func (s *scene) build() {
	s.t.Helper()
	// Leave the tree before removing it: a previous handler run made it
	// the current directory.
	if wd, err := os.Getwd(); err != nil || strings.HasPrefix(wd, s.dir) {
		require.NoError(s.t, os.Chdir(s.base))
	}
	require.NoError(s.t, os.RemoveAll(s.dir))
	require.NoError(s.t, os.MkdirAll(s.dir, 0o755))
	if s.setup == "" {
		return
	}
	cmd := exec.Command("bash", "--noprofile", "--norc", "-euo", "pipefail", "-c", s.setup)
	cmd.Dir, cmd.Env = s.dir, s.env
	out, err := cmd.CombinedOutput()
	require.NoError(s.t, err, string(out))
}

// result is what one run left: its streams, status and the tree.
type result struct {
	Out, Err string
	RC       int
	State    string
}

// mode is how a run sees its terminal (color: a color stdout) and the
// environment entries it adds.
type mode struct {
	color bool
	env   []string
}

// run2x runs the 2.x function in cwd (relative to the scene) of a fresh
// tree: lib/core and lib/modules/git.sh sourced in a non-interactive Bash
// without a controlling terminal, the color gate fixed as m says.
func (s *scene) run2x(m mode, cwd, fn string, args ...string) result {
	s.t.Helper()
	s.build()
	gate := "1"
	if m.color {
		gate = "0"
	}
	script := `root=$1; shift
for f in output utils validation; do source "$root/lib/core/$f.sh"; done
source "$root/lib/modules/git.sh"
__gash_use_color() { return ` + gate + `; }
"$@"`
	cmd := exec.Command("bash", append([]string{"--noprofile", "--norc", "-c", script, "_", s.root, fn}, args...)...)
	cmd.Dir = filepath.Join(s.dir, cwd)
	cmd.Env = append(append([]string{}, s.env...), m.env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // no /dev/tty
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	rc := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		require.True(s.t, errors.As(err, &ee), err)
		rc = ee.ExitCode()
	}
	return result{Out: out.String(), Err: errb.String(), RC: rc, State: s.state()}
}

// runGo runs the handler of fn in cwd of a fresh tree; tm, when not nil,
// changes the call's terminal (a controlling terminal for a question).
func (s *scene) runGo(m mode, cwd, fn string, tm func(*term.Term) *term.Term, args ...string) result {
	s.t.Helper()
	s.build()
	s.t.Chdir(filepath.Join(s.dir, cwd))
	tbl := invoke.NewTable()
	New().Register(tbl)
	env := append(append(append([]string{}, s.env...), m.env...), "GASH__HELP=0")
	e := invoke.EnvFrom(env)
	tt := term.Fixed(e, false, m.color, false)
	if tm != nil {
		tt = tm(tt)
	}
	var out, errb bytes.Buffer
	c := &invoke.Call{Ctx: context.Background(), Name: fn, Args: args, Env: e, Stdout: &out, Stderr: &errb, Term: tt}
	rc := tbl.Run(c)
	return result{Out: out.String(), Err: errb.String(), RC: rc, State: s.state()}
}

// stateScript prints the tree: the files outside git directories with a
// hash of their content, then for every git directory its HEAD, refs,
// local config and empty object files.
const stateScript = `
find . \( -name .git -o -name '*.git' \) -prune -o \( -type f -o -type l \) -print | LC_ALL=C sort |
while IFS= read -r f; do printf '%s %s\n' "$(sha1sum <"$f" | cut -c1-12)" "$f"; done
find . -name HEAD -type f | LC_ALL=C sort | while IFS= read -r h; do
	g=${h%/HEAD}
	[ -d "$g/objects" ] && [ -d "$g/refs" ] || continue
	printf '== %s %s\n' "$g" "$(git --git-dir="$g" symbolic-ref -q HEAD || git --git-dir="$g" rev-parse -q --verify HEAD)"
	git --git-dir="$g" for-each-ref --format='%(refname) %(objectname)'
	git --git-dir="$g" config --local --list
	find "$g/objects" -type f -empty | LC_ALL=C sort
done
`

func (s *scene) state() string {
	s.t.Helper()
	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", stateScript)
	cmd.Dir, cmd.Env = s.dir, s.env
	out, err := cmd.CombinedOutput()
	require.NoError(s.t, err, string(out))
	return string(out)
}

// same runs fn on two fresh trees, the 2.x function then the handler, and
// requires the same streams, status and tree; the handler's result is
// returned.
func (s *scene) same(m mode, cwd, fn string, args ...string) result {
	s.t.Helper()
	want := s.run2x(m, cwd, fn, args...).masked()
	got := s.runGo(m, cwd, fn, nil, args...).masked()
	require.Equal(s.t, want, got, "%s %q", fn, args)
	if testing.Verbose() {
		s.t.Logf("rc=%d\nstdout:\n%s\nstderr:\n%s\nstate:\n%s", got.RC, got.Out, got.Err, got.State)
	}
	return got
}

// mktempName is a name mktemp -d made (TMPDIR/tmp.XXXXXXXXXX).
var mktempName = regexp.MustCompile(`/tmp\.[A-Za-z0-9]{10}\b`)

// masked is r with the random mktemp names replaced.
func (r result) masked() result {
	r.Out = mktempName.ReplaceAllString(r.Out, "/tmp.XXXXXXXXXX")
	r.Err = mktempName.ReplaceAllString(r.Err, "/tmp.XXXXXXXXXX")
	return r
}

var (
	plain = mode{}
	color = mode{color: true}
)

// Scene setups. w is a work tree, origin.git its bare origin.
const (
	setupRepo = `git init -q w && cd w && echo a >a && git add a && git commit -qm first`

	setupOrigin = `git init -q --bare origin.git && git init -q w && cd w &&
git remote add origin ../origin.git &&
echo a >a && git add a && git commit -qm first &&
echo b >>a && git commit -qam second &&
git push -q -u origin main 2>/dev/null`
)

// runFake runs the handler of fn with every process from f, without a
// terminal, and returns its streams and status.
func runFake(t *testing.T, f *runner.Fake, fn string, args ...string) (string, string, int) {
	t.Helper()
	tbl := invoke.NewTable()
	(&Module{Run: f}).Register(tbl)
	e := invoke.EnvFrom([]string{"PATH=/usr/bin:/bin", "GASH__HELP=0"})
	var out, errb bytes.Buffer
	c := &invoke.Call{Ctx: context.Background(), Name: fn, Args: args, Env: e, Stdout: &out, Stderr: &errb,
		Term: term.Fixed(e, false, false, false)}
	rc := tbl.Run(c)
	return out.String(), errb.String(), rc
}
