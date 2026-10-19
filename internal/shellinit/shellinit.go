// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package shellinit builds the interactive Bash layer that `gash init bash`
// prints and gash.sh evaluates once per shell start (PRD 5.4, design
// shell.md): the settings assignments, terminal, history, prompt,
// toolchains, alias packs, banner, SSH auto-unlock and the 1.x migration
// reminder.
//
// Everything the layer must do to the calling shell stays Bash. The binary
// computes every machine-dependent decision here (which programs exist, the
// probes, the settings, the banner text, the lesspipe and editor paths, the
// nvm default version) so the emitted script makes no fork of its own, and
// quotes every value from the machine or the user with mvdan.cc/sh/v3/syntax
// as a single word that Bash never re-parses.
//
// One divergence is forced, SHELL-01: the prompt hook stores the git branch,
// VIRTUAL_ENV basename, debian_chroot and screen session name in variables
// and PS1 references them, so no VCS-derived text is ever executed
// (prompt.go).
package shellinit

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"

	"mvdan.cc/sh/v3/syntax"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
)

// Emitter builds the interactive layer. Its seams let tests drive the
// probes, the clock and the file reads without the real system.
type Emitter struct {
	run       runner.Runner
	lookPath  func(name string, env []string, dir string) (string, error)
	readFile  func(string) ([]byte, error)
	stat      func(string) (os.FileInfo, error)
	getwd     func() (string, error)
	mkdirAll  func(string, os.FileMode) error
	writeFile func(string, []byte, os.FileMode) error
	now       func() int64
	// goos is the target OS for the platform-specific alias packs (the ls
	// color flag, the macOS LSCOLORS export); runtime.GOOS in production,
	// a fixed value in tests so the generated layer does not depend on the
	// host the suite runs on.
	goos string
}

// New returns an Emitter wired to the real system.
func New() *Emitter {
	return &Emitter{
		run:       runner.Exec{},
		lookPath:  runner.LookPath,
		readFile:  os.ReadFile,
		stat:      os.Stat,
		getwd:     os.Getwd,
		mkdirAll:  os.MkdirAll,
		writeFile: os.WriteFile,
		now:       func() int64 { return time.Now().Unix() },
		goos:      runtime.GOOS,
	}
}

// Emit returns the Bash layer for the call's shell. The call's environment
// holds the caller's shell state (the GASH__ protocol, the settings passed
// by the compat stub, PATH, HOME, EDITOR...) and its Term already reflects
// the shell's stdout (GASH__TTY_OUT), so the banner colors as the shell
// would.
func (e *Emitter) Emit(c *invoke.Call) (string, error) {
	file := config.LoadCaller(c.Env.Get)
	caller := config.CallerFromEnviron(c.Env.Environ())
	// gash init bash runs only in an interactive shell (gash.sh returns
	// before it in headless mode), so the settings resolve with the
	// interactive rule.
	s := config.NewSettings(file, caller, false)

	st := &state{
		e:       e,
		c:       c,
		file:    file,
		s:       s,
		home:    c.Env.Get("HOME"),
		effPath: "",
	}
	st.effPath = st.effectivePath()

	var b strings.Builder
	st.settings(&b)
	st.timed(&b, "lib/shell/terminal.sh", st.terminal)
	st.timed(&b, "lib/shell/history.sh", func(x *strings.Builder) { x.WriteString(historyBlock); x.WriteByte('\n') })
	st.timed(&b, "lib/shell/prompt.sh", func(x *strings.Builder) { x.WriteString(promptBlock); x.WriteByte('\n') })
	st.timed(&b, "lib/shell/toolchains.sh", st.toolchains)
	st.aliasPacks(&b)
	st.timed(&b, "lib/shell/banner.sh", st.banner)
	st.migrateReminder(&b)
	// Keep the eval's status 0 for a set -e caller.
	b.WriteString("true\n")
	// The timing scratch variable never outlives the layer.
	b.WriteString("unset __gash_init_t 2>/dev/null || true\n")
	return b.String(), nil
}

// state carries the resolved inputs through the section builders.
type state struct {
	e       *Emitter
	c       *invoke.Call
	file    *config.File
	s       *config.Settings
	home    string
	effPath string // PATH after GASH_PATH_ADD and GASH_NVM=lazy, for the probes
}

func (st *state) ctx() context.Context {
	if st.c.Ctx != nil {
		return st.c.Ctx
	}
	return context.Background()
}

// settings emits the known GASH_* settings of the file as guarded
// assignments ([[ -v NAME ]] || NAME='value'), reproducing
// __gash_apply_settings: the environment (already set in the shell) wins,
// the first file line of a name wins, and the values are plain shell
// variables, not exported.
func (st *state) settings(b *strings.Builder) {
	seen := map[string]bool{}
	for _, s := range st.file.Settings {
		if seen[s.Name] {
			continue
		}
		seen[s.Name] = true
		b.WriteString("[[ -v " + s.Name + " ]] || " + s.Name + "=" + quote(s.Value) + "\n")
	}
}

// timed wraps a section with the GASH_TIMING records 2.x kept per file, so
// __GASH_LOAD_TIMES holds a "DURATION lib/shell/prompt.sh" line as before
// (startup_spec.sh). The scratch variable is unset at the end of Emit.
func (st *state) timed(b *strings.Builder, label string, body func(*strings.Builder)) {
	b.WriteString(`((__GASH_TIMING)) && __gash_init_t="$EPOCHREALTIME"` + "\n")
	body(b)
	b.WriteString(`((__GASH_TIMING)) && __gash_time_record ` + quote(label) + ` "$__gash_init_t"` + "\n")
}

// quote renders s as one Bash word that expands to exactly s.
func quote(s string) string {
	q, err := syntax.Quote(s, syntax.LangBash)
	if err != nil {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return q
}

// program reports whether name resolves to a program on the effective PATH
// (type -P NAME), and its resolved path.
func (st *state) program(name string) (string, bool) {
	p, err := st.e.lookPath(name, []string{"PATH=" + st.effPath}, "")
	if err != nil {
		return "", false
	}
	return p, true
}

// has reports whether name is a program on the effective PATH.
func (st *state) has(name string) bool {
	_, ok := st.program(name)
	return ok
}

// probeEnv is the environment of a probe the binary runs for the shell: the
// caller's child environment with the effective PATH.
func (st *state) probeEnv() []string {
	out := []string{}
	for _, kv := range st.c.Env.Child() {
		if strings.HasPrefix(kv, "PATH=") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "PATH="+st.effPath)
}

// runOK runs name with args and reports a zero exit (the shell probes that
// test success, e.g. `ls --color=auto .`).
func (st *state) runOK(name string, args ...string) bool {
	r, err := st.e.run.Run(st.ctx(), runner.Cmd{
		Path: name, Args: args,
		Options: runner.Options{Env: st.probeEnv(), Stdout: discard{}, Stderr: discard{}},
	})
	return err == nil && r.ExitCode == 0
}

// runOut runs name with args and returns its captured stdout (the shell
// probes that read output, e.g. `rm --version`). Empty on any failure.
func (st *state) runOut(name string, args ...string) string {
	r, err := st.e.run.Run(st.ctx(), runner.Cmd{
		Path: name, Args: args,
		Options: runner.Options{Env: st.probeEnv()},
	})
	if err != nil {
		return ""
	}
	return string(r.Stdout)
}

// readable reports whether path is a readable regular file ([[ -r FILE ]]
// for a file the binary reads).
func (st *state) readable(path string) bool {
	fi, err := st.e.stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// isDir reports whether path is a directory ([[ -d DIR ]]).
func (st *state) isDir(path string) bool {
	fi, err := st.e.stat(path)
	return err == nil && fi.IsDir()
}

// discard is an io.Writer that drops everything (the probes' /dev/null).
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
