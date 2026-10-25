// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/gitx"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/runner"
)

// commitTimeout bounds each git step of agent_git_commit that can run the
// user's hooks or reach the network (commit, push) when --timeout is not
// given.
const commitTimeout = 120 * time.Second

// commitDryPaths caps the "paths" list of a --dry-run report when no
// --max-lines is given.
const commitDryPaths = 100

var authorRE = regexp.MustCompile(`^[^<>\n]+ <[^<>\n]*>$`)

const commitHint = "Use -m, --add PATH..., -A, --author, --push [REMOTE], --dry-run, --force, --timeout or a path"

// commitArgs is a parsed agent_git_commit line.
type commitArgs struct {
	messages []string
	adds     []string
	all      bool
	author   string
	push     bool
	remote   string
	dry      bool
	force    bool
	path     string
	pathSet  bool
}

// commitResult is the report of agent_git_commit (G2), at most ~150
// bytes: the new commit, its branch and file count, the push and how far
// the branch is ahead of what it is compared with (null without one).
type commitResult struct {
	Hash   string `json:"hash"`
	Branch string `json:"branch"`
	Files  int    `json:"files"`
	Pushed bool   `json:"pushed"`
	Remote string `json:"remote"`
	Ahead  *int   `json:"ahead"`
}

// commitPlan is the --dry-run report: what the commit would hold and where
// it would go, nothing written.
type commitPlan struct {
	Branch    string   `json:"branch"`
	Files     int      `json:"files"`
	Paths     []string `json:"paths"`
	Truncated int      `json:"truncated,omitempty"`
	Push      bool     `json:"push"`
	Remote    string   `json:"remote"`
	Ref       string   `json:"ref,omitempty"`
	Ahead     *int     `json:"ahead"`
	DryRun    bool     `json:"dry_run"`
	Written   bool     `json:"written"`
}

func (m *Module) parseCommit(c *invoke.Call, o *llmfmt.Options) (commitArgs, bool) {
	var a commitArgs
	pushWord := ""
	args := c.Args
	for len(args) > 0 {
		n, ok := o.Take(c, args)
		if !ok {
			return a, false
		}
		if n > 0 {
			args = args[n:]
			continue
		}
		w := args[0]
		switch w {
		case "-m", "--message", "--author":
			if len(args) < 2 {
				fail(c, "missing_value", w, w+" needs a value")
				return a, false
			}
			if w == "--author" {
				a.author = args[1]
			} else {
				a.messages = append(a.messages, args[1])
			}
			args = args[2:]
			continue
		case "--add":
			args = args[1:]
			k := 0
			for k < len(args) && !strings.HasPrefix(args[k], "-") {
				a.adds = append(a.adds, args[k])
				k++
			}
			if k == 0 {
				fail(c, "missing_value", w, "--add needs one or more paths")
				return a, false
			}
			args = args[k:]
			continue
		case "-A", "--all":
			a.all = true
		case "--push":
			a.push = true
			if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
				pushWord = args[1]
				args = args[2:]
				continue
			}
		case "--dry-run":
			a.dry = true
		case "--force":
			a.force = true
		default:
			if strings.HasPrefix(w, "-") {
				fail(c, "unknown_option", w, commitHint)
				return a, false
			}
			if a.pathSet {
				fail(c, "unexpected_argument", w, "Pass at most one PATH (the repository)")
				return a, false
			}
			a.path, a.pathSet = w, true
		}
		args = args[1:]
	}
	if pushWord != "" {
		// The word after --push is the REMOTE, unless it names a directory
		// and no PATH was given: then it is the PATH (agent_git_commit ...
		// --push repo).
		if st, err := os.Stat(pushWord); err == nil && st.IsDir() && !a.pathSet {
			a.path, a.pathSet = pushWord, true
		} else {
			a.remote = pushWord
		}
	}
	if !a.pathSet {
		a.path = "."
	}
	switch {
	case len(a.messages) == 0 || strings.TrimSpace(a.messages[0]) == "":
		fail(c, "missing_message", "", "Give the subject with -m SUBJECT (and -m BODY for each paragraph)")
		return a, false
	case !a.all && len(a.adds) == 0:
		fail(c, "missing_target", "", "Name what to commit: --add PATH... or -A for every change (no default)")
		return a, false
	case a.all && len(a.adds) > 0:
		fail(c, "conflicting_options", "--add with -A", "Use --add PATH... or -A, not both")
		return a, false
	case a.author != "" && !authorRE.MatchString(a.author):
		fail(c, "invalid_author", a.author, `Use --author "Name <email>"`)
		return a, false
	case o.Format != llmfmt.Default && o.Format != llmfmt.JSON && o.Format != llmfmt.JSONL:
		fail(c, "invalid_format", string(o.Format), "The report is one JSON object: --format json or jsonl")
		return a, false
	}
	return a, true
}

// gitCommit is agent_git_commit (plan G2): stage explicit paths (or every
// change with -A), commit them with the given message, optionally push the
// branch, and report the final state in one small object. Everything that
// can be checked is checked before the first write: a detached HEAD,
// unresolved conflicts, nothing to commit, a secret file in the commit, a
// push that has no target or would be refused as behind its upstream.
// The commit and the push run the user's git with its configuration and
// hooks (the user's own policies, as git_sync runs it), never prompting;
// the push is never forced.
func (m *Module) gitCommit(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags | llmfmt.TimeoutFlag)
	a, ok := m.parseCommit(c, o)
	if !ok {
		return 1
	}
	env := c.Env.Child()
	if _, err := runner.LookPath("git", env, ""); err != nil {
		return fail(c, "git_not_found", "Git is not installed", "Git binary not found. Skip git operations")
	}
	g := m.guard(c)
	dir, ok := m.target(c, g, a.path, a.force)
	if !ok {
		return 1
	}
	ctx := c.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	res := gitx.Safe(ctx, m.run, env, nil, nil, io.Discard, "-C", dir, "rev-parse", "--show-toplevel")
	top := strings.TrimSpace(string(res.Stdout))
	if !res.Success() || top == "" {
		return fail(c, "not_a_git_repo", dir+" is not a git work tree", "Not inside a git repository. Skip git operations")
	}
	res = gitx.Safe(ctx, m.run, env, nil, nil, io.Discard, "-C", top, "status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all")
	if !res.Success() {
		return fail(c, "git_error", gitx.LastLine(res.Stderr, 300), "")
	}
	st := gitx.ParseStatus(res.Stdout)
	if st.Detached || st.Branch == "" {
		return fail(c, "detached_head", "HEAD is not on a branch", "Check out a branch first (ask the user which one)")
	}
	_, _, _, conflicted := st.Counts()
	if conflicted > 0 {
		return fail(c, "merge_conflict", strconv.Itoa(conflicted)+" unmerged path(s)", "Resolve the conflicts first; nothing was staged or committed")
	}

	// The --add targets, relative to the repository root.
	var rels []string
	for _, p := range a.adds {
		if strings.Contains("/"+p+"/", "/../") {
			// The traversal rule holds for the word as given, before the
			// join with PATH cleans it away.
			if _, err := g.ValidatePath(p); err != nil {
				refusal(c, err)
				return 1
			}
		}
		abs := p
		if !filepath.IsAbs(p) && !strings.HasPrefix(p, "~") {
			abs = filepath.Join(dir, p)
		}
		safe, ok := m.target(c, g, abs, a.force)
		if !ok {
			return 1
		}
		rel, err := filepath.Rel(top, safe)
		if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
			return fail(c, "invalid_target", p, "The path is outside the repository "+top)
		}
		if rel == "." {
			return fail(c, "invalid_target", p, "That is the whole repository: use -A to commit every change")
		}
		rels = append(rels, filepath.ToSlash(rel))
	}
	paths, removed, unmatched := commitPaths(st, a.all, rels)
	for _, p := range paths {
		if !removed[p] && pathguard.SecretName(p) {
			return fail(c, "secret_file_blocked", p, "agent_* functions never commit secret files; nothing was staged or committed")
		}
	}
	for _, r := range unmatched {
		if _, err := os.Lstat(filepath.Join(top, r)); err != nil {
			return fail(c, "file_not_found", r, "Nothing to stage at that path; nothing was staged or committed")
		}
	}
	if len(unmatched) > 0 {
		// A target with no change git knows of may be ignored: let git say
		// so now, before the index changes.
		res = gitx.Safe(ctx, m.run, env, nil, io.Discard, nil,
			append([]string{"-C", top, "--literal-pathspecs", "add", "--dry-run", "-A", "--"}, unmatched...)...)
		if !res.Success() {
			return fail(c, "git_add_failed", tailLines(res.Stderr, 300), "Nothing was staged or committed")
		}
	}
	if len(paths) == 0 {
		return fail(c, "nothing_to_commit", "", "Nothing staged or changed under the given paths; check them with llm_git_status --files")
	}

	// The push target, checked before anything is written.
	remote, ref := "", ""
	if a.push {
		var code, msg string
		remote, ref, code, msg = m.pushTarget(ctx, env, top, st, a.remote)
		if code != "" {
			return fail(c, code, msg, pushHint(code))
		}
	}
	ahead := aheadAfter(st, a.push)

	if a.dry {
		plan := commitPlan{Branch: st.Branch, Files: len(paths), Paths: paths, Push: a.push, Remote: remote, Ref: ref,
			Ahead: ahead, DryRun: true, Written: false}
		budget := *o
		if !budget.Capped() {
			budget.Head = commitDryPaths
		}
		head, tail, omitted := llmfmt.Cap(&budget, paths)
		plan.Paths = []string{}
		for _, p := range append(append([]string{}, head...), tail...) {
			plan.Paths = append(plan.Paths, llmfmt.Cut(p, o.Width))
		}
		if omitted > 0 {
			plan.Truncated = omitted
			c.NoteTruncated(omitted)
		}
		_ = llmfmt.JSONLine(c.Stdout, plan)
		return 0
	}

	timeout := o.TimeoutOr(commitTimeout)
	quiet := append([]string{"GIT_TERMINAL_PROMPT=0"}, env...)
	addArgs := []string{"-C", top, "--literal-pathspecs", "add", "-A"}
	if !a.all {
		addArgs = append(append(addArgs, "--"), rels...)
	}
	res = m.git(ctx, quiet, nil, timeout, addArgs...)
	if !res.Success() {
		return fail(c, "git_add_failed", tailLines(res.Stderr, 300), "Nothing was committed")
	}
	commitArgv := []string{"-C", top, "commit", "-q", "-F", "-"}
	if a.author != "" {
		commitArgv = append(commitArgv, "--author="+a.author)
	}
	res = m.git(ctx, quiet, strings.NewReader(strings.Join(a.messages, "\n\n")+"\n"), timeout, commitArgv...)
	if !res.Success() {
		return fail(c, "commit_failed", tailLines(res.Stderr, 400),
			"Nothing was committed; the --add paths stay staged. Fix what the message says (a hook, the identity) and run again")
	}
	res = gitx.Safe(ctx, m.run, env, nil, nil, io.Discard, "-C", top, "log", "-1", "--no-show-signature", "--format=%h", "--name-only", "-z", "HEAD", "--")
	t := gitx.SplitZ(res.Stdout)
	out := commitResult{Branch: st.Branch, Ahead: ahead}
	if len(t) > 0 {
		out.Hash, out.Files = t[0], len(t)-1
	}
	if a.push {
		res = m.git(ctx, quiet, nil, timeout, "-C", top, "push", "--porcelain", remote, "HEAD:"+ref)
		if !res.Success() {
			e := pushError(res, timeout)
			return c.Fail(e.With("hash", out.Hash).With("branch", out.Branch))
		}
		out.Pushed, out.Remote = true, remote
	}
	_ = llmfmt.JSONLine(c.Stdout, out)
	return 0
}

// git runs the user's git (no hardening: their configuration and hooks
// apply) with no terminal to prompt on, bounded by timeout.
func (m *Module) git(ctx context.Context, env []string, stdin io.Reader, timeout time.Duration, args ...string) runner.Result {
	res, _ := m.run.Run(ctx, runner.Cmd{Path: "git", Args: args, Options: runner.Options{
		Env: env, Stdin: stdin, Timeout: timeout, Setsid: true,
	}})
	return res
}

// commitPaths are the paths the commit would change: what is staged
// already plus, for each target (every path with -A), its changes in the
// work tree and its untracked files. removed are the ones among them the
// commit deletes (no content to check). unmatched are the targets with no
// change git reports.
func commitPaths(st gitx.Status, all bool, targets []string) (paths []string, removed map[string]bool, unmatched []string) {
	removed = map[string]bool{}
	in := func(p, t string) bool { return p != "" && (p == t || strings.HasPrefix(p, t+"/")) }
	matched := map[string]bool{}
	seen := map[string]bool{}
	add := func(p string, deleted bool) {
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
		if deleted {
			removed[p] = true
		}
	}
	for _, e := range st.Entries {
		inTarget := all
		for _, t := range targets {
			if in(e.Path, t) || in(e.Orig, t) {
				inTarget, matched[t] = true, true
			}
		}
		switch {
		case e.Conflicted() || e.Kind == '!':
		case e.Untracked():
			if inTarget {
				add(e.Path, false)
			}
		case inTarget && (e.Staged() || e.Modified()):
			// What lands in the commit is the work tree's version.
			add(e.Path, e.Y == 'D' || (e.Y == '.' && e.X == 'D'))
		case e.Staged():
			add(e.Path, e.X == 'D')
		}
	}
	for _, t := range targets {
		if !matched[t] {
			unmatched = append(unmatched, t)
		}
	}
	return paths, removed, unmatched
}

// pushTarget resolves where --push goes: the branch's upstream (remote and
// branch) without REMOTE, REMOTE's branch of the same name otherwise. A
// branch behind its upstream would be refused, so it is refused here,
// before the commit. code is "" when the push can go ahead.
func (m *Module) pushTarget(ctx context.Context, env []string, top string, st gitx.Status, remote string) (r, ref, code, msg string) {
	res := gitx.Safe(ctx, m.run, env, nil, nil, io.Discard, "-C", top, "config", "--list", "-z")
	cfg := map[string]string{}
	for _, rec := range gitx.SplitZ(res.Stdout) {
		k, v, _ := strings.Cut(rec, "\n")
		cfg[k] = v
	}
	upRemote, upMerge := cfg["branch."+st.Branch+".remote"], cfg["branch."+st.Branch+".merge"]
	if remote == "" {
		if upRemote == "" || upMerge == "" || upRemote == "." {
			return "", "", "no_upstream", "branch " + st.Branch + " has no upstream"
		}
		remote = upRemote
	}
	if _, ok := cfg["remote."+remote+".url"]; !ok {
		return "", "", "unknown_remote", remote
	}
	ref = "refs/heads/" + st.Branch
	if remote == upRemote && strings.HasPrefix(upMerge, "refs/heads/") {
		ref = upMerge
		if st.HasAB && st.Behind > 0 {
			return "", "", "push_rejected", st.Branch + " is " + strconv.Itoa(st.Behind) + " commit(s) behind " + st.Upstream +
				": the push would be refused; nothing was staged or committed"
		}
	}
	return remote, ref, "", ""
}

func pushHint(code string) string {
	switch code {
	case "no_upstream":
		return "Name the remote: --push REMOTE (the branch is pushed under its own name)"
	case "unknown_remote":
		return "Name a configured remote (git remote -v lists them)"
	}
	return "Integrate the upstream first (pull or rebase, ask the user), then commit again"
}

// aheadAfter is how far the branch will be ahead once the call is done:
// 0 after a push, one more than now against an upstream, null without
// anything to compare with.
func aheadAfter(st gitx.Status, push bool) *int {
	n := 0
	switch {
	case push:
	case st.Upstream != "" && st.HasAB:
		n = st.Ahead + 1
	default:
		return nil
	}
	return &n
}

// pushError is the envelope of a failed push: push_rejected (RETRY) when
// the remote refused the update (its "!" porcelain line and the remote's
// own messages), push_failed (STOP) otherwise (network, credentials, a
// timeout). The commit exists either way.
func pushError(res runner.Result, timeout time.Duration) contract.Envelope {
	if res.TimedOut {
		return contract.New("push_failed", "push timed out after "+timeout.String(),
			"The commit is local; check the network or the credentials with the user, then push")
	}
	var reasons []string
	for _, l := range strings.Split(string(res.Stdout), "\n") {
		if strings.HasPrefix(l, "!") {
			f := strings.Split(l, "\t")
			reasons = append(reasons, strings.TrimSpace(f[len(f)-1]))
		}
	}
	if len(reasons) > 0 {
		for _, l := range strings.Split(string(res.Stderr), "\n") {
			if strings.HasPrefix(l, "remote: ") && strings.TrimSpace(strings.TrimPrefix(l, "remote:")) != "" {
				reasons = append(reasons, strings.TrimSpace(l))
			}
		}
		return contract.New("push_rejected", cutBytes(strings.Join(reasons, " | "), 400),
			"The commit is local. Integrate the remote branch first (pull or rebase), then push again; never force")
	}
	return contract.New("push_failed", tailLines(res.Stderr, 400),
		"The commit is local; the push did not reach the remote (network, credentials or a pre-push hook)")
}

// tailLines is the last non-empty lines of git's stderr joined with " | ",
// at most max bytes (a hook's message is usually more than one line).
func tailLines(stderr []byte, max int) string {
	var keep []string
	lines := strings.Split(strings.TrimSpace(string(stderr)), "\n")
	size := 0
	for i := len(lines) - 1; i >= 0 && len(keep) < 6; i-- {
		l := strings.TrimSpace(lines[i])
		if l == "" || strings.HasPrefix(l, "hint:") {
			continue
		}
		size += len(l) + 3
		if size > max && len(keep) > 0 {
			break
		}
		keep = append([]string{l}, keep...)
	}
	return cutBytes(strings.Join(keep, " | "), max)
}

func cutBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && s[cut]&0xc0 == 0x80 {
		cut--
	}
	return s[:cut] + "…"
}
