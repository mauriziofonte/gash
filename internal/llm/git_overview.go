// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mauriziofonte/gash/internal/gitx"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
)

// defaultFetchTimeout bounds llm_git_status --fetch when --timeout is not
// given (G7).
const defaultFetchTimeout = 20 * time.Second

// ovCounts, ovLog, ovDiffstat and ovFiles are the parts of the
// llm_git_overview object, in their documented key order.
type ovCounts struct {
	Staged     int `json:"staged"`
	Modified   int `json:"modified"`
	Untracked  int `json:"untracked"`
	Conflicted int `json:"conflicted"`
}

type ovLog struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
}

type ovDiffstat struct {
	Files      int `json:"files"`
	Insertions int `json:"insertions"`
	Deletions  int `json:"deletions"`
}

type ovFiles struct {
	Staged     []string `json:"staged"`
	Modified   []string `json:"modified"`
	Untracked  []string `json:"untracked"`
	Conflicted []string `json:"conflicted"`
}

// overviewJSON is the llm_git_overview object (G1), also printed by
// llm_git_status --fetch (G7) with fetched and error.
type overviewJSON struct {
	Branch    string     `json:"branch"`
	Upstream  string     `json:"upstream"`
	Ahead     int        `json:"ahead"`
	Behind    int        `json:"behind"`
	Counts    ovCounts   `json:"counts"`
	Log       []ovLog    `json:"log"`
	Diffstat  ovDiffstat `json:"diffstat"`
	Files     *ovFiles   `json:"files,omitempty"`
	Truncated int        `json:"truncated,omitempty"`
	Fetched   *bool      `json:"fetched,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// summaryJSON is the llm_git_status --summary object (S4): the state and
// the number of paths in each.
type summaryJSON struct {
	Branch     string `json:"branch"`
	Upstream   string `json:"upstream"`
	Ahead      int    `json:"ahead"`
	Behind     int    `json:"behind"`
	Staged     int    `json:"staged"`
	Modified   int    `json:"modified"`
	Untracked  int    `json:"untracked"`
	Conflicted int    `json:"conflicted"`
	Fetched    *bool  `json:"fetched,omitempty"`
	Error      string `json:"error,omitempty"`
}

var summaryCols = []string{"branch", "upstream", "ahead", "behind", "staged", "modified", "untracked", "conflicted"}

func summaryOf(st gitx.Status) summaryJSON {
	s, mo, u, co := st.Counts()
	return summaryJSON{Branch: st.Branch, Upstream: st.Upstream, Ahead: st.Ahead, Behind: st.Behind,
		Staged: s, Modified: mo, Untracked: u, Conflicted: co}
}

func (s summaryJSON) row() []any {
	return []any{s.Branch, s.Upstream, s.Ahead, s.Behind, s.Staged, s.Modified, s.Untracked, s.Conflicted}
}

// readStatus runs the hardened `git status --porcelain=v2 --branch -z` in
// dir. ok is false when git fails: dir is not inside a work tree (or git
// refuses it, as for a repository of another owner).
func (m *Module) readStatus(ctx context.Context, env []string, dir string) (gitx.Status, bool) {
	res := gitx.Safe(ctx, m.run, env, nil, nil, io.Discard, "-C", dir, "status", "--porcelain=v2", "--branch", "-z")
	if !res.Success() {
		return gitx.Status{}, false
	}
	return gitx.ParseStatus(res.Stdout), true
}

// gitPresent is the git_not_found check of requireGitRepo, without the
// repository test (the 3.4 modes learn that from their first git call):
// git found as gash_doctor finds it, so that the macOS /usr/bin/git stub
// without the Command Line Tools counts as missing (3.4; before, the stub
// ran and its failure read as not_a_git_repo or a git error), reported
// with its install command for need.
func (m *Module) gitPresent(c *invoke.Call, need string) bool {
	if _, ok := m.locate(c, "git"); !ok {
		prereq.Fail(c, "git", need, true)
		return false
	}
	return true
}

func notARepo(c *invoke.Call, safe string) int {
	return fail(c, "not_a_git_repo", safe+" is not a git repository", "Not inside a git repository. Skip git operations")
}

// overviewData is what llm_git_overview collects: the status, the last
// subjects and the line counts of the tracked changes against HEAD.
type overviewData struct {
	st   gitx.Status
	log  []ovLog
	diff []gitx.FileChange
}

// collectOverview runs the three reads of the overview at once (status,
// log, diff --numstat). ok is false when dir is not a repository.
func (m *Module) collectOverview(ctx context.Context, env []string, dir string, logN int) (overviewData, bool) {
	var d overviewData
	var ok bool
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if logN == 0 {
			return
		}
		res := gitx.Safe(ctx, m.run, env, nil, nil, io.Discard, "-C", dir, "log", "-n", strconv.Itoa(logN),
			"--no-show-signature", "-z", "--format=%h%x00%s")
		f := gitx.SplitZ(res.Stdout)
		for i := 0; i+1 < len(f); i += 2 {
			d.log = append(d.log, ovLog{Hash: f[i], Subject: f[i+1]})
		}
	}()
	go func() {
		defer wg.Done()
		d.diff = m.numstat(ctx, env, dir, "HEAD")
	}()
	d.st, ok = m.readStatus(ctx, env, dir)
	wg.Wait()
	if d.log == nil {
		d.log = []ovLog{}
	}
	if ok && d.st.Oid == "" {
		// An unborn branch has no HEAD: its tracked changes are against
		// the empty tree.
		d.diff = m.numstat(ctx, env, dir, gitx.EmptyTree)
	}
	return d, ok
}

// numstat is `git diff --numstat -z REV` in dir: the tracked changes
// (staged and not) against REV.
func (m *Module) numstat(ctx context.Context, env []string, dir, rev string) []gitx.FileChange {
	res := gitx.Safe(ctx, m.run, env, nil, nil, io.Discard, "-C", dir, "diff", "--numstat", "-z",
		"--no-color", "--no-ext-diff", "--no-textconv", rev, "--")
	return gitx.ParseRawNumstat(gitx.SplitZ(res.Stdout))
}

func (d overviewData) object(filesN int) overviewJSON {
	s, mo, u, co := d.st.Counts()
	n, ins, del := gitx.Totals(d.diff)
	out := overviewJSON{
		Branch: d.st.Branch, Upstream: d.st.Upstream, Ahead: d.st.Ahead, Behind: d.st.Behind,
		Counts:   ovCounts{Staged: s, Modified: mo, Untracked: u, Conflicted: co},
		Log:      d.log,
		Diffstat: ovDiffstat{Files: n, Insertions: ins, Deletions: del},
	}
	if filesN > 0 {
		ls, lm, lu, lc := d.st.Lists()
		capList := func(l []string) []string {
			if len(l) > filesN {
				out.Truncated += len(l) - filesN
				return l[:filesN]
			}
			return l
		}
		out.Files = &ovFiles{Staged: capList(ls), Modified: capList(lm), Untracked: capList(lu), Conflicted: capList(lc)}
	}
	return out
}

// gitOverview is llm_git_overview (G1): the state of a repository in one
// small object, the answer to `git status --short | wc -l; git log
// --oneline -3; git diff --stat | tail -1`.
func (m *Module) gitOverview(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	logN, filesN := "3", "0"
	pt := pathTaker{path: "."}
	args := c.Args
	for len(args) > 0 {
		a := args[0]
		switch {
		case a == "--log" || a == "--files":
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a value")
			}
			if a == "--log" {
				logN = args[1]
			} else {
				filesN = args[1]
			}
			args = args[2:]
			continue
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, "Use --log N, --files N or a path")
		default:
			if !pt.take(c, a) {
				return 1
			}
		}
		args = args[1:]
	}
	if !validUint(c, "log", logN) || !validUint(c, "files", filesN) {
		return 1
	}
	safe, ok := m.validatePath(c, pt.path)
	if !ok {
		return 1
	}
	if !m.gitPresent(c, c.Name) {
		return 1
	}
	d, ok := m.collectOverview(c.Ctx, c.Env.Child(), safe, atoi(logN))
	if !ok {
		return notARepo(c, safe)
	}
	d.st = m.repoGuard(c, safe).keepStatus(d.st)
	obj := d.object(atoi(filesN))
	if obj.Truncated > 0 {
		c.NoteTruncated(obj.Truncated)
	}
	_ = llmfmt.JSONLine(c.Stdout, obj)
	return 0
}

// statusMode is what the options of llm_git_status asked for.
type statusMode struct {
	summary, files, fetch, repos bool
	repoList                     []string
}

func (s statusMode) any() bool { return s.summary || s.files || s.fetch || s.repos }

const statusHint = "Use --summary, --files, --repos PATH..., --fetch, --format, -j, --timeout or a path"

// gitStatus is llm_git_status: the 2.x pretty object by default (gitStatus2x),
// and the 3.4 modes --summary (S4), --files (S1 rows), --repos (G6) and
// --fetch (G7).
func (m *Module) gitStatus(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags | llmfmt.JobsFlag | llmfmt.TimeoutFlag)
	var mode statusMode
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
		case a == "--summary":
			mode.summary = true
		case a == "--files":
			mode.files = true
		case a == "--fetch":
			mode.fetch = true
		case a == "--repos":
			mode.repos = true
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, statusHint)
		case mode.repos:
			mode.repoList = append(mode.repoList, a)
		default:
			if pt.set {
				return fail(c, "unexpected_argument", a, "Use a path or no argument")
			}
			pt.path, pt.set = a, true
		}
		args = args[1:]
	}
	shared := o.Format != llmfmt.Default || o.Width > 0 || o.Capped() || o.Jobs > 0 || o.Timeout > 0
	if !mode.any() {
		if shared {
			return fail(c, "conflicting_options", "--format, the output budget, -j and --timeout",
				"They apply to --files, --summary, --repos and --fetch")
		}
		return m.gitStatus2x(c, pt.path)
	}
	if msg := mode.conflict(o, pt.set); msg != "" {
		return fail(c, "conflicting_options", msg, statusHint)
	}
	if mode.repos && len(mode.repoList) == 0 {
		return fail(c, "missing_value", "--repos", "--repos needs one or more paths")
	}
	if !m.gitPresent(c, c.Name) {
		return 1
	}
	if mode.repos {
		return m.statusRepos(c, o, mode)
	}
	safe, ok := m.validatePath(c, pt.path)
	if !ok {
		return 1
	}
	env := c.Env.Child()
	var fetched *bool
	fetchErr := ""
	if mode.fetch {
		fetchErr = m.fetchUpstream(c.Ctx, env, safe, o.TimeoutOr(defaultFetchTimeout))
		f := fetchErr == ""
		fetched = &f
	}
	switch {
	case mode.files:
		st, ok := m.readStatus(c.Ctx, env, safe)
		if !ok {
			return notARepo(c, safe)
		}
		st = m.repoGuard(c, safe).keepStatus(st)
		rows := make([][]any, len(st.Entries))
		for i, e := range st.Entries {
			rows[i] = []any{e.Path, e.XY()}
		}
		o.Write(c, o.Format, llmfmt.Table{Cols: []string{"p", "xy"}, Rows: rows}, nil)
	case mode.summary:
		st, ok := m.readStatus(c.Ctx, env, safe)
		if !ok {
			return notARepo(c, safe)
		}
		s := summaryOf(st)
		s.Fetched, s.Error = fetched, fetchErr
		if o.Format == llmfmt.TSV || o.Format == llmfmt.TableF {
			cols, row := summaryCols, s.row()
			if fetched != nil {
				cols = append(append([]string(nil), cols...), "fetched", "error")
				row = append(row, *fetched, nullable(fetchErr))
			}
			o.Write(c, o.Format, llmfmt.Table{Cols: cols, Rows: [][]any{row}}, nil)
			return 0
		}
		_ = llmfmt.JSONLine(c.Stdout, s)
	default: // --fetch: the overview, fetched and the fetch error
		d, ok := m.collectOverview(c.Ctx, env, safe, 3)
		if !ok {
			return notARepo(c, safe)
		}
		obj := d.object(0)
		obj.Fetched, obj.Error = fetched, fetchErr
		_ = llmfmt.JSONLine(c.Stdout, obj)
	}
	return 0
}

// conflict names the options of a call that do not go together, "" when
// they do.
func (s statusMode) conflict(o *llmfmt.Options, pathSet bool) string {
	switch {
	case s.files && (s.summary || s.repos || s.fetch):
		return "--files with --summary, --repos or --fetch"
	case s.repos && pathSet:
		return "PATH with --repos (list every repository after --repos)"
	case (o.Width > 0 || o.Capped()) && !s.files && !s.repos:
		return "the output budget applies to --files and --repos"
	case o.Jobs > 0 && !s.repos:
		return "-j applies to --repos"
	case o.Timeout > 0 && !s.repos && !s.fetch:
		return "--timeout applies to --repos and --fetch"
	case (o.Format == llmfmt.TSV || o.Format == llmfmt.TableF) && s.fetch && !s.summary && !s.repos:
		return "--format tsv|table with --fetch needs --summary or --repos (the overview is not one row)"
	}
	return ""
}

// repoRow is one row of llm_git_status --repos.
type repoRow struct {
	repo    string
	safe    string
	refusal string
}

// statusRepos is llm_git_status --repos (G6): the summary of each
// repository, -j at a time, in input order; a path that is refused or is
// not a repository is a row with "error".
func (m *Module) statusRepos(c *invoke.Call, o *llmfmt.Options, mode statusMode) int {
	g := m.guard(c)
	items := make([]repoRow, len(mode.repoList))
	for i, p := range mode.repoList {
		items[i].repo = p
		safe, err := g.ValidatePath(p)
		if err != nil {
			items[i].refusal = refusalCode(err)
			continue
		}
		if _, err := os.Stat(safe); err != nil {
			items[i].refusal = "path_not_found"
			continue
		}
		items[i].safe = safe
	}
	env := c.Env.Child()
	jobs := o.JobsOr(llmfmt.LocalJobs())
	if mode.fetch {
		jobs = o.JobsOr(llmfmt.NetworkJobs)
	}
	fetchTimeout := o.TimeoutOr(defaultFetchTimeout)
	rows := llmfmt.Map(c.Ctx, jobs, o.Timeout, items, func(ctx context.Context, it repoRow) []any {
		row := []any{it.repo, nil, nil, nil, nil, nil, nil, nil, nil}
		var fetched any
		errCode := it.refusal
		if errCode == "" {
			fetchErr := ""
			if mode.fetch {
				fetchErr = m.fetchUpstream(ctx, env, it.safe, fetchTimeout)
				fetched = fetchErr == ""
			}
			st, ok := m.readStatus(ctx, env, it.safe)
			switch {
			case ctx.Err() != nil:
				errCode = "timeout"
			case !ok:
				errCode = "not_a_git_repo"
			default:
				row = append([]any{it.repo}, summaryOf(st).row()...)
				if fetchErr != "" {
					errCode = fetchErr
				}
			}
		}
		if mode.fetch {
			row = append(row, fetched)
		}
		return append(row, nullable(errCode))
	})
	cols := append([]string{"repo"}, summaryCols...)
	if mode.fetch {
		cols = append(cols, "fetched")
	}
	cols = append(cols, "error")
	o.Write(c, o.Format, llmfmt.Table{Cols: cols, Rows: rows}, nil)
	return 0
}

// refusalCode is the error code of a path guard refusal.
func refusalCode(err error) string {
	var r *pathguard.Refusal
	if errors.As(err, &r) {
		return r.Code
	}
	return "invalid_path"
}

// fetchUpstream fetches the upstream branch of the repository at dir and
// nothing else (G7): no tags, no pruning, no submodules, no maintenance
// afterwards; bounded by timeout, with no terminal to prompt on
// (GIT_TERMINAL_PROMPT=0, a new session). It returns "" when the fetch
// succeeded, else why it did not happen or failed.
//
// The rails of the llm_git_* readers hold: a fetch can run programs that
// the repository's own configuration names (core.sshCommand, an
// upload-pack, a credential helper, a URL rewrite to another transport),
// so a repository whose local configuration sets one of those is not
// fetched.
func (m *Module) fetchUpstream(ctx context.Context, env []string, dir string, timeout time.Duration) string {
	res := gitx.Safe(ctx, m.run, env, nil, nil, io.Discard, "-C", dir, "symbolic-ref", "-q", "--short", "HEAD")
	branch := strings.TrimSpace(string(res.Stdout))
	if !res.Success() || branch == "" {
		return "no branch checked out (detached HEAD or not a repository): nothing to fetch"
	}
	res = gitx.Safe(ctx, m.run, env, nil, nil, io.Discard, "-C", dir, "config", "--local", "--includes", "--list", "-z")
	if !res.Success() {
		return "cannot read the repository configuration"
	}
	cfg := parseConfigZ(res.Stdout)
	if k := blockedFetchKey(cfg); k != "" {
		return "not fetched: the repository configuration sets " + k + ", which can run a program"
	}
	remote, merge := cfg["branch."+branch+".remote"], cfg["branch."+branch+".merge"]
	switch {
	case remote == "" || merge == "":
		return "no upstream branch to fetch"
	case remote == ".":
		return "the upstream is a local branch: nothing to fetch"
	}
	o := runner.Options{Env: append([]string{"GIT_TERMINAL_PROMPT=0"}, env...), Stdout: io.Discard,
		Timeout: timeout, Setsid: true}
	res = gitx.SafeWith(ctx, m.run, o, "-C", dir,
		"-c", "fetch.prune=false", "-c", "fetch.pruneTags=false", "-c", "fetch.recurseSubmodules=false",
		"-c", "gc.auto=0", "-c", "maintenance.auto=false",
		"fetch", "--quiet", "--no-tags", "--no-recurse-submodules", remote, merge)
	switch {
	case res.TimedOut || ctx.Err() != nil:
		return "fetch timed out after " + timeout.String()
	case !res.Success():
		if msg := gitx.LastLine(res.Stderr, 200); msg != "" {
			return msg
		}
		return "fetch failed (exit " + strconv.Itoa(res.ExitCode) + ")"
	}
	return ""
}

// parseConfigZ reads `git config --list -z`: "key\nvalue" records (a key
// without a value has no newline); of repeated keys the last wins, as git
// reads a single-valued key.
func parseConfigZ(out []byte) map[string]string {
	cfg := map[string]string{}
	for _, rec := range gitx.SplitZ(out) {
		k, v, _ := strings.Cut(rec, "\n")
		cfg[k] = v
	}
	return cfg
}

// blockedFetchKey returns the first (sorted) local configuration key that
// lets a fetch run a program or reach another transport, "" when none.
// Keys come lowercased from git, except the subsection.
func blockedFetchKey(cfg map[string]string) string {
	best := ""
	for k := range cfg {
		lk := strings.ToLower(k)
		section, _, _ := strings.Cut(lk, ".")
		last := lk[strings.LastIndexByte(lk, '.')+1:]
		blocked := lk == "core.sshcommand" || lk == "core.gitproxy" || lk == "core.askpass" ||
			section == "credential" || section == "protocol" ||
			(section == "remote" && (last == "uploadpack" || last == "vcs")) ||
			(section == "url" && last == "insteadof")
		if blocked && (best == "" || k < best) {
			best = k
		}
	}
	return best
}
