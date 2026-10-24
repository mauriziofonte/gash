// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package shellinit

import (
	"os"
	"strings"

	htestate "github.com/mauriziofonte/gash/internal/hte/state"
	"github.com/mauriziofonte/gash/internal/platform"
)

// autostartShell is the hte.toml autostart mode this section serves
// (service.AutostartShell; the service package is not imported for one
// string).
const autostartShell = "shell"

// hteAutostart emits the shell autostart of gash hte (design hte.md 5):
// when hte.toml says autostart = "shell", the interactive shell starts the
// stack in the background when it is down. The binary reads hte.toml here,
// so a shell without it gets no line at all; a file that does not decode
// is left to `gash hte doctor` and gets none either.
//
// The emitted block runs once per shell (a re-evaluation of the layer
// skips it; gash_unload sweeps the __GASH_ guard), only when the shell is
// interactive, and never blocks or fails the load: `gash hte __autostart`
// runs detached in a subshell (no job-table entry, no "[1] PID" line) with
// no terminal input, its stdout discarded and its stderr (the error
// envelope of a failed start, nothing on success) kept in the logs
// directory as autostart.log, replaced by each attempt (discarded when that
// directory does not exist).
func (st *state) hteAutostart(b *strings.Builder) {
	dirs, err := platform.ResolveDirs(st.c.Env.Get)
	if err != nil {
		return
	}
	paths := htestate.NewPaths(dirs, os.Getuid())
	data, err := st.e.readFile(paths.Settings())
	if err != nil {
		return
	}
	set, err := htestate.DecodeSettings(data, paths.Settings())
	if err != nil || set.Autostart != autostartShell {
		return
	}
	// The logs directory exists once install ran; without it the
	// redirection would fail and the start would not run at all.
	log := "/dev/null"
	if st.isDir(paths.LogsDir()) {
		log = quote(paths.LogsDir() + "/autostart.log")
	}
	b.WriteString(`if [[ $- == *i* && -z "${__GASH_HTE_AUTOSTART-}" && -n "${__GASH_BIN-}" ]]; then` + "\n")
	b.WriteString("    __GASH_HTE_AUTOSTART=1\n")
	b.WriteString(`    ("$__GASH_BIN" hte __autostart </dev/null >/dev/null 2>` + log + ` &) 2>/dev/null || true` + "\n")
	b.WriteString("fi\n")
}
