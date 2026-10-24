// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package dev

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
)

// reNodeVer is grep -oE '[0-9]+(\.[0-9]+)*'.
var reNodeVer = regexp.MustCompile(`[0-9]+(\.[0-9]+)*`)

// reLTSNum is the Bash test ^v?([0-9]+) of an nvm alias file: an optional
// leading v and the first run of digits.
var reLTSNum = regexp.MustCompile(`^v?([0-9]+)`)

// ltsTable is the built-in codename → major map, consulted after the nvm
// alias files (lower-cased names).
var ltsTable = map[string]string{
	"krypton": "24", "jod": "22", "iron": "20",
	"hydrogen": "18", "gallium": "16", "fermium": "14",
}

// nvmUpgradeMsg names the nvm installer, as node_detect_version does when
// NVM_DIR is missing.
const nvmUpgradeMsg = "NVM directory not found (%s). Install NVM: curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash"

// nvmDir is ${NVM_DIR:-$HOME/.nvm}.
func nvmDir(c *invoke.Call) string {
	if d := c.Env.Get("NVM_DIR"); d != "" {
		return d
	}
	return filepath.Join(c.Env.Get("HOME"), ".nvm")
}

// nodeUpgrade is node_upgrade run outside the shim: help, or the missing
// nvm (its check runs after the options; the options are parsed first so
// that a bad one is reported). The generated shim runs the function in
// the caller's shell for every nvm call (LEDGER DEV-16).
func (m *Module) nodeUpgrade(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	for _, a := range c.Args {
		switch a {
		case "-y", "--yes", "--force", "--dry-run":
		default:
			printer(c).Error("Unknown option: " + a + " (usage: node_upgrade [-y] [--dry-run])")
			return 1
		}
	}
	printer(c).Error(nvmMissing)
	return 1
}

// nodeDetect is node_detect_version [project_dir] [--json|--llm].
func (m *Module) nodeDetect(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	// The arguments come first, then the environment (LEDGER DEV-20).
	projectDir, llm := ".", headless(c)
	for _, a := range c.Args {
		switch {
		case a == "--json" || a == "--llm":
			llm = true
		case strings.HasPrefix(a, "-"):
			printer(c).Error("Unknown option: " + a)
			return 1
		default:
			projectDir = a
		}
	}
	dir := nvmDir(c)
	if !isDir(dir) {
		printer(c).Error(fmt.Sprintf(nvmUpgradeMsg, dir))
		return 1
	}
	if !isDir(projectDir) {
		// Same answer and rc 0, with a warning (LEDGER DEV-18).
		printer(c).Warning("Directory not found: " + projectDir)
	}

	major := m.nodeMajor(c, dir, projectDir)
	full := m.latestForMajor(dir, major)
	if full == "" {
		latest := m.latestInstalled(dir)
		if latest == "" {
			printer(c).Error("No Node.js versions installed in NVM. Install: nvm install 20")
			return 1
		}
		printer(c).Warning("Node " + major + " not installed. Using " + latest + ". Run: nvm install " + major)
		full = latest
		major, _, _ = strings.Cut(full, ".")
	}

	nodeCmd := "node" + major
	npmCmd := "npm" + major
	npxCmd := "npx" + major

	if llm {
		var b []byte
		b = append(b, `{"version":`...)
		b = contract.AppendString(b, major, contract.EscapeBash)
		b = append(b, `,"full_version":`...)
		b = contract.AppendString(b, full, contract.EscapeBash)
		b = append(b, `,"node_command":`...)
		b = contract.AppendString(b, nodeCmd, contract.EscapeBash)
		b = append(b, `,"npm_command":`...)
		b = contract.AppendString(b, npmCmd, contract.EscapeBash)
		b = append(b, `,"npx_command":`...)
		b = contract.AppendString(b, npxCmd, contract.EscapeBash)
		b = append(b, "}\n"...)
		_, _ = c.Stdout.Write(b)
		return 0
	}
	printer(c).Info("Node.js version for '" + projectDir + "': " + full + " (major " + major + ")")
	m.style(c).KV(c.Stdout, "--indent", "2", "--width", "15", "--value-color", "accent",
		"node_command", nodeCmd,
		"npm_command", npmCmd,
		"npx_command", npxCmd)
	return 0
}

// nodeMajor is __node_detect_version_major: .nvmrc, then .node-version,
// then package.json engines.node, then the newest installed major (20
// when none).
func (m *Module) nodeMajor(c *invoke.Call, dir, projectDir string) string {
	for _, name := range []string{".nvmrc", ".node-version"} {
		f := filepath.Join(projectDir, name)
		if !isFile(f) {
			continue
		}
		content := trimSpaceAll(firstLine(headLine(f)))
		if content == "" {
			continue
		}
		if major := m.parseToMajor(dir, content); major != "" {
			return major
		}
	}
	if pkg := filepath.Join(projectDir, "package.json"); isFile(pkg) {
		if data, err := m.readFile(pkg); err == nil {
			engines := subst(jqPathRaw(data, "engines", "node"))
			if engines != "" {
				if major := m.parseToMajor(dir, engines); major != "" {
					return major
				}
			}
		}
	}
	if latest := m.latestInstalled(dir); latest != "" {
		major, _, _ := strings.Cut(latest, ".")
		return major
	}
	return "20"
}

// parseToMajor is __node_parse_version_to_major: an lts/<name> alias
// resolved through nvm's alias files then the codename table, or the
// major of the first version-like run of a plain string.
func (m *Module) parseToMajor(dir, s string) string {
	if strings.HasPrefix(s, "lts/") {
		return m.ltsMajor(dir, strings.TrimPrefix(s, "lts/"))
	}
	v := reNodeVer.FindString(s)
	if v == "" {
		return ""
	}
	major, _, _ := strings.Cut(v, ".")
	return major
}

// ltsMajor resolves lts/<name>: the alias file $NVM_DIR/alias/lts/<name>
// (lower-cased, no slash) when its first line is v?NUMBER, then the
// built-in codename table, then the newest installed major.
func (m *Module) ltsMajor(dir, name string) string {
	if !strings.Contains(name, "/") {
		aliasFile := filepath.Join(dir, "alias", "lts", strings.ToLower(name))
		if target := firstLine(headLine(aliasFile)); target != "" {
			target = strings.TrimRight(target, "\n")
			if mm := reLTSNum.FindStringSubmatch(target); mm != nil {
				return mm[1]
			}
		}
	}
	if major, ok := ltsTable[strings.ToLower(name)]; ok {
		return major
	}
	latest := m.latestInstalled(dir)
	major, _, _ := strings.Cut(latest, ".")
	return major
}

// reRelease is an installed Node release directory, as node_upgrade
// keeps them: vX.Y.Z.
var reRelease = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

// installedVersions is __node_installed_versions: the vX.Y.Z entries of
// $NVM_DIR/versions/node with the v stripped, in sort -V order; a stray
// name or a prerelease is not an installed version (LEDGER DEV-05).
func (m *Module) installedVersions(dir string) []string {
	entries, err := os.ReadDir(filepath.Join(dir, "versions", "node"))
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if reRelease.MatchString(e.Name()) {
			names = append(names, strings.TrimPrefix(e.Name(), "v"))
		}
	}
	return sortV(names)
}

// latestForMajor is __node_latest_for_major: the newest installed version
// of a major.
func (m *Module) latestForMajor(dir, major string) string {
	var last string
	for _, v := range m.installedVersions(dir) {
		if strings.HasPrefix(v, major+".") {
			last = v
		}
	}
	return last
}

// latestInstalled is __node_latest_installed.
func (m *Module) latestInstalled(dir string) string {
	v := m.installedVersions(dir)
	if len(v) == 0 {
		return ""
	}
	return v[len(v)-1]
}
