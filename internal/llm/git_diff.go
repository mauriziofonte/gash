// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mauriziofonte/gash/internal/gitx"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/runner"
)

const diffHint = "Use --staged, --files, --numstat, --summary, --patch, REV..REV or a path"

// isRevRange reports a word that reads as a revision range (A..B, A...B,
// A.., ..B) rather than a path: it holds "..", does not start like a path
// ("." "/" "~") and names nothing in the working directory.
func isRevRange(a string) bool {
	if !strings.Contains(a, "..") || strings.HasPrefix(a, ".") || strings.HasPrefix(a, "/") || strings.HasPrefix(a, "~") {
		return false
	}
	if strings.ContainsAny(a, " \t\n") {
		return false
	}
	_, err := os.Lstat(a)
	return err != nil
}

// gitDiff is llm_git_diff: `git diff --stat` text by default (gitDiff2x),
// and the 3.4 modes (G5): --files rows {p, add, del, status}, --numstat
// rows {p, add, del}, --summary totals (S4), --patch capped per file, a
// REV..REV range, the output budget on the text outputs.
func (m *Module) gitDiff(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags)
	staged := false
	var files, numstat, summary, patch bool
	rng := ""
	pt := pathTaker{path: "."}
	args := c.Args
	for len(args) > 0 {
		n, ok := o.Take(c, args)
		if !ok {
			return 1
		}
		if n > 0 {
			args = args[n:]
			continue
		}
		a := args[0]
		switch {
		case a == "--staged" || a == "--cached":
			staged = true
		case a == "--files":
			files = true
		case a == "--numstat":
			numstat = true
		case a == "--summary":
			summary = true
		case a == "--patch":
			patch = true
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, diffHint)
		case rng == "" && isRevRange(a):
			rng = a
		default:
			if !pt.take(c, a) {
				return 1
			}
		}
		args = args[1:]
	}
	modes := 0
	for _, b := range []bool{files, numstat, summary, patch} {
		if b {
			modes++
		}
	}
	budget := o.Width > 0 || o.Capped()
	switch {
	case modes == 0 && rng == "" && !budget && o.Format == llmfmt.Default:
		return m.gitDiff2x(c, pt.path, staged)
	case modes > 1:
		return fail(c, "conflicting_options", "--files, --numstat, --summary and --patch", "Pick one of them")
	case staged && rng != "":
		return fail(c, "conflicting_options", "--staged with "+rng, "A range compares two commits; drop --staged")
	case o.Format != llmfmt.Default && !files && !numstat && !summary:
		return fail(c, "conflicting_options", "--format", "--format applies to --files, --numstat and --summary")
	case budget && summary:
		return fail(c, "conflicting_options", "the output budget with --summary", "--summary prints three numbers")
	}
	safe, ok := m.validatePath(c, pt.path)
	if !ok {
		return 1
	}
	if !m.gitPresent(c, c.Name) {
		return 1
	}
	// A FILE as PATH limits the diff to that file (the repository is its
	// directory's).
	dir, spec := safe, ""
	if st, err := os.Stat(safe); err == nil && st.Mode().IsRegular() && modes > 0 {
		dir, spec = filepath.Dir(safe), filepath.Base(safe)
		if patch && m.guard(c).IsSecretFile(safe) {
			return fail(c, "secret_file_blocked", safe, "This file contains secrets: its patch is not shown")
		}
	}
	gitArgs := []string{"-C", dir}
	if spec != "" {
		gitArgs = append(gitArgs, "--literal-pathspecs")
	}
	gitArgs = append(gitArgs, "-c", "core.quotePath=false", "diff", "--no-color", "--no-ext-diff", "--no-textconv")
	if staged {
		gitArgs = append(gitArgs, "--staged")
	}
	tail := []string{}
	if rng != "" {
		tail = append(tail, rng)
	}
	tail = append(tail, "--")
	if spec != "" {
		tail = append(tail, spec)
	}
	env := c.Env.Child()
	switch {
	case patch:
		pf := newPatchFilter(c.Stdout, o)
		res := gitx.Safe(c.Ctx, m.run, env, nil, pf, nil, append(append(gitArgs, patchArgs...), tail...)...)
		if !res.Success() {
			return m.gitFailure(c, dir, res, rng)
		}
		if k := pf.Close(); k > 0 {
			c.NoteTruncated(k)
		}
		return 0
	case modes == 0:
		// --stat text of a range, or under the output budget.
		res := gitx.Safe(c.Ctx, m.run, env, nil, nil, nil, append(append(gitArgs, "--stat"), tail...)...)
		if !res.Success() {
			return m.gitFailure(c, dir, res, rng)
		}
		lw := o.Lines(c.Stdout, "lines")
		for _, l := range strings.SplitAfter(string(res.Stdout), "\n") {
			if l != "" {
				lw.Line(strings.TrimSuffix(l, "\n"))
			}
		}
		if k := lw.Close(); k > 0 {
			c.NoteTruncated(k)
		}
		return 0
	}
	mode := []string{"--numstat", "-z"}
	if files {
		mode = []string{"--raw", "--numstat", "-z"}
	}
	res := gitx.Safe(c.Ctx, m.run, env, nil, nil, nil, append(append(gitArgs, mode...), tail...)...)
	if !res.Success() {
		return m.gitFailure(c, dir, res, rng)
	}
	changes := gitx.ParseRawNumstat(gitx.SplitZ(res.Stdout))
	if summary {
		n, ins, del := gitx.Totals(changes)
		t := ovDiffstat{Files: n, Insertions: ins, Deletions: del}
		if o.Format == llmfmt.TSV || o.Format == llmfmt.TableF {
			o.Write(c, o.Format, llmfmt.Table{Cols: []string{"files", "insertions", "deletions"}, Rows: [][]any{{n, ins, del}}}, nil)
			return 0
		}
		_ = llmfmt.JSONLine(c.Stdout, t)
		return 0
	}
	cols := []string{"p", "add", "del"}
	if files {
		cols = append(cols, "status")
	}
	rows := make([][]any, len(changes))
	for i, f := range changes {
		rows[i] = changeRow(f, files)
	}
	o.Write(c, o.Format, llmfmt.Table{Cols: cols, Rows: rows}, nil)
	return 0
}

// changeRow is one file of a diff as a row: path, added and deleted lines
// (null for a binary file), and the status letter when withStatus.
func changeRow(f gitx.FileChange, withStatus bool) []any {
	var add, del any = f.Add, f.Del
	if f.Binary {
		add, del = nil, nil
	}
	row := []any{f.Path, add, del}
	if withStatus {
		row = append(row, f.Status)
	}
	return row
}

// gitFailure writes the envelope of a git read that failed in dir:
// not_a_git_repo when dir is not in a repository, unknown_revision when
// one of revs does not resolve, git_error with git's own message
// otherwise. The checks run only on this failure path, so a successful
// read costs one git process.
func (m *Module) gitFailure(c *invoke.Call, dir string, res runner.Result, revs ...string) int {
	env := c.Env.Child()
	if !gitx.Safe(c.Ctx, m.run, env, nil, io.Discard, io.Discard, "-C", dir, "rev-parse", "--git-dir").Success() {
		return notARepo(c, dir)
	}
	for _, r := range revs {
		if r == "" {
			continue
		}
		if !gitx.Safe(c.Ctx, m.run, env, nil, io.Discard, io.Discard, "-C", dir, "rev-parse", r, "--").Success() {
			return fail(c, "unknown_revision", r, "Name a commit that exists: a hash, a branch, a tag, HEAD~N")
		}
	}
	return fail(c, "git_error", gitx.LastLine(res.Stderr, 300), "")
}
