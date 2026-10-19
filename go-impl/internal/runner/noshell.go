// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runner

import (
	"fmt"
	"path/filepath"
	"strings"
)

// shells are the program names checkNoShell treats as command shells.
var shells = map[string]bool{
	"sh": true, "bash": true, "dash": true, "ash": true, "zsh": true, "ksh": true, "ksh93": true,
	"mksh": true, "pdksh": true, "yash": true, "posh": true, "fish": true, "csh": true, "tcsh": true,
}

// checkNoShell refuses an argv whose program is a shell with a command
// string (sh -c, bash -ec, fish --command), also behind the wrappers that
// run their operand as a program (sudo sh -c, env -i bash -c, nice dash
// -c, busybox sh -c, timeout 5 bash -c, xargs sh -c), and su -c. Only the program and
// the programs those wrappers start count: an operand of any other program
// is that program's data (git tag -a bash -m -c makes a tag named bash).
// It is a tripwire against a shell string reaching Run, not a sandbox:
// what a program does with its arguments is its own business.
func checkNoShell(argv []string) error {
	for len(argv) > 0 {
		base := filepath.Base(argv[0])
		args := argv[1:]
		switch {
		case base == "su":
			if hasCommandOption(args, false) {
				return fmt.Errorf("%w: %s -c", ErrShellCommand, base)
			}
			return nil
		case shells[base]:
			if hasCommandOption(args, true) {
				return fmt.Errorf("%w: %s -c", ErrShellCommand, base)
			}
			return nil
		}
		next, ok := wrapped(base, args)
		if !ok {
			return nil
		}
		argv = next
	}
	return nil
}

// wrapperOptions are, for each wrapper that runs its operand as a
// program, the options that take a separate value (short letters, long
// names) and the leading operands that come before the program.
var wrapperOptions = map[string]struct {
	short    string
	long     []string
	operands int
	assigns  bool // NAME=VALUE words before the program
}{
	"sudo":    {short: "CDghpRrTtUu", long: []string{"--close-from", "--chdir", "--group", "--host", "--prompt", "--chroot", "--role", "--command-timeout", "--type", "--other-user", "--user"}, assigns: true},
	"doas":    {short: "Cu"},
	"env":     {short: "CSu", long: []string{"--chdir", "--split-string", "--unset"}, assigns: true},
	"nice":    {short: "n", long: []string{"--adjustment"}},
	"nohup":   {},
	"setsid":  {},
	"stdbuf":  {short: "eio", long: []string{"--error", "--input", "--output"}},
	"ionice":  {short: "cnpPu", long: []string{"--class", "--classdata", "--pid", "--pgid", "--uid"}},
	"timeout": {short: "ks", long: []string{"--kill-after", "--signal"}, operands: 1},
	"chroot":  {operands: 1},
	"xargs": {short: "adEILnPs", long: []string{"--arg-file", "--delimiter", "--max-args", "--max-chars",
		"--max-lines", "--max-procs", "--process-slot-var"}},
}

// wrapped is the argv a wrapper runs (busybox APPLET, or the program after
// the wrapper's options and leading operands); false when base is no
// wrapper or runs no program. env -S STRING splits STRING into the start
// of that argv.
func wrapped(base string, args []string) ([]string, bool) {
	if base == "busybox" {
		if len(args) == 0 || strings.HasPrefix(args[0], "-") {
			return nil, false
		}
		return args, true
	}
	w, ok := wrapperOptions[base]
	if !ok {
		return nil, false
	}
	var split []string
	i := 0
options:
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			i++
			break options
		case strings.HasPrefix(a, "--"):
			name, value, hasValue := strings.Cut(a, "=")
			takes := false
			for _, l := range w.long {
				takes = takes || l == name
			}
			if takes && !hasValue {
				i++
				if i < len(args) {
					value = args[i]
				}
			}
			if base == "env" && name == "--split-string" {
				split = append(split, strings.Fields(value)...)
			}
		case len(a) > 1 && a[0] == '-':
			for k := 1; k < len(a); k++ {
				if !strings.ContainsRune(w.short, rune(a[k])) {
					continue
				}
				value := a[k+1:]
				if value == "" {
					i++
					if i < len(args) {
						value = args[i]
					}
				}
				if base == "env" && a[k] == 'S' {
					split = append(split, strings.Fields(value)...)
				}
				break
			}
		default:
			break options
		}
	}
	for w.assigns && i < len(args) && strings.Contains(args[i], "=") {
		i++
	}
	i += w.operands
	if i > len(args) {
		i = len(args)
	}
	next := append(split, args[i:]...)
	return next, len(next) > 0
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
