// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

// fakeProcs answers command -v from have and each start from out (keyed
// by argv, stdout), through a runner.Fake that expects the starts in ran.
type fakeProcs struct {
	have map[string]bool
	out  map[string]string
	ran  []string
}

func (f *fakeProcs) module(t *testing.T, passwd string, readErr error) *Module {
	var ex []runner.Expect
	for _, k := range f.ran {
		ex = append(ex, runner.Expect{Argv: strings.Fields(k), Stdout: f.out[k] + "\n"})
	}
	return &Module{
		run:        runner.NewFake(t, ex...),
		lookPathFn: func(name string) (string, bool) { return "/bin/" + name, f.have[name] },
		passwd:     "/etc/passwd",
		readFile:   func(string) ([]byte, error) { return []byte(passwd), readErr },
	}
}

func run(t *testing.T, m *Module, env map[string]string, args ...string) (string, int) {
	t.Helper()
	out, errOut, rc := runErr(m, env, args...)
	require.Empty(t, errOut)
	return out, rc
}

// runErr is run with the stderr of the call.
func runErr(m *Module, env map[string]string, args ...string) (string, string, int) {
	tbl := invoke.NewTable()
	m.Register(tbl)
	var out, errb bytes.Buffer
	e := invoke.EnvMap(env)
	c := &invoke.Call{Name: "gash_username", Args: args, Env: e, Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, false, false)}
	rc := tbl.Run(c)
	return out.String(), errb.String(), rc
}

func TestUsernameChain(t *testing.T) {
	cases := []struct {
		name   string
		procs  *fakeProcs
		passwd string
		want   string
		ran    []string
	}{
		{"whoami", &fakeProcs{have: map[string]bool{"whoami": true, "id": true}, out: map[string]string{"whoami": "alice"}}, "", "alice\n", []string{"whoami"}},
		{"id -un", &fakeProcs{have: map[string]bool{"whoami": true, "id": true}, out: map[string]string{"id -un": "bob"}}, "", "bob\n", []string{"whoami", "id -un"}},
		{"no whoami", &fakeProcs{have: map[string]bool{"id": true}, out: map[string]string{"id -un": "bob"}}, "", "bob\n", []string{"id -un"}},
		{"passwd", &fakeProcs{have: map[string]bool{"id": true}, out: map[string]string{"id -u": "1001"}},
			"root:x:0:0::/root:/bin/sh\ncarol:x:1001:1001::/home/carol:/bin/sh\n", "carol\n", []string{"id -un", "id -u"}},
		{"passwd two names", &fakeProcs{have: map[string]bool{"id": true}, out: map[string]string{"id -u": "7"}},
			"a:x:7:7::/:/bin/sh\nb:x:17:7::/:/bin/sh\nc:x:7:1::/:/bin/sh", "a\nc\n", []string{"id -un", "id -u"}},
		{"passwd no match", &fakeProcs{have: map[string]bool{"id": true}, out: map[string]string{"id -u": "5"}},
			"a:x:55:7::/:/bin/sh\n", "UNKNOWN_USERNAME\n", []string{"id -un", "id -u"}},
		// 3.4: without whoami and id, the passwd entry of the effective uid.
		{"no tools", &fakeProcs{}, "me:x:" + strconv.Itoa(os.Geteuid()) + ":1::/:/bin/sh\n", "me\n", nil},
		{"nothing", &fakeProcs{}, "", "UNKNOWN_USERNAME\n", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.procs.ran = tc.ran
			out, _, rc := runErr(tc.procs.module(t, tc.passwd, nil), nil)
			require.Equal(t, 0, rc)
			require.Equal(t, tc.want, out)
		})
	}
}

// 3.4: UNKNOWN_USERNAME comes with a warning that says why (it was
// silent).
func TestUsernameUnknownSaysWhy(t *testing.T) {
	uid := strconv.Itoa(os.Geteuid())
	for _, tc := range []struct {
		procs  *fakeProcs
		passwd string
		err    error
		want   string
	}{
		{&fakeProcs{}, "", nil, "Warning: Cannot tell the name of uid " + uid + ": whoami is not installed, id is not installed, and /etc/passwd has no entry for it.\n"},
		{&fakeProcs{have: map[string]bool{"whoami": true, "id": true}, out: map[string]string{"id -u": "5"}, ran: []string{"whoami", "id -un", "id -u"}},
			"a:x:55:7::/:/bin/sh\n", nil, "Warning: Cannot tell the name of uid 5: whoami knows no name for it, id knows no name for it, and /etc/passwd has no entry for it.\n"},
		{&fakeProcs{have: map[string]bool{"id": true}, out: map[string]string{"id -u": "0"}, ran: []string{"id -un", "id -u"}},
			"root:x:0:0::/root:/bin/sh\n", errors.New("denied"), "Warning: Cannot tell the name of uid 0: whoami is not installed, id knows no name for it, and /etc/passwd cannot be read.\n"},
		{&fakeProcs{have: map[string]bool{"id": true}, ran: []string{"id -un", "id -u"}},
			"", nil, "Warning: Cannot tell the name of the current user: whoami is not installed, id knows no name for it, and id -u printed no uid.\n"},
	} {
		out, errOut, rc := runErr(tc.procs.module(t, tc.passwd, tc.err), nil)
		require.Equal(t, 0, rc)
		require.Equal(t, "UNKNOWN_USERNAME\n", out)
		require.Equal(t, tc.want, errOut)
	}
}

// 3.0: SELF-15 fixed: an extra word is refused with rc 2 before whoami
// runs (2.x ignored it and printed the name).
func TestUsernameRefusesExtraArguments(t *testing.T) {
	p := &fakeProcs{have: map[string]bool{"whoami": true}, out: map[string]string{"whoami": "alice"}}
	out, errOut, rc := callModule(t, p.module(t, "", nil), "gash_username", nil, "extra", "words")
	require.Equal(t, 2, rc)
	require.Empty(t, out)
	require.Equal(t, "Error: Unexpected argument: extra (usage: gash_username)\n", errOut)
}

func TestUsernameHelp(t *testing.T) {
	p := &fakeProcs{}
	// Registry loaded (direct call or gash.sh): the registered help page.
	out, rc := run(t, p.module(t, "", nil), nil, "--help")
	require.Equal(t, 0, rc)
	require.Equal(t, "\n  gash_username  (module: gash)\n\n  Print the current user name\n\n"+
		"  USAGE\n    gash_username\n\n  EXAMPLES\n    # The name the banner and the prompt show\n    gash_username\n\n"+
		"  NOTES\n    Tries whoami, then id -un, then the /etc/passwd entry of the\n"+
		"    current user id; prints UNKNOWN_USERNAME when all of them fail.\n\n", out)
	// Registry not loaded in the caller's shell: the needs_help fallback.
	out, rc = run(t, p.module(t, "", nil), map[string]string{"GASH__HELP": "0"}, "-h")
	require.Equal(t, 0, rc)
	require.Equal(t, "gash_username\nUsage: gash_username\nPrint the current user name.\n", out)
}

// The real chain on this machine: whoami from a fake PATH.
func TestUsernameRealProcess(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "whoami")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho 'dave'\necho\n"), 0o755))
	out, rc := run(t, New(), map[string]string{"PATH": dir})
	require.Equal(t, 0, rc)
	require.Equal(t, "dave\n", out)
}
