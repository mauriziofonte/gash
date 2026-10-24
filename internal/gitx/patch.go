// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"io"
	"os"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
)

const applyPatchUsage = "git_apply_patch MAIN_BRANCH FEATURE_BRANCH COMMIT_HASH [-y|--yes] [--dry-run]"

// applyPatch is git_apply_patch MAIN FEATURE COMMIT [-y|--yes]
// [--dry-run]: after the confirmation (LEDGER GIT-01), check out and pull
// MAIN, check out FEATURE, write `git diff-index COMMIT --binary` to a
// temporary gash-patch-*.patch file, check out MAIN and `git apply
// --3way` it, with `git reset --hard HEAD` when that fails. Any failure
// checks out the starting branch again (GIT-06).
func (m *Module) applyPatch(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	x := m.begin(c)
	pos, yes, dryRun := gateArgs(c.Args)
	if !x.requireRepo() {
		return 1
	}
	mainBranch, feature, commit := arg(pos, 0), arg(pos, 1), arg(pos, 2)
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
	plan := "check out " + mainBranch + ", pull origin " + mainBranch + " and apply " + feature + "'s changes since " + commit
	if dryRun {
		_, _ = io.WriteString(c.Stdout, "Would "+plan+".\n")
		return 0
	}
	if !x.confirm(yes, "apply the patch", "Will "+plan) {
		return 1
	}

	// The starting branch (or commit, on a detached HEAD), checked out
	// again on every failure.
	start, ok := x.gitOutQuiet("symbolic-ref", "--quiet", "--short", "HEAD")
	if !ok {
		start, _ = x.gitOutQuiet("rev-parse", "HEAD")
	}
	fail := func() int {
		if start != "" {
			x.cmd(io.Discard, c.Stderr, "git", "checkout", "--quiet", start)
		}
		return 1
	}

	x.p.Step("1", "4", "Checking out to main branch '"+mainBranch+"'...")
	if !x.git("checkout", mainBranch).Success() || !x.git("pull", "origin", mainBranch).Success() {
		x.p.Error("Failed to checkout or pull the main branch.")
		return fail()
	}

	x.p.Step("2", "4", "Checking out to feature branch '"+feature+"'...")
	if !x.git("checkout", feature).Success() {
		x.p.Error("Failed to checkout the feature branch.")
		return fail()
	}

	x.p.Step("3", "4", "Generating patch from '"+feature+"' since commit '"+commit+"'...")
	f, err := os.CreateTemp(x.Env.Get("TMPDIR"), "gash-patch-*.patch")
	if err != nil {
		x.p.Error("Failed to create patch file.")
		return fail()
	}
	patch := f.Name()
	ok = x.cmd(f, c.Stderr, "git", "diff-index", commit, "--binary").Success()
	if cerr := f.Close(); cerr != nil {
		ok = false
	}
	if !ok {
		x.p.Error("Failed to create patch file.")
		_ = os.Remove(patch)
		return fail()
	}

	x.p.Step("4", "4", "Applying patch to '"+mainBranch+"'...")
	if !x.git("checkout", mainBranch).Success() {
		x.p.Error("Failed to checkout the main branch.")
		_ = os.Remove(patch)
		return fail()
	}
	if !x.git("apply", "--3way", patch).Success() {
		x.p.Error("Failed to apply the patch. Rolling back working tree.")
		x.gitQuiet("reset", "--hard", "HEAD")
		x.p.Info("Patch file kept for inspection: " + patch)
		return fail()
	}
	_ = os.Remove(patch)
	x.p.Success("Patch applied successfully from '" + feature + "' to '" + mainBranch + "'.")
	return 0
}
