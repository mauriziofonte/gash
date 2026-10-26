// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package shellinit

import (
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/selfmgmt"
)

// banner is lib/shell/banner.sh: the `mesg n` refusal, the optional inxi
// summary, the banner line with the inspiring quote, and the SSH
// auto-unlock. The binary resolves mesg and inxi on the effective PATH and
// renders the banner and the quote with the shell's color decision (its
// stdout, GASH__TTY_OUT); the script keeps only what must run in the shell:
// the `[[ -t 0 ]]` test (stdin is the shell's), inxi writing to the
// terminal, the printf of the rendered banner, and the SSH hand-off.
func (st *state) banner(b *strings.Builder) {
	// Refuse `write`/`talk` on this terminal, only with a terminal on stdin.
	if st.has("mesg") {
		b.WriteString("[[ -t 0 ]] && mesg n 2>/dev/null || true\n")
	}

	// A short `inxi` system summary when asked for and inxi exists; it writes
	// straight to the terminal at eval time.
	if st.s.Get("GASH_SHOW_INXI") == "1" && st.has("inxi") {
		b.WriteString("inxi -IpRS -v0 -c5\n")
	}

	// "Gash VERSION — USER" and the inspiring quote, rendered once by the
	// binary and emitted as one word so the shell forks nothing.
	if st.s.Get("GASH_BANNER") == "1" {
		b.WriteString("printf '%s' " + quote(selfmgmt.Banner(st.c)) + "\n")
	}

	// Auto-unlock the SSH keys of ~/.gash_env. With keys configured, hand off
	// to the ssh module through the stub the shim defined before this eval
	// (it drives ssh-add through a PTY and exports the agent variables); with
	// none, only the run-once marker, as 2.x. The binary knows the keys from
	// the file, so no fork happens when there are none (design shell.md 7).
	// Its status 1 on a failure (3.4) never fails the shell's start.
	if len(st.file.SSHKeys) > 0 {
		b.WriteString("if declare -F gash_ssh_auto_unlock >/dev/null 2>&1; then\n")
		b.WriteString("    gash_ssh_auto_unlock || :\n")
		b.WriteString("fi\n")
	} else {
		b.WriteString("export GASH_SSH_AUTOUNLOCK_RAN=1\n")
	}
}

// migrateSchemaVersion is lib/modules/migrate.sh __GASH_SCHEMA_VERSION: the
// configuration schema an account has once it is on Gash 2.x or later.
const migrateSchemaVersion = 2

// migrateNotice is the once-a-day reminder for a 1.x account. 3.0: SHELL-18
// fixed: it points at the D-6 route instead of the 2.x review it no longer
// runs (2.x: "... run "gash migrate" to review the changes in 2.0").
const migrateNotice = `gash: your configuration is from Gash 1.x, which Gash 3 does not migrate; run "gash migrate" for the steps`

// migrateReminder is lib/modules/migrate.sh __gash_migrate_autostart as rc.A
// keeps it (D-6: the guided review no longer starts by itself, so even on a
// terminal the interactive shell shows only the reminder). It fires when the
// account still has a 1.x configuration and the daily notice is due: the
// binary stamps the notice file and the script prints the one line on the
// shell's stderr. Headless shells return before this layer, so the notice is
// never printed and the state file never written while sourcing (SHELL-13).
func (st *state) migrateReminder(b *strings.Builder) {
	if !st.migrateNeeded() {
		return
	}
	dir := st.stateDir()
	now := st.e.now()
	// Once a day: skip while the last notice is under 24 h old.
	if data, err := st.e.readFile(dir + "/notice"); err == nil {
		if n := digitsOnly(string(data)); n != "" {
			if last, perr := strconv.ParseInt(n, 10, 64); perr == nil && now-last < 86400 {
				return
			}
		}
	}
	if err := st.e.mkdirAll(dir, 0o755); err == nil {
		_ = st.e.writeFile(dir+"/notice", []byte(strconv.FormatInt(now, 10)+"\n"), 0o644)
	}
	b.WriteString("__gash_note " + quote(migrateNotice) + "\n")
}

// migrateNeeded is lib/modules/migrate.sh __gash_migrate_needed: the schema
// is not recorded at version 2 or more, and ~/.gashrc is the 1.x loader (it
// exports GASH_BASH, which 2.x and 3.x never write). The schema is matched
// with a glob the 2.x code was careful to keep, so no regexp state is left
// behind; done in Go here.
func (st *state) migrateNeeded() bool {
	if data, err := st.e.readFile(st.stateDir() + "/schema"); err == nil {
		s := stripSpace(string(data))
		if s != "" && allDigits(s) {
			if v, perr := strconv.ParseInt(s, 10, 64); perr == nil && v >= migrateSchemaVersion {
				return false
			}
		}
	}
	data, err := st.e.readFile(st.home + "/.gashrc")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "GASH_BASH")
}

// stateDir is ${XDG_STATE_HOME:-$HOME/.local/state}/gash.
func (st *state) stateDir() string {
	xdg := st.c.Env.Get("XDG_STATE_HOME")
	if xdg == "" {
		xdg = st.home + "/.local/state"
	}
	return xdg + "/gash"
}

// stripSpace removes every whitespace byte, like the 2.x ${schema//[[:space:]]/}.
func stripSpace(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			return -1
		}
		return r
	}, s)
}

// allDigits reports whether s is non-empty and every byte is 0-9 (the 2.x
// "$s" != *[!0-9]* test on a non-empty string).
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// digitsOnly keeps only the digit bytes, like the 2.x ${last//[^0-9]/}.
func digitsOnly(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
