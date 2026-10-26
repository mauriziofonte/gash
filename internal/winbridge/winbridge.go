// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package winbridge is the only place Gash talks to Windows from a WSL
// distribution. Every Windows program Gash runs (reg.exe, wevtutil.exe,
// whoami.exe, Windows PowerShell) goes through a Bridge, so that the rules
// hold in one place:
//
//   - a program is found by its absolute path under the Windows System32
//     directory, never through the PATH, so interop works with
//     [interop] appendWindowsPath=false (D0 of the plan);
//   - the system drive is the drive that holds Windows, found from the
//     mounts, not the hard-wired /mnt/c (a distribution with
//     [automount] root=/ mounts it on /c);
//   - every call has a timeout, so a broken interop (a stale socket, a
//     fragmented VM) never hangs a shell; it ends with interop_timeout;
//   - interop that the policy or wsl.conf turned off is reported as
//     interop_unavailable, not worked around;
//   - a PowerShell request runs a fixed, versioned script with only
//     read verbs; the caller's arguments reach it as data (one JSON object
//     on its standard input), never as code.
//
// The bridge reads nothing from Windows itself: it is handed the facts
// that package wsl found, and a runner that starts the processes, so the
// tests drive it with a fake runner and a fixture system root.
package winbridge

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/wsl"
)

// DefaultTimeout bounds one Windows call. The plan's 10 seconds: long
// enough for a cold PowerShell start (600–800 ms) and a real query, short
// enough that a wedged interop fails fast.
const DefaultTimeout = 10 * time.Second

// MaxTimeout caps GASH_WIN_TIMEOUT, so a typo cannot hang a shell for
// minutes.
const MaxTimeout = 120 * time.Second

// Bridge runs Windows programs for one call. The zero value is not ready;
// New builds one from the facts, the system root and a runner.
type Bridge struct {
	facts   wsl.Facts
	sys     platform.Sysroot
	run     runner.Runner
	getenv  func(string) string
	timeout time.Duration
}

// New returns a bridge over the facts wsl.Detect found, reading files
// through sys and starting processes through run. getenv reads the
// caller's environment (it may be nil).
func New(facts wsl.Facts, sys platform.Sysroot, run runner.Runner, getenv func(string) string) *Bridge {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	b := &Bridge{facts: facts, sys: sys, run: run, getenv: getenv, timeout: DefaultTimeout}
	if d := parseTimeout(getenv("GASH_WIN_TIMEOUT")); d > 0 {
		b.timeout = min(d, MaxTimeout)
	}
	return b
}

// Facts returns the WSL facts the bridge was built with.
func (b *Bridge) Facts() wsl.Facts { return b.facts }

// Timeout is the per-call timeout in force.
func (b *Bridge) Timeout() time.Duration { return b.timeout }

// parseTimeout reads a GASH_WIN_TIMEOUT value: a whole number of seconds,
// or a Go duration ("8s", "500ms"). "" or a bad value gives 0 (keep the
// default).
func parseTimeout(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return d
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return 0
}

// Available reports why the bridge cannot reach Windows, nil when it can:
// not WSL, interop turned off, or no system drive mounted.
func (b *Bridge) Available() *Error {
	if !b.facts.WSL {
		return &Error{Code: CodeUnavailable, Details: "not running under WSL", Hint: "this command needs a WSL distribution on Windows"}
	}
	if !b.facts.Interop.Usable() {
		why := "Windows interop is off"
		hint := "enable it in /etc/wsl.conf ([interop] enabled=true) or run eval \"$(gash wsl interop --export)\" in this shell"
		switch {
		case !b.facts.Interop.Registered:
			why = "the WSLInterop handler is not registered"
		case !b.facts.Interop.Enabled:
			why = "the WSLInterop handler is disabled"
		case b.facts.Interop.Stale:
			why = "WSL_INTEROP points at a socket that is gone (a reattached tmux or screen session)"
			hint = "run eval \"$(gash wsl interop --export)\" in this shell, or open a new terminal"
		case b.facts.Interop.Socket == "":
			why = "no live interop server was found in " + wsl.InteropDir
		}
		return &Error{Code: CodeUnavailable, Details: why, Hint: hint}
	}
	if _, ok := b.System32(); !ok {
		return &Error{Code: CodeUnavailable, Details: "no Windows system drive is mounted",
			Hint: "mount the drive that holds Windows, or check [automount] in /etc/wsl.conf"}
	}
	return nil
}

// Decode turns the bytes a Windows program printed into UTF-8 text with
// Unix line endings: a UTF-16 console (PowerShell without a forced UTF-8
// output encoding) or a UTF-8 BOM is decoded, and CR before LF is dropped.
func Decode(b []byte) string {
	b = wsl.DecodeText(b)
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// errFrom maps a process result and start error to a bridge error, nil
// when the program ran and exited 0.
func (b *Bridge) errFrom(what string, res runner.Result, err error) *Error {
	var se *runner.StartError
	if errors.As(err, &se) {
		return &Error{Code: CodeNotFound, Details: what + ": " + se.Error(),
			Hint: "the Windows program could not be started; check that interop works (gash wsl doctor)"}
	}
	if res.TimedOut {
		return &Error{Code: CodeTimeout, Details: what + " did not answer within " + b.timeout.String(),
			Hint: "the Windows side may be slow or wedged; try wsl_compact or gash wsl doctor"}
	}
	if err != nil {
		return &Error{Code: CodeUnavailable, Details: what + ": " + err.Error(), Hint: "check that interop works (gash wsl doctor)"}
	}
	if res.ExitCode != 0 {
		out := strings.TrimSpace(Decode(res.Stderr))
		if out == "" {
			out = strings.TrimSpace(Decode(res.Stdout))
		}
		code, hint := classify(out)
		if out == "" {
			out = what + " exited with status " + strconv.Itoa(res.ExitCode)
		}
		return &Error{Code: code, Details: out, Hint: hint}
	}
	return nil
}

// classify reads a Windows error message and returns the contract code and
// a hint. Access-denied and elevation phrases appear in English on every
// Windows; Gash matches those, and falls back to windows_access_denied for
// anything else a program printed on a non-zero exit.
func classify(msg string) (code, hint string) {
	l := strings.ToLower(msg)
	switch {
	case strings.Contains(l, "requested operation requires elevation"),
		strings.Contains(l, "run as administrator"),
		strings.Contains(l, "requires elevation"):
		return CodeElevation, "open the terminal as administrator, or run the human win fix with a UAC prompt"
	case strings.Contains(l, "access is denied"), strings.Contains(l, "access denied"),
		strings.Contains(l, "permission"):
		return CodeAccessDenied, "the Windows user cannot read this; it may need an elevated terminal"
	}
	return CodeAccessDenied, "the Windows program reported an error; see the details"
}
