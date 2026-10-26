// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// smokeTimeout bounds each smoke-test shell.
var smokeTimeout = 60 * time.Second

// updateUsage is the usage line of gash update.
const updateUsage = "gash update [--rollback]"

// tree is what a commit of the install holds, as the update flow needs it.
type tree struct {
	// ref is what git checkout takes (a tag or a commit), name how the
	// messages show it.
	ref, name string
	// gashSh: the tree has a gash.sh (a Gash tree, 2.x or 3.x), smoke
	// tested after the checkout. bootstrap: it has the 3.x compat
	// bootstrap, so it needs the binary of its VERSION.
	gashSh, bootstrap bool
	// version is the first line of its VERSION file ("" without one).
	version string
	pins    map[string]string
}

// inspect reads the tree of ref in the install at dir.
func (m *Module) inspect(c *invoke.Call, dir, ref, name string) tree {
	t := tree{ref: ref, name: name}
	t.gashSh = m.gitOK(c, dir, "cat-file", "-e", ref+":gash.sh")
	t.bootstrap = m.gitOK(c, dir, "cat-file", "-e", ref+":lib/compat/bootstrap.sh")
	if res, err := m.git(c, dir, "show", ref+":VERSION"); err == nil && res.Success() {
		t.version, _, _ = strings.Cut(string(res.Stdout), "\n")
	}
	t.pins = map[string]string{}
	if plat := platform(); plat != "" {
		if res, err := m.git(c, dir, "show", ref+":release/SHA256SUMS"); err == nil && res.Success() {
			t.pins = parsePins(string(res.Stdout), plat)
		}
	}
	return t
}

// head is the tree the install is on: its commit, shown as its version
// tag, else git describe, else the short commit.
func (m *Module) head(c *invoke.Call, dir, current string) tree {
	sha := m.output(c, "git", "-C", dir, "rev-parse", "HEAD")
	name := current
	if name == "" {
		name = m.output(c, "git", "-C", dir, "describe", "--tags")
	}
	if name == "" {
		name = m.output(c, "git", "-C", dir, "rev-parse", "--short", "HEAD")
	}
	return m.inspect(c, dir, sha, name)
}

func (m *Module) git(c *invoke.Call, dir string, args ...string) (runner.Result, error) {
	return m.run.Run(ctxOf(c), runner.Cmd{Path: "git", Args: append([]string{"-C", dir}, args...),
		Options: runner.Options{Env: c.Env.Child()}})
}

func (m *Module) gitOK(c *invoke.Call, dir string, args ...string) bool {
	res, err := m.git(c, dir, args...)
	return err == nil && res.Success()
}

// checkout is git checkout --quiet REF on the terminal, as 2.x ran it.
func (m *Module) checkout(c *invoke.Call, dir, ref string) bool {
	return m.tool(c, "git", "-C", dir, "-c", "advice.detachedHead=false", "checkout", "--quiet", ref)
}

// moveTo is steps 4 to 7 of upgrade.md 3.6: the binary of a 3.x target is
// installed and verified before the tree is touched, then the checkout,
// then the smoke test; a failed smoke test checks out from again (its
// binary was never removed). The status: 0 when the install is on to.
func (m *Module) moveTo(c *invoke.Call, p ui.Printer, dir string, from, to tree) int {
	if to.bootstrap {
		if to.version == "" {
			p.Error(to.name + " has no VERSION file; not upgrading.")
			return 1
		}
		how, err := m.installBinary(c, release{version: to.version, pins: to.pins})
		if err != nil {
			p.Error("Could not install the gash binary of " + to.name + " (" + err.Error() + "); nothing was changed.")
			return 1
		}
		if how != "" {
			p.Success("Installed gash " + to.version + " for " + runtime.GOOS + "/" + runtime.GOARCH + " (verified against " + how + ").")
		}
	}
	if !m.checkout(c, dir, to.ref) {
		p.Error("Failed to check out " + to.name + ".")
		return 1
	}
	if !to.gashSh {
		return 0
	}
	reason := m.smoke(c, dir, to.version)
	if reason == "" {
		return 0
	}
	p.Error(to.name + " failed its smoke test (" + reason + ").")
	if !m.checkout(c, dir, from.ref) {
		p.Error("Could not check out " + from.name + " again. Run: git -C " + dir + " checkout " + from.ref)
		return 1
	}
	p.Info("Went back to " + from.name + "; nothing else was changed.")
	return 1
}

// smoke loads the install at dir in two child shells (upgrade.md 3.6
// step 6) and returns why it failed, "" when it passed: headless, sourcing
// gash.sh prints nothing, returns 0 and sets GASH_VERSION to version (the
// tree's VERSION, "unknown" without one), and gash_username answers;
// interactive (bash -i), sourcing returns 0. The children get the caller's
// environment without GASH_BIN, so the binary is the one the bootstrap of
// the tree resolves, and without what would run or print on their own
// (BASH_ENV, ENV, GASH_DEBUG, GASH_TIMING).
func (m *Module) smoke(c *invoke.Call, dir, version string) string {
	if version == "" {
		version = "unknown"
	}
	bash := smokeBash(c)
	env := func() *runner.Env {
		e := runner.NewEnv().AllowAll(c.Env.Child())
		for _, n := range []string{"GASH_BIN", "BASH_ENV", "ENV", "GASH_DEBUG", "GASH_TIMING", "GASH_HEADLESS"} {
			e.Unset(n)
		}
		for _, n := range []string{"HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
			if v, ok := c.Env.Lookup(n); ok {
				e.Set(n, v)
			}
		}
		return e
	}
	res, err := m.run.RunBash(ctxOf(c), runner.Bash{Use: runner.BashUpdateSmoke, Bash: bash, Args: []string{dir},
		Options: runner.Options{Env: env().Set("GASH_HEADLESS", "1").Environ(), Timeout: smokeTimeout, Setsid: true}})
	if err != nil {
		return "cannot start " + bash
	}
	fields := strings.Split(string(res.Stdout), "\x00")
	switch {
	case len(fields) < 5:
		return "the headless load did not finish"
	case fields[0] != "":
		return "the headless load printed output"
	case fields[1] != "0":
		return "the headless load returned " + fields[1]
	case fields[2] != version:
		return "GASH_VERSION is " + strconv.Quote(fields[2]) + ", not " + strconv.Quote(version)
	case fields[4] != "0" || strings.TrimSpace(fields[3]) == "":
		return "gash_username did not answer: " + firstLine(res.Stderr)
	case len(bytes.TrimSpace(res.Stderr)) > 0:
		return "the headless load printed output"
	}
	res, err = m.run.RunBash(ctxOf(c), runner.Bash{Use: runner.BashUpdateSmokeInteractive, Bash: bash, Args: []string{dir},
		Options: runner.Options{Env: env().Set("HISTFILE", "/dev/null").Set("GASH_SSH_AUTOUNLOCK_RAN", "1").Environ(),
			Stdout: io.Discard, Stderr: io.Discard, Timeout: smokeTimeout, Setsid: true}})
	if err != nil {
		return "cannot start " + bash
	}
	if !res.Success() {
		return "the interactive load returned " + strconv.Itoa(res.ExitCode)
	}
	return ""
}

// smokeBash is the bash of the smoke test: the caller's ($BASH), else
// $SHELL when it is a bash, else bash on PATH.
func smokeBash(c *invoke.Call) string {
	for _, b := range []string{c.Env.Get("BASH"), c.Env.Get("SHELL")} {
		if filepath.IsAbs(b) && filepath.Base(b) == "bash" && isExecutable(b) {
			return b
		}
	}
	if b := c.Env.Get("BASH"); filepath.IsAbs(b) && isExecutable(b) {
		return b
	}
	return "bash"
}

func firstLine(b []byte) string {
	s, _, _ := strings.Cut(strings.TrimSpace(string(b)), "\n")
	if s == "" {
		return "no output"
	}
	return s
}

// update is `gash update [--rollback]`: gash_upgrade's flow, or the way
// back to the version it left.
func (m *Module) update(c *invoke.Call) int {
	if badArgs(c, updateUsage, "--rollback") {
		return usageStatus
	}
	if slices.Contains(c.Args, "--rollback") {
		return m.rollback(c)
	}
	return m.upgradeFlow(c)
}

// rollback returns the install to the version the last gash update left
// (the state file names it), with the same checks as an update: a clean
// tree, the binary installed and verified first, the smoke test and the
// way back. The version it leaves becomes the one to return to.
func (m *Module) rollback(c *invoke.Call) int {
	p := ui.StyleOf(c.Term, false).Printer(c.Stderr)
	dir := installDir(c)
	if !m.cleanCheckout(c, p, dir) {
		return 1
	}
	st := readUpdateState(c)
	if st.dir != dir || st.previous == "" || !m.gitOK(c, dir, "cat-file", "-e", st.previous+"^{commit}") {
		p.Error("No previous version of " + dir + " to return to (gash update records it when it upgrades). Run: git -C " + dir + " checkout vX.Y.Z")
		return 1
	}
	from := m.head(c, dir, m.output(c, "git", "-C", dir, "describe", "--tags", "--exact-match", "HEAD"))
	if from.ref == st.previous {
		p.Success("Gash is already at " + st.name + ".")
		return 0
	}
	to := m.inspect(c, dir, st.previous, st.name)
	if rc := m.moveTo(c, p, dir, from, to); rc != 0 {
		return rc
	}
	p.Success("Rolled back Gash from " + from.name + " to " + to.name + ".")
	m.recordUpdate(c, dir, from, to)
	p.Info("Open a new terminal (or run 'source ~/.gashrc') to load it.")
	return 0
}

// cleanCheckout is the first check of gash_upgrade: git installed, and a
// git checkout at dir without local modifications. A missing git is the
// prerequisite report (3.4: it used to pass this check, as an empty git
// status, and fail the fetch as a network problem); a git status that
// fails refuses too, since it proves nothing about local edits.
func (m *Module) cleanCheckout(c *invoke.Call, p ui.Printer, dir string) bool {
	if _, ok := prereq.Require(c, "git", updateNeed(c), false); !ok {
		return false
	}
	if st, err := os.Stat(dir + "/.git"); err != nil || !st.IsDir() {
		p.Error("Gash is not installed in " + dir + " as a git checkout. See https://github.com/mauriziofonte/gash.")
		return false
	}
	// The checkout would overwrite local edits: refuse instead.
	res, err := m.git(c, dir, "status", "--porcelain")
	if err != nil || !res.Success() {
		why := firstLine(res.Stderr)
		if err != nil {
			why = err.Error()
		}
		p.Error("Cannot tell whether the working tree at " + dir + " has local modifications (git status failed: " + why + "); not upgrading.")
		return false
	}
	if runner.CommandSubst(res.Stdout) != "" {
		p.Error("Working tree at " + dir + " has local modifications. Commit, stash, or discard them before upgrading.")
		return false
	}
	return true
}

// updateNeed names the call in a prerequisite report: gash_upgrade, gash
// update or gash update --rollback.
func updateNeed(c *invoke.Call) string {
	if c.Name == "gash_upgrade" {
		return c.Name
	}
	if slices.Contains(c.Args, "--rollback") {
		return "gash update --rollback"
	}
	return "gash update"
}

// upgradeState is the update state file,
// ${XDG_STATE_HOME:-~/.local/state}/gash/update: the install it is about
// and the commit (and its name) gash update --rollback returns to.
type upgradeState struct{ dir, previous, name string }

func updateStatePath(c *invoke.Call) string { return stateDir(c) + "/update" }

func readUpdateState(c *invoke.Call) upgradeState {
	var st upgradeState
	b, err := os.ReadFile(updateStatePath(c))
	if err != nil {
		return st
	}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, _ := strings.Cut(line, "=")
		switch k {
		case "dir":
			st.dir = v
		case "previous":
			st.previous = v
		case "previous_name":
			st.name = v
		}
	}
	return st
}

// recordUpdate remembers from as the version to return to, then keeps the
// binaries of the two versions (and of the running binary): the other
// version directories that hold nothing but the release binaries are
// removed. Best effort: a failure costs only the rollback or the space.
func (m *Module) recordUpdate(c *invoke.Call, dir string, from, to tree) {
	if strings.ContainsAny(dir+from.ref+from.name, "\n") {
		return
	}
	sd := stateDir(c)
	if err := os.MkdirAll(sd, 0o755); err == nil {
		tmp := updateStatePath(c) + ".tmp." + strconv.Itoa(os.Getpid())
		body := "dir=" + dir + "\nprevious=" + from.ref + "\nprevious_name=" + from.name + "\n"
		if os.WriteFile(tmp, []byte(body), 0o644) == nil && os.Rename(tmp, updateStatePath(c)) != nil {
			_ = os.Remove(tmp)
		}
	}
	keep := map[string]bool{to.version: true, from.version: true, m.version: true}
	base := dataDir(c) + "/versions"
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if keep[name] || !e.IsDir() || e.Type()&os.ModeSymlink != 0 {
			continue
		}
		if _, ok := compareVersions(name, name); !ok {
			continue
		}
		if onlyBinaries(base + "/" + name) {
			_ = os.RemoveAll(base + "/" + name)
		}
	}
}

// onlyBinaries reports whether dir holds nothing but the release binaries
// as regular files (no lock or download in progress, nothing of the user).
func onlyBinaries(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if (e.Name() != binGash && e.Name() != binHelper) || !e.Type().IsRegular() {
			return false
		}
	}
	return true
}
