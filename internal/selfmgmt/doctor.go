// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// The variables the gash_doctor stub passes (compat/stubs.tmpl): the
// shell's __GASH_LOAD_ERRORS, one per line, and its known settings that
// are set, NAME<TAB>VALUE lines; backslash and newline escaped.
const (
	LoadErrorsVar = "GASH__LOAD_ERRORS"
	SettingsVar   = "GASH__SETTINGS"
)

// DoctorCoreFiles are the files of the 3.x tree that gash_doctor checks
// under "Core Files" (LEDGER SELF-17, decision D-9): the generated entry
// point, shims, completion and launcher, then the files the tree keeps.
var DoctorCoreFiles = []string{
	"gash.sh",
	"lib/compat/bootstrap.sh",
	"lib/core/output.sh",
	"lib/core/utils.sh",
	"lib/core/validation.sh",
	"lib/core/config.sh",
	"bash_completion",
	"bin/gash",
	"VERSION",
	"quotes/list.txt",
	".gash_env.template",
}

// DoctorModules are the module shims of the 3.x tree (lib/modules/NAME.sh,
// the paths external harnesses source), in the 2.x order.
var DoctorModules = []string{"remote", "node", "php"}

// doctorTools are the external tools gash_doctor looks up.
var doctorTools = []string{"git", "curl", "jq", "sqlite3", "docker"}

// doctor is gash_doctor [--timing] [--no-color] (lib/modules/gash.sh): the
// 2.x headings and lines, rc 0 even with issues and unknown options
// ignored (LEDGER SELF-06, SELF-15, reproduced), over the 3.x file list
// plus a line for the binary (SELF-17).
func (m *Module) doctor(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	noColor, timing := false, false
	for _, a := range c.Args {
		switch a {
		case "--no-color":
			noColor = true
		case "--timing":
			timing = true
		}
	}
	s := ui.StyleOf(c.Term, !c.Term.WantColors(noColor))
	if timing {
		return m.doctorTiming(c, s)
	}

	p := s.Printer(c.Stderr)
	out := c.Stdout
	dir := installDir(c)
	issues := 0
	version := c.Env.Get("GASH_VERSION")
	if version == "" {
		version = "unknown"
	}
	p.Info("Gash Doctor — checking installation (v" + version + ")...")
	_, _ = io.WriteString(out, "\n")

	// 1. Runtime
	s.Heading(out, "Runtime:")
	bv := c.Env.Get("BASH_VERSION")
	switch major, minor := bashVersinfo(bv); {
	case bv == "":
		p.Info("  bash — not called from Bash")
	case major > 4 || (major == 4 && minor >= 4):
		p.Success("  bash " + bv)
	default:
		p.Error("  bash " + bv + " — Gash needs 4.4 or newer")
		issues++
	}
	if !m.binaryLine(p, dir) {
		issues++
	}
	loadErrors := escapedLines(c.Env.Get(LoadErrorsVar))
	if len(loadErrors) == 0 {
		p.Success("  all files loaded")
	}
	for _, f := range loadErrors {
		p.Error("  failed to load " + f)
		issues++
	}

	// 2. Core files
	s.Heading(out, "Core Files:")
	for _, f := range DoctorCoreFiles {
		if isFile(dir + "/" + f) {
			p.Success("  " + f)
		} else {
			p.Error("  " + f + " — MISSING")
			issues++
		}
	}

	// 3. Modules
	s.Heading(out, "Modules:")
	for _, mod := range DoctorModules {
		if isFile(dir + "/lib/modules/" + mod + ".sh") {
			p.Success("  " + mod)
		} else {
			p.Warning("  " + mod + " — not found")
			issues++
		}
	}

	// 4. Config file and settings
	s.Heading(out, "Configuration:")
	envFile := c.Env.Get("GASH_ENV_FILE")
	if envFile == "" {
		envFile = c.Env.Get("HOME") + "/.gash_env"
	}
	if isFile(envFile) {
		perms := statPerms(envFile)
		if perms == "600" || perms == "400" {
			p.Success("  " + envFile + " (permissions: " + perms + ")")
		} else {
			p.Warning("  " + envFile + " (permissions: " + perms + " — should be 600)")
			issues++
		}
	} else {
		p.Info("  " + envFile + " — not found (optional, run gash_env_init to create)")
	}
	for _, kv := range shellSettings(c) {
		s.KV(out, "--indent", "4", "--width", "0", "--sep", "=", kv[0], kv[1])
	}

	// 5. External tools
	s.Heading(out, "External Tools:")
	for _, tool := range doctorTools {
		if path, ok := m.lookPath(c, tool); ok {
			p.Success("  " + tool + " (" + path + ")")
		} else {
			p.Warning("  " + tool + " — not found (some features may be unavailable)")
		}
	}

	// 6. Summary
	_, _ = io.WriteString(out, "\n")
	if issues == 0 {
		p.Success("All checks passed.")
	} else {
		p.Warning(strconv.Itoa(issues) + " issue(s) found.")
	}
	return 0
}

// binaryLine reports the binary that runs this call: its version and path,
// and an issue when a released build is not the version of the install's
// VERSION file (a development build matches any).
func (m *Module) binaryLine(p ui.Printer, dir string) bool {
	path, err := m.executable()
	if err != nil || path == "" {
		path = "unknown path"
	}
	line := "  gash " + m.version + " (" + path + ")"
	want := ""
	if b, err := m.readFile(dir + "/VERSION"); err == nil {
		want, _, _ = strings.Cut(string(b), "\n")
	}
	if m.version != "dev" && want != "" && want != m.version {
		p.Error(line + " — this install is " + want)
		return false
	}
	p.Success(line)
	return true
}

// bashVersinfo is BASH_VERSINFO[0] and [1] of a BASH_VERSION value such as
// "5.2.21(1)-release".
func bashVersinfo(v string) (major, minor int) {
	majorText, rest, _ := strings.Cut(v, ".")
	minorText, _, _ := strings.Cut(rest, ".")
	return leadingInt(majorText), leadingInt(minorText)
}

func leadingInt(s string) int {
	n := 0
	for i := 0; i < len(s) && s[i] >= '0' && s[i] <= '9' && n < 1<<20; i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// escapedLines decodes a list the stubs pass: each value escaped and
// followed by a newline, so an empty line is an empty value.
func escapedLines(v string) []string {
	if v == "" {
		return nil
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSuffix(v, "\n"), "\n") {
		out = append(out, unescapeLine(line))
	}
	return out
}

// shellSettings are the known settings set in the caller's shell, in the
// __GASH_SETTINGS_KNOWN order, as the stub passes them; without the stub
// (a direct call), the known settings in the environment.
func shellSettings(c *invoke.Call) [][2]string {
	var out [][2]string
	if v, ok := c.Env.Lookup(SettingsVar); ok {
		for _, line := range strings.Split(v, "\n") {
			name, value, ok := strings.Cut(line, "\t")
			if ok && name != "" {
				out = append(out, [2]string{name, unescapeLine(value)})
			}
		}
		return out
	}
	for _, s := range config.Known {
		if v, ok := c.Env.Lookup(s.Name); ok {
			out = append(out, [2]string{s.Name, v})
		}
	}
	return out
}

// statPerms is stat -L -c '%a' FILE: the permission bits in octal, the
// setuid, setgid and sticky bits included; "unknown" when it fails.
func statPerms(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return "unknown"
	}
	mode := st.Mode()
	bits := int64(mode.Perm())
	if mode&os.ModeSetuid != 0 {
		bits |= 0o4000
	}
	if mode&os.ModeSetgid != 0 {
		bits |= 0o2000
	}
	if mode&os.ModeSticky != 0 {
		bits |= 0o1000
	}
	return strconv.FormatInt(bits, 8)
}
