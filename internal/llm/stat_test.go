// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// statFixture is a directory with a file of three lines (no final
// newline), an empty file, a directory, a symlink, a dangling symlink and
// a secret file, all with one fixed mtime.
func statFixture(t *testing.T) string {
	t.Helper()
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "a.txt"), "one\ntwo\nthree")
	mustWrite(t, filepath.Join(d, "empty"), "")
	mustWrite(t, filepath.Join(d, ".env"), "K=v\nL=w\n")
	mustMkdir(t, filepath.Join(d, "dir"))
	require.NoError(t, os.Symlink("a.txt", filepath.Join(d, "link")))
	require.NoError(t, os.Symlink("nowhere", filepath.Join(d, "dangling")))
	mt := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	for _, n := range []string{"a.txt", "empty", ".env", "dir"} {
		require.NoError(t, os.Chtimes(filepath.Join(d, n), mt, mt))
	}
	require.NoError(t, os.Chmod(filepath.Join(d, "a.txt"), 0o640))
	return d
}

func statRows(t *testing.T, r result) []map[string]any {
	t.Helper()
	require.Equal(t, 0, r.RC, r.Stderr)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &rows), r.Stdout)
	return rows
}

func TestStat_RowsInInputOrder(t *testing.T) {
	d := statFixture(t)
	env := baseEnv(physTemp(t), nil)
	r := runGo(t, New(), d, env, "llm_stat", "a.txt", "missing", "dir", "empty", "--lines", "--format", "json")
	rows := statRows(t, r)
	require.Len(t, rows, 4)
	require.Equal(t, "a.txt", rows[0]["p"])
	require.Equal(t, "f", rows[0]["t"])
	require.EqualValues(t, 13, rows[0]["size"])
	require.Equal(t, "640", rows[0]["mode"])
	require.Equal(t, "2026-10-25T12:00:00Z", rows[0]["mtime"])
	require.EqualValues(t, 3, rows[0]["lines"], "a last line without a newline counts")
	require.Contains(t, rows[0]["owner"], ":")
	require.Equal(t, map[string]any{"p": "missing", "t": "missing", "size": nil, "mode": nil, "owner": nil, "mtime": nil, "lines": nil}, rows[1])
	require.Equal(t, "d", rows[2]["t"])
	require.Nil(t, rows[2]["lines"])
	require.EqualValues(t, 0, rows[3]["lines"])
	// No link and no error: neither column.
	_, hasTarget := rows[0]["target"]
	_, hasErr := rows[0]["error"]
	require.False(t, hasTarget)
	require.False(t, hasErr)
}

func TestStat_DefaultIsTSV(t *testing.T) {
	d := statFixture(t)
	r := runGo(t, New(), d, baseEnv(physTemp(t), nil), "llm_stat", "a.txt", "nope")
	require.Equal(t, 0, r.RC)
	lines := strings.Split(strings.TrimSuffix(r.Stdout, "\n"), "\n")
	require.Equal(t, "p\tt\tsize\tmode\towner\tmtime", lines[0])
	require.True(t, strings.HasPrefix(lines[1], "a.txt\tf\t13\t640\t"), lines[1])
	require.True(t, strings.HasSuffix(lines[1], "\t2026-10-25T12:00:00Z"), lines[1])
	require.Equal(t, "nope\tmissing\t\\N\t\\N\t\\N\t\\N", lines[2])
	require.Empty(t, r.Stderr)
}

func TestStat_Links(t *testing.T) {
	d := statFixture(t)
	env := baseEnv(physTemp(t), nil)
	rows := statRows(t, runGo(t, New(), d, env, "llm_stat", "link", "dangling", "a.txt", "--format", "json"))
	require.Equal(t, "l", rows[0]["t"])
	require.Equal(t, "a.txt", rows[0]["target"], "the link text without --follow")
	require.Equal(t, "l", rows[1]["t"])
	require.Equal(t, "nowhere", rows[1]["target"])
	require.Nil(t, rows[2]["target"])

	rows = statRows(t, runGo(t, New(), d, env, "llm_stat", "link", "dangling", "--follow", "--lines", "--format", "json"))
	require.Equal(t, "f", rows[0]["t"])
	require.Equal(t, filepath.Join(d, "a.txt"), rows[0]["target"])
	require.EqualValues(t, 13, rows[0]["size"])
	require.EqualValues(t, 3, rows[0]["lines"])
	require.Equal(t, "l", rows[1]["t"], "a dangling link stays l with --follow")
}

func TestStat_SecretMetadataButNoLines(t *testing.T) {
	d := statFixture(t)
	rows := statRows(t, runGo(t, New(), d, baseEnv(physTemp(t), nil), "llm_stat", ".env", "a.txt", "--lines", "--format", "json"))
	require.Equal(t, "f", rows[0]["t"])
	require.EqualValues(t, 8, rows[0]["size"])
	require.Nil(t, rows[0]["lines"])
	require.Equal(t, "secret_file_blocked", rows[0]["error"])
	require.EqualValues(t, 3, rows[1]["lines"])
	require.Nil(t, rows[1]["error"])
}

func TestStat_AbsoluteAndGuards(t *testing.T) {
	d := statFixture(t)
	env := baseEnv(physTemp(t), nil)
	rows := statRows(t, runGo(t, New(), d, env, "llm_stat", "a.txt", "--absolute", "--format", "json"))
	require.Equal(t, filepath.Join(d, "a.txt"), rows[0]["p"])

	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"../x"}, "path_traversal_blocked"},
		{[]string{"a.txt", "/proc/self"}, "forbidden_path"},
		{[]string{}, "missing_file"},
		{[]string{"a.txt", "--bogus"}, "unknown_option"},
		{[]string{"a.txt", "--format", "xml"}, "invalid_format"},
		{[]string{"a.txt", "-j", "0"}, "invalid_jobs"},
	} {
		r := runGo(t, New(), d, env, "llm_stat", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
		require.Empty(t, r.Stdout, tc.args)
	}
}

func TestStat_LargeFileLinesInParts(t *testing.T) {
	d := physTemp(t)
	line := strings.Repeat("x", 99) + "\n"
	mustWrite(t, filepath.Join(d, "big"), strings.Repeat(line, (parallelMin/100)+12345))
	rows := statRows(t, runGo(t, New(), d, baseEnv(physTemp(t), nil), "llm_stat", "big", "--lines", "--format", "json"))
	require.EqualValues(t, parallelMin/100+12345, rows[0]["lines"])
}

func TestHash_Digests(t *testing.T) {
	d := statFixture(t)
	env := baseEnv(physTemp(t), nil)
	rows := statRows(t, runGo(t, New(), d, env, "llm_hash", "a.txt", ".env", "nope", "dir"))
	// sha256sum of "one\ntwo\nthree" and of "K=v\nL=w\n".
	require.Equal(t, map[string]any{"p": "a.txt", "sha256": "058053d87c818d699cde0f00d670bca0e1c6ad857caa9758ea6a556d7c64fcee", "size": float64(13), "error": nil}, rows[0])
	require.Equal(t, "441df5a6571fe19a36deba7f4633dda807868c2c2c77fc85592e652bc47a8ff6", rows[1]["sha256"], "a secret file is hashed")
	require.Equal(t, map[string]any{"p": "nope", "sha256": nil, "size": nil, "error": "file_not_found"}, rows[2])
	require.Equal(t, "not_a_file", rows[3]["error"])

	md5 := statRows(t, runGo(t, New(), d, env, "llm_hash", "a.txt", "--algo", "md5", "--short", "8"))
	require.Equal(t, []map[string]any{{"p": "a.txt", "md5": "af80be2e", "size": float64(13)}}, md5)

	r := runGo(t, New(), d, env, "llm_hash", "a.txt", "--format", "tsv", "-j", "1")
	require.Equal(t, 0, r.RC)
	require.True(t, strings.HasPrefix(r.Stdout, "p\tsha256\tsize\na.txt\t"), r.Stdout)

	for _, tc := range [][]string{{"a.txt", "--algo", "sha1"}, {"a.txt", "--short", "0"}, {"a.txt", "--algo"}} {
		r := runGo(t, New(), d, env, "llm_hash", tc...)
		require.Equal(t, 1, r.RC, tc)
		require.Regexp(t, `"error":"(invalid_algo|invalid_short|missing_value)"`, r.Stderr, tc)
	}
}

// The digests are those of sha256sum and md5sum.
func TestHash_MatchesCoreutils(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "f"), "hello\n")
	rows := statRows(t, runGo(t, New(), d, baseEnv(physTemp(t), nil), "llm_hash", "f", "-j", "4"))
	require.Equal(t, "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03", rows[0]["sha256"])
	rows = statRows(t, runGo(t, New(), d, baseEnv(physTemp(t), nil), "llm_hash", "f", "--algo", "md5"))
	require.Equal(t, "b1946ac92492d2347c6235b4d2611184", rows[0]["md5"])
}

// A target that runs out of --timeout is a field of its row, not a failed
// call (plan S6).
func TestTextReads_TimeoutIsAField(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "a"), "x\n")
	env := baseEnv(physTemp(t), nil)
	r := runGo(t, New(), d, env, "llm_hash", "a", "--timeout", "1ns")
	require.Equal(t, result{`[{"p":"a","sha256":null,"size":null,"error":"timeout"}]` + "\n", "", 0}, r)
	r = runGo(t, New(), d, env, "llm_count", "a", "--pat", "x", "--timeout", "1ns")
	require.Equal(t, result{`{"a":{"error":"timeout"}}` + "\n", "", 0}, r)
	r = runGo(t, New(), d, env, "llm_assert", "a", "--has", "x", "--timeout", "1ns")
	require.Equal(t, result{`{"pass":false,"checked":1,"failed":[{"file":"a","error":"timeout"}]}` + "\n", "", 1}, r)
}
