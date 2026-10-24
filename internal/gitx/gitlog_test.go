// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

// logScene runs one gl-family handler with a fake git and reports the stdout,
// stderr, exit status and the git argv it would have run.
type logScene struct {
	out, err string
	rc       int
	gitArgv  []string
}

// runLog answers `git config --get log.graphColors` (configured iff cfgOK)
// and any `git ... log ...` with success, and records the log argv.
func runLog(t *testing.T, env map[string]string, cfgOK bool, fn string, args ...string) logScene {
	t.Helper()
	fake := runner.NewFake(t)
	var scene logScene
	fake.Respond(func(c runner.Call) (runner.Expect, bool) {
		if len(c.Argv) < 2 || c.Argv[0] != "git" {
			return runner.Expect{}, false
		}
		rest := c.Argv[1:]
		if len(rest) >= 3 && rest[0] == "config" && rest[1] == "--get" && rest[2] == "log.graphColors" {
			if cfgOK {
				return runner.Expect{Stdout: "39,245\n", Status: 0}, true
			}
			return runner.Expect{Status: 1}, true
		}
		for _, a := range rest {
			if a == "log" {
				scene.gitArgv = append([]string(nil), c.Argv...)
				return runner.Expect{Stdout: "<log>\n", Status: 0}, true
			}
		}
		return runner.Expect{}, false
	})
	e := invoke.EnvMap(env)
	var out, errb bytes.Buffer
	c := &invoke.Call{
		Name: fn, Args: args, Env: e,
		Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, true, true),
	}
	tbl := invoke.NewTable()
	(&Module{Run: fake}).Register(tbl)
	scene.rc = tbl.Run(c)
	scene.out, scene.err = out.String(), errb.String()
	return scene
}

func envColor() map[string]string   { return map[string]string{"PATH": "/usr/bin:/bin", "HOME": "/h"} }
func envNoColor() map[string]string { m := envColor(); m["NO_COLOR"] = "1"; return m }

// TestShellInit_GitLog_glg proves glg runs the first-parent graph with the
// Gash palette injected through log.graphColors when the user has not set it.
func TestShellInit_GitLog_glg(t *testing.T) {
	s := runLog(t, envColor(), false, "glg", "-5")
	require.Equal(t, 0, s.rc)
	require.Equal(t, []string{
		"git", "-c", "log.graphColors=39,245", "log",
		"--graph", "--pretty=format:" + gitLogFormat, "--abbrev-commit", "--first-parent", "-5",
	}, s.gitArgv)

	// When the user already set log.graphColors, Gash does not inject it.
	s2 := runLog(t, envColor(), true, "glg")
	require.Equal(t, []string{
		"git", "log", "--graph", "--pretty=format:" + gitLogFormat, "--abbrev-commit", "--first-parent",
	}, s2.gitArgv)

	// NO_COLOR forces --no-color right after log.
	s3 := runLog(t, envNoColor(), false, "glg")
	require.Equal(t, []string{
		"git", "log", "--no-color", "--graph", "--pretty=format:" + gitLogFormat, "--abbrev-commit", "--first-parent",
	}, s3.gitArgv)
}

// TestShellInit_GitLog_glf proves glf logs one file with patches, and without
// a file prints the usage and the note and returns 1.
func TestShellInit_GitLog_glf(t *testing.T) {
	s := runLog(t, envNoColor(), false, "glf", "src/main.go")
	require.Equal(t, 0, s.rc)
	require.Equal(t, []string{"git", "log", "--no-color", "--follow", "-p", "--", "src/main.go"}, s.gitArgv)

	// No file: usage and note on stderr, rc 1, no git run.
	nf := runLog(t, envColor(), false, "glf")
	require.Equal(t, 1, nf.rc)
	require.Nil(t, nf.gitArgv)
	require.Contains(t, nf.err, "glf <file> [options]")
	require.Contains(t, nf.err, "Shows commit history for a specific file with patches")
}

// The -h/--help flag of every gl variant prints the shared help and runs no
// git.
func TestGl_Help(t *testing.T)  { assertHelp(t, "gl") }
func TestGla_Help(t *testing.T) { assertHelp(t, "gla") }
func TestGlo_Help(t *testing.T) { assertHelp(t, "glo") }
func TestGlg_Help(t *testing.T) { assertHelp(t, "glg") }
func TestGls_Help(t *testing.T) { assertHelp(t, "gls") }
func TestGlf_Help(t *testing.T) { assertHelp(t, "glf") }

func assertHelp(t *testing.T, fn string) {
	t.Helper()
	for _, flag := range []string{"-h", "--help"} {
		s := runLog(t, envColor(), false, fn, flag)
		require.Equal(t, 0, s.rc, "%s %s", fn, flag)
		require.Nil(t, s.gitArgv, "%s %s ran git", fn, flag)
		require.True(t, strings.HasPrefix(s.out, "Git Log Aliases - Professional log visualization\n"), "%s %s help", fn, flag)
		require.Contains(t, s.out, "glf [file]")
	}
}
