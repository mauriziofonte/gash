// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// doctorUsage is the usage line of gash_doctor.
const doctorUsage = "gash_doctor [--timing] [--no-color] [--prereqs] [--json] [--install [--yes] [--all] [TOOL...]]"

// doctorFlags are the parsed arguments of gash_doctor.
type doctorFlags struct {
	timing, noColor, prereqs, json, install, yes, all bool
	tools                                             []string
}

// parseDoctor reads the arguments; ok is false after a usage error.
func parseDoctor(c *invoke.Call) (doctorFlags, bool) {
	var f doctorFlags
	for _, a := range c.Args {
		switch a {
		case "--timing":
			f.timing = true
		case "--no-color":
			f.noColor = true
		case "--prereqs":
			f.prereqs = true
		case "--json":
			f.json = true
		case "--install":
			f.install = true
		case "--yes", "-y":
			f.yes = true
		case "--all":
			f.all = true
		default:
			if !strings.HasPrefix(a, "-") && f.install {
				f.tools = append(f.tools, a)
				continue
			}
			badArgs(c, doctorUsage)
			return f, false
		}
	}
	if (f.yes || f.all) && !f.install {
		ui.StyleOf(c.Term, f.noColor).Printer(c.Stderr).Error("--yes and --all go with --install (usage: " + doctorUsage + ")")
		return f, false
	}
	return f, true
}

// applies reports whether a tool belongs on this system's report.
func applies(sys *prereq.System, t *prereq.Tool) bool {
	return t.On(sys.OS.GOOS, sys.OS.WSL)
}

// doctorPrereqs is the Prerequisites section: the system, then each area
// with its tools, found (with the path) or missing (with what needs it
// and the install command). A missing required tool is an issue; the
// others are warnings, since only their functions need them. It returns
// the number of issues.
func (m *Module) doctorPrereqs(c *invoke.Call, s ui.Style, sys *prereq.System) int {
	out, p := c.Stdout, s.Printer(c.Stderr)
	s.Heading(out, "--note", sys.Describe(), "Prerequisites:")
	issues, installable := 0, 0
	for _, area := range prereq.Areas {
		var tools []*prereq.Tool
		for _, t := range prereq.Catalog() {
			if t.Area == area.Key && applies(sys, t) {
				tools = append(tools, t)
			}
		}
		if len(tools) == 0 {
			continue
		}
		s.Heading(out, "--indent", "2", area.Title)
		for _, t := range tools {
			if path, ok := sys.Find(t); ok {
				p.Success("    " + t.Name + " (" + path + ")")
				continue
			}
			r := sys.Report(t.Name, "")
			if r.Install() != "" && !sys.NeedsRoot() {
				installable++
			}
			line := "    " + t.Name + " — not installed: " + t.Purpose + users(t)
			switch t.Level {
			case prereq.Required:
				p.Error(line)
				issues++
			case prereq.Feature:
				p.Warning(line)
			default:
				p.Info(line + " (optional)")
			}
			advice := "          " + r.Advice()
			if s.Color {
				advice = s.P.Muted + advice + s.P.Off
			}
			_, _ = io.WriteString(c.Stderr, advice+"\n")
		}
	}
	if installable > 0 {
		p.Hint("gash_doctor --install installs the missing tools after asking (--all adds the optional ones)")
	}
	return issues
}

// users is " (needed by A, B and C)", or " (needed by A, B and N more)"
// past three; "" when no function needs the tool.
func users(t *prereq.Tool) string {
	u := prereq.Users(t.Name)
	switch {
	case len(u) == 0:
		return ""
	case len(u) > 3:
		return " (needed by " + strings.Join(u[:2], ", ") + " and " + strconv.Itoa(len(u)-2) + " more)"
	case len(u) == 1:
		return " (needed by " + u[0] + ")"
	}
	return " (needed by " + strings.Join(u[:len(u)-1], ", ") + " and " + u[len(u)-1] + ")"
}

// doctorSSHKeys checks the SSH: keys of the settings file: each must exist
// and be readable by its owner only. The number of issues.
func doctorSSHKeys(c *invoke.Call, p ui.Printer) int {
	issues := 0
	for _, k := range config.LoadCaller(c.Env.Get).SSHKeys {
		switch perms := statPerms(k.Name); {
		case !isFile(k.Name):
			p.Error("  SSH key " + k.Name + " — not found (line " + strconv.Itoa(k.Line) + " of the settings file)")
			issues++
		case perms != "600" && perms != "400":
			p.Warning("  SSH key " + k.Name + " (permissions: " + perms + " — ssh refuses a key others can read; chmod 600 it)")
			issues++
		default:
			p.Success("  SSH key " + k.Name)
		}
	}
	return issues
}

// doctorJSON is gash_doctor --json: the system and every tool of the
// catalog that applies to it, for agents and scripts. Status 1 when a
// required tool is missing.
func (m *Module) doctorJSON(c *invoke.Call, sys *prereq.System) int {
	q := func(v string) string { return contract.Quote(v, contract.EscapeJQ) }
	var b strings.Builder
	b.WriteString(`{"system":{"name":` + q(sys.OS.Name) + `,"id":` + q(sys.OS.ID) + `,"version":` + q(sys.OS.VersionID) +
		`,"manager":` + q(string(sys.Manager)) + `,"root":` + strconv.FormatBool(sys.Root) + `,"sudo":` + strconv.FormatBool(sys.Sudo) +
		`,"wsl":` + strconv.FormatBool(sys.OS.WSL) + `},"tools":[`)
	missing, missingRequired, n := 0, 0, 0
	for _, t := range prereq.Catalog() {
		if !applies(sys, t) {
			continue
		}
		if n > 0 {
			b.WriteByte(',')
		}
		n++
		b.WriteString(`{"name":` + q(t.Name) + `,"level":` + q(t.Level.String()) + `,"area":` + q(t.Area))
		if path, ok := sys.Find(t); ok {
			b.WriteString(`,"found":true,"path":` + q(path) + `}`)
			continue
		}
		missing++
		if t.Level == prereq.Required {
			missingRequired++
		}
		r := sys.Report(t.Name, "")
		b.WriteString(`,"found":false,"needed_by":[`)
		for i, u := range prereq.Users(t.Name) {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(q(u))
		}
		b.WriteString(`]`)
		if pkg := r.Package(); pkg != "" {
			b.WriteString(`,"package":` + q(pkg))
		}
		if cmd := r.Install(); cmd != "" {
			b.WriteString(`,"install":` + q(cmd))
		}
		if t.NonFree {
			b.WriteString(`,"non_free":true`)
		}
		b.WriteString(`,"advice":` + q(r.Advice()) + `}`)
	}
	b.WriteString(`],"missing":` + strconv.Itoa(missing) + `,"missing_required":` + strconv.Itoa(missingRequired) + "}\n")
	_, _ = io.WriteString(c.Stdout, b.String())
	if missingRequired > 0 {
		return 1
	}
	return 0
}

// doctorInstall is gash_doctor --install [--yes] [--all] [TOOL...]: the
// named tools, or every missing required and feature tool (--all adds the
// optional ones), installed with the system's package manager after a
// person confirms. It never installs for an agent (GASH_HEADLESS=1) or
// without a terminal to ask on, unless --yes.
func (m *Module) doctorInstall(c *invoke.Call, s ui.Style, sys *prereq.System, f doctorFlags) int {
	p := s.Printer(c.Stderr)
	var picked []prereq.Report
	if len(f.tools) > 0 {
		for _, name := range f.tools {
			t, ok := prereq.Lookup(name)
			if !ok {
				p.Error("Unknown tool: " + name + " (gash_doctor --prereqs lists them)")
				return usageStatus
			}
			if path, found := sys.Find(t); found {
				p.Success("  " + t.Name + " is installed (" + path + ")")
				continue
			}
			picked = append(picked, sys.Report(t.Name, ""))
		}
	} else {
		for _, t := range prereq.Catalog() {
			if !applies(sys, t) || (t.Level == prereq.Optional && !f.all) {
				continue
			}
			if _, found := sys.Find(t); !found {
				picked = append(picked, sys.Report(t.Name, ""))
			}
		}
	}
	if len(picked) == 0 {
		p.Success("Nothing to install.")
		return 0
	}

	// A tool without an install command is left to the person, and so is
	// a non-free one unless it was named: its repository may be off.
	manualOnly := func(r prereq.Report) bool {
		return r.InstallArgv() == nil || (r.Tool.NonFree && len(f.tools) == 0)
	}
	var pkgs []string
	epelOf := map[string]bool{}
	epel, manual := false, 0
	for _, r := range picked {
		if manualOnly(r) {
			p.Info("  " + r.Tool.Name + ": " + r.Advice())
			manual++
			continue
		}
		if pkg := r.Package(); !slices.Contains(pkgs, pkg) {
			pkgs = append(pkgs, pkg)
		}
		epelOf[r.Package()] = r.Tool.EPEL
		epel = epel || r.Tool.EPEL
	}
	if len(pkgs) == 0 {
		return 1
	}
	cmds := sys.InstallArgv(epel, pkgs...)
	line := prereq.ShellJoin(cmds)

	if s.Headless || c.Env.Get("GASH_HEADLESS") == "1" {
		return c.Fail(contract.New("install_needs_confirmation",
			"gash_doctor installs packages only for a person at a terminal",
			"Ask the user to run: "+line).With("install", line))
	}
	if sys.NeedsRoot() {
		p.Error("Installing packages needs root, and sudo is not installed. As root, run: " + line)
		return 1
	}
	p.Info("To install on " + sys.Describe() + ": " + strings.Join(pkgs, " "))
	p.Info("Command: " + line)
	if !f.yes && !s.Confirm(c.Term, c.Stderr, "Install now?") {
		p.Info("Nothing installed.")
		return 1
	}
	if refresh := sys.RefreshArgv(); refresh != nil {
		if rc := m.runInstall(c, refresh); rc != 0 {
			p.Error("Fetching the package lists failed (status " + strconv.Itoa(rc) + "): " + prereq.ShellJoin([][]string{refresh}))
			return 1
		}
	}
	if !m.runAll(c, p, cmds) && len(pkgs) > 1 {
		// One package the system does not have fails the whole command:
		// install the others one at a time.
		p.Warning("Installing the packages one at a time.")
		for _, pkg := range pkgs {
			m.runAll(c, p, sys.InstallArgv(epelOf[pkg], pkg))
		}
	}

	after := sys.Fresh()
	failed := 0
	for _, r := range picked {
		if manualOnly(r) {
			continue
		}
		if path, ok := after.Find(r.Tool); ok {
			p.Success("  " + r.Tool.Name + " (" + path + ")")
		} else {
			p.Error("  " + r.Tool.Name + " — still not found after the install")
			failed++
		}
	}
	if failed > 0 || manual > 0 {
		return 1
	}
	return 0
}

// runAll runs install commands in order, and stops at the first that
// fails, with its status.
func (m *Module) runAll(c *invoke.Call, p ui.Printer, cmds [][]string) bool {
	for _, argv := range cmds {
		if rc := m.runInstall(c, argv); rc != 0 {
			p.Error("The install command failed (status " + strconv.Itoa(rc) + "): " + prereq.ShellJoin([][]string{argv}))
			return false
		}
	}
	return true
}

// runInstall runs one install command on the caller's terminal (sudo asks
// for the password there) and returns its status.
func (m *Module) runInstall(c *invoke.Call, argv []string) int {
	stdin := c.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	res, err := m.run.Run(ctxOf(c), runner.Cmd{Path: argv[0], Args: argv[1:], Options: runner.Options{
		Env: c.Env.Child(), Stdin: stdin, Stdout: c.Stdout, Stderr: c.Stderr}})
	if err != nil && res.ExitCode == 0 {
		return 1
	}
	return res.ExitCode
}
