// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package files ports archive_extract and file_backup from
// lib/modules/files.sh. The extraction tools run through internal/runner
// with the caller's environment and standard streams (architecture.md 7),
// so the results are those of the installed tools, as in 2.x; the 2.x
// file effects and statuses are reproduced (LEDGER FILE-*), with the
// FILE-22 member pre-check added (option b).
package files

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// Register binds archive_extract and file_backup to t.
func Register(t *invoke.Table) {
	t.Handle("archive_extract", archiveExtract)
	t.Handle("file_backup", fileBackup)
}

// runnerFor returns the runner a files handler uses; a test seam
// (runner.Fake in the LEDGER tests).
var runnerFor = func(*invoke.Call) runner.Runner { return runner.Exec{} }

// now is the clock of the file_backup stamp when date is missing.
var now = time.Now

func callCtx(c *invoke.Call) context.Context {
	if c.Ctx != nil {
		return c.Ctx
	}
	return context.Background()
}

// isRegularFile reports Bash's `[[ -f PATH ]]`: a regular file after
// following a final symlink. A directory, a FIFO or a dangling symlink is
// not one (FILE-12: 2.x then says "does not exist").
func isRegularFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

// kind classifies an archive by its suffix, case-insensitive, in the 2.x
// `case` order (so .tar.gz beats .gz), with .tar.Z and .taz routed to the
// uncompress-and-unpack branch before the single .Z branch (FILE-13).
type kind int

const (
	kUnsupported kind = iota
	kTarBz2           // tar xvjf
	kTarGz            // tar xvzf
	kTarXz            // tar xvJf (.tar.xz, .txz)
	kTarZst           // zstd -dc | tar xvf -
	kSingleXz
	kSingleZst
	kSingleBz2
	kRar
	kSingleGz
	kTarPlain // tar xvf
	kTbz2     // tar xvjf
	kTgz      // tar xvzf
	kZip
	kTarZ    // uncompress -c | tar xvf - (.tar.Z, .taz; FILE-13)
	kSingleZ // uncompress -c
	kSevenZip
)

// classify returns the kind and, for a single-file format, the suffix that
// the output name drops (matched case-insensitively, FILE-01/02).
func classify(name string) (kind, string) {
	l := strings.ToLower(name)
	switch {
	case strings.HasSuffix(l, ".tar.bz2"):
		return kTarBz2, ""
	case strings.HasSuffix(l, ".tar.gz"):
		return kTarGz, ""
	case strings.HasSuffix(l, ".tar.xz"), strings.HasSuffix(l, ".txz"):
		return kTarXz, ""
	case strings.HasSuffix(l, ".tar.zst"), strings.HasSuffix(l, ".tzst"):
		return kTarZst, ""
	case strings.HasSuffix(l, ".xz"):
		return kSingleXz, ".xz"
	case strings.HasSuffix(l, ".zst"):
		return kSingleZst, ".zst"
	case strings.HasSuffix(l, ".bz2"):
		return kSingleBz2, ".bz2"
	case strings.HasSuffix(l, ".rar"):
		return kRar, ""
	case strings.HasSuffix(l, ".gz"):
		return kSingleGz, ".gz"
	case strings.HasSuffix(l, ".tar"):
		return kTarPlain, ""
	case strings.HasSuffix(l, ".tbz2"):
		return kTbz2, ""
	case strings.HasSuffix(l, ".tgz"):
		return kTgz, ""
	case strings.HasSuffix(l, ".zip"):
		return kZip, ""
	case strings.HasSuffix(l, ".tar.z"), strings.HasSuffix(l, ".taz"):
		return kTarZ, ""
	case strings.HasSuffix(l, ".z"):
		return kSingleZ, ".z"
	case strings.HasSuffix(l, ".7z"):
		return kSevenZip, ""
	default:
		return kUnsupported, ""
	}
}

// program is one a branch runs: cmd as it is run, tool its name in the
// prerequisite catalog (gunzip comes with gzip, bunzip2 with bzip2).
type program struct{ cmd, tool string }

// bsdTar is set where tar is bsdtar (macOS), which decompresses .gz and
// .bz2 itself; GNU tar runs gzip and bzip2 (3.4: they were not checked,
// and tar's "Cannot exec" lines came after OUTPUT_DIR was made).
var bsdTar = runtime.GOOS == "darwin"

// needs are the programs a branch runs, in the order they are checked.
// xz is needed for .tar.xz everywhere: the FILE-22 guard reads the members
// through it, and without it the archive is refused, not extracted
// unchecked (3.4).
func (k kind) needs() []program {
	tar := program{"tar", "tar"}
	switch k {
	case kTarPlain:
		return []program{tar}
	case kTarGz, kTgz:
		if bsdTar {
			return []program{tar}
		}
		return []program{tar, {"gzip", "gzip"}}
	case kTarBz2, kTbz2:
		if bsdTar {
			return []program{tar}
		}
		return []program{tar, {"bzip2", "bzip2"}}
	case kTarXz:
		return []program{tar, {"xz", "xz"}}
	case kTarZst:
		return []program{{"zstd", "zstd"}, tar}
	case kTarZ:
		return []program{{"uncompress", "uncompress"}, tar}
	case kSingleXz:
		return []program{{"xz", "xz"}}
	case kSingleZst:
		return []program{{"zstd", "zstd"}}
	case kSingleBz2:
		return []program{{"bunzip2", "bzip2"}}
	case kSingleGz:
		return []program{{"gunzip", "gzip"}}
	case kSingleZ:
		return []program{{"uncompress", "uncompress"}}
	case kRar:
		return []program{{"unrar", "unrar"}}
	case kZip:
		return []program{{"unzip", "unzip"}}
	case kSevenZip:
		return []program{{"7z", "7z"}}
	}
	return nil
}

// labels name each kind in the prerequisite report, "archive_extract
// (.tar.xz) needs xz".
var labels = map[kind]string{
	kTarBz2: ".tar.bz2", kTarGz: ".tar.gz", kTarXz: ".tar.xz", kTarZst: ".tar.zst",
	kSingleXz: ".xz", kSingleZst: ".zst", kSingleBz2: ".bz2", kRar: ".rar", kSingleGz: ".gz",
	kTarPlain: ".tar", kTbz2: ".tbz2", kTgz: ".tgz", kZip: ".zip", kTarZ: ".tar.Z",
	kSingleZ: ".Z", kSevenZip: ".7z",
}

// missingTool is the prerequisite report of a branch program that is not
// on PATH, for the archive of the call (3.4: "Command 'X' is not installed
// or not in PATH.", and "Extracting .zst needs zstd." for zstd, FILE-19).
func missingTool(c *invoke.Call, cmd string) int {
	k, _ := classify(c.Arg(0))
	tool := cmd
	for _, n := range k.needs() {
		if n.cmd == cmd {
			tool = n.tool
		}
	}
	return prereq.Fail(c, tool, "archive_extract ("+labels[k]+")", false)
}

// hasWildcard reports whether name holds a character that unzip and unrar
// expand as a wildcard in the archive operand (FILE-10).
func hasWildcard(name string) bool {
	return strings.ContainsAny(name, "*?[")
}

// notRegular reports an existing path that is not a regular file: a
// directory, a FIFO, a socket, a device or a dangling symlink (FILE-12).
func notRegular(path string) bool {
	if st, err := os.Stat(path); err == nil {
		return !st.Mode().IsRegular()
	}
	_, err := os.Lstat(path)
	return err == nil
}

// interactive reports whether the extraction tools may prompt: never in
// headless mode or when stdin is not a terminal (FILE-11).
func interactive(c *invoke.Call) bool {
	return !c.Term.Headless() && c.Term.StdinTTY()
}

// dashSafe prefixes "./" to a name that starts with "-", so no tool reads
// it as an option (FILE-08, FILE-09).
func dashSafe(name string) string {
	if strings.HasPrefix(name, "-") {
		return "./" + name
	}
	return name
}

func archiveExtract(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	s := ui.StyleOf(c.Term, false)
	p := s.Printer(c.Stderr)

	archive := c.Arg(0)
	outputDir := c.Arg(1)
	if outputDir == "" {
		outputDir = "."
	}
	if archive == "" {
		p.Error("Missing archive file. Usage: archive_extract <archive_file> [output_dir]")
		return 1
	}
	if notRegular(archive) {
		p.Error("'" + archive + "' is not a regular file.")
		return 1
	}
	if !isRegularFile(archive) {
		p.Error("File '" + archive + "' does not exist.")
		return 1
	}

	// Everything is checked before OUTPUT_DIR is created (FILE-07): the
	// format, the archive name, the tools, the members and the output name.
	k, suffix := classify(archive)
	if k == kUnsupported {
		p.Error("Cannot extract '" + archive + "', unsupported file type.")
		return 1
	}
	if (k == kZip || k == kRar) && hasWildcard(baseName(archive)) {
		tool := k.needs()[0].cmd
		p.Error("Archive name '" + archive + "' contains wildcard characters (* ? [) that " + tool +
			" would expand; rename it.")
		return 1
	}
	for _, n := range k.needs() {
		if _, err := runner.LookPath(n.cmd, c.Env.Child(), ""); err != nil {
			return missingTool(c, n.cmd)
		}
	}
	// FILE-22 (option b): refuse a hostile member before OUTPUT_DIR exists,
	// and an archive whose members cannot be listed for want of a program.
	member, unsafe, err := memberGuard(c, k, archive, outputDir)
	if me := (*missingErr)(nil); errors.As(err, &me) {
		return missingTool(c, me.cmd)
	}
	if unsafe {
		p.Error("Archive '" + archive + "' has an unsafe member '" + member +
			"' (absolute path, .. component or link outside the destination); nothing extracted.")
		return 1
	}
	var target string
	if suffix != "" {
		target = outputDir + "/" + singleFileName(archive, suffix)
		if sameFile(target, archive) {
			p.Error("Output '" + target + "' would overwrite the archive '" + archive + "'.")
			return 1
		}
	}

	// FILE-08: mkdir -p -- OUTPUT_DIR (mkdir's own lines are kept).
	if st, err := os.Stat(outputDir); err != nil || !st.IsDir() {
		res, rerr := runnerFor(c).Run(callCtx(c), runner.Cmd{
			Path: "mkdir", Args: []string{"-p", "--", outputDir},
			Options: runner.Options{Env: c.Env.Child(), Stdin: c.Stdin, Stdout: c.Stdout, Stderr: c.Stderr},
		})
		if rerr != nil || res.ExitCode != 0 {
			p.Error("Failed to create output directory '" + outputDir + "'.")
			return 1
		}
	}

	// A leading-dash name must not look like an option to the tools.
	src, dir := dashSafe(archive), dashSafe(outputDir)
	ask := interactive(c)

	switch k {
	case kTarBz2, kTbz2:
		return runTar(c, "xvjf", src, dir)
	case kTarGz, kTgz:
		return runTar(c, "xvzf", src, dir)
	case kTarXz:
		return runTar(c, "xvJf", src, dir)
	case kTarPlain:
		return runTar(c, "xvf", src, dir)
	case kTarZst:
		return pipeline(c, runnerFor(c),
			runner.Cmd{Path: "zstd", Args: []string{"-dc", "--", src}},
			runner.Cmd{Path: "tar", Args: []string{"xvf", "-", "-C", dir, "--no-same-owner", "--no-same-permissions"}})
	case kTarZ:
		args, in, err := uncompressArgs(src)
		if err != nil {
			return 1
		}
		if in != nil {
			defer in.Close()
		}
		left := runner.Cmd{Path: "uncompress", Args: args}
		if in != nil {
			left.Stdin = in
		}
		return pipeline(c, runnerFor(c), left,
			runner.Cmd{Path: "tar", Args: []string{"xvf", "-", "-C", dir, "--no-same-owner", "--no-same-permissions"}})
	case kSingleXz:
		return runSingle(c, "xz", []string{"-dc", "--", src}, target, nil)
	case kSingleZst:
		return runSingle(c, "zstd", []string{"-dc", "--", src}, target, nil)
	case kSingleBz2:
		return runSingle(c, "bunzip2", []string{"-c", "--", src}, target, nil)
	case kSingleGz:
		return runSingle(c, "gunzip", []string{"-c", "--", src}, target, nil)
	case kSingleZ:
		args, in, err := uncompressArgs(src)
		if err != nil {
			return 1
		}
		if in == nil {
			return runSingle(c, "uncompress", args, target, nil)
		}
		defer in.Close()
		return runSingle(c, "uncompress", args, target, in)
	case kRar:
		args := []string{"x"}
		if !ask {
			args = append(args, "-o-", "-p-")
		}
		if !strings.HasSuffix(dir, "/") {
			dir += "/"
		}
		return runTool(c, "unrar", append(args, "--", src, dir), ask)
	case kZip:
		args := []string{}
		if !ask {
			args = append(args, "-n")
		}
		return runTool(c, "unzip", append(args, src, "-d", dir), ask)
	default: // kSevenZip
		args := []string{"x", "-spd"}
		if !ask {
			args = append(args, "-aos")
		}
		return runTool(c, "7z", append(args, "-o"+dir, "--", src), ask)
	}
}

// runTool runs a multi-member extractor with the caller's environment and
// standard output. When it may not prompt (FILE-11) its stdin is /dev/null
// and it runs in a new session, without a controlling terminal. Any
// non-zero status collapses to 1 (FILE-23 option a).
func runTool(c *invoke.Call, tool string, args []string, ask bool) int {
	o := runner.Options{Env: c.Env.Child(), Stdin: c.Stdin, Stdout: c.Stdout, Stderr: c.Stderr}
	if !ask {
		o.Stdin, o.Setsid = nil, true
	}
	res, err := runnerFor(c).Run(callCtx(c), runner.Cmd{Path: tool, Args: args, Options: o})
	if notFound(err) {
		return missingTool(c, tool)
	}
	if err != nil || res.ExitCode != 0 {
		return 1
	}
	return 0
}

func runTar(c *invoke.Call, flags, archive, outputDir string) int {
	return runTool(c, "tar", []string{flags, archive, "-C", outputDir, "--no-same-owner", "--no-same-permissions"}, true)
}

// runSingle decompresses one file to target (FILE-03): into a new file in
// the target's directory, opened O_EXCL with mode 0666 (the umask applies),
// renamed over target only when the tool succeeds and removed otherwise, so
// a corrupt archive never destroys an existing file and a symlink at target
// is replaced, never written through. stdin, when not nil, replaces the
// caller's stdin.
func runSingle(c *invoke.Call, tool string, args []string, target string, stdin io.Reader) int {
	if st, err := os.Lstat(target); err == nil && st.IsDir() {
		_, _ = io.WriteString(c.Stderr, target+": Is a directory\n")
		return 1
	}
	if stdin == nil {
		stdin = c.Stdin
	}
	f, tmp, err := createTemp(filepath.Dir(target))
	if err != nil {
		_, _ = io.WriteString(c.Stderr, target+": "+redirReason(err)+"\n")
		return 1
	}
	res, rerr := runnerFor(c).Run(callCtx(c), runner.Cmd{
		Path: tool, Args: args,
		Options: runner.Options{Env: c.Env.Child(), Stdin: stdin, Stdout: f, Stderr: c.Stderr},
	})
	cerr := f.Close()
	if notFound(rerr) {
		_ = os.Remove(tmp)
		return missingTool(c, tool)
	}
	if rerr != nil || res.ExitCode != 0 || cerr != nil {
		_ = os.Remove(tmp)
		return 1
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		_, _ = io.WriteString(c.Stderr, target+": "+redirReason(err)+"\n")
		return 1
	}
	return 0
}

// createTemp creates dir/.axe-XXXXXX with O_EXCL and mode 0666, so the
// kernel umask gives the file the mode a shell redirection would.
func createTemp(dir string) (*os.File, string, error) {
	var b [6]byte
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for {
		if _, err := rand.Read(b[:]); err != nil {
			return nil, "", err
		}
		for i := range b {
			b[i] = chars[int(b[i])%len(chars)]
		}
		name := filepath.Join(dir, ".axe-"+string(b[:]))
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return f, name, err
	}
}

// pipeline runs left | right through an in-process pipe and returns 1 when
// either process exits non-zero (set -o pipefail; FILE-17).
func pipeline(c *invoke.Call, rnr runner.Runner, left, right runner.Cmd) int {
	pr, pw := io.Pipe()
	env := c.Env.Child()
	stderr := c.Stderr
	if _, isFile := stderr.(*os.File); !isFile && stderr != nil {
		stderr = &lockedWriter{w: stderr} // both children copy into it at once
	}
	if left.Stdin == nil {
		left.Stdin = c.Stdin
	}
	left.Env, left.Stdout, left.Stderr = env, pw, stderr
	right.Env, right.Stdin, right.Stdout, right.Stderr = env, pr, c.Stdout, stderr

	done := make(chan int, 1)
	go func() {
		res, err := rnr.Run(callCtx(c), left)
		_ = pw.CloseWithError(err) // closing the write end gives the reader EOF
		if err != nil || res.ExitCode != 0 {
			done <- 1
		} else {
			done <- 0
		}
	}()
	rres, rerr := rnr.Run(callCtx(c), right)
	_ = pr.Close()
	leftRC := <-done
	if rerr != nil || rres.ExitCode != 0 || leftRC != 0 {
		return 1
	}
	return 0
}

// lockedWriter serializes the writes of the two sides of a pipeline into
// one writer that is not a file (a file is handed to both children as is).
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(b)
}

// redirReason is the strerror text of a failed output open.
func redirReason(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ENOENT:
			return "No such file or directory"
		case syscall.EACCES:
			return "Permission denied"
		case syscall.EISDIR:
			return "Is a directory"
		case syscall.ENOTDIR:
			return "Not a directory"
		case syscall.EROFS:
			return "Read-only file system"
		}
		return errno.Error()
	}
	return err.Error()
}

// notFound reports a runner start error for a program that is not on PATH.
func notFound(err error) bool {
	var se *runner.StartError
	if errors.As(err, &se) {
		return se.Status == 127
	}
	return false
}

// baseName is the last path component of name (trailing slashes dropped).
func baseName(name string) string {
	trimmed := strings.TrimRight(name, "/")
	if trimmed == "" {
		return "/"
	}
	if i := strings.LastIndexByte(trimmed, '/'); i >= 0 {
		trimmed = trimmed[i+1:]
	}
	return trimmed
}

// singleFileName is the output name of a single-file format: the archive's
// base name with the matched suffix removed case-insensitively, when
// something remains (FILE-01, FILE-02), kept byte for byte otherwise:
// trailing newlines included (FILE-15).
func singleFileName(archive, suffix string) string {
	b := baseName(archive)
	if len(b) > len(suffix) && strings.EqualFold(b[len(b)-len(suffix):], suffix) {
		return b[:len(b)-len(suffix)]
	}
	return b
}

// sameFile reports whether target names the archive itself, through
// symlinks too (FILE-01).
func sameFile(target, archive string) bool {
	ts, err := os.Stat(target)
	if err != nil {
		return false
	}
	as, err := os.Stat(archive)
	return err == nil && os.SameFile(ts, as)
}

func fileBackup(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	s := ui.StyleOf(c.Term, false)
	p := s.Printer(c.Stderr)

	file := c.Arg(0)
	if file == "" {
		p.Error("Missing file. Usage: file_backup <file>")
		return 1
	}
	if notRegular(file) {
		p.Error("'" + file + "' is not a regular file.")
		return 1
	}
	st, err := os.Stat(file)
	if err != nil || !st.Mode().IsRegular() {
		p.Error("File '" + file + "' does not exist.")
		return 1
	}

	rnr := runnerFor(c)
	// FILE-25 (option a): the stamp comes from `date +…` on PATH, so POSIX
	// TZ strings match 2.x and a user `date` function stays ignored (FILE-20).
	// Without date, or from one that prints nothing, the stamp is the local
	// time read here (3.4: it was empty, FILE_backup_).
	dres, _ := rnr.Run(callCtx(c), runner.Cmd{
		Path: "date", Args: []string{"+%Y%m%d%H%M%S"},
		Options: runner.Options{Env: c.Env.Child()},
	})
	stamp := runner.CommandSubst(dres.Stdout)
	if stamp == "" {
		stamp = now().Format("20060102150405")
	}

	// FILE-04/05: reserve a free name atomically. O_EXCL|O_NOFOLLOW fails on
	// any existing entry (a file, a symlink, a dangling symlink, a hard link,
	// a directory), which is never opened: the next suffix is tried instead.
	backup, finalMode, fixMode, err := reserveBackup(file+"_backup_"+stamp, st.Mode().Perm())
	if err != nil {
		p.Error("Failed to create backup file.")
		return 1
	}

	// FILE-21: cp runs with LC_ALL=C added, so the -v line always has C-locale
	// quoting; it rewrites the reserved file in place. FILE-06: a failing cp
	// is a failure, and its partial file is removed.
	res, rerr := rnr.Run(callCtx(c), runner.Cmd{
		Path: "cp", Args: []string{"-v", "--", file, backup},
		Options: runner.Options{Env: withLCAllC(c.Env.Child()), Stdin: c.Stdin, Stdout: c.Stdout, Stderr: c.Stderr},
	})
	if notFound(rerr) {
		// cp is POSIX, outside the prerequisite catalog: named, not
		// "Failed to create backup file." (3.4).
		_ = os.Remove(backup)
		p.Error("file_backup needs cp, which is not on PATH.")
		return 1
	}
	if rerr != nil || res.ExitCode != 0 || !isRegularFile(backup) {
		_ = os.Remove(backup)
		p.Error("Failed to create backup file.")
		return 1
	}
	if fixMode {
		_ = os.Chmod(backup, finalMode)
	}
	return 0
}

// reserveBackup creates base, or base_1, base_2... when it exists, with
// O_CREAT|O_EXCL|O_NOFOLLOW, and returns the name taken. The file gets the
// source's permission bits with the umask applied, as the file cp would
// create; it is created owner-writable so cp can rewrite it, and for a
// source without owner write fixMode asks for finalMode once the copy is
// done.
func reserveBackup(base string, perm os.FileMode) (name string, finalMode os.FileMode, fixMode bool, err error) {
	for n := 0; ; n++ {
		name = base
		if n > 0 {
			name = base + "_" + strconv.Itoa(n)
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, perm|0o200)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", 0, false, err
		}
		st, serr := f.Stat()
		cerr := f.Close()
		if serr == nil && perm&0o200 == 0 {
			finalMode, fixMode = st.Mode().Perm()&^0o200, true
		}
		return name, finalMode, fixMode, cerr
	}
}

// withLCAllC returns env with LC_ALL forced to C (FILE-21).
func withLCAllC(env []string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, "LC_ALL=") {
			out = append(out, kv)
		}
	}
	return append(out, "LC_ALL=C")
}

// uncompressArgs is the uncompress argv for src. A BSD uncompress appends
// .Z to an operand that lacks it (s.z becomes s.z.Z), so a name that does
// not end in .Z (.z, .taz) is fed on stdin instead; the returned file is
// then open and the caller closes it.
func uncompressArgs(src string) ([]string, *os.File, error) {
	if strings.HasSuffix(src, ".Z") {
		return []string{"-c", "--", src}, nil, nil
	}
	f, err := os.Open(src)
	if err != nil {
		return nil, nil, err
	}
	return []string{"-c"}, f, nil
}
