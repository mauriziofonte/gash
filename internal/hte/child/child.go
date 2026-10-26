// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package child holds what every process gash hte starts has in common:
// an absolute binary, an explicit argument vector, the fixed PATH and C
// locale of BaseEnv (nothing of the caller's environment reaches a child,
// so nothing secret does), / as the working directory, a new session
// without a controlling terminal, output captured through files, and a
// timeout. The processes themselves are started by internal/runner, the
// only package of Gash that starts any.
package child

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/runner"
)

// DefaultTimeout bounds a child that sets no other timeout.
const DefaultTimeout = 60 * time.Second

// BasePath is the PATH of every child.
const BasePath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// BaseEnv is the environment of a child: the fixed PATH, the C locale,
// and HOME when given.
func BaseEnv(home string) []string {
	env := []string{"PATH=" + BasePath, "LC_ALL=C", "LANG=C"}
	if home != "" {
		env = append(env, "HOME="+home)
	}
	return env
}

// Find returns an executable named name in the system directories of
// BasePath, as seen through exists; "" when there is none. Programs run as
// root, and sudo, are looked up here only.
func Find(name string, exists func(string) bool) string {
	return FindIn(name, strings.Split(BasePath, ":"), exists)
}

// FindIn returns an executable named name in dirs, as seen through
// exists; "" when there is none.
func FindIn(name string, dirs []string, exists func(string) bool) string {
	for _, dir := range dirs {
		if p := filepath.Join(dir, name); exists(p) {
			return p
		}
	}
	return ""
}

// UserDirs is where gash hte looks for a program it runs as the user:
// BasePath, then Homebrew's bin directories, ~/.local/bin and the
// directories of the user's PATH (3.4: BasePath only, so an openssl
// installed there was "not installed"). An entry that is not absolute, is
// not a directory, or that group or others may write is left out: what is
// found there could be anyone's program. stat sees the system.
func UserDirs(home, path string, stat func(string) (fs.FileInfo, error)) []string {
	dirs := strings.Split(BasePath, ":")
	extra := []string{"/opt/homebrew/bin", "/opt/homebrew/sbin"}
	if filepath.IsAbs(home) {
		extra = append(extra, filepath.Join(home, ".local/bin"))
	}
	for _, d := range append(extra, filepath.SplitList(path)...) {
		if !filepath.IsAbs(d) {
			continue
		}
		d = filepath.Clean(d)
		if slices.Contains(dirs, d) {
			continue
		}
		if st, err := stat(d); err != nil || !st.IsDir() || st.Mode().Perm()&0o022 != 0 {
			continue
		}
		dirs = append(dirs, d)
	}
	return dirs
}

// Cmd is the start of path with args: env as the whole environment (nil
// means BaseEnv("")), / as the working directory, DefaultTimeout, a new
// session, and the output captured through files, so that a daemon that
// forks and keeps it (php-fpm, httpd -k start) does not hold the caller.
func Cmd(path string, env []string, args ...string) runner.Cmd {
	if env == nil {
		env = BaseEnv("")
	}
	return runner.Cmd{Path: path, Args: args, Options: runner.Options{
		Env: env, Dir: "/", Timeout: DefaultTimeout, Setsid: true, FileCapture: true,
	}}
}

// Run runs c through r and waits for it. A non-zero exit status is a
// Result, not an error; a child that could not start, or that the timeout
// ended, is an error.
func Run(ctx context.Context, r runner.Runner, c runner.Cmd) (runner.Result, error) {
	if !filepath.IsAbs(c.Path) {
		return runner.Result{ExitCode: 127}, fmt.Errorf("%q is not an absolute path", c.Path)
	}
	res, err := r.Run(ctx, c)
	if err == nil && res.TimedOut {
		err = fmt.Errorf("%s: timed out after %s", c.Path, c.Timeout)
	}
	return res, err
}

// Output is stderr, else stdout, trimmed: the text for an error message.
func Output(r runner.Result) string {
	if s := strings.TrimSpace(string(r.Stderr)); s != "" {
		return s
	}
	return strings.TrimSpace(string(r.Stdout))
}

// Line is a command line for messages and tests: the arguments are quoted
// when they hold blanks or quotes. It never holds a secret: none is ever
// in argv.
func Line(argv []string) string {
	parts := make([]string, 0, len(argv))
	for i, a := range argv {
		if i > 0 && (a == "" || strings.ContainsAny(a, " \t\n'\"\\$")) {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

// Fake is a runner.Fake for the tests of gash hte, whose starts follow
// from what it discovers rather than a fixed order: every start, Run or
// StartSession, is answered by answer.
func Fake(t runner.TB, answer func(c runner.Call) runner.Expect) *runner.Fake {
	return runner.NewFake(t).Respond(func(c runner.Call) (runner.Expect, bool) { return answer(c), true })
}

// Lines returns the command lines of calls, for tests.
func Lines(calls []runner.Call) []string {
	var out []string
	for _, c := range calls {
		out = append(out, Line(c.Argv))
	}
	return out
}
