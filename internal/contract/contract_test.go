// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package contract

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Vectors captured from 2.x __gash_json_error (lib/core/utils.sh, v2.1.0).
func TestEnvelopeBytesMatch2x(t *testing.T) {
	cases := []struct {
		name string
		env  Envelope
		want string
	}{
		{
			"all fields",
			Legacy("bad_flag", `--x "q"`, "RETRY", "use --help"),
			`{"error":"bad_flag","details":"--x \"q\"","action":"RETRY","recoverable":true,"hint":"use --help"}` + "\n",
		},
		{
			"fatal without details",
			Legacy("blocked", "", "FATAL", ""),
			`{"error":"blocked","action":"FATAL","recoverable":false}` + "\n",
		},
		{
			"unknown action becomes STOP",
			Legacy("x", "", "MAYBE", ""),
			`{"error":"x","action":"STOP","recoverable":true}` + "\n",
		},
		{
			"empty type and action",
			Legacy("", "d", "", ""),
			`{"error":"unknown_error","details":"d","action":"STOP","recoverable":true}` + "\n",
		},
		{
			"controls, DEL and raw bytes",
			Legacy("c", "a\x01b\x1f\x7f\tc\\d\ne\rf\bg\fh\u2028i\xff", "STOP", "h\x1b"),
			"{\"error\":\"c\",\"details\":\"a\\u0001b\\u001f\x7f\\tc\\\\d\\ne\\rf\\bg\\fh\u2028i\xff\",\"action\":\"STOP\",\"recoverable\":true,\"hint\":\"h\\u001b\"}\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, string(c.env.Bytes()))
		})
	}
}

func TestEnvelopeExtraUsesJQEscaping(t *testing.T) {
	e := New("sqlite_error", "bad\x7f\xff", "").With("query_time", "1.2ms")
	require.Equal(t, "{\"error\":\"sqlite_error\",\"details\":\"bad\\u007f\uFFFD\",\"action\":\"RETRY\",\"recoverable\":true,\"query_time\":\"1.2ms\"}\n", string(e.Bytes()))
}

func TestNewUsesTheFixedAction(t *testing.T) {
	require.Equal(t, Fatal, New("secret_file_blocked", "", "").Action)
	require.Equal(t, Continue, New("not_a_git_repo", "", "").Action)
	require.Equal(t, Stop, New("internal_error", "", "").Action)
	require.Panics(t, func() { New("no_such_code", "", "") })
}

func TestEveryCodeHasOneValidAction(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range codeTable {
		require.False(t, seen[c.Name], "duplicate code %s", c.Name)
		seen[c.Name] = true
		require.Equal(t, c.Action, NormalizeAction(string(c.Action)), c.Name)
		require.NotEmpty(t, c.Area, c.Name)
	}
	require.Len(t, Codes(), len(codeTable))
}

// The AGENTS.md section 5 table of 2.1.1, code by code.
func TestAgentsTableActions(t *testing.T) {
	table := map[Action]string{
		Retry: "unknown_option missing_value unexpected_argument invalid_limit invalid_depth invalid_size invalid_top invalid_context invalid_filter invalid_type invalid_database_name missing_pattern missing_query missing_table invalid_table_name missing_file empty_command " +
			"path_not_found not_a_directory path_unreadable file_not_found file_too_large construct_not_allowed unterminated_quote glob_matches_option " +
			"executable_comment_blocked backslash_in_literal unterminated_literal sqlite_error mysql_error pgsql_error sqlite_file_not_found missing_sqlite_file compose_not_found",
		Fatal: "path_traversal_blocked forbidden_path root_scan_blocked secret_file_blocked dangerous_command_blocked option_not_allowed " +
			"write_operation_blocked multiple_statements_blocked client_command_blocked dot_command_blocked sqlite_not_found mysql_not_found psql_not_found invalid_sqlite_file jq_not_found",
		Stop:     "command_not_allowed sandbox_unavailable no_db_config no_database unknown_db_type",
		Continue: "not_a_git_repo git_not_found no_package_file ports_tool_not_found no_services docker_not_found compose_not_installed",
	}
	for action, codes := range table {
		for _, code := range strings.Fields(codes) {
			got, ok := ActionOf(code)
			require.True(t, ok, code)
			require.Equal(t, action, got, code)
		}
	}
}

func TestAppendStringJQ(t *testing.T) {
	// jq 1.7: jq -cn '"\u007f\u0001\u001b\u2028<>&é\u00ad\ufeff"'
	got := Quote("\x7f\x01\x1b\u2028<>&é\u00ad\ufeff", EscapeJQ)
	require.Equal(t, "\"\\u007f\\u0001\\u001b\u2028<>&é\u00ad\ufeff\"", got)
	// printf '"\xff\xfe a"' | jq -c .
	require.Equal(t, "\"\uFFFD\uFFFD a\"", Quote("\xff\xfe a", EscapeJQ))
	require.Equal(t, `"\u0000"`, Quote("\x00", EscapeBash))
}

func TestObjectOrderAndRaw(t *testing.T) {
	var o Object
	o.String("b", "x").Int("a", -3).Bool("c", false).Raw("d", []byte(` { "n" : 1.0 } `))
	b, err := o.Bytes(EscapeJQ)
	require.NoError(t, err)
	require.Equal(t, `{"b":"x","a":-3,"c":false,"d":{"n":1.0}}`, string(b))

	var bad Object
	bad.Raw("x", []byte(`{`))
	_, err = bad.Bytes(EscapeJQ)
	require.Error(t, err)
}

func TestEnvelopeWrite(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, New("binary_not_found", "d", "h").Write(&buf))
	require.Equal(t, `{"error":"binary_not_found","details":"d","action":"STOP","recoverable":true,"hint":"h"}`+"\n", buf.String())
	require.Equal(t, `{"error":"binary_not_found","details":"d","action":"STOP","recoverable":true,"hint":"h"}`, New("binary_not_found", "d", "h").String())
}

// The jq encoding of invalid UTF-8, sequence by sequence, against the jq
// on PATH (jq 1.7 replaces each bad sequence the way jvp_utf8_next reads
// it) and against the pinned bytes.
func TestLedger_CORE09_JQBytes(t *testing.T) {
	vectors := map[string]string{
		"\xed\xa0\x80":         "\"�\"",
		"\xc0\x80":             "\"��\"",
		"\xe2\x82":             "\"�\"",
		"\xe2\x82x":            "\"�x\"",
		"\xe2x":                "\"�\"",
		"\xf8\x88\x80\x80\x80": "\"�����\"",
		"\xf4\x90\x80\x80":     "\"�\"",
		"ab\xf0\x9fc":          "\"ab�\"",
		"\xe0\x80\x80":         "\"�\"",
		"\xef\xbf\xbf\x7f\x01": "\"￿\\u007f\\u0001\"",
		"ok é   <&>":           "\"ok é   <&>\"",
	}
	jq, err := exec.LookPath("jq")
	for in, want := range vectors {
		require.Equal(t, want+"\n", JQString(in), "%q", in)
		if err == nil {
			got, jerr := exec.Command(jq, "-cn", "--arg", "s", in, "$s").Output()
			require.NoError(t, jerr)
			require.Equal(t, string(got), JQString(in), "jq %q", in)
		}
	}
	require.Equal(t, "[\"a\",\"�\"]\n", JQArray("a", "\xe2\x82"))
	require.Equal(t, "[]", JQArray())
}

// Every 3.x code outside gash hte (docs/hte.md lists those) is in the
// AGENTS.md section 5 table, under its own action: a 3.4 code cannot ship
// undocumented, nor documented with another action.
func TestAgentsTableHas3xCodes(t *testing.T) {
	b, err := os.ReadFile("../../AGENTS.md")
	require.NoError(t, err)
	doc := string(b)
	s5 := doc[strings.Index(doc, "\n## 5. "):strings.Index(doc, "\n## 6. ")]
	rows := map[string]string{}
	for _, line := range strings.Split(s5, "\n") {
		if !strings.HasPrefix(line, "| ") {
			continue
		}
		cells := strings.Split(line, " | ")
		if len(cells) < 2 {
			continue
		}
		for _, part := range strings.Split(cells[len(cells)-1], ";") {
			action := ""
			for _, a := range []string{"RETRY", "STOP", "CONTINUE", "FATAL"} {
				if strings.Contains(part, "("+a+")") {
					action = a
				}
			}
			for _, f := range strings.Fields(part) {
				if strings.HasPrefix(f, "`") && strings.HasSuffix(f, "`") && len(f) > 2 {
					rows[strings.Trim(f, "`")] = action
				}
			}
		}
	}
	for _, c := range Codes() {
		if c.Origin != Origin3x || c.Area == "hte" || c.Area == "compat" || c.Area == "binary" {
			continue
		}
		action, ok := rows[c.Name]
		require.True(t, ok, "AGENTS.md section 5 lacks %s", c.Name)
		if action != "" {
			require.Equal(t, string(c.Action), action, "AGENTS.md section 5 action of %s", c.Name)
		}
	}
}
