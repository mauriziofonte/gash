// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"context"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/runner"
)

// The parsers below read git's NUL-separated machine formats for the 3.4
// agent functions (llm_git_overview, the llm_git_status, llm_git_diff and
// llm_git_show modes, agent_git_commit). They are additive: the 2.x-parity
// readers of internal/llm keep their own parsers.

// SafeWith is Safe with the options of a bounded call: o.Timeout, o.Setsid
// (no controlling terminal, so nothing git starts can prompt on /dev/tty)
// and o.Env, the caller's exported environment, to which
// GIT_OPTIONAL_LOCKS=0 is prepended as Safe does.
func SafeWith(ctx context.Context, run runner.Runner, o runner.Options, args ...string) runner.Result {
	argv := append(append([]string(nil), pathguard.GitSafeConfig...), args...)
	o.Env = append([]string{"GIT_OPTIONAL_LOCKS=0"}, o.Env...)
	res, _ := run.Run(ctx, runner.Cmd{Path: "git", Args: argv, Options: o})
	return res
}

// Status is `git status --porcelain=v2 --branch -z`, parsed.
type Status struct {
	// Oid is the full id of HEAD, "" before the first commit.
	Oid string
	// Branch is the checked-out branch, "" on a detached HEAD.
	Branch   string
	Detached bool
	// Upstream is the upstream as git names it ("origin/main"), "" when
	// the branch has none.
	Upstream string
	// Ahead and Behind count the commits against the upstream; HasAB is
	// false when the upstream is configured but its ref is missing.
	Ahead, Behind int
	HasAB         bool
	Entries       []StatusEntry
}

// StatusEntry is one changed path.
type StatusEntry struct {
	// Kind is the record type: '1' changed, '2' renamed or copied, 'u'
	// unmerged, '?' untracked, '!' ignored.
	Kind byte
	// X and Y are the index and work tree states ('.' unchanged); "??" for
	// an untracked path.
	X, Y byte
	// Path is relative to the repository root; Orig is the source of a
	// rename or copy.
	Path, Orig string
}

// Staged reports a change in the index.
func (e StatusEntry) Staged() bool { return (e.Kind == '1' || e.Kind == '2') && e.X != '.' }

// Modified reports a change in the work tree that is not staged.
func (e StatusEntry) Modified() bool { return (e.Kind == '1' || e.Kind == '2') && e.Y != '.' }

// Untracked reports an untracked path.
func (e StatusEntry) Untracked() bool { return e.Kind == '?' }

// Conflicted reports an unmerged path.
func (e StatusEntry) Conflicted() bool { return e.Kind == 'u' }

// XY is the two-letter state of `git status --short`: "??" untracked, the
// porcelain v2 letters otherwise ('.' for unchanged).
func (e StatusEntry) XY() string { return string([]byte{e.X, e.Y}) }

// Counts are the number of staged, modified, untracked and conflicted
// paths. A path both staged and modified counts in both.
func (s Status) Counts() (staged, modified, untracked, conflicted int) {
	for _, e := range s.Entries {
		if e.Staged() {
			staged++
		}
		if e.Modified() {
			modified++
		}
		if e.Untracked() {
			untracked++
		}
		if e.Conflicted() {
			conflicted++
		}
	}
	return
}

// Lists are the paths of each state, in git's order.
func (s Status) Lists() (staged, modified, untracked, conflicted []string) {
	staged, modified, untracked, conflicted = []string{}, []string{}, []string{}, []string{}
	for _, e := range s.Entries {
		if e.Staged() {
			staged = append(staged, e.Path)
		}
		if e.Modified() {
			modified = append(modified, e.Path)
		}
		if e.Untracked() {
			untracked = append(untracked, e.Path)
		}
		if e.Conflicted() {
			conflicted = append(conflicted, e.Path)
		}
	}
	return
}

// ParseStatus reads `git status --porcelain=v2 --branch -z`. The path of a
// record is what follows its fixed fields (it may hold spaces); a rename
// or copy record is followed by its source as the next NUL field.
func ParseStatus(out []byte) Status {
	var st Status
	recs := strings.Split(string(out), "\x00")
	for i := 0; i < len(recs); i++ {
		rec := recs[i]
		switch {
		case strings.HasPrefix(rec, "# branch.oid "):
			if v := strings.TrimPrefix(rec, "# branch.oid "); v != "(initial)" {
				st.Oid = v
			}
		case strings.HasPrefix(rec, "# branch.head "):
			st.Branch = strings.TrimPrefix(rec, "# branch.head ")
			if st.Branch == "(detached)" {
				st.Branch, st.Detached = "", true
			}
		case strings.HasPrefix(rec, "# branch.upstream "):
			st.Upstream = strings.TrimPrefix(rec, "# branch.upstream ")
		case strings.HasPrefix(rec, "# branch.ab "):
			f := strings.Fields(strings.TrimPrefix(rec, "# branch.ab "))
			if len(f) == 2 {
				a, ea := strconv.Atoi(strings.TrimPrefix(f[0], "+"))
				b, eb := strconv.Atoi(strings.TrimPrefix(f[1], "-"))
				if ea == nil && eb == nil {
					st.Ahead, st.Behind, st.HasAB = a, b, true
				}
			}
		case len(rec) > 4 && (rec[0] == '1' || rec[0] == '2' || rec[0] == 'u') && rec[1] == ' ':
			fields := map[byte]int{'1': 8, '2': 9, 'u': 10}[rec[0]]
			parts := strings.SplitN(rec, " ", fields+1)
			if len(parts) <= fields {
				continue
			}
			e := StatusEntry{Kind: rec[0], X: rec[2], Y: rec[3], Path: parts[fields]}
			if rec[0] == '2' && i+1 < len(recs) {
				i++
				e.Orig = recs[i]
			}
			st.Entries = append(st.Entries, e)
		case len(rec) > 2 && (rec[0] == '?' || rec[0] == '!') && rec[1] == ' ':
			st.Entries = append(st.Entries, StatusEntry{Kind: rec[0], X: rec[0], Y: rec[0], Path: rec[2:]})
		}
	}
	return st
}

// FileChange is one file of a diff: its path (relative to the repository
// root), the source of a rename or copy, the status letter of --raw (M,
// A, D, R, C, T, U) and the line counts of --numstat (Binary: git counts
// no lines).
type FileChange struct {
	Path, Orig string
	Status     string
	Add, Del   int
	Binary     bool
}

// ParseRawNumstat reads the NUL fields of `git diff --raw --numstat -z`
// (either part may be absent): the --raw records give the status letters,
// the --numstat records the counts, both in the same order. fields is the
// output split on NUL; a leading part that is neither (a commit header) is
// the caller's to remove.
func ParseRawNumstat(fields []string) []FileChange {
	var raw, num []FileChange
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		switch {
		case f == "":
			continue
		case f[0] == ':':
			// ":old new oldsha newsha STATUS" then the path, or the source
			// and the destination for a rename or copy.
			meta := strings.Fields(f)
			status := ""
			if len(meta) > 0 {
				status = meta[len(meta)-1]
			}
			fc := FileChange{Status: status}
			if status != "" {
				fc.Status = status[:1]
			}
			if (fc.Status == "R" || fc.Status == "C") && i+2 < len(fields) {
				fc.Orig, fc.Path = fields[i+1], fields[i+2]
				i += 2
			} else if i+1 < len(fields) {
				fc.Path = fields[i+1]
				i++
			}
			raw = append(raw, fc)
		default:
			add, rest, ok1 := strings.Cut(f, "\t")
			del, path, ok2 := strings.Cut(rest, "\t")
			if !ok1 || !ok2 {
				continue
			}
			fc := FileChange{}
			if add == "-" && del == "-" {
				fc.Binary = true
			} else {
				fc.Add, _ = strconv.Atoi(add)
				fc.Del, _ = strconv.Atoi(del)
			}
			if path == "" && i+2 < len(fields) {
				fc.Orig, fc.Path = fields[i+1], fields[i+2]
				i += 2
			} else {
				fc.Path = path
			}
			num = append(num, fc)
		}
	}
	switch {
	case len(raw) == 0:
		return num
	case len(num) == 0:
		return raw
	}
	// Both parts: the counts join the raw records, in order when the two
	// lists line up, by path otherwise.
	byPath := map[string]FileChange{}
	for _, n := range num {
		byPath[n.Path] = n
	}
	for i := range raw {
		n, ok := FileChange{}, false
		if len(raw) == len(num) && num[i].Path == raw[i].Path {
			n, ok = num[i], true
		} else {
			n, ok = byPath[raw[i].Path]
		}
		if ok {
			raw[i].Add, raw[i].Del, raw[i].Binary = n.Add, n.Del, n.Binary
		}
	}
	return raw
}

// Totals sums a diff: files, inserted and deleted lines.
func Totals(files []FileChange) (n, insertions, deletions int) {
	for _, f := range files {
		insertions += f.Add
		deletions += f.Del
	}
	return len(files), insertions, deletions
}

// SplitZ splits NUL-terminated output into its fields (the trailing NUL
// gives no empty last field).
func SplitZ(out []byte) []string {
	s := strings.TrimSuffix(string(out), "\x00")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\x00")
}

// EmptyTree is the id of the empty tree: what an unborn branch diffs
// against.
const EmptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// LastLine is the last non-empty line of git's stderr, the message an
// error envelope quotes ("fatal: ..."), cut at max bytes on a rune
// boundary.
func LastLine(stderr []byte, max int) string {
	lines := strings.Split(strings.TrimSpace(string(stderr)), "\n")
	msg := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			msg = l
			break
		}
	}
	for _, l := range lines {
		// A "fatal:" or "error:" line says more than a trailing hint.
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "fatal:") || strings.HasPrefix(t, "error:") {
			msg = t
			break
		}
	}
	if max > 0 && len(msg) > max {
		cut := max
		for cut > 0 && msg[cut]&0xc0 == 0x80 {
			cut--
		}
		msg = msg[:cut] + "…"
	}
	return msg
}
