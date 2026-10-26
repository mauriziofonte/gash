// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/dev"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
)

// phpLintTree is ok.php, m80.php (match: a parse error before PHP 8.0) and
// bad.php (a parse error on line 3 in every version).
func phpLintTree(t *testing.T) string {
	t.Helper()
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "ok.php"), "<?php\necho 1;\n")
	mustWrite(t, filepath.Join(d, "m80.php"), "<?php echo match(1) { 1 => \"x\" };\n")
	mustWrite(t, filepath.Join(d, "bad.php"), "<?php\necho 1\necho 2;\n")
	return d
}

// fakePHP is a PATH directory whose php is testdata/fakephp/php.
func fakePHP(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs("testdata/fakephp/php")
	require.NoError(t, err)
	bin := toolsPath(t)
	require.NoError(t, os.Symlink(src, filepath.Join(bin, "php")))
	return bin
}

// TestLLMLintPHPVerdicts: php -l A B ... checks only A before PHP 8.3, so
// there each file gets its own run; from 8.3 one run checks them all. A
// file passes only on its own verdict line (3.4 before: a file php never
// checked passed, or took the first file's error with line 0).
func TestLLMLintPHPVerdicts(t *testing.T) {
	d := phpLintTree(t)
	m := New()
	m80 := `{"file":"m80.php","line":1,"error":"syntax error, unexpected '('"}`
	bad := `{"file":"bad.php","line":3,"error":"syntax error, unexpected token \"echo\", expecting \",\" or \";\""}`
	for _, tc := range []struct {
		name, version, firstOnly string
		args                     []string
		want                     string
		runs                     string
	}{
		{"7.4 ok first", "7.4.33", "", []string{"ok.php", "m80.php"},
			`{"checked":2,"failed":1,"failures":[` + m80 + `]}`, "ok.php\nm80.php\n"},
		{"7.4 one file", "7.4.33", "", []string{"m80.php"},
			`{"checked":1,"failed":1,"failures":[` + m80 + `]}`, "m80.php\n"},
		{"7.4 broken first", "7.4.33", "", []string{"bad.php", "m80.php"},
			`{"checked":2,"failed":2,"failures":[` + bad + `,` + m80 + `]}`, "bad.php\nm80.php\n"},
		{"8.2", "8.2.20", "", []string{"bad.php", "ok.php"},
			`{"checked":2,"failed":1,"failures":[` + bad + `]}`, "bad.php\nok.php\n"},
		{"8.3 batch", "8.3.6", "", []string{"bad.php", "m80.php", "ok.php"},
			`{"checked":3,"failed":1,"failures":[` + bad + `]}`, "bad.php m80.php ok.php\n"},
		// A php that says 8.3 but answers for the first file only: the
		// others are checked again, alone.
		{"8.3 first only", "8.3.6", "1", []string{"bad.php", "m80.php", "ok.php"},
			`{"checked":3,"failed":1,"failures":[` + bad + `]}`, "bad.php m80.php ok.php\nm80.php\nok.php\n"},
		// An unknown version is treated as old.
		{"no version", "unknown", "", []string{"ok.php", "bad.php"},
			`{"checked":2,"failed":1,"failures":[` + bad + `]}`, "ok.php\nbad.php\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := filepath.Join(physTemp(t), "runs")
			env := baseEnv(physTemp(t), map[string]string{"PATH": fakePHP(t), "STUB_LOG": log,
				"FAKE_PHP_VERSION": tc.version, "FAKE_PHP_FIRST_ONLY": tc.firstOnly})
			r := runGo(t, m, d, env, "llm_lint", append(tc.args, "-j", "1")...)
			require.Equal(t, result{tc.want + "\n", "", 0}, r)
			runs, err := os.ReadFile(log)
			require.NoError(t, err)
			require.Equal(t, tc.runs, strings.ReplaceAll(string(runs), d+"/", ""))
		})
	}
}

// TestLLMLintPHPInstalled: every php installed here gives each file its
// own verdict: m80.php fails before 8.0 only, never with bad.php's error.
func TestLLMLintPHPInstalled(t *testing.T) {
	d := phpLintTree(t)
	m := New()
	env := baseEnv(physTemp(t), nil)
	list := dev.NewResolver(context.Background(), invoke.EnvMap(env), runner.Exec{}, false).Installed("php")
	if len(list) == 0 {
		t.Skip("php not installed")
	}
	for _, p := range list {
		major, rest, _ := strings.Cut(p.Version, ".")
		minor, _, _ := strings.Cut(rest, ".")
		t.Run(p.Version, func(t *testing.T) {
			o := parseLint(t, runGo(t, m, d, env, "llm_lint", "bad.php", "m80.php", "ok.php", "--php", major+"."+minor))
			require.Equal(t, 3, o.Checked)
			require.Equal(t, "bad.php", o.Failures[0].File)
			require.Equal(t, 3, o.Failures[0].Line)
			if major < "8" {
				require.Equal(t, 2, o.Failed, "%+v", o)
				require.Equal(t, "m80.php", o.Failures[1].File)
				require.Equal(t, 1, o.Failures[1].Line)
				require.NotEqual(t, o.Failures[0].Error, o.Failures[1].Error)
			} else {
				require.Equal(t, 1, o.Failed, "%+v", o)
			}
		})
	}
}

// TestLLMLintNoVerdict: a python that prints nothing passes no file: each
// is checked again alone, then fails naming itself (3.4 before: every
// file without a line passed).
func TestLLMLintNoVerdict(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "a.py"), "x = 1\n")
	mustWrite(t, filepath.Join(d, "b.py"), "y = 2\n")
	bin := stubBin(t, map[string]string{"python3": `printf '%s\n' "${@: -1}" >>"$STUB_LOG"`})
	log := filepath.Join(physTemp(t), "runs")
	env := baseEnv(physTemp(t), map[string]string{"PATH": bin + ":" + toolsPath(t), "STUB_LOG": log})
	r := runGo(t, New(), d, env, "llm_lint", "a.py", "b.py", "-j", "1")
	require.Equal(t, result{`{"checked":2,"failed":2,"failures":[` +
		`{"file":"a.py","line":0,"error":"python3 gave no verdict for a.py"},` +
		`{"file":"b.py","line":0,"error":"python3 gave no verdict for b.py"}]}` + "\n", "", 0}, r)
	runs, err := os.ReadFile(log)
	require.NoError(t, err)
	require.Equal(t, "b.py\na.py\nb.py\n", strings.ReplaceAll(string(runs), d+"/", ""))
}
