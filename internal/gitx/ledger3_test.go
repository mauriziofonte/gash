// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

// The proving tests of the 3.0 GIT fixes. They need no 2.x sources: the
// handler runs on a real repository the scene setup builds (bash and git
// only), or with a runner fake.

// goScene is a built scene for the 3.0 tests.
type goScene struct{ *scene }

func newGoScene(t *testing.T, setup string, extra ...string) goScene {
	t.Helper()
	for _, p := range []string{"bash", "git"} {
		if _, err := exec.LookPath(p); err != nil {
			t.Skip(p + " not available")
		}
	}
	s := goScene{sceneFor(t, setup, extra...)}
	s.build()
	return s
}

// run runs the handler in cwd of the current tree (not rebuilt), without
// a controlling terminal unless tm gives one.
func (s goScene) run(m mode, cwd string, tm func(*term.Term) *term.Term, fn string, args ...string) result {
	s.t.Helper()
	if tm == nil {
		tm = (*term.Term).WithoutTTY
	}
	return s.handler(m, cwd, fn, tm, args...)
}

// git runs git ARGS in cwd of the scene and returns its trimmed stdout
// and whether it succeeded.
func (s goScene) git(cwd string, args ...string) (string, bool) {
	s.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = filepath.Join(s.dir, cwd), s.env
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err == nil
}

func tty(input string) func(*term.Term) *term.Term {
	return func(t *term.Term) *term.Term { return t.WithTTY(input) }
}

// 3.0: GIT-01 fixed. A tag push, a tag deletion, a sync and a patch ask
// first; -y/--yes (any position) skips the question and headless or
// without a terminal they refuse without it.
func TestLedger_GIT01(t *testing.T) {
	t.Run("delete tag", func(t *testing.T) {
		s := newGoScene(t, setupTags)
		r := s.run(headless, "w", nil, "git_delete_tag", "v2")
		require.Equal(t, 1, r.RC)
		require.Equal(t, "Error: Refusing to delete tag 'v2' in headless mode without --yes.\n", r.Err)
		out, _ := s.git("w", "ls-remote", "--tags", "origin", "refs/tags/v2")
		require.Contains(t, out, "refs/tags/v2")
		_, ok := s.git("w", "rev-parse", "-q", "--verify", "refs/tags/v2")
		require.True(t, ok)

		r = s.run(plain, "w", nil, "git_delete_tag", "v2")
		require.Equal(t, 1, r.RC, "no terminal")
		require.Contains(t, r.Err, "Refusing to delete tag 'v2'")

		r = s.run(plain, "w", tty("n\n"), "git_delete_tag", "v2")
		require.Equal(t, 1, r.RC)
		origin, _ := s.git("w", "remote", "get-url", "origin")
		require.Equal(t, "Will delete refs/tags/v2 locally and on origin ("+origin+")\n", r.Out)
		require.Contains(t, r.Err, "Proceed? [y/N]")
		require.Contains(t, r.Err, "Info: Cancelled.\n")

		r = s.run(headless, "w", nil, "git_delete_tag", "v2", "--dry-run")
		require.Equal(t, 0, r.RC)
		require.Equal(t, "Would delete refs/tags/v2 locally and on origin ("+origin+").\n", r.Out)

		r = s.run(headless, "w", nil, "git_delete_tag", "v2", "--yes")
		require.Equal(t, 0, r.RC, r.Err)
		out, _ = s.git("w", "ls-remote", "--tags", "origin", "refs/tags/v2")
		require.Empty(t, out)
		_, ok = s.git("w", "rev-parse", "-q", "--verify", "refs/tags/v2")
		require.False(t, ok)
	})

	t.Run("add tag", func(t *testing.T) {
		s := newGoScene(t, setupOrigin)
		r := s.run(headless, "w", nil, "git_add_tag", "v2")
		require.Equal(t, 1, r.RC)
		require.Equal(t, "Error: Refusing to push tag 'v2' in headless mode without --yes.\n", r.Err)
		_, ok := s.git("w", "rev-parse", "-q", "--verify", "refs/tags/v2")
		require.False(t, ok, "no local tag")

		r = s.run(headless, "w", nil, "git_add_tag", "--dry-run", "v2")
		require.Equal(t, 0, r.RC)
		require.Contains(t, r.Out, "Would create tag 'v2' and push refs/tags/v2 to origin (")
		_, ok = s.git("w", "rev-parse", "-q", "--verify", "refs/tags/v2")
		require.False(t, ok)

		r = s.run(plain, "w", tty("y\n"), "git_add_tag", "v2", "two")
		require.Equal(t, 0, r.RC, r.Err)
		require.True(t, strings.HasPrefix(r.Out, "Will push refs/tags/v2 to origin ("), r.Out)
		require.Contains(t, r.Err, "OK: Tag 'v2' pushed to remote.")

		// --yes first, then TAG and MESSAGE.
		r = s.run(headless, "w", nil, "git_add_tag", "-y", "v3", "three")
		require.Equal(t, 0, r.RC, r.Err)
		msg, _ := s.git("w", "tag", "-l", "--format=%(contents:subject)", "v3")
		require.Equal(t, "three", msg)
		out, _ := s.git("w", "ls-remote", "--tags", "origin", "refs/tags/v3")
		require.Contains(t, out, "refs/tags/v3")
	})

	t.Run("sync", func(t *testing.T) {
		s := newGoScene(t, setupOrigin+" && echo s >.env.local && echo m >>a")
		head, _ := s.git("w", "rev-parse", "HEAD")
		status, _ := s.git("w", "status", "--porcelain")
		r := s.run(headless, "w", nil, "git_sync")
		require.Equal(t, 1, r.RC)
		require.Equal(t, "Error: Refusing to commit and push in headless mode without --yes.\n", r.Err)
		now, _ := s.git("w", "rev-parse", "HEAD")
		require.Equal(t, head, now)
		st, _ := s.git("w", "status", "--porcelain")
		require.Equal(t, status, st)

		r = s.run(headless, "w", nil, "git_sync", "--no-push")
		require.Equal(t, "Error: Refusing to commit in headless mode without --yes.\n", r.Err)

		r = s.run(plain, "w", tty("n\n"), "git_sync")
		require.Equal(t, 1, r.RC)
		require.Equal(t, "Will commit 2 file(s) on main (1 untracked) and push to origin/main\n", r.Out)
		require.Contains(t, r.Err, "Info: Cancelled.\n")

		r = s.run(headless, "w", nil, "git_sync", "--yes")
		require.Equal(t, 0, r.RC, r.Err)
		require.Contains(t, r.Err, "OK: Pushed main to origin/main.")
		local, _ := s.git("w", "rev-parse", "HEAD")
		remote, _ := s.git("origin.git", "rev-parse", "main")
		require.Equal(t, local, remote)

		// Nothing to commit and nothing to push: no question.
		r = s.run(headless, "w", nil, "git_sync")
		require.Equal(t, 0, r.RC, r.Err)
	})

	t.Run("apply patch", func(t *testing.T) {
		s := newGoScene(t, setupPatch+" && git checkout -q -b other")
		r := s.run(headless, "w", nil, "git_apply_patch", "main", "feature", "base")
		require.Equal(t, 1, r.RC)
		require.Equal(t, "Error: Refusing to apply the patch in headless mode without --yes.\n", r.Err)
		b, _ := s.git("w", "symbolic-ref", "--short", "HEAD")
		require.Equal(t, "other", b)

		r = s.run(headless, "w", nil, "git_apply_patch", "main", "feature", "base", "--dry-run")
		require.Equal(t, 0, r.RC)
		require.Equal(t, "Would check out main, pull origin main and apply feature's changes since base.\n", r.Out)

		r = s.run(headless, "w", nil, "git_apply_patch", "--yes", "main", "feature", "base")
		require.Equal(t, 0, r.RC, r.Err)
		require.Contains(t, r.Err, "Patch applied successfully from 'feature' to 'main'.")
	})
}

// 3.0: GIT-02 fixed. Peeled tags are not listed, and a failing ls-remote
// is an error.
func TestLedger_GIT02(t *testing.T) {
	s := newGoScene(t, setupTags)
	r := s.run(plain, "w", nil, "git_list_tags")
	require.Equal(t, 0, r.RC, r.Err)
	require.Equal(t, "Local tags:\nv1\nv2\nRemote tags:\nv0.9\nv2\n", r.Out)

	_, ok := s.git("w", "remote", "set-url", "origin", "../none.git")
	require.True(t, ok)
	r = s.run(plain, "w", nil, "git_list_tags")
	require.Equal(t, 1, r.RC)
	require.Equal(t, "Local tags:\nv1\nv2\n", r.Out)
	require.True(t, strings.HasSuffix(r.Err, "Error: Cannot list the tags of origin.\n"), r.Err)
}

// 3.0: GIT-03 fixed. An unreachable origin is an error before any local
// change.
func TestLedger_GIT03(t *testing.T) {
	s := newGoScene(t, setupRepo+" && git remote add origin ../none.git && git tag v9")
	r := s.run(headless, "w", nil, "git_delete_tag", "v9", "--yes")
	require.Equal(t, 1, r.RC)
	require.Equal(t, "Error: Cannot reach origin (../none.git): nothing was created or deleted.\n", r.Err)
	_, ok := s.git("w", "rev-parse", "-q", "--verify", "refs/tags/v9")
	require.True(t, ok, "the local tag is kept")

	r = s.run(headless, "w", nil, "git_add_tag", "v8", "--yes")
	require.Equal(t, 1, r.RC)
	require.Equal(t, "Error: Cannot reach origin (../none.git): nothing was created or deleted.\n", r.Err)
	_, ok = s.git("w", "rev-parse", "-q", "--verify", "refs/tags/v8")
	require.False(t, ok, "no local tag is created")
}

// 3.0: GIT-04 fixed. A tag name starting with '-' is refused before any
// git tag call.
func TestLedger_GIT04(t *testing.T) {
	for _, fn := range []string{"git_add_tag", "git_delete_tag"} {
		for _, name := range []string{"-s", "-f", "--list"} {
			f := runner.NewFake(t,
				runner.Expect{Argv: []string{"git", "rev-parse", "--is-inside-work-tree"}},
				runner.Expect{Argv: []string{"git", "remote", "get-url", "origin"}},
			)
			out, errOut, rc := runFake(t, f, fn, "--", name, "--yes")
			require.Equal(t, 1, rc)
			require.Empty(t, out)
			require.Equal(t, "Error: Invalid tag name: '"+name+"'.\n", errOut)
			require.NoError(t, f.Verify())
			for _, c := range f.Calls() {
				require.NotEqual(t, "tag", c.Argv[1], "no git tag call")
			}
		}
	}
	// Without --, an option-like word that is not a gash flag is the name.
	f := runner.NewFake(t,
		runner.Expect{Argv: []string{"git", "rev-parse", "--is-inside-work-tree"}},
		runner.Expect{Argv: []string{"git", "remote", "get-url", "origin"}},
	)
	_, errOut, rc := runFake(t, f, "git_add_tag", "-s")
	require.Equal(t, 1, rc)
	require.Equal(t, "Error: Invalid tag name: '-s'.\n", errOut)
}

// 3.0: GIT-05 fixed. FILE follows --, is read at its work-tree path, and
// a git failure is status 1 with no empty file left.
func TestLedger_GIT05(t *testing.T) {
	s := newGoScene(t, setupRepo+" && echo m >main && git add main && git commit -qm m")
	r := s.run(plain, "w", nil, "git_dump_revisions", "main")
	require.Equal(t, 0, r.RC, r.Err)
	b, err := os.ReadFile(filepath.Join(s.dir, "w", "main.001."+short(s, "w")))
	require.NoError(t, err)
	require.Equal(t, "m\n", string(b))

	s = newGoScene(t, setupHistory)
	rel := s.run(plain, "w", nil, "git_dump_revisions", "sub/f.txt")
	require.Equal(t, 0, rel.RC, rel.Err)
	files := dumped(t, filepath.Join(s.dir, "w/sub"))
	require.Len(t, files, 6)
	abs := filepath.Join(s.dir, "w/sub/f.txt")
	for _, f := range files {
		require.NoError(t, os.Remove(filepath.Join(s.dir, "w/sub", f)))
	}
	r = s.run(plain, "w", nil, "git_dump_revisions", abs)
	require.Equal(t, 0, r.RC, r.Err)
	require.Equal(t, files, dumped(t, filepath.Join(s.dir, "w/sub")))
	newest, err := os.ReadFile(filepath.Join(s.dir, "w/sub", files[0]))
	require.NoError(t, err)
	require.Equal(t, "three\n", string(newest), "001 is the newest revision")

	// A git failure: the file is not tracked.
	s = newGoScene(t, setupHistory+" && touch new")
	r = s.run(plain, "w", nil, "git_dump_revisions", "new")
	require.Equal(t, 1, r.RC)
	require.True(t, strings.HasSuffix(r.Err, "Error: git failed on 'new'.\n"), r.Err)
	require.Empty(t, dumped(t, filepath.Join(s.dir, "w")))
}

// short is the short hash of HEAD in cwd.
func short(s goScene, cwd string) string {
	h, _ := s.git(cwd, "rev-parse", "--short", "HEAD")
	return h
}

// dumped lists the FILE.NNN.HASH outputs of dir, sorted.
func dumped(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		if n := e.Name(); strings.Contains(n, ".00") {
			out = append(out, n)
		}
	}
	return out
}

// 3.0: GIT-06 fixed. The patch goes to a temporary file (a slash in
// FEATURE is fine), and a failure checks out the starting branch again.
func TestLedger_GIT06(t *testing.T) {
	setup := setupOrigin + ` && git tag base && git checkout -q -b feat/x && echo f >f && git add f &&
git commit -qm f && git checkout -q main && git checkout -q -b other`
	s := newGoScene(t, setup)
	r := s.run(plain, "w", nil, "git_apply_patch", "main", "feat/x", "base", "--yes")
	require.Equal(t, 0, r.RC, r.Err)
	b, err := os.ReadFile(filepath.Join(s.dir, "w/f"))
	require.NoError(t, err)
	require.Equal(t, "f\n", string(b))
	_, err = os.Stat(filepath.Join(s.dir, "w/feat"))
	require.True(t, os.IsNotExist(err), "no patch file in the work tree")

	// main changes a's line after base: the 3-way apply conflicts.
	setup = setupPatch + ` && printf 'z\nz\n' >a && git commit -qam "main conflict" && git push -q origin main 2>/dev/null &&
git checkout -q -b start`
	s = newGoScene(t, setup)
	tmp := filepath.Join(s.base, "tmp")
	r = s.run(plain, "w", nil, "git_apply_patch", "main", "feature", "base", "-y")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Err, "Info: Patch file kept for inspection: "+tmp+"/gash-patch-")
	br, _ := s.git("w", "symbolic-ref", "--short", "HEAD")
	require.Equal(t, "start", br)
	kept, _ := filepath.Glob(filepath.Join(tmp, "gash-patch-*.patch"))
	require.Len(t, kept, 1)

	// A failing pull: the starting branch again.
	s = newGoScene(t, setupPatch+" && git checkout -q -b start && git remote set-url origin ../none.git")
	r = s.run(plain, "w", nil, "git_apply_patch", "main", "feature", "base", "-y")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Err, "Error: Failed to checkout or pull the main branch.")
	br, _ = s.git("w", "symbolic-ref", "--short", "HEAD")
	require.Equal(t, "start", br)
}

// 3.0: GIT-07 fixed. The push goes to the upstream branch by name, the
// target --dry-run prints.
func TestLedger_GIT07(t *testing.T) {
	setup := setupOrigin + " && git checkout -q -b work --track origin/main && echo b >b"
	s := newGoScene(t, setup)
	r := s.run(plain, "w", nil, "git_sync", "--dry-run")
	require.Equal(t, 0, r.RC)
	require.Contains(t, r.Out, "Would push work to origin/main.\n")

	r = s.run(plain, "w", nil, "git_sync", "-m", "x", "--yes")
	require.Equal(t, 0, r.RC, r.Err)
	require.Contains(t, r.Err, "OK: Pushed work to origin/main.")
	local, _ := s.git("w", "rev-parse", "HEAD")
	remote, _ := s.git("origin.git", "rev-parse", "main")
	require.Equal(t, local, remote, "origin/main advanced")
}

// 3.0: GIT-08 fixed. An option-like origin URL is refused before the
// plan, only empty loose objects count, and a failed clone deletes
// nothing.
func TestLedger_GIT08(t *testing.T) {
	emptyCount := func(s goScene) int {
		dir, _ := s.git("w", "rev-parse", "--path-format=absolute", "--git-path", "objects")
		return len(emptyFiles(dir))
	}
	s := newGoScene(t, setupCorrupt+` && git config remote.origin.url "--upload-pack=touch marker"`)
	r := s.run(plain, "w", nil, "git_repair", "-y")
	require.Equal(t, 1, r.RC)
	require.Equal(t, "Error: The origin URL starts with '-': refusing to pass it to git clone.\n", r.Err)
	require.Empty(t, r.Out, "refused before the plan")
	require.Equal(t, 2, emptyCount(s), "the empty objects are still there")

	s = newGoScene(t, setupCorrupt+" && mkdir -p .git/objects/pack .git/objects/info && : >.git/objects/pack/x.keep && : >.git/objects/info/x")
	require.Equal(t, 2, emptyCount(s), "a .keep or info file is not a loose object")
	r = s.run(plain, "w", nil, "git_repair", "-y")
	require.Equal(t, 0, r.RC, r.Err)
	require.Contains(t, r.Err, "[2/5] Removing 2 corrupted (0-byte) loose object(s)...")
	_, err := os.Stat(filepath.Join(s.dir, "w/.git/objects/pack/x.keep"))
	require.NoError(t, err, "the .keep file stays")

	s = newGoScene(t, setupCorrupt+" && git remote set-url origin ../none.git")
	r = s.run(plain, "w", nil, "git_repair", "-y")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Err, "nothing was deleted")
	require.Equal(t, 2, emptyCount(s), "a failed clone deletes nothing")
}

// 3.0: GIT-11 fixed. The registered help (the source of `gash help`,
// NAME -h and docs/git/reference.md) says what the code does.
func TestLedger_GIT11(t *testing.T) {
	body := func(fn string) (string, *registry.Usage) {
		f, ok := registry.Lookup(fn)
		require.True(t, ok, fn)
		return f.Help.Body, f.Fallback
	}
	add, _ := body("git_add_tag")
	require.NotContains(t, add, "clean")
	patch, fb := body("git_apply_patch")
	require.NotContains(t, patch, "gap")
	require.NotContains(t, fb.Description, "gap")
	require.Contains(t, fb.Description, "gpatch")
	require.Contains(t, patch, "git diff-index COMMIT --binary")
	require.Contains(t, patch, "COMMIT only has to exist")
	dump, _ := body("git_dump_revisions")
	require.Contains(t, dump, "001 is the newest revision")
	// Every gated function names its flags in the usage line.
	for _, fn := range []string{"git_add_tag", "git_delete_tag", "git_apply_patch", "git_sync"} {
		b, fb := body(fn)
		line, _, _ := strings.Cut(strings.TrimPrefix(b, "USAGE\n"), "\n")
		for _, w := range []string{"-y", "--yes", "--dry-run"} {
			require.Contains(t, line, w, fn)
			require.Contains(t, fb.Usage, w, fn)
		}
	}
	require.Contains(t, testFile(t, "../../docs/security.md"), "| `git_add_tag`, `git_delete_tag` |")
	require.Contains(t, testFile(t, "../../docs/security.md"), "| `git_sync` |")
	require.Contains(t, testFile(t, "../../docs/security.md"), "| `git_apply_patch` |")
	ref := testFile(t, "../../docs/git/reference.md")
	require.Contains(t, ref, "001 is the newest revision")
	require.NotContains(t, ref, "Requires a clean git repo")
}

func testFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}
