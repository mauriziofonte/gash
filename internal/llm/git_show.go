// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/gitx"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/pathguard"
)

// showContentLines is the line budget of llm_git_show REV:FILE when no
// --max-lines, --head or --tail is given.
const showContentLines = 2000

const showHint = "Use --stat, --files, --patch, --max-lines N, REV, REV:FILE or a path"

// showFile and showFileStatus are the rows of the "files" array of
// llm_git_show (with --files, the status letter).
type showFile struct {
	P   string `json:"p"`
	Add any    `json:"add"`
	Del any    `json:"del"`
}

type showFileStatus struct {
	P      string `json:"p"`
	Add    any    `json:"add"`
	Del    any    `json:"del"`
	Status string `json:"status"`
}

// showJSON is the llm_git_show object (G4).
type showJSON struct {
	Hash      string      `json:"hash"`
	Subject   string      `json:"subject"`
	Author    string      `json:"author"`
	Date      string      `json:"date"`
	Files     []any       `json:"files"`
	Stat      *ovDiffstat `json:"stat,omitempty"`
	Truncated int         `json:"truncated,omitempty"`
}

// splitRevFile splits REV:FILE at its first colon outside braces (git's
// own rule, so HEAD@{10:00}:f works); ":/text" is a commit search, not a
// file.
func splitRevFile(s string) (rev, file string, ok bool) {
	if strings.HasPrefix(s, ":/") {
		return "", "", false
	}
	depth := 0
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '{':
			depth++
		case s[i] == '}' && depth > 0:
			depth--
		case s[i] == ':' && depth == 0:
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}

// repoDirSpec splits a validated PATH into the directory git runs in and
// a literal pathspec: a FILE limits the output to itself.
func repoDirSpec(safe string) (dir, spec string) {
	if st, err := os.Stat(safe); err == nil && st.Mode().IsRegular() {
		return filepath.Dir(safe), filepath.Base(safe)
	}
	return safe, ""
}

// gitShow is llm_git_show (G4): a commit's metadata and files as one
// object (--stat adds the totals, --files the status letters), its patch
// capped per file (--patch), or the content of FILE at REV (REV:FILE)
// under the output budget. REV defaults to HEAD.
func (m *Module) gitShow(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.BudgetFlags)
	var stat, files, patch bool
	var pos []string
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
		case a == "--stat":
			stat = true
		case a == "--files":
			files = true
		case a == "--patch":
			patch = true
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, showHint)
		case len(pos) == 2:
			return fail(c, "unexpected_argument", a, "Pass REV and at most one PATH")
		default:
			pos = append(pos, a)
		}
		args = args[1:]
	}
	rev, path := "HEAD", "."
	if len(pos) > 0 {
		rev = pos[0]
	}
	if len(pos) > 1 {
		path = pos[1]
	}
	if rev == "" {
		return fail(c, "unknown_revision", "", "Name a commit: a hash, a branch, a tag, HEAD~N")
	}
	if patch && (stat || files) {
		return fail(c, "conflicting_options", "--patch with --stat or --files", "--patch prints the diff text; drop --stat and --files")
	}
	safe, ok := m.validatePath(c, path)
	if !ok {
		return 1
	}
	if !m.gitPresent(c, c.Name) {
		return 1
	}
	if r, file, ok := splitRevFile(rev); ok {
		if stat || files || patch {
			return fail(c, "conflicting_options", "REV:FILE with --stat, --files or --patch", "REV:FILE prints the file's content")
		}
		return m.showContent(c, o, safe, r, file)
	}
	dir, spec := repoDirSpec(safe)
	gitArgs := []string{"-C", dir}
	if spec != "" {
		gitArgs = append(gitArgs, "--literal-pathspecs")
	}
	tail := []string{rev + "^{commit}", "--"}
	if spec != "" {
		tail = append(tail, spec)
	}
	env := c.Env.Child()
	if patch {
		if spec != "" && m.guard(c).IsSecretFile(safe) {
			return fail(c, "secret_file_blocked", safe, "This file contains secrets: its patch is not shown")
		}
		pf := newPatchFilter(c.Stdout, o)
		a := append(append(gitArgs, "-c", "core.quotePath=false", "show", "--first-parent",
			"--format=commit %h%nAuthor: %an%nDate:   %ci%n%n%w(0,4,4)%B"), patchArgs...)
		res := gitx.Safe(c.Ctx, m.run, env, nil, pf, nil, append(a, tail...)...)
		if !res.Success() {
			return m.gitFailure(c, dir, res, rev)
		}
		if k := pf.Close(); k > 0 {
			c.NoteTruncated(k)
		}
		return 0
	}
	a := append(gitArgs, "show", "--first-parent", "--no-color", "--no-ext-diff", "--no-textconv", "-z", "--numstat",
		"--format=%h%x00%s%x00%an%x00%ci%x00")
	if files {
		a = append(a, "--raw")
	}
	res := gitx.Safe(c.Ctx, m.run, env, nil, nil, nil, append(a, tail...)...)
	if !res.Success() {
		return m.gitFailure(c, dir, res, rev)
	}
	t := gitx.SplitZ(res.Stdout)
	if len(t) < 4 {
		return fail(c, "git_error", "unexpected git show output", "")
	}
	rest := t[4:]
	for i, f := range rest {
		if f != "" {
			rest[i] = strings.TrimPrefix(f, "\n")
			break
		}
	}
	changes := gitx.ParseRawNumstat(rest)
	obj := showJSON{Hash: t[0], Subject: llmfmt.Cut(t[1], o.Width), Author: t[2], Date: t[3], Files: []any{}}
	if stat {
		nf, ins, del := gitx.Totals(changes)
		obj.Stat = &ovDiffstat{Files: nf, Insertions: ins, Deletions: del}
	}
	head, tl, omitted := llmfmt.Cap(o, changes)
	for _, part := range [][]gitx.FileChange{head, tl} {
		for _, f := range part {
			row := changeRow(f, files)
			p := llmfmt.Cut(f.Path, o.Width)
			if files {
				obj.Files = append(obj.Files, showFileStatus{P: p, Add: row[1], Del: row[2], Status: f.Status})
			} else {
				obj.Files = append(obj.Files, showFile{P: p, Add: row[1], Del: row[2]})
			}
		}
	}
	if omitted > 0 {
		obj.Truncated = omitted
		c.NoteTruncated(omitted)
	}
	_ = llmfmt.JSONLine(c.Stdout, obj)
	return 0
}

// showContent prints FILE at REV (git show REV:FILE: a blob's content, a
// tree's entry names, one per line) under the output budget, at most showContentLines
// lines by default. A secret FILE is refused as llm_config refuses it; a
// binary blob is binary_file.
func (m *Module) showContent(c *invoke.Call, o *llmfmt.Options, safe, rev, file string) int {
	if file != "" && pathguard.SecretName(strings.TrimPrefix(file, "./")) {
		return fail(c, "secret_file_blocked", rev+":"+file, "This file contains secrets and cannot be read")
	}
	env := c.Env.Child()
	res := gitx.Safe(c.Ctx, m.run, env, nil, nil, nil, "-C", safe, "show", "--no-color", "--no-ext-diff", "--no-textconv", rev+":"+file, "--")
	if !res.Success() {
		if !gitx.Safe(c.Ctx, m.run, env, nil, io.Discard, io.Discard, "-C", safe, "rev-parse", "--git-dir").Success() {
			return notARepo(c, safe)
		}
		if rev != "" && !gitx.Safe(c.Ctx, m.run, env, nil, io.Discard, io.Discard, "-C", safe, "rev-parse", "--verify", "--quiet", rev+"^{commit}").Success() {
			return fail(c, "unknown_revision", rev, "Name a commit that exists: a hash, a branch, a tag, HEAD~N")
		}
		return fail(c, "file_not_found", file+" at "+rev, "Paths in REV:FILE are relative to the repository root (./FILE: to PATH)")
	}
	head := res.Stdout
	if len(head) > 8000 {
		head = head[:8000]
	}
	if bytes.IndexByte(head, 0) >= 0 {
		return fail(c, "binary_file", rev+":"+file+" ("+strconv.Itoa(len(res.Stdout))+" bytes)", "Binary content is not printed")
	}
	budget := *o
	if !budget.Capped() {
		budget.Head = showContentLines
	}
	lw := budget.Lines(c.Stdout, "lines")
	// A tree prints its entries (directories end in "/") under a "tree
	// REV:FILE" header, which is left out.
	s := strings.TrimPrefix(string(res.Stdout), "tree "+rev+":"+file+"\n\n")
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			lw.Line(s)
			break
		}
		lw.Line(s[:i])
		s = s[i+1:]
	}
	if k := lw.Close(); k > 0 {
		c.NoteTruncated(k)
	}
	return 0
}
