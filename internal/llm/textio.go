// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

// The shared pieces of the 3.4 text reads (llm_stat, llm_count,
// llm_assert, llm_read, llm_log, llm_hash, llm_textstats, llm_textcheck):
// the PATH guard of a multi-target call, the secret-file rule of
// llm_config, the binary test and the block readers the scanners share.
// Every function here works on paths and an open file only: no state
// outside its arguments, so the same code answers a remote call.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/pathguard"
)

// textTarget is one PATH of a text read.
type textTarget struct {
	// arg is the PATH as given: the name the output prints (plan R3).
	arg string
	// abs is the absolute path with symlinks kept (~ expanded): what
	// lstat reads and what --absolute prints.
	abs string
	// real is the guarded realpath: the file that is opened.
	real string
	// secret marks a secret file (pathguard IsSecretFile, the llm_config
	// rule).
	secret bool
}

// name is the path a row prints: as given, or absolute with --absolute.
func (t textTarget) name(o *llmfmt.Options) string {
	if o != nil && o.Absolute {
		return t.abs
	}
	return t.arg
}

// refuseSecret is the llm_config rule for a file an llm_* function
// would read: a secret file (pathguard IsSecretFile, judged on the path as
// given and on a symlink's target) is refused before any other check.
// details says what was refused.
func (m *Module) refuseSecret(c *invoke.Call, file, details string) bool {
	if m.guard(c).IsSecretFile(file) {
		secretBlocked(c, details)
		return true
	}
	return false
}

// secretBlocked writes the refusal of a secret file: secret_file_blocked,
// FATAL, the hint of llm_config.
func secretBlocked(c *invoke.Call, details string) int {
	return fail(c, "secret_file_blocked", details, "This file contains secrets and cannot be read")
}

// textTargets guards every PATH of a multi-target call before anything is
// read: a traversal, a ~user path or a protected path refuses the whole
// call (the envelope of the first one, in input order), and so does a
// secret file when refuseSecrets is set (the content readers: llm_read,
// llm_log, llm_count, llm_assert, llm_textcheck). Duplicates are kept.
func (m *Module) textTargets(c *invoke.Call, paths []string, refuseSecrets bool) ([]textTarget, bool) {
	g := m.guard(c)
	// The paths of one call share their leading directories: each is
	// looked up once.
	g.FS = &cachedFS{fs: g.FS}
	res := llmfmt.Map(c.Ctx, llmfmt.LocalJobs(), 0, paths, func(_ context.Context, p string) checkedTarget {
		return checkTarget(g, p)
	})
	for _, r := range res {
		if r.err != nil {
			refusal(c, r.err)
			return nil, false
		}
		if refuseSecrets && r.t.secret {
			secretBlocked(c, r.t.arg)
			return nil, false
		}
	}
	out := make([]textTarget, len(res))
	for i, r := range res {
		out[i] = r.t
	}
	return out, true
}

// checkedTarget is a guarded PATH, or the refusal of the guard.
type checkedTarget struct {
	t   textTarget
	err error
}

// checkTarget is the guard of one PATH: ValidatePath (traversal, ~user,
// protected paths), the lexical absolute form and the secret test.
func checkTarget(g *pathguard.Guard, p string) checkedTarget {
	real, err := g.ValidatePath(p)
	if err != nil {
		return checkedTarget{err: err}
	}
	expanded := p
	if p == "~" || strings.HasPrefix(p, "~/") {
		expanded = g.Home + p[1:]
	}
	abs, err := g.Realpath(expanded, true)
	if err != nil {
		abs = real
	}
	return checkedTarget{t: textTarget{arg: p, abs: abs, real: real, secret: g.IsSecretFile(p)}}
}

// cachedFS is the guard's filesystem for one call: each Lstat and
// Readlink answer is kept, so a hundred paths below one directory do not
// resolve its components a hundred times.
type cachedFS struct {
	fs    pathguard.FS
	mu    sync.Mutex
	stats map[string]lstatAnswer
	links map[string]readlinkAnswer
}

type lstatAnswer struct {
	info fs.FileInfo
	err  error
}

type readlinkAnswer struct {
	target string
	err    error
}

func (c *cachedFS) Lstat(name string) (fs.FileInfo, error) {
	c.mu.Lock()
	a, ok := c.stats[name]
	c.mu.Unlock()
	if ok {
		return a.info, a.err
	}
	info, err := c.fs.Lstat(name)
	c.mu.Lock()
	if c.stats == nil {
		c.stats = map[string]lstatAnswer{}
	}
	c.stats[name] = lstatAnswer{info, err}
	c.mu.Unlock()
	return info, err
}

func (c *cachedFS) Readlink(name string) (string, error) {
	c.mu.Lock()
	a, ok := c.links[name]
	c.mu.Unlock()
	if ok {
		return a.target, a.err
	}
	target, err := c.fs.Readlink(name)
	c.mu.Lock()
	if c.links == nil {
		c.links = map[string]readlinkAnswer{}
	}
	c.links[name] = readlinkAnswer{target, err}
	c.mu.Unlock()
	return target, err
}

// binarySniff is how much of a file the binary test reads (git's figure).
const binarySniff = 8000

// isBinary is the binary test of the text reads: a NUL byte in the first
// binarySniff bytes.
func isBinary(head []byte) bool {
	if len(head) > binarySniff {
		head = head[:binarySniff]
	}
	return bytes.IndexByte(head, 0) >= 0
}

// sniffBinary reads the head of f (at offset 0) and runs isBinary.
func sniffBinary(f *os.File) (bool, error) {
	buf := make([]byte, binarySniff)
	n, err := f.ReadAt(buf, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	return isBinary(buf[:n]), nil
}

// blockSize is the read unit of the forward scanners.
const blockSize = 1 << 20

// readBuffer is the size of a read buffer for n bytes: n+1 (so one read
// sees the end of file), at most limit, at least 512.
func readBuffer(n int64, limit int) int {
	return int(max(512, min(n+1, int64(limit))))
}

// parallelMin is the file size from which a scanner splits one file
// across goroutines; each part is at least parallelPart bytes.
const (
	parallelMin  = 16 << 20
	parallelPart = 8 << 20
)

// fileParts is how many parts a scan of a file of size bytes uses.
func fileParts(size int64) int {
	if size < parallelMin {
		return 1
	}
	return int(max(1, min(int64(runtime.NumCPU()), size/parallelPart)))
}

// lineBounds splits [0, size) of f into parts ranges that start at a line
// start: bounds[k]..bounds[k+1] is part k, len(bounds) is parts+1.
func lineBounds(f *os.File, size int64, parts int) ([]int64, error) {
	bounds := make([]int64, 0, parts+1)
	bounds = append(bounds, 0)
	buf := make([]byte, 64<<10)
	for k := 1; k < parts; k++ {
		off := size * int64(k) / int64(parts)
		if off <= bounds[len(bounds)-1] {
			continue
		}
		// The line start at or after off: one past the first newline at
		// or after off-1.
		start := size
		for p := off - 1; p < size; {
			n, err := f.ReadAt(buf, p)
			if i := bytes.IndexByte(buf[:n], '\n'); i >= 0 {
				start = p + int64(i) + 1
				break
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return nil, err
			}
			p += int64(n)
		}
		if start > bounds[len(bounds)-1] && start < size {
			bounds = append(bounds, start)
		}
	}
	return append(bounds, size), nil
}

// scanBlocks reads the bytes [from, to) of f in blocks that end at a line
// end (the last one at to) and calls fn with each, in order; the slice is
// valid until fn returns. A line longer than the buffer grows it. ctx
// ends the scan between blocks.
func scanBlocks(ctx context.Context, f *os.File, from, to int64, fn func([]byte)) error {
	buf := make([]byte, readBuffer(to-from, blockSize))
	carry := 0
	off := from
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if carry == len(buf) {
			nb := make([]byte, 2*len(buf))
			copy(nb, buf)
			buf = nb
		}
		want := min(int64(len(buf)-carry), to-off)
		n, err := f.ReadAt(buf[carry:carry+int(want)], off)
		off += int64(n)
		data := buf[:carry+n]
		end := off >= to || (err != nil && n == 0)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if end {
			if len(data) > 0 {
				fn(data)
			}
			return nil
		}
		cut := bytes.LastIndexByte(data, '\n')
		if cut < 0 {
			carry = len(data)
			continue
		}
		fn(data[:cut+1])
		carry = copy(buf, data[cut+1:])
	}
}

// countLines counts the lines of the open file f of size bytes: its
// newlines, plus one for a last line without one (so "a\nb" has 2 lines,
// "a\n" 1, "" 0). A large file is counted in parallel parts.
func countLines(ctx context.Context, f *os.File, size int64) (int64, error) {
	if size == 0 {
		return 0, nil
	}
	n, err := countNewlines(ctx, f, 0, size)
	if err != nil {
		return 0, err
	}
	last := make([]byte, 1)
	if _, err := f.ReadAt(last, size-1); err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	if last[0] != '\n' {
		n++
	}
	return n, nil
}

// countNewlines counts the newlines in [from, to) of f, in parallel parts
// for a large range.
func countNewlines(ctx context.Context, f *os.File, from, to int64) (int64, error) {
	parts := fileParts(to - from)
	type part struct {
		n   int64
		err error
	}
	ranges := make([][2]int64, parts)
	for k := range parts {
		ranges[k] = [2]int64{from + (to-from)*int64(k)/int64(parts), from + (to-from)*int64(k+1)/int64(parts)}
	}
	res := llmfmt.Map(ctx, parts, 0, ranges, func(ctx context.Context, r [2]int64) part {
		buf := make([]byte, readBuffer(r[1]-r[0], blockSize))
		var n int64
		for off := r[0]; off < r[1]; {
			if err := ctx.Err(); err != nil {
				return part{err: err}
			}
			want := min(int64(len(buf)), r[1]-off)
			got, err := f.ReadAt(buf[:want], off)
			n += int64(bytes.Count(buf[:got], []byte{'\n'}))
			off += int64(got)
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return part{err: err}
			}
		}
		return part{n: n}
	})
	var total int64
	for _, r := range res {
		if r.err != nil {
			return 0, r.err
		}
		total += r.n
	}
	return total, nil
}

// errTimeout is the per-target timeout of a multi-target call, reported
// as the target's "timeout" error field (plan S6).
const errTimeout = "timeout"

// targetError names the error of one target for its row: timeout when
// ctx ended, file_not_found, not_a_file, path_unreadable otherwise.
func targetError(ctx context.Context, err error) string {
	switch {
	case ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		return errTimeout
	case errors.Is(err, os.ErrNotExist):
		return "file_not_found"
	case errors.Is(err, errNotAFile):
		return "not_a_file"
	}
	return "path_unreadable"
}

// errNotAFile is a PATH that is not a regular file where one is read.
var errNotAFile = errors.New("not a regular file")

// openRegular opens path for reading when it is a regular file (symlinks
// followed) and returns it with its size.
func openRegular(path string) (*os.File, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	if !st.Mode().IsRegular() {
		_ = f.Close()
		return nil, 0, errNotAFile
	}
	return f, st.Size(), nil
}

// takePositive parses a positive integer option value (1 to 999999999),
// writing the code's envelope when it is not one.
func takePositive(c *invoke.Call, code, v string) (int, bool) {
	if !uintRE.MatchString(v) || atoi(v) == 0 {
		fail(c, code, v, "Give a positive integer (1 to 999999999)")
		return 0, false
	}
	return atoi(v), true
}

// takeOption is the value of the option at args[0], or a missing_value
// envelope.
func takeOption(c *invoke.Call, args []string) (string, bool) {
	if len(args) < 2 {
		fail(c, "missing_value", args[0], args[0]+" needs a value")
		return "", false
	}
	return args[1], true
}
