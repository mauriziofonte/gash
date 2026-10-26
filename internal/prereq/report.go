// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package prereq

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// CodeMissing is the contract code of a missing tool that has no code of
// its own.
const CodeMissing = "prerequisite_missing"

// Report is what Gash says about a tool that is not installed: what needs
// it, what it is for, and how to install it on this system.
type Report struct {
	Tool *Tool
	// Need is the function, or function and flag, that needs it
	// ("process_find", "llm_procs --port"); empty for a doctor line.
	Need   string
	System *System
}

// Report describes the named tool for need. A name outside the catalog is
// reported as a program with no package.
func (s *System) Report(name, need string) Report {
	t, ok := Lookup(name)
	if !ok {
		t = &Tool{Name: name, Level: Feature}
	}
	return Report{Tool: t, Need: need, System: s}
}

// InstallArgv is the command that installs the tool here (EPEL first on
// the RHEL rebuilds, so possibly two); nil when there is none to run.
func (r Report) InstallArgv() [][]string {
	t, s := r.Tool, r.System
	if t.NoInstall || s.builtin(t) || (s.Manager == Brew && s.Brew == "") {
		return nil
	}
	pkg := s.Package(t)
	if pkg == "" || (t.EPEL && s.OS.ID == "amzn") {
		return nil
	}
	return s.InstallArgv(t.EPEL, pkg)
}

// Install is InstallArgv as one line of shell; "" when there is none.
func (r Report) Install() string {
	cmds := r.InstallArgv()
	if cmds == nil {
		return ""
	}
	line := ShellJoin(cmds)
	if r.System.NeedsRoot() {
		line += "   (as root)"
	}
	return line
}

// Package is the package that provides the tool here; "" when none.
func (r Report) Package() string {
	if r.Tool.NoInstall || r.System.builtin(r.Tool) {
		return ""
	}
	return r.System.Package(r.Tool)
}

// Advice is one sentence on how to get the tool, for a hint.
func (r Report) Advice() string {
	t, s := r.Tool, r.System
	if cmd := r.Install(); cmd != "" {
		a := "Install " + r.Package() + ": " + cmd
		if t.NonFree {
			a += " (the package is in the " + nonFreeRepo(s.Manager) + " repository)"
		}
		if t.Note != "" {
			a += ". " + t.Note
		}
		return a
	}
	switch {
	case s.builtin(t):
		return t.Name + " ships with " + osName(s) + ": check that the system directories (/usr/bin, /usr/sbin) are on PATH"
	case t.Manual != "":
		a := "Install " + t.Name + ": " + t.Manual
		if t.Note != "" {
			a += ". " + t.Note
		}
		return a
	case t.Note != "":
		return t.Note
	case s.Manager == Brew && s.Brew == "" && t.Packages[Brew] != "":
		return "Install Homebrew (" + brewURL + "), then: brew install " + t.Packages[Brew]
	case t.EPEL && s.OS.ID == "amzn":
		return t.Name + " is not in the Amazon Linux repositories: install it from its project page"
	}
	return "Install " + t.Name + " with the package manager of " + osName(s)
}

func nonFreeRepo(m Manager) string {
	switch m {
	case Apt:
		return "non-free (Debian) or multiverse (Ubuntu)"
	case Zypper:
		return "non-oss"
	}
	return "non-free"
}

func osName(s *System) string {
	if s.OS.Name != "" {
		return shortName(s.OS.Name)
	}
	if s.OS.GOOS == "darwin" {
		return "macOS"
	}
	return "this system"
}

// Sentence is the error line: "NEED needs TOOL, which is not installed"
// or "TOOL is not installed".
func (r Report) Sentence() string {
	if r.Need == "" {
		return r.Tool.Name + " is not installed"
	}
	return r.Need + " needs " + r.Tool.Name + ", which is not installed"
}

// Code is the contract code of the missing tool.
func (r Report) Code() string {
	if r.Tool.Code != "" {
		return r.Tool.Code
	}
	return CodeMissing
}

// Envelope is the machine error: the tool's code, the sentence, the
// advice as hint, and the members "missing" (the tool) and "install" (the
// command, when there is one), which mark every missing prerequisite.
func (r Report) Envelope() contract.Envelope {
	e := contract.New(r.Code(), r.Sentence(), r.Advice()).With("missing", r.Tool.Name)
	if cmd := r.Install(); cmd != "" {
		e = e.With("install", cmd)
	}
	return e
}

// Print writes the explanation for a person: the error line, what the
// tool does, the install command and the gash_doctor way to run it.
func (r Report) Print(p ui.Printer) {
	p.Error(r.Sentence() + ".")
	w := p.W
	line := "  " + r.Tool.Name + " " + r.Tool.Purpose
	if pkg := r.Package(); pkg != "" {
		line += " (package " + pkg + " on " + osName(r.System) + ")"
	}
	writeln(w, line+".")
	if cmd := r.Install(); cmd != "" {
		writeln(w, "  Install it:  "+cmd)
		if !r.System.NeedsRoot() {
			writeln(w, "  Or run:      gash_doctor --install "+r.Tool.Name)
		}
		if r.Tool.NonFree {
			writeln(w, "  The package is in the "+nonFreeRepo(r.System.Manager)+" repository.")
		}
		if r.Tool.Note != "" {
			writeln(w, "  "+r.Tool.Note+".")
		}
		return
	}
	writeln(w, "  "+r.Advice()+".")
}

func writeln(w io.Writer, s string) { _, _ = io.WriteString(w, strings.TrimSuffix(s, "..")+"\n") }

var (
	overrideMu sync.Mutex
	override   *System
)

// SetSystem makes Current return s until the returned function runs: the
// tests of other packages pin the system their expected errors name.
func SetSystem(s *System) (restore func()) {
	overrideMu.Lock()
	old := override
	override = s
	overrideMu.Unlock()
	return func() {
		overrideMu.Lock()
		override = old
		overrideMu.Unlock()
	}
}

// Current is the system of a call: the running one, in the caller's
// environment.
func Current(c *invoke.Call) *System {
	overrideMu.Lock()
	o := override
	overrideMu.Unlock()
	if o != nil {
		return o
	}
	return Detect(c.Env.Child())
}

// Fail reports that need cannot run because the named tool is missing,
// and returns status 1. machine selects the JSON envelope (llm_*,
// agent_*, --json and headless calls); a person gets the explanation
// gash_doctor gives for the tool.
func Fail(c *invoke.Call, name, need string, machine bool) int {
	r := Current(c).Report(name, need)
	if machine || c.Env.Get("GASH_HEADLESS") == "1" {
		return c.Fail(r.Envelope())
	}
	r.Print(ui.StyleOf(c.Term, false).Printer(c.Stderr))
	return 1
}

// Require looks the named tool up for the call: its path, or the report of
// Fail and false.
func Require(c *invoke.Call, name, need string, machine bool) (string, bool) {
	t, ok := Lookup(name)
	if !ok {
		panic(fmt.Sprintf("prereq: %q is not in the catalog", name))
	}
	if p, ok := Current(c).Find(t); ok {
		return p, true
	}
	Fail(c, name, need, machine)
	return "", false
}

// shortName drops the trailing code name of a PRETTY_NAME: "Rocky Linux
// 9.4 (Blue Onyx)" is "Rocky Linux 9.4".
func shortName(n string) string {
	if strings.HasSuffix(n, ")") {
		if i := strings.LastIndex(n, " ("); i > 0 {
			return n[:i]
		}
	}
	return n
}
