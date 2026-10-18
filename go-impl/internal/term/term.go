// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package term is the one place that decides whether Gash prints ANSI
// colors, and which glyph set it uses.
//
// It reproduces the policy of the 2.x lib/core/output.sh header:
//
//   - Load-time gate: GASH_HEADLESS=1, a non-empty NO_COLOR, or
//     GASH_NO_COLOR non-empty and not 0 zero every color variable
//     (Palette returns empty strings then).
//   - Runtime gate: the emitters color only when the environment allows it
//     and stdout is a terminal, even for text written to stderr (UseColor).
//   - Explicit flag: --no-color forces colors off for one call
//     (WantColors).
//
// Stream policy: data on stdout, every diagnostic, prompt and progress line
// on stderr.
package term

import (
	"os"
	"strings"

	xterm "golang.org/x/term"
)

// Env is a read-only view of the caller's variables (the environment the
// shim passed, which holds the caller's shell variables too).
type Env interface {
	Lookup(name string) (string, bool)
}

// get returns the value of name, or "" when unset.
func get(env Env, name string) string {
	if env == nil {
		return ""
	}
	v, _ := env.Lookup(name)
	return v
}

// ColorEnvOK reports whether the environment permits colors
// (__gash_color_env_ok): false with GASH_HEADLESS=1, a non-empty NO_COLOR,
// or GASH_NO_COLOR non-empty and other than 0.
func ColorEnvOK(env Env) bool {
	if get(env, "GASH_HEADLESS") == "1" {
		return false
	}
	if get(env, "NO_COLOR") != "" {
		return false
	}
	if v := get(env, "GASH_NO_COLOR"); v != "" && v != "0" {
		return false
	}
	return true
}

// LocaleUTF8 reports whether the locale is UTF-8 (__gash_locale_utf8): the
// first non-empty of LC_ALL, LC_CTYPE and LANG contains UTF-8 or UTF8, in
// any case.
func LocaleUTF8(env Env) bool {
	loc := get(env, "LC_ALL")
	if loc == "" {
		loc = get(env, "LC_CTYPE")
	}
	if loc == "" {
		loc = get(env, "LANG")
	}
	l := strings.ToUpper(loc)
	return strings.Contains(l, "UTF-8") || strings.Contains(l, "UTF8")
}

// Term holds the terminal facts of one call: the caller's environment and
// whether each standard stream is a terminal.
type Term struct {
	env       Env
	stdinTTY  bool
	stdoutTTY bool
	stderrTTY bool
	// The standard streams, for the window size and the key reader; nil in
	// a Fixed term.
	stdin, stdout, stderr *os.File
	// size answers the window size question in tests (Fixed terms).
	size int
	// keys is the key source of a Fixed term (WithKeys).
	keys []byte
	// color is the caller shell's __gash_use_color answer (GASH__COLOR),
	// 0 when not given.
	color int8
	// tty is the controlling terminal of a test term (Fixed, WithTTY);
	// nil opens /dev/tty.
	tty *fakeTTY
}

// New returns the Term of a process whose standard streams are the given
// files (nil means not a terminal). GASH__TTY_OUT, set by the shim when it
// captures the binary's stdout (gash init bash), overrides the stdout test
// with the answer the caller's shell gave ("1" or "0"); GASH__TTY_IN does
// the same for stdin when the shim pipes data into the binary
// (__gash_box). GASH__COLOR ("1" or "0") is the answer the caller's
// __gash_use_color gave, for a compat operation whose streams the shim
// redirected: UseColor returns it.
func New(env Env, stdin, stdout, stderr *os.File) *Term {
	t := &Term{
		env:       env,
		stdinTTY:  isTTY(stdin),
		stdoutTTY: isTTY(stdout),
		stderrTTY: isTTY(stderr),
		stdin:     stdin,
		stdout:    stdout,
		stderr:    stderr,
	}
	if v, ok := lookup(env, "GASH__TTY_OUT"); ok && (v == "0" || v == "1") {
		t.stdoutTTY = v == "1"
	}
	if v, ok := lookup(env, "GASH__TTY_IN"); ok && (v == "0" || v == "1") {
		t.stdinTTY = v == "1"
	}
	switch v, _ := lookup(env, "GASH__COLOR"); v {
	case "1":
		t.color = 1
	case "0":
		t.color = -1
	}
	return t
}

// Fixed returns a Term with the given answers, for tests.
func Fixed(env Env, stdin, stdout, stderr bool) *Term {
	return &Term{env: env, stdinTTY: stdin, stdoutTTY: stdout, stderrTTY: stderr, tty: &fakeTTY{}}
}

func lookup(env Env, name string) (string, bool) {
	if env == nil {
		return "", false
	}
	return env.Lookup(name)
}

func isTTY(f *os.File) bool {
	return f != nil && xterm.IsTerminal(int(f.Fd()))
}

// Getenv returns the caller's value of name, or "" when unset.
func (t *Term) Getenv(name string) string { return get(t.env, name) }

// StdinTTY reports [[ -t 0 ]].
func (t *Term) StdinTTY() bool { return t.stdinTTY }

// StdoutTTY reports [[ -t 1 ]].
func (t *Term) StdoutTTY() bool { return t.stdoutTTY }

// StderrTTY reports [[ -t 2 ]].
func (t *Term) StderrTTY() bool { return t.stderrTTY }

// Headless reports GASH_HEADLESS=1.
func (t *Term) Headless() bool { return get(t.env, "GASH_HEADLESS") == "1" }

// ColorEnvOK reports whether the environment permits colors.
func (t *Term) ColorEnvOK() bool { return ColorEnvOK(t.env) }

// UseColor is the runtime gate of the emitters (__gash_use_color): the
// environment permits colors and stdout is a terminal.
func (t *Term) UseColor() bool {
	if t.color != 0 {
		return t.color > 0
	}
	return t.ColorEnvOK() && t.stdoutTTY
}

// WantColors is the gate of functions that take --no-color
// (__gash_want_colors): off when the flag is given, else UseColor.
func (t *Term) WantColors(noColor bool) bool { return !noColor && t.UseColor() }

// StderrFancy reports whether stderr may carry escape sequences
// (__gash_stderr_fancy): stderr is a terminal and UseColor.
func (t *Term) StderrFancy() bool { return t.stderrTTY && t.UseColor() }

// TTYIO reports whether stdin and stdout are both terminals
// (__gash_tty_io).
func (t *Term) TTYIO() bool { return t.stdinTTY && t.stdoutTTY }

// Palette returns the color variables as 2.x sets them at load time: the
// real sequences when the environment permits colors, empty strings
// otherwise. Call sites that 2.x gated at runtime check UseColor first.
func (t *Term) Palette() Palette {
	if t.ColorEnvOK() {
		return Colors
	}
	return NoColors
}

// PaletteIf returns Colors when on is true, NoColors otherwise (the
// __GASH_COLOR_OFF_LOCALS pattern of 2.x).
func PaletteIf(on bool) Palette {
	if on {
		return Colors
	}
	return NoColors
}

// Glyphs returns the glyph set of the caller's locale.
func (t *Term) Glyphs() Glyphs {
	if LocaleUTF8(t.env) {
		return UnicodeGlyphs
	}
	return ASCIIGlyphs
}
