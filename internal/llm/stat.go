// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/registry"
)

// statLinesMax is the size from which llm_stat --lines leaves a file's
// lines out (null): counting it would cost more than the call is worth.
const statLinesMax = 64 << 20

// statRow is the answer for one PATH of llm_stat.
type statRow struct {
	name   string
	t      string // f d l p s c b, "missing", or "" when lstat failed otherwise
	size   int64
	mode   string
	uid    uint32
	gid    uint32
	mtime  string
	lines  int64 // -1: not counted
	target string
	err    string
}

// stat is llm_stat PATH... [--lines] [--follow] (plan F1): metadata per
// path in input order, the lines of regular files with --lines, symlinks
// resolved with --follow. A missing path is a row, not an error.
func (m *Module) stat(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags | llmfmt.JobsFlag | llmfmt.TimeoutFlag | llmfmt.PathFlags)
	var paths []string
	lines, follow := false, false
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
		switch a := args[0]; {
		case a == "--lines":
			lines = true
		case a == "--follow":
			follow = true
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, registry.OptionsHint(c.Name))
		default:
			paths = append(paths, a)
		}
		args = args[1:]
	}
	if len(paths) == 0 {
		return fail(c, "missing_file", "", "Give one or more paths: llm_stat PATH...")
	}
	targets, ok := m.textTargets(c, paths, false)
	if !ok {
		return 1
	}
	rows := llmfmt.Map(c.Ctx, o.JobsOr(llmfmt.LocalJobs()), o.Timeout, targets, func(ctx context.Context, t textTarget) statRow {
		r := statPath(ctx, t, lines, follow)
		r.name = t.name(o)
		return r
	})
	o.Write(c, o.FormatOr(llmfmt.TSV), statTable(rows, lines, follow), nil)
	return 0
}

// statPath is the metadata of one guarded PATH: lstat of the path as given
// (symlinks kept), or stat of its target with follow; the line count of a
// regular file under statLinesMax with lines (refused for a secret file).
func statPath(ctx context.Context, t textTarget, lines, follow bool) statRow {
	r := statRow{lines: -1}
	info, err := os.Lstat(t.abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			r.t = "missing"
			return r
		}
		r.err = "path_unreadable"
		return r
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		r.target, _ = os.Readlink(t.abs)
	}
	if follow {
		if fi, err := os.Stat(t.abs); err == nil {
			if info.Mode()&fs.ModeSymlink != 0 || t.real != t.abs {
				r.target = t.real
			}
			info = fi
		}
		// A link that cannot be followed (dangling, a loop) stays "l".
	}
	r.t = typeLetter(info.Mode())
	r.size = info.Size()
	r.mode = modeOctal(info.Mode())
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		r.uid, r.gid = st.Uid, st.Gid
	}
	r.mtime = info.ModTime().UTC().Format("2006-01-02T15:04:05Z")
	if lines && r.t == "f" && r.size < statLinesMax {
		if t.secret {
			r.err = "secret_file_blocked"
			return r
		}
		f, err := os.Open(t.abs)
		if err != nil {
			r.err = targetError(ctx, err)
			return r
		}
		n, err := countLines(ctx, f, r.size)
		_ = f.Close()
		if err != nil {
			r.err = targetError(ctx, err)
			return r
		}
		r.lines = n
	}
	return r
}

// typeLetter is find -type's letter for a file mode.
func typeLetter(m fs.FileMode) string {
	switch {
	case m.IsRegular():
		return "f"
	case m.IsDir():
		return "d"
	case m&fs.ModeSymlink != 0:
		return "l"
	case m&fs.ModeNamedPipe != 0:
		return "p"
	case m&fs.ModeSocket != 0:
		return "s"
	case m&fs.ModeCharDevice != 0:
		return "c"
	case m&fs.ModeDevice != 0:
		return "b"
	}
	return "?"
}

// modeOctal is the permission bits as chmod reads them: "644", "4755".
func modeOctal(m fs.FileMode) string {
	p := uint64(m.Perm())
	if m&fs.ModeSetuid != 0 {
		p |= 0o4000
	}
	if m&fs.ModeSetgid != 0 {
		p |= 0o2000
	}
	if m&fs.ModeSticky != 0 {
		p |= 0o1000
	}
	return strconv.FormatUint(p, 8)
}

// statTable is the rows of llm_stat: p, t, size, mode, owner (user:group,
// as chown reads it), mtime (UTC); lines with --lines; target with
// --follow or when a row is a symlink; error when a row has one.
func statTable(rows []statRow, lines, follow bool) llmfmt.Table {
	withTarget, withErr := follow, false
	for _, r := range rows {
		withTarget = withTarget || r.target != ""
		withErr = withErr || r.err != ""
	}
	cols := []string{"p", "t", "size", "mode", "owner", "mtime"}
	if lines {
		cols = append(cols, "lines")
	}
	if withTarget {
		cols = append(cols, "target")
	}
	if withErr {
		cols = append(cols, "error")
	}
	users, groups := map[uint32]string{}, map[uint32]string{}
	name := func(cache map[uint32]string, id uint32, lookup func(string) (string, error)) string {
		if s, ok := cache[id]; ok {
			return s
		}
		s, err := lookup(strconv.FormatUint(uint64(id), 10))
		if err != nil || s == "" {
			s = strconv.FormatUint(uint64(id), 10)
		}
		cache[id] = s
		return s
	}
	userName := func(id string) (string, error) {
		u, err := user.LookupId(id)
		if err != nil {
			return "", err
		}
		return u.Username, nil
	}
	groupName := func(id string) (string, error) {
		g, err := user.LookupGroupId(id)
		if err != nil {
			return "", err
		}
		return g.Name, nil
	}
	out := llmfmt.Table{Cols: cols, Rows: make([][]any, len(rows))}
	for i, r := range rows {
		row := []any{r.name, nil, nil, nil, nil, nil}
		if r.t != "" {
			row[1] = r.t
		}
		if r.mode != "" {
			row[2], row[3] = r.size, r.mode
			row[4] = name(users, r.uid, userName) + ":" + name(groups, r.gid, groupName)
			row[5] = r.mtime
		}
		if lines {
			if r.lines >= 0 {
				row = append(row, r.lines)
			} else {
				row = append(row, nil)
			}
		}
		if withTarget {
			if r.target != "" {
				row = append(row, r.target)
			} else {
				row = append(row, nil)
			}
		}
		if withErr {
			if r.err != "" {
				row = append(row, r.err)
			} else {
				row = append(row, nil)
			}
		}
		out.Rows[i] = row
	}
	return out
}
