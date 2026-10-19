// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package shellinit

import (
	"strings"

	"github.com/mauriziofonte/gash/internal/runner"
)

// terminal is lib/shell/terminal.sh: checkwinsize, the lesspipe LESSOPEN /
// LESSCLOSE pair, debian_chroot, the default EDITOR and the GASH_UMASK mask.
// The binary resolves lesspipe, nano and vim on PATH and validates the mask
// (no =~ in the shell, SHELL-10); the emitted script tests the caller's own
// LESSOPEN, debian_chroot, EDITOR and umask state, so a value already set is
// kept, exactly as 2.x.
func (st *state) terminal(b *strings.Builder) {
	// Keep LINES/COLUMNS in sync after each command.
	b.WriteString("shopt -s checkwinsize\n")

	// less friendlier for non-text input files: the two lines
	// `eval "$(lesspipe)"` sets, only when LESSOPEN is unset and lesspipe
	// exists (path resolved by the binary).
	if p, ok := st.program("lesspipe"); ok {
		p = st.absProgram(p)
		b.WriteString("if [[ -z \"${LESSOPEN-}\" ]]; then\n")
		b.WriteString("    export LESSOPEN=" + quote("| "+p+" %s") + "\n")
		b.WriteString("    export LESSCLOSE=" + quote(p+" %s %s") + "\n")
		b.WriteString("fi\n")
	}

	// Identify the chroot you work in (used by the prompt), only when unset.
	if st.readable("/etc/debian_chroot") {
		if data, err := st.e.readFile("/etc/debian_chroot"); err == nil {
			chroot := runner.CommandSubst(data) // $(</etc/debian_chroot): strip trailing newlines
			b.WriteString("[[ -z \"${debian_chroot-}\" ]] && debian_chroot=" + quote(chroot) + "\n")
		}
	}

	// Default editor, only when the user has not chosen one (nano, else vim).
	if st.has("nano") {
		b.WriteString("[[ -z \"${EDITOR-}\" ]] && export EDITOR='nano'\n")
	} else if st.has("vim") {
		b.WriteString("[[ -z \"${EDITOR-}\" ]] && export EDITOR='vim'\n")
	}

	// Default file-creation mask: untouched unless GASH_UMASK is a valid
	// octal mask (validated by the binary; a bad value warns).
	if m := st.s.Get("GASH_UMASK"); m != "" {
		if validUmask(m) {
			b.WriteString("if [[ -n \"${__GASH_SNAPSHOT_TAKEN-}\" && \" ${__GASH_SNAPSHOT[*]-} \" != *\" umask=\"* ]]; then\n")
			b.WriteString("    __GASH_SNAPSHOT+=(\"umask=$(umask)\")\n")
			b.WriteString("fi\n")
			b.WriteString("umask " + quote(m) + "\n")
		} else {
			b.WriteString("__gash_warning " + quote("GASH_UMASK must be an octal mask such as 022 or 077 (ignored)") + "\n")
		}
	}
}

// absProgram reproduces __gash_lesspipe_env's path fix: a resolved program
// that is not absolute is taken relative to the shell's working directory
// (the binary's own, which the compat subshell inherits).
func (st *state) absProgram(p string) string {
	if strings.HasPrefix(p, "/") {
		return p
	}
	p = strings.TrimPrefix(p, "./")
	if wd, err := st.e.getwd(); err == nil {
		return wd + "/" + p
	}
	return p
}

// validUmask is the 2.x [[ GASH_UMASK =~ ^[0-7]{3,4}$ ]] test, done in Go so
// the shell leaves no BASH_REMATCH behind (SHELL-10).
func validUmask(m string) bool {
	if len(m) < 3 || len(m) > 4 {
		return false
	}
	for i := 0; i < len(m); i++ {
		if m[i] < '0' || m[i] > '7' {
			return false
		}
	}
	return true
}
