// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package dev holds lib/modules/php.sh and lib/modules/node.sh:
// php_detect_version and node_detect_version (project files and the NVM
// tree read natively, jq and the text tools evaluated in Go), node_upgrade
// (the plan; its nvm calls stay in the caller's shell, LEDGER DEV-16) and
// nvm_prune_versions (every step is an nvm call: the generated shim runs
// it, the binary answers only help and the missing nvm).
package dev

import (
	"os"
	"path/filepath"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/system/handoff"
	"github.com/mauriziofonte/gash/internal/ui"
)

// Module is the dev handlers with their seams: the sysroot of the
// /usr/bin and /usr/local/bin probes, and the hand-off descriptor.
type Module struct {
	sys  platform.Sysroot
	open handoff.Opener
}

// New returns the module wired to the real system.
func New() *Module { return &Module{sys: platform.Real(), open: handoff.OpenFD} }

// Register binds the dev functions to t.
func Register(t *invoke.Table) { New().Register(t) }

// Register binds the functions of m to t.
func (m *Module) Register(t *invoke.Table) {
	t.Handle("php_detect_version", m.phpDetect)
	t.Handle("node_detect_version", m.nodeDetect)
	t.Handle("node_upgrade", m.nodeUpgrade)
	t.Handle("nvm_prune_versions", m.nvmPrune)
}

// printer is the 2.x emitters on stderr.
func printer(c *invoke.Call) ui.Printer { return ui.NewPrinter(c.Stderr, c.Term, false) }

// nvmMissing is __gash_require_nvm failing: nvm is a shell function, which
// only the caller's shell has.
const nvmMissing = "nvm is not loaded in this shell. Source $NVM_DIR/nvm.sh first."

// nvmPruneUsage is the usage line of nvm_prune_versions: -y and --yes
// are synonyms of --force (LEDGER DEV-20).
const nvmPruneUsage = "nvm_prune_versions [-y|--yes|--force]"

// nvmPrune is nvm_prune_versions run outside the shim: help, an unknown
// option (the arguments come before the environment, LEDGER DEV-20), or
// the missing nvm. The shim runs the function itself, in the caller's
// shell (LEDGER DEV-16).
func (m *Module) nvmPrune(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	for _, a := range c.Args {
		switch a {
		case "--force", "-f", "-y", "--yes":
		default:
			printer(c).Error("Unknown option: " + a + " (usage: " + nvmPruneUsage + ")")
			return 1
		}
	}
	printer(c).Error(nvmMissing)
	return 1
}

// headless is __gash_headless_gate_active.
func headless(c *invoke.Call) bool { return c.Env.Get("GASH_HEADLESS") == "1" }

// cwd is the logical working directory of the caller (`pwd`): $PWD when it
// names the current directory, else the physical path.
func cwd(c *invoke.Call) string {
	wd, err := os.Getwd()
	if p := c.Env.Get("PWD"); filepath.IsAbs(p) {
		if a, e1 := os.Stat(p); e1 == nil {
			if b, e2 := os.Stat("."); e2 == nil && os.SameFile(a, b) {
				return p
			}
		}
	}
	if err != nil {
		return "."
	}
	return wd
}

// isFile is [[ -f P ]].
func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// isDir is [[ -d P ]].
func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// style is the output style of a call (no --no-color flag on the dev
// functions).
func (m *Module) style(c *invoke.Call) ui.Style { return ui.StyleOf(c.Term, false) }

// executable is [[ -x SYSPATH ]] on an absolute system path, mapped
// through the sysroot (the /usr/bin and /usr/local/bin probes): it exists
// and its mode carries an execute bit (symlinks followed).
func (m *Module) executable(sysPath string) bool {
	st, err := os.Stat(m.sys.Path(sysPath))
	return err == nil && st.Mode().Perm()&0o111 != 0
}

// readFile reads a project file whole (head, jq and grep read it in 2.x).
func (m *Module) readFile(path string) ([]byte, error) { return os.ReadFile(path) }

// logicalDir is the directory `cd "$dir" && pwd` reaches with CDPATH
// ignored (LEDGER DEV-11): dir joined to the logical cwd and cleaned
// lexically (cd -L keeps symlinks), or "" when the result is not a
// directory (cd fails and the walk-up starts at the literal argument).
func (m *Module) logicalDir(c *invoke.Call, dir string) string {
	var cleaned string
	if filepath.IsAbs(dir) {
		cleaned = filepath.Clean(dir)
	} else {
		cleaned = filepath.Join(cwd(c), dir)
	}
	if isDir(cleaned) {
		return cleaned
	}
	return ""
}

// gnuDirname is `dirname PATH` (GNU coreutils): the path without its last
// component, trailing slashes trimmed, "." when there is no slash, "/"
// for the root and for an all-slash path.
func gnuDirname(p string) string {
	i := len(p)
	for i > 0 && p[i-1] == '/' {
		i--
	}
	if i == 0 {
		return "/"
	}
	j := i
	for j > 0 && p[j-1] != '/' {
		j--
	}
	if j == 0 {
		return "."
	}
	for j > 0 && p[j-1] == '/' {
		j--
	}
	if j == 0 {
		return "/"
	}
	return p[:j]
}
