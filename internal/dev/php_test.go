// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package dev

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/registry"
)

// phpJSON is the one-line JSON php_detect_version prints.
func phpJSON(version, phpRaw, phpCmd, composerRaw, composerCmd string) string {
	return `{"version":"` + version + `","php_raw_binary":"` + phpRaw +
		`","php_command":"` + phpCmd + `","composer_raw_binary":"` + composerRaw +
		`","composer_command":"` + composerCmd + `"}` + "\n"
}

// TestPHPDetect pins the sources, the probe order and the JSON contract.
func TestPHPDetect(t *testing.T) {
	m := devMod(t, phys(t))
	proj := phys(t)
	env := baseEnv(phys(t), nil)

	// .target-php-version wins over composer.json.
	mkfile(t, filepath.Join(proj, ".target-php-version"), "8.1\n")
	mkfile(t, filepath.Join(proj, "composer.json"), `{"require":{"php":"^8.3"}}`)
	r := call(m, "", env, "php_detect_version", proj, "--llm")
	require.Equal(t, phpJSON("8.1", "php", "php81", "", "composer"), r.out)
	// 3.0: DEV-01 fixed: the missing PHP 8.1 is named on stderr.
	require.Equal(t, "Warning: PHP 8.1 is not installed; php_raw_binary falls back to php\n", r.err)
	require.Equal(t, 0, r.rc)

	// Walk-up to a parent's .target-php-version.
	proj2 := phys(t)
	mkfile(t, filepath.Join(proj2, ".target-php-version"), "7.4\n")
	require.NoError(t, os.MkdirAll(filepath.Join(proj2, "sub", "deep"), 0o755))
	r = call(m, "", env, "php_detect_version", filepath.Join(proj2, "sub", "deep"), "--llm")
	require.Equal(t, "7.4", version(r.out))

	// composer.json .require.php, then the 8.4 default.
	proj3 := phys(t)
	mkfile(t, filepath.Join(proj3, "composer.json"), `{"require":{"php":"^8.2"}}`)
	require.Equal(t, "8.2", version(call(m, "", env, "php_detect_version", proj3, "--llm").out))
	require.Equal(t, "8.4", version(call(m, "", env, "php_detect_version", phys(t), "--llm").out))

	// JSON implied by GASH_HEADLESS without --json; human text otherwise.
	require.Equal(t, "8.4", version(call(m, "", env, "php_detect_version", phys(t)).out))
	human := call(m, "", baseEnv(phys(t), map[string]string{"GASH_HEADLESS": ""}), "php_detect_version", phys(t))
	require.Contains(t, human.err, "PHP version for")
	require.Contains(t, human.out, "php_raw_binary")
	require.NotContains(t, human.out, `"version"`)
}

// 3.0: DEV-01 fixed. A version whose binary is not installed is named in
// a warning on stderr; stdout (the JSON) is the 2.x one, rc 0.
func TestLedger_DEV01_fix(t *testing.T) {
	root := phys(t)
	m := devMod(t, root)
	proj := phys(t)
	mkfile(t, filepath.Join(proj, ".target-php-version"), "8.1\n")
	env := baseEnv(phys(t), nil)

	// No /usr/bin/php8.1 or /usr/bin/php8 in the sysroot: php_raw_binary
	// falls to the literal "php", php_command stays php81.
	r := call(m, "", env, "php_detect_version", proj, "--llm")
	require.Equal(t, phpJSON("8.1", "php", "php81", "", "composer"), r.out)
	require.Equal(t, "Warning: PHP 8.1 is not installed; php_raw_binary falls back to php\n", r.err)
	require.Equal(t, 0, r.rc)

	// The human mode warns too.
	human := call(m, "", baseEnv(phys(t), map[string]string{"GASH_HEADLESS": ""}), "php_detect_version", proj)
	require.Contains(t, human.err, "Warning: PHP 8.1 is not installed;")

	// /usr/bin/php8.1 present: it is reported, no warning.
	mkexe(t, filepath.Join(root, "usr", "bin", "php8.1"))
	r = call(m, "", env, "php_detect_version", proj, "--llm")
	require.Equal(t, phpJSON("8.1", "/usr/bin/php8.1", "php81", "", "composer"), r.out)
	require.Empty(t, r.err)

	// Only the major probe matches: /usr/bin/php8 for 8.1.
	root2 := phys(t)
	m2 := devMod(t, root2)
	mkexe(t, filepath.Join(root2, "usr", "bin", "php8"))
	r = call(m2, "", env, "php_detect_version", proj, "--llm")
	require.Equal(t, phpJSON("8.1", "/usr/bin/php8", "php81", "", "composer"), r.out)
	require.Equal(t, "Warning: PHP 8.1 is not installed; using /usr/bin/php8\n", r.err)
}

// 3.0: DEV-02 fixed. An OR constraint resolves to its highest alternative
// that is installed, else its highest alternative.
func TestLedger_DEV02_fix(t *testing.T) {
	root := phys(t)
	m := devMod(t, root)
	env := baseEnv(phys(t), nil)
	detect := func(m *Module, constraint string) string {
		proj := phys(t)
		mkfile(t, filepath.Join(proj, "composer.json"), `{"require":{"php":"`+constraint+`"}}`)
		return version(call(m, "", env, "php_detect_version", proj, "--llm").out)
	}
	for _, c := range []struct{ constraint, want string }{
		{`^7.4|^8.0`, "8.0"},
		{`^8.0 || ^7.4`, "8.0"},
		{`>=7.2 <8.0`, "7.2"},
		{`~8.2.10`, "8.2"},
		{`^7.4 || ^8.10 || ^8.2`, "8.10"},
	} {
		require.Equal(t, c.want, detect(m, c.constraint), c.constraint)
	}
	// Only php7.4 installed: the installed alternative wins.
	mkexe(t, filepath.Join(root, "usr", "bin", "php7.4"))
	require.Equal(t, "7.4", detect(m, `^7.4|^8.0`))
}

// 3.0: DEV-03 fixed. A bare major is X.0; * alone has no bound and falls
// through to the next source.
func TestLedger_DEV03_fix(t *testing.T) {
	m := devMod(t, phys(t))
	env := baseEnv(phys(t), nil)
	for _, c := range []struct{ constraint, want string }{
		{"^7", "7.0"}, {">=8", "8.0"}, {"8.*", "8.0"}, {"*", "8.4"}, {"^7 || ^8", "8.0"},
	} {
		proj := phys(t)
		mkfile(t, filepath.Join(proj, "composer.json"), `{"require":{"php":"`+c.constraint+`"}}`)
		require.Equal(t, c.want, version(call(m, "", env, "php_detect_version", proj, "--llm").out), c.constraint)
	}
}

// 3.0: DEV-04 fixed. Only the first X.Y of line 1 of .target-php-version
// counts.
func TestLedger_DEV04_fix(t *testing.T) {
	m := devMod(t, phys(t))
	env := baseEnv(phys(t), nil)
	for _, c := range []struct{ line, want string }{
		{"7.4 8.0\n", "7.4"},
		{"8.2.15.3\n", "8.2"},
	} {
		proj := phys(t)
		mkfile(t, filepath.Join(proj, ".target-php-version"), c.line)
		r := call(m, "", env, "php_detect_version", proj, "--llm")
		require.Equal(t, c.want, version(r.out), c.line)
		require.Equal(t, "php"+strings.ReplaceAll(c.want, ".", ""), field(r.out, "php_command"), c.line)
	}
}

// TestLedger_DEV09_2x: the copilot-instructions source, read as the
// PCRE2 >= 10.43 expression intended.
func TestLedger_DEV09_2x(t *testing.T) {
	m := devMod(t, phys(t))
	env := baseEnv(phys(t), nil)
	copilot := func(body string) string {
		proj := phys(t)
		mkfile(t, filepath.Join(proj, ".github", "copilot-instructions.md"), body)
		return version(call(m, "", env, "php_detect_version", proj, "--llm").out)
	}
	require.Equal(t, "8.2", copilot("This project targets PHP 8.2 on Laravel 11.\n"))
	require.Equal(t, "8.3", copilot("Runtime: php:8.3-fpm\n"))
	require.Equal(t, "8.4", copilot("Uses PHPUnit 10.5 for tests.\n")) // no space/colon: PHPUnit is not php<X.Y>
	require.Equal(t, "8.4", copilot("php     8.2\n"))                  // four spaces: outside {0,3}
	require.Equal(t, "8.4", copilot("php\n8.2\n"))                     // different lines

	// Its slot: after composer.json, before 8.4.
	proj := phys(t)
	mkfile(t, filepath.Join(proj, ".github", "copilot-instructions.md"), "PHP 8.2\n")
	mkfile(t, filepath.Join(proj, "composer.json"), `{"require":{"php":"^8.1"}}`)
	require.Equal(t, "8.1", version(call(m, "", env, "php_detect_version", proj, "--llm").out))
	proj2 := phys(t)
	mkfile(t, filepath.Join(proj2, ".github", "copilot-instructions.md"), "PHP 8.2\n")
	mkfile(t, filepath.Join(proj2, ".target-php-version"), "7.4\n")
	require.Equal(t, "7.4", version(call(m, "", env, "php_detect_version", proj2, "--llm").out))
}

// TestLedger_DEV10_2x: composer.json read as jq 1.7 read it (number text
// kept, a non-string rendered then searched), with or without jq.
func TestLedger_DEV10_2x(t *testing.T) {
	m := devMod(t, phys(t))
	env := baseEnv(phys(t), nil)
	comp := func(require_ string) string {
		proj := phys(t)
		mkfile(t, filepath.Join(proj, "composer.json"), require_)
		return version(call(m, "", env, "php_detect_version", proj, "--llm").out)
	}
	require.Equal(t, "8.10", comp(`{"require":{"php":8.10}}`))
	require.Equal(t, "7.3", comp(`{"require":{"php":{"v":"7.3"}}}`))
	require.Equal(t, "8.4", comp(`{"require":"x"}`)) // non-object require → jq error → fall through
	require.Equal(t, "8.4", comp(`{bad json`))       // invalid JSON → skipped
}

// TestLedger_DEV11_2x: a relative DIR is resolved against the logical
// cwd, never through CDPATH; a missing relative DIR skips the walk-up.
func TestLedger_DEV11_2x(t *testing.T) {
	m := devMod(t, phys(t))
	home := phys(t)
	mkfile(t, filepath.Join(home, ".target-php-version"), "7.4\n")
	require.NoError(t, os.MkdirAll(filepath.Join(home, "proj"), 0o755))
	// cwd is home; "proj" resolves to home/proj, walk-up finds home's
	// file. CDPATH in the environment has no effect.
	env := baseEnv(home, map[string]string{"CDPATH": "/nowhere", "PWD": home})
	require.Equal(t, "7.4", version(call(m, home, env, "php_detect_version", "proj", "--llm").out))

	// A missing relative DIR: no walk-up (the while loop needs an
	// absolute path), so the cwd's file is not read → 8.4.
	env2 := baseEnv(home, map[string]string{"PWD": filepath.Join(home, "proj")})
	require.Equal(t, "8.4", version(call(m, filepath.Join(home, "proj"), env2, "php_detect_version", "missing", "--llm").out))

	// A missing absolute DIR walks up from the literal path → 8.4.
	require.Equal(t, "8.4", version(call(m, "", env, "php_detect_version", "/does/not/exist", "--llm").out))
}

// TestLedger_DEV12_2x: the binaries come from PATH (LookPath), not a hash
// table; the probe order holds.
func TestLedger_DEV12_2x(t *testing.T) {
	m := devMod(t, phys(t))
	bin := phys(t)
	mkexe(t, filepath.Join(bin, "php"))
	mkexe(t, filepath.Join(bin, "composer"))
	env := baseEnv(phys(t), map[string]string{"PATH": bin})
	proj := phys(t)
	r := call(m, "", env, "php_detect_version", proj, "--llm")
	require.Equal(t, phpJSON("8.4", filepath.Join(bin, "php"), "php84", filepath.Join(bin, "composer"), "composer84"), r.out)
}

// 3.0: DEV-18 fixed (operator: keep stdout and rc 0, add the warning). A
// missing DIR answers as 2.x did, with `Warning: Directory not found:
// DIR` on stderr, in both functions.
func TestLedger_DEV18_fix(t *testing.T) {
	m := devMod(t, phys(t))
	env := baseEnv(phys(t), nil)
	r := call(m, "", env, "php_detect_version", "/does/not/exist", "--llm")
	require.Equal(t, "8.4", version(r.out))
	require.True(t, strings.HasPrefix(r.err, "Warning: Directory not found: /does/not/exist\n"), r.err)
	require.Equal(t, 0, r.rc)

	nvm := nvmTree(t, "v20.11.1")
	r = call(m, "", baseEnv(phys(t), map[string]string{"NVM_DIR": nvm}), "node_detect_version", "/does/not/exist", "--llm")
	require.Equal(t, "20", field(r.out, "version"))
	require.Equal(t, "Warning: Directory not found: /does/not/exist\n", r.err)
	require.Equal(t, 0, r.rc)

	// An existing DIR: no warning.
	r = call(m, "", env, "php_detect_version", phys(t), "--llm")
	require.NotContains(t, r.err, "Directory not found")
}

// version is the .version field of the one-line JSON.
func version(out string) string {
	const key = `"version":"`
	i := strings.Index(out, key)
	if i < 0 {
		return ""
	}
	rest := out[i+len(key):]
	return rest[:strings.Index(rest, `"`)]
}

// TestLedger_DEV21 proves the php alias table of docs/runtimes/reference.md
// lists every alias the php pack defines, each with its condition, and
// none of the dropped hte aliases. (TestRegistry_AliasDocsMatchConditions
// in internal/registry is outside this slice; the `gash dev` card is
// generated elsewhere.)
func TestLedger_DEV21(t *testing.T) {
	_, self, _, _ := runtime.Caller(0) // call() changes the working directory
	data, err := os.ReadFile(filepath.Join(filepath.Dir(self), "..", "..", "docs", "runtimes", "reference.md"))
	require.NoError(t, err)
	doc := string(data)
	i := strings.Index(doc, "### The php pack\n")
	require.GreaterOrEqual(t, i, 0)
	rows := map[string]string{}
	for _, line := range strings.Split(doc[i:], "\n") {
		cells := strings.Split(line, "|")
		if len(cells) != 5 || !strings.HasPrefix(strings.TrimSpace(cells[1]), "`") {
			continue
		}
		name := strings.Trim(strings.Fields(strings.TrimSpace(cells[1]))[0], "`")
		rows[name] = strings.TrimSpace(cells[3])
	}
	want := map[string]string{
		"phpXY":                    "for each installed phpX.Y",
		"composerXY":               "for each installed phpX.Y, with composer",
		"1composerXY":              "for each installed phpX.Y up to 7.2, with composer1",
		"php":                      "with GASH_REPLACE_TOOLS=1",
		"composer":                 "with GASH_REPLACE_TOOLS=1",
		"composer-packages-update": "when composer is installed",
		"composer-self-update":     "when composer is installed",
		"composer1-self-update":    "when composer1 is installed",
	}
	require.Equal(t, want, rows)

	// Every php-pack alias of the registry has its row (loop names as
	// their family), except the hte aliases 3.0 dropped (DEV-17).
	for _, a := range registry.Aliases() {
		if a.Pack != "php" {
			continue
		}
		name := strings.ReplaceAll(strings.Trim(a.Name, `"`), "${__gash_v//./}", "XY")
		if strings.HasPrefix(name, "hte") {
			require.NotContains(t, rows, name, "dropped alias %s has a row", name)
			continue
		}
		require.Contains(t, rows, name, "registry alias %s has no row", a.Name)
	}
}

// TestLedger_DEV22 proves the php_detect_version and node_detect_version
// help NOTES state how far detection reaches and what it checks.
// (TestRegistry_HelpGolden is not in internal/registry; this pins the
// rules in the owning package.)
func TestLedger_DEV22(t *testing.T) {
	for name, rules := range map[string][]string{
		"php_detect_version": {
			"then in every parent directory except /",
			"composer.json and copilot-instructions.md are read in project_dir only",
			"/usr/bin/phpX.Y is not installed, a warning",
			"Warning: Directory not found: DIR",
			"--json (or --llm)",
		},
		"node_detect_version": {
			"read in project_dir only (no walk-up)",
			"reduced to its major",
			"When the major is not installed, a warning names it and the newest installed version is used",
			"prereleases and other names do not count",
			"Warning: Directory not found: DIR",
			"--json (or --llm)",
		},
	} {
		f, ok := registry.Lookup(name)
		require.True(t, ok, name)
		body := strings.Join(strings.Fields(f.Help.Body), " ")
		for _, r := range rules {
			require.Contains(t, body, r, name)
		}
		require.Equal(t, name+" [project_dir] [--json|--llm]", f.Fallback.Usage, name)
	}
}
