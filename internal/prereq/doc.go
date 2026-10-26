// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package prereq

import "strings"

// managerTitles are the columns of the package tables, in Managers order.
var managerTitles = map[Manager]string{
	Apt:    "Debian, Ubuntu (apt)",
	Dnf:    "Fedora, RHEL family, Amazon Linux (dnf)",
	Pacman: "Arch (pacman)",
	Zypper: "openSUSE (zypper)",
	Apk:    "Alpine (apk)",
	Brew:   "macOS (Homebrew)",
}

// RequirementsDoc is docs/requirements.md: every tool of the catalog by
// area, what it is for, the functions that need it and its package on
// each system. go test ./internal/prereq -update rewrites the file.
func RequirementsDoc() string {
	var b strings.Builder
	w := func(s ...string) {
		for _, l := range s {
			b.WriteString(l + "\n")
		}
	}
	w("<!-- Generated from internal/prereq (go test ./internal/prereq -update): do not edit. -->",
		"",
		"# Requirements",
		"",
		"Gash is one static binary plus the Bash that loads it. The binary does",
		"its work in process (search, hashing, JSON, databases, HTTP), so most of",
		"Gash needs nothing else. What it does need is listed here, from the same",
		"catalog that `install.sh` and `gash_doctor` read.",
		"",
		"- **Required** tools are needed to install Gash and to load it: Bash 4.4",
		"  or newer, git, curl or wget, tar, gzip, sha256sum or shasum, grep and",
		"  sed. `install.sh --check` reports them, and `install.sh` offers to",
		"  install the missing ones (`--assume-yes` installs them without asking).",
		"- **Feature** tools are needed only by the functions listed with them.",
		"  Without one, those functions stop with an error that names the tool and",
		"  the command that installs it on your system.",
		"- **Optional** tools add something to a function that works without them.",
		"",
		"`gash_doctor --prereqs` shows what this system has and lacks, with the",
		"install command of each missing tool. `gash_doctor --install` installs",
		"the missing required and feature tools after asking (`--all` adds the",
		"optional ones, or name the tools: `gash_doctor --install rsync zstd`).",
		"It never installs anything for an AI agent (`GASH_HEADLESS=1`): agents get",
		"the `prerequisite_missing` envelope described in [AGENTS.md](../AGENTS.md)",
		"and ask the user. `gash_doctor --json` gives the same report as JSON.",
		"",
		"In the tables, a dash means the system has no package for the tool (see",
		"its notes), \"built in\" means macOS ships it, and \"(EPEL)\" means RHEL,",
		"Rocky, AlmaLinux and Oracle Linux take it from EPEL, which `gash_doctor",
		"--install` enables first. Amazon Linux has no EPEL.",
	)
	for _, area := range Areas {
		var tools []*Tool
		for _, t := range Catalog() {
			if t.Area == area.Key {
				tools = append(tools, t)
			}
		}
		if len(tools) == 0 {
			continue
		}
		w("", "## "+area.Title, "")
		for _, t := range tools {
			w(toolLine(t))
		}
		var rows []*Tool
		for _, t := range tools {
			if !t.NoInstall {
				rows = append(rows, t)
			}
		}
		if len(rows) == 0 {
			continue
		}
		w("")
		head := "| Tool |"
		rule := "| --- |"
		for _, m := range Managers {
			head += " " + managerTitles[m] + " |"
			rule += " --- |"
		}
		w(head, rule)
		for _, t := range rows {
			row := "| " + t.Name + " |"
			for _, m := range Managers {
				row += " " + packageCell(t, m) + " |"
			}
			w(row)
		}
	}
	return b.String()
}

// toolLine is the list entry of a tool: name, level, purpose, the
// programs that serve it, where it applies, who needs it and its notes.
func toolLine(t *Tool) string {
	s := "- **" + t.Name + "** (" + t.Level.String() + ") " + strings.TrimSuffix(t.Purpose, ".") + "."
	if len(t.Commands) > 1 {
		s += " Any of: `" + strings.Join(t.Commands, "`, `") + "`."
	}
	if len(t.OS) > 0 {
		s += " Only on " + strings.Join(osTitles(t.OS), " and ") + "."
	}
	if u := Users(t.Name); len(u) > 0 {
		s += " Needed by " + joinUsers(u) + "."
	}
	if t.MinVersion != "" && !strings.Contains(t.Purpose, t.MinVersion) {
		s += " Version " + t.MinVersion + " or newer."
	}
	switch {
	case t.NoInstall && t.Manual != "":
		s += " Gash does not install it: see <" + t.Manual + ">."
	case t.NoInstall && t.Note == "":
		s += " Gash does not install it."
	case t.Manual != "":
		s += " Where no package exists: <" + t.Manual + ">."
	}
	if t.NonFree {
		s += " Its package is in the non-free repositories (Debian non-free, Ubuntu multiverse, openSUSE non-oss)."
	}
	if t.Note != "" {
		s += " " + strings.ToUpper(t.Note[:1]) + strings.TrimSuffix(t.Note[1:], ".") + "."
	}
	return s
}

func osTitles(list []string) []string {
	out := make([]string, len(list))
	for i, o := range list {
		switch o {
		case "linux":
			out[i] = "Linux"
		case "darwin":
			out[i] = "macOS"
		case "wsl":
			out[i] = "WSL"
		default:
			out[i] = o
		}
	}
	return out
}

// joinUsers is "`a`, `b` and `c`", each function in code; a qualifier in
// parentheses stays outside the code span.
func joinUsers(u []string) string {
	parts := make([]string, len(u))
	for i, x := range u {
		if strings.HasPrefix(x, "every ") {
			parts[i] = strings.Replace(x, "--host", "`--host`", 1)
			continue
		}
		name, rest, _ := strings.Cut(x, " ")
		if name == "gash" {
			// A "gash WORD" command.
			word, more, _ := strings.Cut(rest, " ")
			name, rest = "gash "+word, more
		}
		parts[i] = "`" + name + "`"
		if rest != "" {
			parts[i] += " " + rest
		}
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

// packageCell is the table cell of a tool for a manager.
func packageCell(t *Tool, m Manager) string {
	if m == Brew && contains(t.Builtin, "darwin") {
		return "built in"
	}
	pkg := t.Packages[m]
	if pkg == "" {
		return "—"
	}
	cell := "`" + pkg + "`"
	if m == Dnf && t.EPEL {
		cell += " (EPEL)"
	}
	if t.NonFree && m != Pacman && m != Apk && m != Brew {
		cell += " (non-free)"
	}
	return cell
}
