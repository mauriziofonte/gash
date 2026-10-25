// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runner

import (
	"errors"
	"fmt"
)

// BashUse names one of the sanctioned uses of bash -c.
type BashUse int

const (
	// BashLLMExec is llm_exec's child: the canonical script that
	// internal/llmexec rendered from its own parse of the line (every word
	// single-quoted), or the raw line in the user's file-only unrestricted
	// mode, after its prelude.
	BashLLMExec BashUse = iota + 1
	// BashNVMBridge runs nvm, a shell function, with the fixed script
	// NVMBridgeScript; the nvm arguments are positional parameters, never
	// part of the script.
	BashNVMBridge
	// BashDoctorLoad is the load probe of gash_doctor --timing: a new
	// interactive bash --noprofile --norc runs the fixed script
	// DoctorLoadScript, which sources the gash.sh and bash_completion of
	// the install directory (its one argument, $1; $0 is "gash-timing")
	// and prints the load times the files recorded (GASH_TIMING=1 in Env).
	BashDoctorLoad
	// BashDoctorShell is the whole-shell timing of gash_doctor --timing:
	// `bash -i -c exit`, a new interactive shell with the user's startup
	// files (~/.bashrc runs, as in 2.x: LEDGER SELF-15).
	BashDoctorShell
	// BashDoctorBare is the same timing of bash alone: `bash --noprofile
	// --norc -i -c exit`.
	BashDoctorBare
	// BashUpdateSmoke is the headless smoke test of gash update (upgrade.md
	// 3.6): bash --noprofile --norc runs the fixed script UpdateSmokeScript,
	// which sources the gash.sh of the install directory (its one argument,
	// $1; $0 is "gash-update") with GASH_HEADLESS=1 in Env, then calls one
	// function. The NUL-separated fields it prints follow what the load
	// printed.
	BashUpdateSmoke
	// BashUpdateSmokeInteractive is the interactive smoke test of gash
	// update: bash --noprofile --norc -i runs UpdateSmokeInteractiveScript,
	// which sources the gash.sh of the install directory ($1).
	BashUpdateSmokeInteractive
	// BashUserCommand runs the caller's own command line of gash run and
	// llm_test when it is a shell with a command string (bash -c '...'),
	// which Run refuses: the fixed script UserCommandScript execs the
	// argv, passed as the positional parameters, never as script text
	// (RunUser).
	BashUserCommand
)

func (u BashUse) String() string {
	switch u {
	case BashLLMExec:
		return "llm_exec"
	case BashNVMBridge:
		return "nvm bridge"
	case BashDoctorLoad:
		return "doctor load"
	case BashDoctorShell:
		return "doctor shell"
	case BashDoctorBare:
		return "doctor bare"
	case BashUpdateSmoke:
		return "update smoke"
	case BashUpdateSmokeInteractive:
		return "update smoke interactive"
	case BashUserCommand:
		return "user command"
	}
	return fmt.Sprintf("BashUse(%d)", int(u))
}

// DoctorLoadScript is the whole script of BashDoctorLoad, the 2.x probe
// with its redirections (3>&1 >/dev/null 2>&1) moved inside: the load
// times go to the probe's stdout, everything the files print is dropped.
const DoctorLoadScript = `exec 3>&1 >/dev/null 2>&1
source "$1/gash.sh"
[[ -f "$1/bash_completion" ]] && source "$1/bash_completion"
printf "%s\n" "${__GASH_LOAD_TIMES[@]}" >&3`

// UpdateSmokeScript is the whole script of BashUpdateSmoke: whatever the
// load prints comes first, then NUL, the status of the source, NUL,
// $GASH_VERSION, NUL, the output of gash_username, NUL, its status, NUL.
const UpdateSmokeScript = `source "$1/gash.sh"
printf '\0%s\0%s\0' "$?" "${GASH_VERSION-}"
gash_username
printf '\0%s\0' "$?"`

// UpdateSmokeInteractiveScript is the whole script of
// BashUpdateSmokeInteractive.
const UpdateSmokeInteractiveScript = `source "$1/gash.sh"`

// UserCommandScript is the whole script of BashUserCommand: exec the
// positional parameters.
const UserCommandScript = `exec "$@"`

// doctorExit is the script of the whole-shell timings.
const doctorExit = "exit"

// NVMBridgeScript is the whole script of BashNVMBridge: it loads nvm the
// way the 2.x lazy stub does (nvm.sh --no-use, so PATH keeps the version
// already chosen) and runs nvm with the bridge's arguments.
const NVMBridgeScript = `source "$NVM_DIR/nvm.sh" --no-use || exit 1
nvm "$@"`

// Bash is a run of bash --noprofile --norc -c SCRIPT [NAME ARGS...]; the
// doctor uses add -i, and BashDoctorShell runs `bash -i -c exit`.
type Bash struct {
	Use BashUse
	// Wrap is an argv prefix the shell runs under (llm_exec's bwrap
	// sandbox); it may not be a shell with a command string itself.
	// BashNVMBridge takes none.
	Wrap []string
	// Bash is the bash program, looked up as Cmd.Path is; "" means "bash".
	Bash string
	// Script is the script: required for BashLLMExec, empty (or exactly
	// the fixed script of the use) for the others.
	Script string
	// Name is $0 and Args are $1...; with both empty no positional
	// parameters follow the script. BashNVMBridge always passes them, with
	// Name "nvm" when empty.
	Name string
	Args []string
	Options
}

// ErrBashUse is returned by RunBash for a Bash that does not match its use.
var ErrBashUse = errors.New("runner: not a sanctioned bash -c use")

// argv builds the command line of b, after checking it against its use.
func (b Bash) argv() ([]string, error) {
	script := b.Script
	name, args := b.Name, b.Args
	switch b.Use {
	case BashLLMExec:
		if script == "" {
			return nil, fmt.Errorf("%w: %s needs its script", ErrBashUse, b.Use)
		}
	case BashNVMBridge:
		if script != "" && script != NVMBridgeScript {
			return nil, fmt.Errorf("%w: %s runs only NVMBridgeScript", ErrBashUse, b.Use)
		}
		if len(b.Wrap) > 0 {
			return nil, fmt.Errorf("%w: %s takes no wrapper", ErrBashUse, b.Use)
		}
		script = NVMBridgeScript
		if name == "" {
			name = "nvm"
		}
	case BashDoctorLoad, BashDoctorShell, BashDoctorBare:
		want := doctorExit
		if b.Use == BashDoctorLoad {
			want = DoctorLoadScript
		}
		if script != "" && script != want {
			return nil, fmt.Errorf("%w: %s runs only its fixed script", ErrBashUse, b.Use)
		}
		if len(b.Wrap) > 0 {
			return nil, fmt.Errorf("%w: %s takes no wrapper", ErrBashUse, b.Use)
		}
		if b.Use == BashDoctorLoad && (len(args) != 1 || (name != "" && name != "gash-timing")) {
			return nil, fmt.Errorf("%w: %s takes the install directory only", ErrBashUse, b.Use)
		}
		if b.Use != BashDoctorLoad && (name != "" || len(args) > 0) {
			return nil, fmt.Errorf("%w: %s takes no arguments", ErrBashUse, b.Use)
		}
		script = want
		if b.Use == BashDoctorLoad {
			name = "gash-timing"
		}
	case BashUpdateSmoke, BashUpdateSmokeInteractive:
		want := UpdateSmokeScript
		if b.Use == BashUpdateSmokeInteractive {
			want = UpdateSmokeInteractiveScript
		}
		if script != "" && script != want {
			return nil, fmt.Errorf("%w: %s runs only its fixed script", ErrBashUse, b.Use)
		}
		if len(b.Wrap) > 0 {
			return nil, fmt.Errorf("%w: %s takes no wrapper", ErrBashUse, b.Use)
		}
		if len(args) != 1 || (name != "" && name != "gash-update") {
			return nil, fmt.Errorf("%w: %s takes the install directory only", ErrBashUse, b.Use)
		}
		script, name = want, "gash-update"
	case BashUserCommand:
		if script != "" && script != UserCommandScript {
			return nil, fmt.Errorf("%w: %s runs only UserCommandScript", ErrBashUse, b.Use)
		}
		if len(b.Wrap) > 0 {
			return nil, fmt.Errorf("%w: %s takes no wrapper", ErrBashUse, b.Use)
		}
		if len(args) == 0 {
			return nil, fmt.Errorf("%w: %s needs the command", ErrBashUse, b.Use)
		}
		script, name = UserCommandScript, "gash-run"
	default:
		return nil, fmt.Errorf("%w: %s", ErrBashUse, b.Use)
	}
	if err := checkNoShell(b.Wrap); err != nil {
		return nil, err
	}
	bash := b.Bash
	if bash == "" {
		bash = "bash"
	}
	flags := []string{"--noprofile", "--norc", "-c"}
	switch b.Use {
	case BashDoctorLoad, BashDoctorBare, BashUpdateSmokeInteractive:
		flags = []string{"--noprofile", "--norc", "-i", "-c"}
	case BashDoctorShell:
		flags = []string{"-i", "-c"}
	}
	argv := append(append(append([]string{}, b.Wrap...), bash), flags...)
	argv = append(argv, script)
	if name != "" || len(args) > 0 {
		if name == "" {
			name = "bash"
		}
		argv = append(append(argv, name), args...)
	}
	return argv, nil
}
