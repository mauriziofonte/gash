// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// fakeProcs answers command -v and $(...) from a table keyed by argv.
type fakeProcs struct {
	have map[string]bool
	out  map[string]string
	ran  []string
}

func (f *fakeProcs) found(_ string, name string) bool { return f.have[name] }

func (f *fakeProcs) output(_ *invoke.Call, _ string, argv ...string) string {
	k := strings.Join(argv, " ")
	f.ran = append(f.ran, k)
	return f.out[k]
}

func run(t *testing.T, m *Module, env map[string]string, args ...string) (string, int) {
	t.Helper()
	tbl := invoke.NewTable()
	m.Register(tbl)
	var out, errb bytes.Buffer
	e := invoke.EnvMap(env)
	c := &invoke.Call{Name: "gash_username", Args: args, Env: e, Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, false, false)}
	rc := tbl.Run(c)
	require.Empty(t, errb.String())
	return out.String(), rc
}

func module(p *fakeProcs, passwd string, readErr error) *Module {
	return &Module{proc: p, passwd: "/etc/passwd", readFile: func(string) ([]byte, error) {
		return []byte(passwd), readErr
	}}
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
		{"nothing", &fakeProcs{}, "", "UNKNOWN_USERNAME\n", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, rc := run(t, module(tc.procs, tc.passwd, nil), nil)
			require.Equal(t, 0, rc)
			require.Equal(t, tc.want, out)
			require.Equal(t, tc.ran, tc.procs.ran)
		})
	}
}

func TestUsernameUnreadablePasswd(t *testing.T) {
	p := &fakeProcs{have: map[string]bool{"id": true}, out: map[string]string{"id -u": "0"}}
	out, rc := run(t, module(p, "root:x:0:0::/root:/bin/sh\n", errors.New("denied")), nil)
	require.Equal(t, 0, rc)
	require.Equal(t, "UNKNOWN_USERNAME\n", out)
}

func TestUsernameIgnoresExtraArguments(t *testing.T) {
	p := &fakeProcs{have: map[string]bool{"whoami": true}, out: map[string]string{"whoami": "alice"}}
	out, _ := run(t, module(p, "", nil), nil, "extra", "words")
	require.Equal(t, "alice\n", out)
}

func TestUsernameHelp(t *testing.T) {
	p := &fakeProcs{}
	// Registry loaded (direct call or gash.sh): the registered help page.
	out, rc := run(t, module(p, "", nil), nil, "--help")
	require.Equal(t, 0, rc)
	require.Equal(t, "\n  gash_username  (module: gash)\n\n  Print the current user name\n\n"+
		"  USAGE\n    gash_username\n\n  EXAMPLES\n    # The name the banner and the prompt show\n    gash_username\n\n"+
		"  NOTES\n    Tries whoami, then id -un, then the /etc/passwd entry of the\n"+
		"    current user id; prints UNKNOWN_USERNAME when all of them fail.\n\n", out)
	// Registry not loaded in the caller's shell: the needs_help fallback.
	out, rc = run(t, module(p, "", nil), map[string]string{"GASH__HELP": "0"}, "-h")
	require.Equal(t, 0, rc)
	require.Equal(t, "gash_username\nUsage: gash_username\nPrint the current user name.\n", out)
	require.Empty(t, p.ran)
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
