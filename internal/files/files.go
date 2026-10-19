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
	"errors"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
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

// kind classifies an archive by its suffix, case-insensitive, in the exact
// 2.x `case` order (so .tar.gz beats .gz and .tar.Z falls to the single
// uncompress branch, FILE-13).
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
	kSingleZ // uncompress (also .tar.Z, FILE-13)
	kSevenZip
)

// classify returns the kind and, for a single-file format, the literal
// lowercase suffix 2.x handed to `basename` (case-sensitive strip, FILE-01/02).
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
	case strings.HasSuffix(l, ".z"):
		return kSingleZ, ".z"
	case strings.HasSuffix(l, ".7z"):
		return kSevenZip, ""
	default:
		return kUnsupported, ""
	}
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
	if !isRegularFile(archive) {
		p.Error("File '" + archive + "' does not exist.")
		return 1
	}

	k, suffix := classify(archive)

	// FILE-22 (option b): refuse a hostile member before OUTPUT_DIR exists.
	if member, unsafe, _ := memberGuard(c, k, archive, outputDir); unsafe {
		p.Error("Archive '" + archive + "' has an unsafe member '" + member +
			"' (absolute path, .. component or link outside the destination); nothing extracted.")
		return 1
	}

	// FILE-07/08: mkdir -p OUTPUT_DIR, 2.x argv (no --), before the dispatch.
	if st, err := os.Stat(outputDir); err != nil || !st.IsDir() {
		rnr := runnerFor(c)
		res, rerr := rnr.Run(callCtx(c), runner.Cmd{
			Path: "mkdir", Args: []string{"-p", outputDir},
			Options: runner.Options{Env: c.Env.Child(), Stdin: c.Stdin, Stdout: c.Stdout, Stderr: c.Stderr},
		})
		if rerr != nil || res.ExitCode != 0 {
			p.Error("Failed to create output directory '" + outputDir + "'.")
			return 1
		}
	}

	// A leading-dash archive name must not look like an option to the tools.
	if strings.HasPrefix(archive, "-") {
		archive = "./" + archive
	}

	switch k {
	case kTarBz2:
		return runTar(c, "xvjf", archive, outputDir)
	case kTarGz, kTgz:
		return runTar(c, "xvzf", archive, outputDir)
	case kTarXz:
		return runTar(c, "xvJf", archive, outputDir)
	case kTarPlain:
		return runTar(c, "xvf", archive, outputDir)
	case kTbz2:
		return runTar(c, "xvjf", archive, outputDir)
	case kTarZst:
		return runTarZst(c, archive, outputDir)
	case kSingleXz:
		return runSingle(c, "xz", []string{"-dc", "--", archive}, archive, outputDir, suffix, false)
	case kSingleZst:
		return runSingle(c, "zstd", []string{"-dc", "--", archive}, archive, outputDir, suffix, true)
	case kSingleBz2:
		return runSingle(c, "bunzip2", []string{"-c", archive}, archive, outputDir, suffix, false)
	case kSingleGz:
		return runSingle(c, "gunzip", []string{"-c", archive}, archive, outputDir, suffix, false)
	case kSingleZ:
		return runSingle(c, "uncompress", []string{archive, "-c"}, archive, outputDir, suffix, false)
	case kRar:
		return runTool(c, "unrar", []string{"x", archive, outputDir})
	case kZip:
		return runTool(c, "unzip", []string{archive, "-d", outputDir})
	case kSevenZip:
		return runTool(c, "7z", []string{"x", archive, "-o" + outputDir})
	default:
		p.Error("Cannot extract '" + archive + "', unsupported file type.")
		return 1
	}
}

// runTool runs a multi-member extractor with the caller's environment and
// standard streams; a missing tool gets the 2.x dependency error (FILE-19),
// any non-zero status collapses to 1 (FILE-23 option a).
func runTool(c *invoke.Call, tool string, args []string) int {
	rnr := runnerFor(c)
	res, err := rnr.Run(callCtx(c), runner.Cmd{
		Path: tool, Args: args,
		Options: runner.Options{Env: c.Env.Child(), Stdin: c.Stdin, Stdout: c.Stdout, Stderr: c.Stderr},
	})
	if notFound(err) {
		ui.StyleOf(c.Term, false).Printer(c.Stderr).Error("Command '" + tool + "' is not installed or not in PATH.")
		return 1
	}
	if err != nil || res.ExitCode != 0 {
		return 1
	}
	return 0
}

func runTar(c *invoke.Call, flags, archive, outputDir string) int {
	return runTool(c, "tar", []string{flags, archive, "-C", outputDir, "--no-same-owner", "--no-same-permissions"})
}

// runSingle reproduces the single-file branches (FILE-01/02/03/15): the
// output OUTPUT_DIR/<basename minus the lowercase suffix> is opened with
// O_TRUNC, 0666, following symlinks, before the tool runs; a corrupt input
// or a missing tool leaves the truncated file. zstd is checked first.
func runSingle(c *invoke.Call, tool string, args []string, archive, outputDir, suffix string, zstdGuard bool) int {
	rnr := runnerFor(c)
	p := ui.StyleOf(c.Term, false).Printer(c.Stderr)
	if zstdGuard {
		if _, err := runner.LookPath("zstd", c.Env.Child(), ""); err != nil {
			p.Error("Extracting .zst needs zstd.")
			return 1
		}
	}
	name := strings.TrimRight(basenameSuffix(archive, suffix), "\n") // $(basename …) strips trailing newlines (FILE-15)
	out := outputDir + "/" + name
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		// 2.x: the `>` redirection fails, Bash prints "…: line N: OUT: REASON"
		// and the tool never runs (|| rc=1). The "file: line N:" prefix names
		// the sourced file and cannot be reproduced; the rest is kept.
		_, _ = io.WriteString(c.Stderr, out+": "+redirReason(err)+"\n")
		return 1
	}
	defer f.Close()
	res, rerr := rnr.Run(callCtx(c), runner.Cmd{
		Path: tool, Args: args,
		Options: runner.Options{Env: c.Env.Child(), Stdin: c.Stdin, Stdout: f, Stderr: c.Stderr},
	})
	if notFound(rerr) {
		p.Error("Command '" + tool + "' is not installed or not in PATH.")
		return 1
	}
	if rerr != nil || res.ExitCode != 0 {
		return 1
	}
	return 0
}

// runTarZst reproduces `zstd -dc -- F | tar xvf - …` with pipefail semantics
// (FILE-17): rc 1 when either side fails. zstd is checked first, then tar;
// a missing tar means zstd is never started (FILE-19).
func runTarZst(c *invoke.Call, archive, outputDir string) int {
	rnr := runnerFor(c)
	p := ui.StyleOf(c.Term, false).Printer(c.Stderr)
	if _, err := runner.LookPath("zstd", c.Env.Child(), ""); err != nil {
		p.Error("Extracting .zst needs zstd.")
		return 1
	}
	if _, err := runner.LookPath("tar", c.Env.Child(), ""); err != nil {
		p.Error("Command 'tar' is not installed or not in PATH.")
		return 1
	}
	return pipeline(c, rnr,
		runner.Cmd{Path: "zstd", Args: []string{"-dc", "--", archive}},
		runner.Cmd{Path: "tar", Args: []string{"xvf", "-", "-C", outputDir, "--no-same-owner", "--no-same-permissions"}})
}

// pipeline runs left | right through an in-process pipe and returns 1 when
// either process exits non-zero (set -o pipefail; FILE-17).
func pipeline(c *invoke.Call, rnr runner.Runner, left, right runner.Cmd) int {
	pr, pw := io.Pipe()
	env := c.Env.Child()
	left.Env, left.Stdin, left.Stdout, left.Stderr = env, c.Stdin, pw, c.Stderr
	right.Env, right.Stdin, right.Stdout, right.Stderr = env, pr, c.Stdout, c.Stderr

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

// redirReason is the strerror text Bash prints for a failed `>` open.
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

// basenameSuffix is `basename NAME SUFFIX`: the last path component with a
// trailing SUFFIX removed when it is present and is not the whole name
// (case-sensitive, so a .GZ name keeps its name against a .gz suffix).
func basenameSuffix(name, suffix string) string {
	trimmed := strings.TrimRight(name, "/")
	if trimmed == "" {
		trimmed = "/"
	}
	if i := strings.LastIndexByte(trimmed, '/'); i >= 0 {
		trimmed = trimmed[i+1:]
	}
	if suffix != "" && trimmed != suffix && strings.HasSuffix(trimmed, suffix) {
		trimmed = trimmed[:len(trimmed)-len(suffix)]
	}
	return trimmed
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
	if !isRegularFile(file) {
		p.Error("File '" + file + "' does not exist.")
		return 1
	}

	rnr := runnerFor(c)
	// FILE-25 (option a): the stamp comes from `date +…` on PATH, so POSIX
	// TZ strings match 2.x and a user `date` function stays ignored (FILE-20).
	dres, _ := rnr.Run(callCtx(c), runner.Cmd{
		Path: "date", Args: []string{"+%Y%m%d%H%M%S"},
		Options: runner.Options{Env: c.Env.Child()},
	})
	stamp := runner.CommandSubst(dres.Stdout)
	backup := file + "_backup_" + stamp

	// FILE-21: cp runs with LC_ALL=C added, so the -v line always has C-locale
	// quoting. FILE-06: cp's status is ignored; the regular-file check decides.
	_, _ = rnr.Run(callCtx(c), runner.Cmd{
		Path: "cp", Args: []string{"-v", "--", file, backup},
		Options: runner.Options{Env: withLCAllC(c.Env.Child()), Stdin: c.Stdin, Stdout: c.Stdout, Stderr: c.Stderr},
	})
	if !isRegularFile(backup) {
		p.Error("Failed to create backup file.")
		return 1
	}
	return 0
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
