// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package winbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/mauriziofonte/gash/internal/runner"
)

// System32 returns the Linux path of the Windows System32 directory of the
// system drive, and whether it was found. It never assumes /mnt/c: the
// system drive is the mounted drive that holds Windows (facts.SystemDrive).
func (b *Bridge) System32() (string, bool) {
	d, ok := b.facts.SystemDrive(b.sys.Exists)
	if !ok {
		return "", false
	}
	return d.Mount + "/Windows/System32", true
}

// Exe returns the Linux path of a System32 program and whether the file is
// there. name is a bare file name under System32 ("reg.exe"), or a path
// relative to it ("WindowsPowerShell/v1.0/powershell.exe").
func (b *Bridge) Exe(name string) (string, bool) {
	root, ok := b.System32()
	if !ok {
		return "", false
	}
	p := root + "/" + name
	return p, b.sys.IsRegular(p)
}

// powerShellRel is Windows PowerShell under System32. It is on every
// Windows; pwsh (PowerShell 7) is not, so the bridge does not depend on it.
const powerShellRel = "WindowsPowerShell/v1.0/powershell.exe"

// Native runs a System32 program with an explicit argv and returns its
// standard output. exe is a System32 file name ("reg.exe"); args reach the
// program as separate arguments, never through a shell. The call is bound
// by the bridge timeout.
func (b *Bridge) Native(ctx context.Context, exe string, args ...string) ([]byte, *Error) {
	if e := b.Available(); e != nil {
		return nil, e
	}
	path, ok := b.Exe(exe)
	if !ok {
		return nil, &Error{Code: CodeNotFound, Details: exe + " is not in the Windows System32 directory",
			Hint: "this Windows may not have the program, or the system drive is not fully mounted"}
	}
	var out, errb bytes.Buffer
	res, err := b.run.Run(ctx, runner.Cmd{
		Path: b.sys.Path(path), Args: args,
		Options: runner.Options{
			Env: b.childEnv(), Stdin: nil, Stdout: &out, Stderr: &errb,
			Timeout: b.timeout,
		},
	})
	res.Stdout, res.Stderr = out.Bytes(), errb.Bytes()
	if e := b.errFrom(exe, res, err); e != nil {
		return nil, e
	}
	return out.Bytes(), nil
}

// PowerShell runs a registered read-only script and returns its standard
// output (the script's ConvertTo-Json line). The script's arguments are
// passed as one JSON object on standard input, which the script reads as
// data; no argument is ever part of the script text.
func (b *Bridge) PowerShell(ctx context.Context, s Script, args map[string]string) ([]byte, *Error) {
	if e := b.Available(); e != nil {
		return nil, e
	}
	path, ok := b.Exe(powerShellRel)
	if !ok {
		return nil, &Error{Code: CodeNotFound, Details: "Windows PowerShell is not in System32",
			Hint: "the system drive may not be fully mounted"}
	}
	payload, _ := json.Marshal(argsObject(args))
	var out, errb bytes.Buffer
	res, err := b.run.Run(ctx, runner.Cmd{
		Path: b.sys.Path(path),
		Args: []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
			"-OutputFormat", "Text", "-Command", scriptText(s)},
		Options: runner.Options{
			Env: b.childEnv(), Stdin: bytes.NewReader(payload),
			Stdout: &out, Stderr: &errb, Timeout: b.timeout,
		},
	})
	res.Stdout, res.Stderr = out.Bytes(), errb.Bytes()
	if e := b.errFrom("powershell "+s.Name, res, err); e != nil {
		return nil, e
	}
	return out.Bytes(), nil
}

// DecodeJSON runs a PowerShell script and decodes its JSON into v.
func (b *Bridge) DecodeJSON(ctx context.Context, s Script, args map[string]string, v any) *Error {
	raw, e := b.PowerShell(ctx, s, args)
	if e != nil {
		return e
	}
	text := strings.TrimSpace(Decode(raw))
	if text == "" || text == "null" {
		return nil // an empty result is not an error; v keeps its zero value
	}
	if err := json.Unmarshal([]byte(text), v); err != nil {
		return &Error{Code: CodeUnavailable, Details: "could not read the Windows answer: " + err.Error(),
			Hint: "the PowerShell output was not the expected JSON; run gash wsl doctor"}
	}
	return nil
}

// childEnv is the environment of a Windows program: the caller's, so that
// interop finds its socket (WSL_INTEROP) and the usual variables. The
// sandbox of llm_exec is the one place WSL_INTEROP is stripped; the bridge
// is not reached from there.
func (b *Bridge) childEnv() []string {
	var env []string
	for _, name := range passEnv {
		if v := b.getenv(name); v != "" {
			env = append(env, name+"="+v)
		}
	}
	return env
}

// passEnv are the variables a Windows program needs from the caller.
// WSL_INTEROP selects the interop server; PATH lets the kernel find /init;
// the rest are harmless and expected.
var passEnv = []string{"WSL_INTEROP", "WSL_DISTRO_NAME", "PATH", "HOME", "USER", "LANG", "WSLENV"}

// argsObject keeps nil out of the JSON (an empty object, never "null").
func argsObject(args map[string]string) map[string]string {
	if args == nil {
		return map[string]string{}
	}
	return args
}

// ReadWindowsFile reads a file on a mounted Windows drive through the
// system root (e.g. the hosts file). It is a plain file read, no interop.
func (b *Bridge) ReadWindowsFile(linuxPath string) ([]byte, error) {
	return b.sys.ReadFile(linuxPath)
}

// RunElevated launches a Windows PowerShell that requests elevation for the
// given command through UAC (Start-Process -Verb RunAs), so a human at the
// desktop sees and authorizes it. command must be built from validated
// data, never raw user input. It is the one write path of the bridge and
// is never reached headless. There is no timeout: the user may take time at
// the UAC prompt. It reports whether the elevated process ran and exited 0
// (a cancelled UAC prompt is a non-zero exit).
func (b *Bridge) RunElevated(ctx context.Context, command string) *Error {
	if e := b.Available(); e != nil {
		return e
	}
	path, ok := b.Exe(powerShellRel)
	if !ok {
		return &Error{Code: CodeNotFound, Details: "Windows PowerShell is not in System32", Hint: "the system drive may not be fully mounted"}
	}
	// The outer (non-elevated) shell spawns the elevated child and waits
	// for it; -Wait makes a cancelled UAC prompt a non-zero exit.
	inner := "'-NoProfile','-NonInteractive','-Command'," + psQuote(command)
	outer := "Start-Process -FilePath powershell.exe -Verb RunAs -Wait -ArgumentList " + inner
	var errb bytes.Buffer
	res, err := b.run.Run(ctx, runner.Cmd{
		Path:    b.sys.Path(path),
		Args:    []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", outer},
		Options: runner.Options{Env: b.childEnv(), Stderr: &errb},
	})
	res.Stderr = errb.Bytes()
	var se *runner.StartError
	if errors.As(err, &se) {
		return &Error{Code: CodeNotFound, Details: se.Error(), Hint: "check that interop works (gash wsl doctor)"}
	}
	if err != nil || res.ExitCode != 0 {
		return &Error{Code: CodeElevation, Details: "the elevated action did not complete (the UAC prompt may have been declined)",
			Hint: "approve the Windows prompt when it appears"}
	}
	return nil
}

// psQuote wraps s as a single-quoted PowerShell string (doubling inner
// single quotes), so it is data to the launched shell, never code.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
