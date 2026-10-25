// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package dev

import (
	"path/filepath"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/runner"
)

// reXY is grep -oE '[0-9]+\.[0-9]+'.
var reXY = lazyre.MustCompile(`[0-9]+\.[0-9]+`)

// reCopilot is the copilot-instructions source the 2.x grep -oP intended
// on PCRE2 >= 10.43: php, up to three spaces or colons, then X.Y
// (case-insensitive), the X.Y captured (LEDGER DEV-09).
var reCopilot = lazyre.MustCompile(`(?i)php[ \t\n\v\f\r:]{0,3}([0-9]+\.[0-9]+)`)

// phpDetect is php_detect_version [project_dir] [--json|--llm].
func (m *Module) phpDetect(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
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

	if !isDir(projectDir) {
		// A missing or unusable DIR keeps its answer and rc 0, with a
		// warning (LEDGER DEV-18).
		printer(c).Warning("Directory not found: " + projectDir)
	}
	version := m.phpVersionString(c, projectDir)
	major, _, _ := strings.Cut(version, ".")

	// Binary probes: /usr/bin/php<version>, /usr/bin/php<major>, then
	// `php` on PATH, then the literal "php" (LEDGER DEV-12: PATH, no
	// hash table). When the version's own binary is missing, a warning
	// says so; the JSON is unchanged (DEV-01).
	phpRaw := ""
	switch {
	case m.executable("/usr/bin/php" + version):
		phpRaw = "/usr/bin/php" + version
	case m.executable("/usr/bin/php" + major):
		phpRaw = "/usr/bin/php" + major
		printer(c).Warning("PHP " + version + " is not installed; using " + phpRaw)
	default:
		if p, err := runner.LookPath("php", c.Env.Child(), cwd(c)); err == nil {
			phpRaw = p
		} else {
			phpRaw = "php"
		}
		printer(c).Warning("PHP " + version + " is not installed; php_raw_binary falls back to " + phpRaw)
	}

	vernodot := strings.ReplaceAll(version, ".", "")
	phpCmd := "php" + vernodot

	composerRaw := ""
	switch p, err := runner.LookPath("composer", c.Env.Child(), cwd(c)); {
	case err == nil:
		composerRaw = p
	case m.executable("/usr/local/bin/composer"):
		composerRaw = "/usr/local/bin/composer"
	case m.executable("/usr/bin/composer"):
		composerRaw = "/usr/bin/composer"
	}
	composerCmd := "composer" + vernodot
	if composerRaw == "" {
		composerCmd = "composer"
	}

	if llm {
		var b []byte
		b = append(b, `{"version":`...)
		b = contract.AppendString(b, version, contract.EscapeBash)
		b = append(b, `,"php_raw_binary":`...)
		b = contract.AppendString(b, phpRaw, contract.EscapeBash)
		b = append(b, `,"php_command":`...)
		b = contract.AppendString(b, phpCmd, contract.EscapeBash)
		b = append(b, `,"composer_raw_binary":`...)
		b = contract.AppendString(b, composerRaw, contract.EscapeBash)
		b = append(b, `,"composer_command":`...)
		b = contract.AppendString(b, composerCmd, contract.EscapeBash)
		b = append(b, "}\n"...)
		_, _ = c.Stdout.Write(b)
		return 0
	}
	printer(c).Info("PHP version for '" + projectDir + "': " + version)
	composerShown := composerRaw
	if composerShown == "" {
		composerShown = "<none>"
	}
	m.style(c).KV(c.Stdout, "--indent", "2", "--width", "20", "--value-color", "accent",
		"php_raw_binary", phpRaw,
		"php_command", phpCmd,
		"composer_raw_binary", composerShown,
		"composer_command", composerCmd)
	return 0
}

// phpVersionString is __php_detect_version_string: .target-php-version
// (walk-up), then composer.json .require.php (constraintVersion), then
// .github/copilot-instructions.md, then 8.4.
func (m *Module) phpVersionString(c *invoke.Call, projectDir string) string {
	// The walk-up starts at the directory `cd "$project_dir" && pwd`
	// reaches (logical, CDPATH ignored: LEDGER DEV-11), or the literal
	// argument when it is not a directory.
	searchDir := projectDir
	if logical := m.logicalDir(c, projectDir); logical != "" {
		searchDir = logical
	}
	for strings.HasPrefix(searchDir, "/") && searchDir != "/" {
		f := filepath.Join(searchDir, ".target-php-version")
		if isFile(f) {
			// The first X.Y of line 1 (LEDGER DEV-04: 2.x joined every
			// match with a newline).
			if v := firstXY(firstLine(headLine(f))); v != "" {
				return v
			}
		}
		parent := gnuDirname(searchDir)
		if parent == searchDir {
			break
		}
		searchDir = parent
	}

	// composer.json .require.php, as jq -r prints it, read as a
	// constraint (LEDGER DEV-02, DEV-03).
	if composer := filepath.Join(projectDir, "composer.json"); isFile(composer) {
		if data, err := m.readFile(composer); err == nil {
			if v := m.constraintVersion(subst(jqPathRaw(data, "require", "php"))); v != "" {
				return v
			}
		}
	}

	// .github/copilot-instructions.md: first php X.Y (LEDGER DEV-09).
	if cop := filepath.Join(projectDir, ".github", "copilot-instructions.md"); isFile(cop) {
		if data, err := m.readFile(cop); err == nil {
			if v := firstCopilot(string(data)); v != "" {
				return v
			}
		}
	}
	return "8.4"
}

// reMajor is the first run of digits of a constraint alternative.
var reMajor = lazyre.MustCompile(`[0-9]+`)

// constraintVersion is the X.Y a composer constraint asks for (LEDGER
// DEV-02, DEV-03): the constraint split on || and |, each alternative's
// lower bound (its first X.Y; a bare major such as ^7, >=8 or 8.* is
// X.0; * alone has none), then the highest alternative whose
// /usr/bin/phpX.Y is installed, else the highest one; "" when no
// alternative has a bound.
func (m *Module) constraintVersion(constraint string) string {
	var bounds []string
	for _, alt := range strings.Split(strings.ReplaceAll(constraint, "||", "|"), "|") {
		if v := firstXY(alt); v != "" {
			bounds = append(bounds, v)
		} else if mj := reMajor.FindString(alt); mj != "" {
			bounds = append(bounds, mj+".0")
		}
	}
	best, bestInstalled := "", ""
	for _, v := range bounds {
		if best == "" || xyLess(best, v) {
			best = v
		}
		if m.executable("/usr/bin/php"+v) && (bestInstalled == "" || xyLess(bestInstalled, v)) {
			bestInstalled = v
		}
	}
	if bestInstalled != "" {
		return bestInstalled
	}
	return best
}

// xyLess compares two X.Y versions numerically.
func xyLess(a, b string) bool {
	am, an, _ := strings.Cut(a, ".")
	bm, bn, _ := strings.Cut(b, ".")
	if x, y := atoi(am), atoi(bm); x != y {
		return x < y
	}
	return atoi(an) < atoi(bn)
}

// atoi is the value of a digit string, saturating instead of
// overflowing.
func atoi(s string) int {
	n := 0
	for _, c := range s {
		if n > 1<<40 {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// firstXY is `grep -oE '[0-9]+\.[0-9]+' | head -1`: the first X.Y of s.
func firstXY(s string) string { return reXY.FindString(s) }

// firstCopilot is the first X.Y of a `php`-prefixed match, scanning line
// by line (grep is line based), the first submatch in file order.
func firstCopilot(s string) string {
	for _, l := range lines(s) {
		if m := reCopilot.FindStringSubmatch(l); m != nil {
			return m[1]
		}
	}
	return ""
}

// firstLine is the first line of s without its newline.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
