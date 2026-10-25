// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const dotenvFixture = "\xef\xbb\xbf# app\r\n" +
	"APP_NAME=\"My App\"\n" +
	"APP_URL=http://localhost:8000 # a comment\n" +
	"HASH=a#b\n" +
	"export DB_HOST = 127.0.0.1\n" +
	"DB_PASSWORD=hunter2\n" +
	"DB_URL=mysql://user:pa55@db:3306/app\n" +
	"APP_KEY=\n" +
	"GH=ghp_abcdefghijklmnopqrstuvwxyz0123456789\n" +
	"MULTI=\"line1\nline2 \\\"q\\\"\" # trailing\n" +
	"SINGLE='lit $X \\n'\n" +
	"REF=${DB_PASSWORD}\n" +
	"spring.datasource-url=jdbc:x\n" +
	"DUP=1\n" +
	"DUP=2\n" +
	"noeq\n" +
	"#COMMENTED=1\n" +
	"OPEN=\"never closed\n" +
	"LAST=end"

func TestDotenvValues(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, ".env"), dotenvFixture)
	m := New()
	env := baseEnv(d, nil)
	got := runGo(t, m, d, env, "llm_dotenv", ".env", "APP_NAME", "APP_URL", "HASH", "DB_HOST", "DB_PASSWORD", "DB_URL", "APP_KEY",
		"GH", "MULTI", "SINGLE", "REF", "spring.datasource-url", "DUP", "MISSING", "COMMENTED", "OPEN", "LAST", "APP_NAME")
	require.Equal(t, result{`{"APP_NAME":"My App","APP_URL":"http://localhost:8000","HASH":"a#b","DB_HOST":"127.0.0.1",` +
		`"DB_PASSWORD":"***(set)","DB_URL":"mysql://user:***@db:3306/app","APP_KEY":"","GH":"***",` +
		`"MULTI":"line1\nline2 \"q\"","SINGLE":"lit $X \\n","REF":"${DB_PASSWORD}","spring.datasource-url":"jdbc:x",` +
		`"DUP":"2","MISSING":null,"COMMENTED":null,"OPEN":"never closed","LAST":"end"}` + "\n", "", 0}, got)
	require.NotContains(t, got.Stdout, "hunter2")
	require.NotContains(t, got.Stdout, "pa55")
	require.NotContains(t, got.Stdout, "ghp_")

	got = runGo(t, m, d, env, "llm_dotenv", ".env", "DB_PASSWORD", "MISSING", "--present")
	require.Equal(t, result{`{"DB_PASSWORD":true,"MISSING":false}` + "\n", "", 0}, got)
	got = runGo(t, m, d, env, "llm_dotenv", ".env")
	require.Equal(t, result{`["APP_NAME","APP_URL","HASH","DB_HOST","DB_PASSWORD","DB_URL","APP_KEY","GH","MULTI","SINGLE",` +
		`"REF","spring.datasource-url","DUP","OPEN","LAST"]` + "\n", "", 0}, got)
}

// TestDotenvNeverPrintsSecrets: on the llm_env fixtures, no secret value
// reaches the output whatever key is asked.
func TestDotenvNeverPrintsSecrets(t *testing.T) {
	d := physTemp(t)
	secrets := map[string]string{
		"AWS_SECRET_ACCESS_KEY": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"STRIPE_KEY":            "sk_live_51Habcdefghijklmnop",
		"GITHUB_TOKEN":          "ghp_abcdefghijklmnopqrstuvwxyz0123456789",
		"API_AUTH":              "Basic dXNlcjpwYXNz",
		"SESSION_DRIVER":        "s3cr3t-session",
		"MY_PAT":                "patvalue123",
		"DATABASE_URL":          "postgres://u:dbpass@h/db",
	}
	var b strings.Builder
	keys := []string{".env.production"}
	for k, v := range secrets {
		b.WriteString(k + "=" + v + "\n")
		keys = append(keys, k)
	}
	b.WriteString("NOTE=see sk-ant-abcdefghijklmnopqrstuvwxyz and AKIAABCDEFGHIJKLMNOP\n")
	b.WriteString("CALLBACK=https://bob:hidden@example.com/x\n")
	keys = append(keys, "NOTE", "CALLBACK")
	mustWrite(t, filepath.Join(d, ".env.production"), b.String())
	got := runGo(t, New(), d, baseEnv(d, nil), "llm_dotenv", keys...)
	require.Equal(t, 0, got.RC, got.Stderr)
	for _, v := range secrets {
		require.NotContains(t, got.Stdout, v)
	}
	for _, leak := range []string{"sk-ant-", "AKIAABCD", "hidden", "dbpass"} {
		require.NotContains(t, got.Stdout, leak)
	}
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(got.Stdout), &out))
	require.Equal(t, "***(set)", out["GITHUB_TOKEN"])
	require.Equal(t, "https://bob:***@example.com/x", out["CALLBACK"])
}

func TestDotenvRefusals(t *testing.T) {
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, ".env"), "A=1\n")
	mustWrite(t, filepath.Join(d, "id_ed25519"), "-----BEGIN OPENSSH PRIVATE KEY-----\nA=1\n")
	mustWrite(t, filepath.Join(d, "credentials"), "A=1\n")
	mustMkdir(t, filepath.Join(d, ".aws"))
	mustWrite(t, filepath.Join(d, ".aws", "x.env"), "A=1\n")
	require.NoError(t, os.Symlink("id_ed25519", filepath.Join(d, "link.env")))
	mustWrite(t, filepath.Join(d, "plain.txt"), "A=1\nB_TOKEN=x\n")
	mustWrite(t, filepath.Join(d, "big.env"), strings.Repeat("A=1\n", dotenvMaxBytes/4+1))
	m := New()
	env := baseEnv(d, nil)
	code := func(r result) string {
		t.Helper()
		require.Equal(t, 1, r.RC, r.Stdout)
		require.Empty(t, r.Stdout)
		var e struct{ Error string }
		require.NoError(t, json.Unmarshal([]byte(r.Stderr), &e), r.Stderr)
		return e.Error
	}
	for _, f := range []string{"id_ed25519", "credentials", ".aws/x.env", "link.env"} {
		require.Equal(t, "secret_file_blocked", code(runGo(t, m, d, env, "llm_dotenv", f, "A")), f)
	}
	require.Equal(t, "file_too_large", code(runGo(t, m, d, env, "llm_dotenv", "big.env", "A")))
	require.Equal(t, "file_not_found", code(runGo(t, m, d, env, "llm_dotenv", "none.env", "A")))
	require.Equal(t, "missing_file", code(runGo(t, m, d, env, "llm_dotenv")))
	require.Equal(t, "invalid_env_name", code(runGo(t, m, d, env, "llm_dotenv", ".env", "1A")))
	require.Equal(t, "invalid_env_name", code(runGo(t, m, d, env, "llm_dotenv", ".env", "A=B")))
	require.Equal(t, "unknown_option", code(runGo(t, m, d, env, "llm_dotenv", ".env", "--values")))
	// A file that is not secret reads the same way, secret names masked.
	got := runGo(t, m, d, env, "llm_dotenv", "plain.txt", "A", "B_TOKEN")
	require.Equal(t, result{`{"A":"1","B_TOKEN":"***(set)"}` + "\n", "", 0}, got)
}
