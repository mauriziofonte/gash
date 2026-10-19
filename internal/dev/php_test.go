// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package dev

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
	require.Empty(t, r.err)
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

// TestLedger_DEV01_2x: no installed check (a pinned version absent from
// the host is reported without a warning), the probe order kept.
func TestLedger_DEV01_2x(t *testing.T) {
	root := phys(t)
	m := devMod(t, root)
	proj := phys(t)
	mkfile(t, filepath.Join(proj, ".target-php-version"), "8.1\n")
	env := baseEnv(phys(t), nil)

	// No /usr/bin/php8.1 or /usr/bin/php8 in the sysroot: php_raw_binary
	// falls to the literal "php", php_command stays php81, no warning.
	r := call(m, "", env, "php_detect_version", proj, "--llm")
	require.Equal(t, phpJSON("8.1", "php", "php81", "", "composer"), r.out)
	require.Empty(t, r.err)

	// /usr/bin/php8.1 present: it is reported.
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
}

// TestLedger_DEV02_2x: an OR constraint resolves to the first X.Y of its
// text (the defect), not the highest alternative.
func TestLedger_DEV02_2x(t *testing.T) {
	m := devMod(t, phys(t))
	env := baseEnv(phys(t), nil)
	for _, c := range []struct{ constraint, want string }{
		{`^7.4|^8.0`, "7.4"},
		{`^8.0 || ^7.4`, "8.0"},
		{`>=7.2 <8.0`, "7.2"},
		{`~8.2.10`, "8.2"},
	} {
		proj := phys(t)
		mkfile(t, filepath.Join(proj, "composer.json"), `{"require":{"php":"`+c.constraint+`"}}`)
		require.Equal(t, c.want, version(call(m, "", env, "php_detect_version", proj, "--llm").out), c.constraint)
	}
}

// TestLedger_DEV03_2x: a major-only constraint holds no X.Y, so the
// source is skipped and the 8.4 default is reported.
func TestLedger_DEV03_2x(t *testing.T) {
	m := devMod(t, phys(t))
	env := baseEnv(phys(t), nil)
	for _, constraint := range []string{"^7", ">=8", "8.*", "*"} {
		proj := phys(t)
		mkfile(t, filepath.Join(proj, "composer.json"), `{"require":{"php":"`+constraint+`"}}`)
		require.Equal(t, "8.4", version(call(m, "", env, "php_detect_version", proj, "--llm").out), constraint)
	}
}

// TestLedger_DEV04_2x: every X.Y of line 1 of .target-php-version is
// joined with a newline (the defect).
func TestLedger_DEV04_2x(t *testing.T) {
	m := devMod(t, phys(t))
	env := baseEnv(phys(t), nil)
	for _, c := range []struct{ line, want string }{
		{"7.4 8.0\n", `7.4\n8.0`},
		{"8.2.15.3\n", `8.2\n15.3`},
	} {
		proj := phys(t)
		mkfile(t, filepath.Join(proj, ".target-php-version"), c.line)
		require.Equal(t, c.want, version(call(m, "", env, "php_detect_version", proj, "--llm").out), c.line)
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

// TestLedger_DEV18_fix would add a warning; rc.A keeps the 2.x silence
// (DEV-18 is an operator decision, deferred): a missing DIR answers 8.4,
// rc 0, no stderr.
func TestLedger_DEV18_2x(t *testing.T) {
	m := devMod(t, phys(t))
	env := baseEnv(phys(t), nil)
	r := call(m, "", env, "php_detect_version", "/does/not/exist", "--llm")
	require.Equal(t, "8.4", version(r.out))
	require.Empty(t, r.err)
	require.Equal(t, 0, r.rc)
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
