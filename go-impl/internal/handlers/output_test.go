// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// compat runs a compat operation with stdin and returns stdout, stderr
// and the status.
func compat(t *testing.T, env map[string]string, stdoutTTY bool, stdin string, words ...string) (string, string, int) {
	t.Helper()
	h, _, rest, ok := Table().LookupCompat(words)
	require.True(t, ok, words)
	var out, errb bytes.Buffer
	e := invoke.EnvMap(env)
	rc := h(&invoke.Call{Ctx: t.Context(), Args: rest, Env: e, Stdin: strings.NewReader(stdin),
		Stdout: &out, Stderr: &errb, Term: term.Fixed(e, false, stdoutTTY, false)})
	return out.String(), errb.String(), rc
}

func TestCompatTable(t *testing.T) {
	out, _, rc := compat(t, nil, false, "A\tB\nx\ty\n", "output", "table", "--right", "2")
	require.Equal(t, 0, rc)
	require.Equal(t, "A  B\n----\nx  y\n", out)
	// __gash_table_into decided colors in the caller's shell.
	out, _, _ = compat(t, map[string]string{"GASH__TABLE_COLOR": "1"}, false, "A\tB\nx\ty\n", "output", "table")
	require.Equal(t, "\x1b[1m\x1b[4mA  B\x1b[0m\nx  y\n", out)
	out, _, _ = compat(t, nil, false, "", "output", "table")
	require.Empty(t, out)
}

func TestCompatBoxWrap(t *testing.T) {
	out, _, rc := compat(t, map[string]string{"LANG": "C"}, false, "ab\x00\x00", "output", "box", "--color", "\x1b[1m")
	require.Equal(t, 0, rc)
	require.Equal(t, "+------+\n|  ab  |\n|      |\n+------+\n", out)
	out, _, _ = compat(t, map[string]string{"LANG": "C"}, false, "", "output", "box")
	require.Equal(t, "+----+\n+----+\n", out)

	out, _, rc = compat(t, nil, false, "aaa bbb ccc\n\n- x y", "output", "wrap", "7")
	require.Equal(t, 0, rc)
	require.Equal(t, "aaa bbb\nccc\n\n- x y\n", out)
	out, _, _ = compat(t, nil, false, "", "output", "wrap", "72")
	require.Equal(t, "\n", out)
}

func TestCompatChoose(t *testing.T) {
	var res bytes.Buffer
	old := resultFD
	resultFD = func() io.WriteCloser { return nopCloser{&res} }
	defer func() { resultFD = old }()
	out, _, rc := compat(t, nil, false, "2\n", "output", "choose", "1", "a", "b")
	require.Equal(t, 0, rc)
	require.Equal(t, "  1) a\n  2) b\n  Choice [1]: ", out)
	require.Equal(t, "2", res.String())
	res.Reset()
	_, _, rc = compat(t, nil, false, "q\n", "output", "choose", "1", "a")
	require.Equal(t, 1, rc)
	require.Empty(t, res.String())
	_, _, rc = compat(t, nil, false, "", "output", "choose", "1")
	require.Equal(t, 1, rc)
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

// LEDGER CORE-09: the JSON helpers need no jq; the bytes are jq 1.7's.
func TestLedger_CORE09(t *testing.T) {
	out, _, rc := compat(t, nil, false, "a\"b\\c\n\x01\x7f\xff\xed\xa0\x80é", "json", "str")
	require.Equal(t, 0, rc)
	require.Equal(t, "\"a\\\"b\\\\c\\n\\u0001\\u007f��é\"\n", out)
	out, _, rc = compat(t, nil, false, "a\x00b\"c\x00\x00", "json", "arr")
	require.Equal(t, 0, rc)
	require.Equal(t, "[\"a\",\"b\\\"c\",\"\"]\n", out)
	out, _, _ = compat(t, nil, false, "", "json", "arr")
	require.Equal(t, "[]", out)
}

func TestCompatEnvLoad(t *testing.T) {
	home := t.TempDir()
	key := filepath.Join(home, "id_test")
	require.NoError(t, os.WriteFile(key, nil, 0o600))
	env := filepath.Join(home, ".gash_env")
	require.NoError(t, os.WriteFile(env, []byte("DB:default=mysql://u:p'w@h/d\nAI:claude=sk-x\nSSH:~/id_test=pass phrase\n"+
		"HOST:web=me@web\nGASH_BANNER=0\nbogus\n"), 0o644))
	old := geteuid
	geteuid = func() int { return os.Getuid() }
	defer func() { geteuid = old }()
	out, errOut, rc := compat(t, map[string]string{"HOME": home}, false, "", "env", "load")
	require.Equal(t, 0, rc)
	require.Equal(t, "Warning: ~/.gash_env is accessible by other users (mode 644). Run: chmod 600 ~/.gash_env\n"+
		"Warning: Line 6 of ~/.gash_env: unrecognized entry (expected SSH:, DB:, AI:, HOST: or GASH_NAME=), skipping\n", errOut)
	require.Equal(t, "__GASH_ENV_LOADED=1\n__GASH_ENV_SSH_KEYS=$'"+home+"/id_test\\tpass phrase'\n"+
		"__GASH_ENV_DB_ENTRIES=$'default\\tmysql://u:p\\'w@h/d'\n__GASH_ENV_AI_PROVIDERS=$'claude\\tsk-x'\n"+
		"__GASH_ENV_HOST_ENTRIES=$'web\\tme@web'\n__GASH_ENV_SETTINGS=$'GASH_BANNER\\t0'\n", out)
	// Headless: no warnings unless GASH_DEBUG=1; a missing file is empty.
	_, errOut, _ = compat(t, map[string]string{"HOME": home, "GASH_HEADLESS": "1"}, false, "", "env", "load")
	require.Equal(t, "Warning: Line 6 of ~/.gash_env: unrecognized entry (expected SSH:, DB:, AI:, HOST: or GASH_NAME=), skipping\n", errOut)
	out, errOut, _ = compat(t, map[string]string{"HOME": home, "GASH_ENV_FILE": home + "/none"}, false, "", "env", "load")
	require.Empty(t, errOut)
	require.Equal(t, "__GASH_ENV_LOADED=1\n__GASH_ENV_SSH_KEYS=''\n__GASH_ENV_DB_ENTRIES=''\n__GASH_ENV_AI_PROVIDERS=''\n"+
		"__GASH_ENV_HOST_ENTRIES=''\n__GASH_ENV_SETTINGS=''\n", out)
}
