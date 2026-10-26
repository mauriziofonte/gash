// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"io"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/gitx"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/registry"
)

// parsePathArg is __llm_path_arg: at most one PATH and no options. "." when
// absent, an option is unknown_option, a second path unexpected_argument.
func parsePathArg(c *invoke.Call, hint string) (string, bool) {
	path, seen := ".", false
	for _, a := range c.Args {
		if strings.HasPrefix(a, "-") {
			fail(c, "unknown_option", a, hint)
			return "", false
		}
		if seen {
			fail(c, "unexpected_argument", a, hint)
			return "", false
		}
		path, seen = a, true
	}
	return path, true
}

// requireGitRepo is __llm_git_repo: git must be installed (gitPresent,
// for need) and safe must be inside a work tree or repository
// (not_a_git_repo, CONTINUE). The membership test is the hardened
// `git -C safe rev-parse --git-dir` with its output discarded.
func (m *Module) requireGitRepo(c *invoke.Call, need, safe string) bool {
	if !m.gitPresent(c, need) {
		return false
	}
	res := gitx.Safe(c.Ctx, m.run, c.Env.Child(), nil, io.Discard, io.Discard, "-C", safe, "rev-parse", "--git-dir")
	if !res.Success() {
		fail(c, "not_a_git_repo", safe+" is not a git repository", "Not inside a git repository. Skip git operations")
		return false
	}
	return true
}

// gitStatus2x is llm_git_status without a 3.4 option (git.sh
// __llm_git_status_impl). The JSON is built natively, never through jq:
// the 2.x no-jq degraded path is the forced divergence LEDGER LLM-04,
// which cannot arise here. The pretty 2-space layout is kept byte for
// byte.
func (m *Module) gitStatus2x(c *invoke.Call, path string) int {
	safe, ok := m.validatePath(c, path)
	if !ok {
		return 1
	}
	if !m.requireGitRepo(c, c.Name, safe) {
		return 1
	}
	res := gitx.Safe(c.Ctx, m.run, c.Env.Child(), nil, nil, io.Discard,
		"-C", safe, "status", "--porcelain=v2", "--branch", "-z")
	st := parseGitStatus(res.Stdout)
	if rg := m.repoGuard(c, safe); rg != nil {
		st.staged, st.modified, st.untracked, st.conflicted = rg.keep(st.staged), rg.keep(st.modified), rg.keep(st.untracked), rg.keep(st.conflicted)
	}

	var b bytes.Buffer
	b.WriteString("{\n  \"branch\": ")
	b.Write(contract.AppendString(nil, st.branch, contract.EscapeJQ))
	b.WriteString(",\n  \"ahead\": ")
	b.WriteString(st.ahead)
	b.WriteString(",\n  \"behind\": ")
	b.WriteString(st.behind)
	b.WriteString(",\n  \"staged\": ")
	b.WriteString(jsonStrArray(st.staged))
	b.WriteString(",\n  \"modified\": ")
	b.WriteString(jsonStrArray(st.modified))
	b.WriteString(",\n  \"untracked\": ")
	b.WriteString(jsonStrArray(st.untracked))
	b.WriteString(",\n  \"conflicted\": ")
	b.WriteString(jsonStrArray(st.conflicted))
	b.WriteString("\n}\n")
	_, _ = c.Stdout.Write(b.Bytes())
	return 0
}

// jsonStrArray is $(__gash_json_arr ...) as it reaches the printf: the jq
// array of strings without the trailing newline, "[]" when empty.
func jsonStrArray(vals []string) string {
	return strings.TrimSuffix(contract.JQArray(vals...), "\n")
}

// gitStatusData holds the parsed porcelain v2 output.
type gitStatusData struct {
	branch, ahead, behind                   string
	staged, modified, untracked, conflicted []string
}

// parseGitStatus reads `git status --porcelain=v2 --branch -z` the way
// __llm_git_status_impl does: records split on NUL, X (index) != "." is
// staged, Y (worktree) != "." is modified, unmerged ("u ") are conflicted,
// "? " are untracked; a rename/copy ("2 ") consumes the next NUL field (the
// original path).
func parseGitStatus(out []byte) gitStatusData {
	st := gitStatusData{ahead: "0", behind: "0"}
	recs := strings.Split(string(out), "\x00")
	for i := 0; i < len(recs); i++ {
		rec := recs[i]
		switch {
		case strings.HasPrefix(rec, "# branch.head "):
			st.branch = strings.TrimPrefix(rec, "# branch.head ")
			if st.branch == "(detached)" {
				st.branch = ""
			}
		case strings.HasPrefix(rec, "# branch.ab "):
			rest := strings.TrimPrefix(rec, "# branch.ab +")
			if j := strings.IndexByte(rest, ' '); j >= 0 {
				st.ahead = rest[:j]
			} else {
				st.ahead = rest
			}
			if j := strings.LastIndexByte(rest, '-'); j >= 0 {
				st.behind = rest[j+1:]
			} else {
				st.behind = rest
			}
		case strings.HasPrefix(rec, "1 "), strings.HasPrefix(rec, "2 "):
			var xy string
			if len(rec) >= 4 {
				xy = rec[2:4]
			}
			skip := 8
			if strings.HasPrefix(rec, "2 ") {
				skip = 9
			}
			p := stripFields(rec, skip)
			if len(xy) >= 1 && xy[0] != '.' {
				st.staged = append(st.staged, p)
			}
			if len(xy) >= 2 && xy[1] != '.' {
				st.modified = append(st.modified, p)
			}
			if strings.HasPrefix(rec, "2 ") {
				i++ // the original path follows as the next NUL field
			}
		case strings.HasPrefix(rec, "u "):
			st.conflicted = append(st.conflicted, stripFields(rec, 10))
		case strings.HasPrefix(rec, "? "):
			st.untracked = append(st.untracked, strings.TrimPrefix(rec, "? "))
		}
	}
	if !digitsRE.MatchString(st.ahead) {
		st.ahead = "0"
	}
	if !digitsRE.MatchString(st.behind) {
		st.behind = "0"
	}
	return st
}

// stripFields is n repetitions of the bash ${rest#* }: each drops the
// shortest prefix up to and including the first space; a record with no
// space is left unchanged, as the parameter expansion leaves it.
func stripFields(s string, n int) string {
	for k := 0; k < n; k++ {
		if j := strings.IndexByte(s, ' '); j >= 0 {
			s = s[j+1:]
		}
	}
	return s
}

// gitDiff2x is llm_git_diff without a 3.4 option (git.sh
// __llm_git_diff_impl): `git diff --stat` text, optionally staged, passed
// through verbatim.
func (m *Module) gitDiff2x(c *invoke.Call, path string, staged bool) int {
	safe, ok := m.validatePath(c, path)
	if !ok {
		return 1
	}
	if !m.requireGitRepo(c, c.Name, safe) {
		return 1
	}
	diffArgs := []string{"-C", safe, "diff", "--stat", "--no-color", "--no-ext-diff", "--no-textconv"}
	if staged {
		diffArgs = append(diffArgs, "--staged")
	}
	if ex := m.repoGuard(c, safe).excludes(); len(ex) > 0 {
		diffArgs = append(append(diffArgs, "--"), ex...)
	}
	res := gitx.Safe(c.Ctx, m.run, c.Env.Child(), nil, c.Stdout, io.Discard, diffArgs...)
	return res.ExitCode
}

// gitLog is llm_git_log (git.sh __llm_git_log_impl): the recent log as a
// JSON array of {hash, subject, author, date}. The 0x1f/0x1e field framing
// of the 2.x `jq -cRs` pipeline is reproduced, including its defect of
// dropping a commit whose subject or author holds one of those bytes
// (LEDGER LLM-13).
func (m *Module) gitLog(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	limit := "10"
	pt := pathTaker{path: "."}
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags)
	var q logQuery
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
		case a == "-n" || a == "--limit" || logValueFlags[a]:
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a value")
			}
			if a == "-n" || a == "--limit" {
				limit = args[1]
			} else if !q.set(c, a, args[1]) {
				return 1
			}
			args = args[2:]
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, registry.OptionsHint(c.Name))
		default:
			if !pt.take(c, a) {
				return 1
			}
			args = args[1:]
		}
	}
	if !validUint(c, "limit", limit) {
		return 1
	}
	if q.any() || o.Format != llmfmt.Default || o.Width > 0 || o.Capped() {
		return m.gitLog34(c, o, q, limit, pt.path)
	}
	safe, ok := m.validatePath(c, pt.path)
	if !ok {
		return 1
	}
	if !m.requireGitRepo(c, c.Name, safe) {
		return 1
	}
	// The JSON is built natively: no jq is needed (2.x and 3.x before
	// failed with jq_not_found without one).
	res := gitx.Safe(c.Ctx, m.run, c.Env.Child(), nil, nil, io.Discard,
		"-C", safe, "log", "-n", limit, "--no-ext-diff", "--no-textconv", "--no-show-signature",
		"-z", "--format=%h%x00%s%x00%an%x00%ci")
	_, _ = c.Stdout.Write(gitLogJSON(res.Stdout))
	return 0
}

// gitLogJSON renders the NUL-separated output of git log -z with four
// NUL-separated fields per commit (hash, subject, author, date) as a
// compact jq-style array. NUL cannot occur in a commit header, so a
// subject or author holding 0x1e or 0x1f keeps its commit (LLM-13; 2.x
// split on those bytes and dropped the commit).
func gitLogJSON(out []byte) []byte {
	var b bytes.Buffer
	b.WriteByte('[')
	s := strings.TrimSuffix(string(out), "\x00")
	var fields []string
	if s != "" {
		fields = strings.Split(s, "\x00")
	}
	for i := 0; i+4 <= len(fields); i += 4 {
		f := fields[i : i+4]
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"hash":`)
		b.Write(contract.AppendString(nil, f[0], contract.EscapeJQ))
		b.WriteString(`,"subject":`)
		b.Write(contract.AppendString(nil, f[1], contract.EscapeJQ))
		b.WriteString(`,"author":`)
		b.Write(contract.AppendString(nil, f[2], contract.EscapeJQ))
		b.WriteString(`,"date":`)
		b.Write(contract.AppendString(nil, f[3], contract.EscapeJQ))
		b.WriteByte('}')
	}
	b.WriteString("]\n")
	return b.Bytes()
}
