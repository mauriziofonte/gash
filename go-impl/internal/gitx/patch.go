// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"io"
	"os"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
)

const applyPatchUsage = "git_apply_patch MAIN_BRANCH FEATURE_BRANCH COMMIT_HASH"

// applyPatch is git_apply_patch MAIN FEATURE COMMIT: check out and pull
// MAIN, check out FEATURE, write `git diff-index COMMIT --binary` to
// FEATURE.patch in the current directory, check out MAIN and `git apply
// --3way` it, with `git reset --hard HEAD` when that fails. No question
// is asked (LEDGER GIT-01); a FEATURE with a slash breaks the patch path
// and failures leave another branch checked out (GIT-06).
func (m *Module) applyPatch(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	x := m.begin(c)
	if !x.requireRepo() {
		return 1
	}
	mainBranch, feature, commit := c.Arg(0), c.Arg(1), c.Arg(2)
	if mainBranch == "" || feature == "" || commit == "" {
		x.p.Error("Missing arguments.")
		x.p.Usage(applyPatchUsage)
		return 1
	}
	if !x.git("show-ref", "--verify", "--quiet", "refs/heads/"+mainBranch).Success() {
		x.p.Error("Main branch '" + mainBranch + "' does not exist.")
		return 1
	}
	if !x.git("show-ref", "--verify", "--quiet", "refs/heads/"+feature).Success() {
		x.p.Error("Feature branch '" + feature + "' does not exist.")
		return 1
	}
	if !x.cmd(c.Stdout, io.Discard, "git", "cat-file", "-e", commit).Success() {
		x.p.Error("Commit hash '" + commit + "' is not valid.")
		return 1
	}
	if status, _ := x.gitOutQuiet("status", "--porcelain"); status != "" {
		x.p.Error("Working tree is not clean. Commit or stash your changes before applying a patch.")
		return 1
	}

	x.p.Step("1", "4", "Checking out to main branch '"+mainBranch+"'...")
	if !x.git("checkout", mainBranch).Success() || !x.git("pull", "origin", mainBranch).Success() {
		x.p.Error("Failed to checkout or pull the main branch.")
		return 1
	}

	x.p.Step("2", "4", "Checking out to feature branch '"+feature+"'...")
	if !x.git("checkout", feature).Success() {
		x.p.Error("Failed to checkout the feature branch.")
		return 1
	}

	x.p.Step("3", "4", "Generating patch from '"+feature+"' since commit '"+commit+"'...")
	patch := feature + ".patch"
	ok := false
	if f, opened := redirect(patch, false); opened {
		ok = x.cmd(f, c.Stderr, "git", "diff-index", commit, "--binary").Success()
		_ = f.Close()
	}
	if !ok {
		x.p.Error("Failed to create patch file.")
		x.rmF(patch)
		return 1
	}

	x.p.Step("4", "4", "Applying patch to '"+mainBranch+"'...")
	if !x.git("checkout", mainBranch).Success() {
		x.p.Error("Failed to checkout the main branch.")
		x.rmF(patch)
		return 1
	}
	if !x.git("apply", "--3way", patch).Success() {
		x.p.Error("Failed to apply the patch. Rolling back working tree.")
		x.gitQuiet("reset", "--hard", "HEAD")
		x.p.Info("Patch file kept for inspection: " + patch)
		return 1
	}
	x.rmF(patch)
	x.p.Success("Patch applied successfully from '" + feature + "' to '" + mainBranch + "'.")
	return 0
}

// rmF is rm -f FILE, run as 2.x ran it, so rm's own messages (a
// directory, a permission problem) stay rm's.
func (x *call) rmF(path string) {
	if _, err := os.Lstat(path); err != nil {
		return // rm -f is silent for a missing file
	}
	x.cmd(x.Stdout, x.Stderr, "rm", "-f", path)
}
