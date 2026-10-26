// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"io"
	"strings"

	"github.com/mauriziofonte/gash/internal/gitx"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
)

// logValueFlags are the 3.4 llm_git_log options that take a value (G3).
var logValueFlags = map[string]bool{
	"--since": true, "--until": true, "--grep": true, "--author": true, "--path": true, "--fields": true,
}

// logFieldFormat is the git log format of each --fields name.
var logFieldFormat = map[string]string{
	"hash": "%h", "subject": "%s", "author": "%an", "date": "%ci", "files": "",
}

// logDefaultFields are the fields of llm_git_log without --fields: today's
// four (plan D6: the switch to hash and subject is for a later minor).
var logDefaultFields = []string{"hash", "subject", "author", "date"}

// logQuery holds the G3 filters and the field selection.
type logQuery struct {
	since, until, grep, author string
	paths                      []string
	fields                     []string
	given                      bool
}

func (q *logQuery) any() bool { return q.given }

// set records one option; false when it wrote an error envelope.
func (q *logQuery) set(c *invoke.Call, flag, v string) bool {
	q.given = true
	switch flag {
	case "--since":
		q.since = v
	case "--until":
		q.until = v
	case "--grep":
		q.grep = v
	case "--author":
		q.author = v
	case "--path":
		if v == "" {
			fail(c, "missing_value", flag, "--path needs a path")
			return false
		}
		q.paths = append(q.paths, v)
	case "--fields":
		q.fields = nil
		seen := map[string]bool{}
		for _, f := range strings.Split(v, ",") {
			f = strings.TrimSpace(f)
			if _, ok := logFieldFormat[f]; !ok {
				fail(c, "invalid_fields", v, "Use a comma list of hash, subject, date, author, files")
				return false
			}
			if !seen[f] {
				seen[f] = true
				q.fields = append(q.fields, f)
			}
		}
	}
	return true
}

// gitLog34 is llm_git_log with a 3.4 option (G3): the filters --since,
// --until, --grep, --author (extended regexes, git log -E) and --path,
// the field selection --fields, and the rows written by llmfmt (--format,
// the output budget).
func (m *Module) gitLog34(c *invoke.Call, o *llmfmt.Options, q logQuery, limit, path string) int {
	safe, ok := m.validatePath(c, path)
	if !ok {
		return 1
	}
	if !m.gitPresent(c, c.Name) {
		return 1
	}
	fields := q.fields
	if fields == nil {
		fields = logDefaultFields
	}
	// Every commit starts with its hash, even when not asked for, so the
	// header is never empty (parseLogZ).
	format := "--format=%x00%h"
	withFiles := false
	nf := 1
	for _, f := range fields {
		if f == "files" {
			withFiles = true
			continue
		}
		format += "%x00" + logFieldFormat[f]
		nf++
	}
	args := []string{"-C", safe, "log", "-n", limit, "--no-ext-diff", "--no-textconv", "--no-show-signature", "-z", format}
	if withFiles {
		args = append(args, "--name-only")
	}
	for _, kv := range [][2]string{{"--since=", q.since}, {"--until=", q.until}, {"--grep=", q.grep}, {"--author=", q.author}} {
		if kv[1] != "" {
			args = append(args, kv[0]+kv[1])
		}
	}
	if q.grep != "" || q.author != "" {
		args = append(args, "-E")
	}
	args = append(args, "--")
	args = append(args, q.paths...)
	env := c.Env.Child()
	res := gitx.Safe(c.Ctx, m.run, env, nil, nil, nil, args...)
	var commits []logCommit
	if res.Success() {
		commits = parseLogZ(res.Stdout, nf)
		if rg := m.repoGuard(c, safe); withFiles && rg != nil {
			for i := range commits {
				commits[i].files = rg.keep(commits[i].files)
			}
		}
	} else {
		if !gitx.Safe(c.Ctx, m.run, env, nil, io.Discard, io.Discard, "-C", safe, "rev-parse", "--git-dir").Success() {
			return notARepo(c, safe)
		}
		hasHead := gitx.Safe(c.Ctx, m.run, env, nil, io.Discard, io.Discard, "-C", safe, "rev-parse", "--verify", "--quiet", "HEAD").Success()
		switch {
		case !hasHead:
			// No commit yet: an empty log, as llm_git_log prints.
		case q.grep != "" || q.author != "":
			return fail(c, "invalid_regex", gitx.LastLine(res.Stderr, 300), "--grep and --author take an extended regex (git log -E)")
		default:
			return fail(c, "git_error", gitx.LastLine(res.Stderr, 300), "")
		}
	}
	rows := make([][]any, len(commits))
	for i, cm := range commits {
		row := make([]any, len(fields))
		k := 1
		for j, f := range fields {
			if f == "files" {
				row[j] = cm.files
				continue
			}
			if k < len(cm.header) {
				row[j] = cm.header[k]
			}
			k++
		}
		rows[i] = row
	}
	o.Write(c, o.Format, llmfmt.Table{Cols: fields, Rows: rows}, nil)
	return 0
}

// logCommit is one commit of parseLogZ: its header fields and its files.
type logCommit struct {
	header []string
	files  []string
}

// parseLogZ reads `git log -z --format=%x00F1%x00F2...` with nf >= 1 fields,
// optionally with --name-only. The leading %x00 gives every commit an empty
// first field, which no file name can be: the fields that follow the nf
// header fields, up to the next empty one, are the commit's files (git puts
// one newline before the first).
func parseLogZ(out []byte, nf int) []logCommit {
	t := gitx.SplitZ(out)
	var commits []logCommit
	for i := 0; i < len(t); {
		if t[i] != "" {
			i++
			continue
		}
		i++
		if i+nf > len(t) {
			break
		}
		cm := logCommit{header: t[i : i+nf], files: []string{}}
		i += nf
		for first := true; i < len(t) && t[i] != ""; i++ {
			f := t[i]
			if first {
				f = strings.TrimPrefix(f, "\n")
				first = false
			}
			cm.files = append(cm.files, f)
		}
		commits = append(commits, cm)
	}
	return commits
}
