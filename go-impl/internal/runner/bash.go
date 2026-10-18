// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runner

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// BashUse names one of the two sanctioned uses of bash -c.
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
)

func (u BashUse) String() string {
	switch u {
	case BashLLMExec:
		return "llm_exec"
	case BashNVMBridge:
		return "nvm bridge"
	}
	return fmt.Sprintf("BashUse(%d)", int(u))
}

// NVMBridgeScript is the whole script of BashNVMBridge: it loads nvm the
// way the 2.x lazy stub does (nvm.sh --no-use, so PATH keeps the version
// already chosen) and runs nvm with the bridge's arguments.
const NVMBridgeScript = `source "$NVM_DIR/nvm.sh" --no-use || exit 1
nvm "$@"`

// Bash is a run of bash --noprofile --norc -c SCRIPT [NAME ARGS...].
type Bash struct {
	Use BashUse
	// Wrap is an argv prefix the shell runs under (llm_exec's bwrap
	// sandbox); it may not be a shell with a command string itself.
	// BashNVMBridge takes none.
	Wrap []string
	// Bash is the bash program, looked up as Cmd.Path is; "" means "bash".
	Bash string
	// Script is the script: required for BashLLMExec, empty (or exactly
	// NVMBridgeScript) for BashNVMBridge.
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
	argv := append(append([]string{}, b.Wrap...), bash, "--noprofile", "--norc", "-c", script)
	if name != "" || len(args) > 0 {
		if name == "" {
			name = "bash"
		}
		argv = append(append(argv, name), args...)
	}
	return argv, nil
}

// shells are the program names checkNoShell treats as command shells.
var shells = map[string]bool{
	"sh": true, "bash": true, "dash": true, "ash": true, "zsh": true, "ksh": true, "ksh93": true,
	"mksh": true, "pdksh": true, "yash": true, "posh": true, "fish": true, "csh": true, "tcsh": true,
}

// checkNoShell refuses an argv that starts a shell with a command string
// anywhere in it, also behind a wrapper (sudo sh -c, env bash -c, nice
// dash -ec, busybox sh -c), and su -c. It is a tripwire against a shell
// string reaching Run, not a sandbox: what a program does with its
// arguments is its own business.
func checkNoShell(argv []string) error {
	for i, tok := range argv {
		base := filepath.Base(tok)
		if base == "su" && hasCommandOption(argv[i+1:], false) {
			return fmt.Errorf("%w: %s -c", ErrShellCommand, base)
		}
		if shells[base] && hasCommandOption(argv[i+1:], true) {
			return fmt.Errorf("%w: %s -c", ErrShellCommand, base)
		}
	}
	return nil
}

// hasCommandOption reports a -c (or --command) among the options that
// follow a shell (or su) name, up to the first operand.
func hasCommandOption(args []string, shell bool) bool {
	for j := 0; j < len(args); j++ {
		a := args[j]
		switch {
		case a == "--":
			return false
		case a == "--command" || strings.HasPrefix(a, "--command="):
			return true
		case strings.HasPrefix(a, "--"):
			if shell && (a == "--rcfile" || a == "--init-file") {
				j++
			}
			if !shell && !strings.Contains(a, "=") && suLongWithValue[a] {
				j++
			}
		case len(a) > 1 && (a[0] == '-' || (shell && a[0] == '+')):
			for k := 1; k < len(a); k++ {
				switch a[k] {
				case 'c':
					if a[0] == '-' {
						return true
					}
				case 'o', 'O':
					if shell {
						j++ // -o OPTION, -O SHOPT
					}
				case 'g', 'G', 's', 'w':
					if !shell {
						j++ // su -g GROUP, -s SHELL, ...
					}
				}
			}
		default:
			if shell {
				return false // the script file or a non-option: no command string
			}
		}
	}
	return false
}

// suLongWithValue are su's long options that take a separate value.
var suLongWithValue = map[string]bool{"--group": true, "--supp-group": true, "--shell": true, "--whitelist-environment": true}
