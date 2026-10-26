// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
)

// maxReplaceBytes caps the size of a file agent_replace rewrites in memory.
const maxReplaceBytes = 64 << 20

// replacePair is one --old/--new pair, in command-line order.
type replacePair struct {
	old, new string
	re       *regexp.Regexp
}

// replaceFile is one target with what it held and what it will hold.
type replaceFile struct {
	arg    string // as the caller gave it (the output key)
	path   string // resolved
	info   os.FileInfo
	before []byte
	after  []byte
	counts []int
}

// replace is agent_replace (plan E1): literal (or --regex) replacements in
// explicit files, all or nothing. Every pair's count is checked in every
// file, in memory, before anything is written; one mismatch writes
// nothing. The writes go to a temporary file next to each target, renamed
// over it with its mode (and, as root, its owner), after a check that no
// target changed since it was read.
func (m *Module) replace(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags)
	var args = c.Args
	var files []string
	var pairs []replacePair
	exact, atLeast := 1, 0
	countFlag := ""
	regex, dry, force := false, false, false
	open := -1 // the pair whose --new is still missing
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
		switch a {
		case "--old", "--new", "--count", "--min":
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a value")
			}
			v := args[1]
			switch a {
			case "--old":
				if open >= 0 {
					return fail(c, "invalid_replacement", "pair "+strconv.Itoa(open+1)+": --old without --new", "Give each --old its --new: --old A --new B")
				}
				if v == "" {
					return fail(c, "invalid_replacement", "pair "+strconv.Itoa(len(pairs)+1)+": empty --old", "--old must not be empty")
				}
				pairs = append(pairs, replacePair{old: v})
				open = len(pairs) - 1
			case "--new":
				if open < 0 {
					return fail(c, "invalid_replacement", "--new without --old", "Give each --new after its --old: --old A --new B")
				}
				pairs[open].new = v
				open = -1
			default:
				k, ok := count(v)
				if !ok {
					return fail(c, "invalid_count", v, "Give a positive integer (1 to 999999999)")
				}
				if countFlag != "" && countFlag != a {
					return fail(c, "conflicting_options", countFlag+" "+a, "Use either --count N (exactly N) or --min N (at least N)")
				}
				countFlag = a
				if a == "--count" {
					exact, atLeast = k, 0
				} else {
					exact, atLeast = 0, k
				}
			}
			args = args[2:]
			continue
		case "--regex":
			regex = true
		case "--dry-run":
			dry = true
		case "--force":
			force = true
		default:
			if strings.HasPrefix(a, "-") {
				return fail(c, "unknown_option", a, "Use --old, --new, --count, --min, --regex, --dry-run, --force, --format or --width")
			}
			files = append(files, a)
		}
		args = args[1:]
	}
	if open >= 0 {
		return fail(c, "invalid_replacement", "pair "+strconv.Itoa(open+1)+": --old without --new", "Give each --old its --new: --old A --new B")
	}
	if len(pairs) == 0 {
		return fail(c, "invalid_replacement", "no --old/--new pair", "Give at least one pair: --old A --new B")
	}
	if len(files) == 0 {
		return fail(c, "missing_file", "", "Name the files to change explicitly (no implicit globs)")
	}
	if regex {
		for i := range pairs {
			re, err := regexp.Compile(pairs[i].old)
			if err != nil {
				return fail(c, "invalid_regex", pairs[i].old, "--regex takes Go (RE2) syntax; $1 in --new is a group")
			}
			pairs[i].re = re
		}
	}

	targets, ok := m.loadTargets(c, files, force)
	if !ok {
		return 1
	}
	var mismatches []string
	for _, t := range targets {
		t.after, t.counts = applyPairs(t.before, pairs)
		for i, n := range t.counts {
			if (exact > 0 && n != exact) || (atLeast > 0 && n < atLeast) {
				want := "exactly " + strconv.Itoa(exact)
				if atLeast > 0 {
					want = "at least " + strconv.Itoa(atLeast)
				}
				mismatches = append(mismatches, t.arg+": pair "+strconv.Itoa(i+1)+" found "+strconv.Itoa(n)+", expected "+want)
			}
		}
	}
	if len(mismatches) > 0 {
		return fail(c, "replace_count_mismatch", strings.Join(mismatches, "; "),
			"Nothing was written. Fix --old, or use --count N / --min N")
	}
	if !dry {
		if !m.writeAll(c, targets) {
			return 1
		}
	}
	return writeReplaceReport(c, o, targets, !dry)
}

// count parses a --count/--min value: 1 to 999999999.
func count(v string) (int, bool) {
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 999999999 || len(v) > 9 || strings.TrimLeft(v, "0123456789") != "" {
		return 0, false
	}
	return n, true
}

// loadTargets validates and reads every file before anything else
// happens: the path rails, the secret-file rule, the write path policy,
// a regular file, not too large, not binary, named once.
func (m *Module) loadTargets(c *invoke.Call, files []string, force bool) ([]*replaceFile, bool) {
	g := m.guard(c)
	seen := map[string]bool{}
	out := make([]*replaceFile, 0, len(files))
	for _, f := range files {
		safe, ok := m.target(c, g, f, force)
		if !ok {
			return nil, false
		}
		if seen[safe] {
			fail(c, "unexpected_argument", f, "Name each file once")
			return nil, false
		}
		seen[safe] = true
		st, err := os.Stat(safe)
		if err != nil {
			fail(c, "file_not_found", f, "Name existing files only")
			return nil, false
		}
		if !st.Mode().IsRegular() {
			fail(c, "invalid_type", f, "agent_replace changes regular files only")
			return nil, false
		}
		if st.Size() > maxReplaceBytes {
			fail(c, "file_too_large", f, "agent_replace rewrites files up to 64 MiB")
			return nil, false
		}
		b, err := m.readFile(safe)
		if err != nil {
			fail(c, "path_unreadable", f, "Check the file's permissions")
			return nil, false
		}
		if bytes.IndexByte(b, 0) >= 0 {
			fail(c, "binary_file", f, "agent_replace changes text files only")
			return nil, false
		}
		out = append(out, &replaceFile{arg: f, path: safe, info: st, before: b})
	}
	return out, true
}

// applyPairs applies the pairs in order (each to the result of the one
// before) and returns the new content and each pair's match count.
func applyPairs(b []byte, pairs []replacePair) ([]byte, []int) {
	counts := make([]int, len(pairs))
	cur := b
	for i, p := range pairs {
		if p.re != nil {
			matches := p.re.FindAllIndex(cur, -1)
			counts[i] = len(matches)
			if len(matches) > 0 {
				cur = p.re.ReplaceAll(cur, []byte(p.new))
			}
			continue
		}
		old := []byte(p.old)
		counts[i] = bytes.Count(cur, old)
		if counts[i] > 0 {
			cur = bytes.ReplaceAll(cur, old, []byte(p.new))
		}
	}
	return cur, counts
}

// writeAll writes every changed target: first a temporary file next to
// each (mode and, as root, owner copied), then a check that no target
// changed since it was read, then the renames. A failure before the
// renames removes the temporary files and leaves every target as it was.
func (m *Module) writeAll(c *invoke.Call, targets []*replaceFile) bool {
	type pending struct {
		t   *replaceFile
		tmp string
	}
	var todo []pending
	cleanup := func() {
		for _, p := range todo {
			_ = os.Remove(p.tmp)
		}
	}
	for _, t := range targets {
		if bytes.Equal(t.before, t.after) {
			continue
		}
		tmp, err := writeTemp(t)
		if err != nil {
			cleanup()
			fail(c, "write_failed", t.arg+": "+err.Error(), "Nothing was written. Check the directory's permissions and free space")
			return false
		}
		todo = append(todo, pending{t, tmp})
	}
	for _, p := range todo {
		st, err := os.Stat(p.t.path)
		if err != nil || st.Size() != p.t.info.Size() || !st.ModTime().Equal(p.t.info.ModTime()) {
			cleanup()
			fail(c, "file_changed", p.t.arg, "Nothing was written: the file changed while agent_replace ran; run it again")
			return false
		}
	}
	for i, p := range todo {
		if err := os.Rename(p.tmp, p.t.path); err != nil {
			for _, q := range todo[i:] {
				_ = os.Remove(q.tmp)
			}
			var done []string
			for _, q := range todo[:i] {
				done = append(done, q.t.arg)
			}
			fail(c, "write_failed", p.t.arg+": "+err.Error()+"; already written: "+strings.Join(done, " "),
				"Check the listed files")
			return false
		}
	}
	return true
}

// writeTemp writes t.after to a new file in t's directory with t's mode
// (and owner when running as root) and returns its name.
func writeTemp(t *replaceFile) (string, error) {
	dir, base := filepath.Split(t.path)
	f, err := os.CreateTemp(dir, "."+base+".gash-*")
	if err != nil {
		return "", err
	}
	name := f.Name()
	_, werr := f.Write(t.after)
	cerr := f.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(name, t.info.Mode()&(os.ModePerm|os.ModeSetuid|os.ModeSetgid|os.ModeSticky))
	}
	if werr == nil && os.Geteuid() == 0 {
		if st, ok := t.info.Sys().(*syscall.Stat_t); ok {
			werr = os.Lchown(name, int(st.Uid), int(st.Gid))
		}
	}
	if werr != nil {
		_ = os.Remove(name)
		return "", werr
	}
	return name, nil
}

// writeReplaceReport prints the result: by default one JSON object
//
//	{"files":N,"replaced":{"FILE":[n1,n2]},"written":true}
//
// with "dry_run":true and per-file "diff":{"FILE":{"add":A,"del":D}} (lines
// added and removed) under --dry-run; with --format, one row per file and
// pair (file, pair, count).
func writeReplaceReport(c *invoke.Call, o *llmfmt.Options, targets []*replaceFile, written bool) int {
	if o.Format != llmfmt.Default && o.Format != llmfmt.JSON {
		var rows [][]any
		for _, t := range targets {
			for i, n := range t.counts {
				rows = append(rows, []any{t.arg, i + 1, n})
			}
		}
		o.Write(c, o.Format, llmfmt.Table{Cols: []string{"file", "pair", "count"}, Rows: rows},
			map[string]any{"written": written, "dry_run": !written})
		return 0
	}
	var b strings.Builder
	b.WriteString(`{"files":` + strconv.Itoa(len(targets)) + `,"replaced":{`)
	for i, t := range targets {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(llmfmt.Quote(t.arg) + ":[")
		for j, n := range t.counts {
			if j > 0 {
				b.WriteByte(',')
			}
			b.WriteString(strconv.Itoa(n))
		}
		b.WriteByte(']')
	}
	b.WriteString(`},"written":` + strconv.FormatBool(written))
	if !written {
		b.WriteString(`,"dry_run":true,"diff":{`)
		for i, t := range targets {
			if i > 0 {
				b.WriteByte(',')
			}
			add, del := lineDelta(t.before, t.after)
			b.WriteString(llmfmt.Quote(t.arg) + `:{"add":` + strconv.Itoa(add) + `,"del":` + strconv.Itoa(del) + `}`)
		}
		b.WriteByte('}')
	}
	b.WriteString("}\n")
	_, _ = c.Stdout.Write([]byte(b.String()))
	return 0
}

// lineDelta counts the lines of after that before lacks (add) and the
// lines of before that after lacks (del), as multisets: the diffstat of a
// replacement that does not create copies of lines that already exist.
func lineDelta(before, after []byte) (add, del int) {
	if bytes.Equal(before, after) {
		return 0, 0
	}
	seen := map[string]int{}
	for _, l := range strings.SplitAfter(string(before), "\n") {
		if l != "" {
			seen[l]++
		}
	}
	for _, l := range strings.SplitAfter(string(after), "\n") {
		if l == "" {
			continue
		}
		if seen[l] > 0 {
			seen[l]--
			continue
		}
		add++
	}
	for _, n := range seen {
		del += n
	}
	return add, del
}
