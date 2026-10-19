// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// pgHome is a HOME whose ~/.gash_env names the test cluster.
func pgHome(t *testing.T, port int) (dir, home string) {
	t.Helper()
	dir = physTemp(t)
	home = filepath.Join(dir, "home")
	require.NoError(t, os.Mkdir(home, 0o755))
	mustWrite(t, filepath.Join(home, ".gash_env"), fmt.Sprintf(
		"DB:default=pgsql://postgres:s%%40cret@127.0.0.1:%d/app\nDB:pgn=pgsql://postgres:x@127.0.0.1:%d\nDB:down=pgsql://u:p@127.0.0.1:1/app\n",
		port, port), 0o600)
	return dir, home
}

func TestPgsqlMatches2x(t *testing.T) {
	port := pgServer(t)
	d, home := pgHome(t, port)
	m := frozen(New())
	env := baseEnv(home, nil)
	for _, args := range [][]string{
		{"llm_db_tables"},
		{"llm_db_tables", "-c", "pgn", "-d", "app"},
		{"llm_db_schema", "users"},
		{"llm_db_schema", "nope"},
		{"llm_db_sample", "users"},
		{"llm_db_sample", "users", "-n", "1"},
		{"llm_db_sample", "empty_t"},
		{"llm_db_query", "SELECT * FROM users ORDER BY id"},
		{"llm_db_query", "SELECT * FROM users ORDER BY id -- note", "-r", "2"},
		{"llm_db_query", "SELECT id, meta, tags, amount, born FROM users ORDER BY id;"},
		{"llm_db_query", "TABLE users"},
		{"llm_db_query", "VALUES (1, 'a'), (2, NULL)"},
		{"llm_db_query", "WITH a AS (SELECT 1 AS n, 1.50 AS x) SELECT * FROM a"},
		{"llm_db_query", "SELECT 1 AS a, 2 AS a"},
		{"llm_db_query", "SELECT E'a\\tb' AS e, $$dollar ' quote$$ AS d, 'x''y' AS q"},
		{"llm_db_query", "SHOW default_transaction_read_only"},
		{"llm_db_query", "SHOW standard_conforming_strings"},
		{"llm_db_query", "SHOW statement_timeout"},
		{"llm_db_query", "SHOW statement_timeout", "-c", "pgn", "-d", "app"},
		{"llm_db_query", "EXPLAIN SELECT * FROM users WHERE id = 1"},
		{"llm_db_query", "EXPLAIN (COSTS OFF) SELECT * FROM users ORDER BY name"},
		{"llm_db_query", "SELECT * FROM users WHERE id > 99"},
		{"llm_db_query", "SELECT current_database()", "-c", "pgn", "-d", "other_db"},
		{"llm_db_explain", "SELECT * FROM users WHERE id = 1"},
		{"llm_db_explain", "SELECT name FROM users ORDER BY name"},
		{"llm_db_query", "SELECT 1", "-c", "pgn"},
		{"llm_db_query", "SELECT 1", "-c", "pgn", "-d", "a;b"},
		{"llm_db_query", "SELECT pg_read_file('/etc/passwd')"},
		{"llm_db_query", "SELECT * FROM users FOR UPDATE"},
	} {
		same(t, m, d, env, args[0], args[1:]...)
	}
	// .target-database when neither -d nor the URL names one.
	mustWrite(t, filepath.Join(d, ".target-database"), "  other_db  # comment\n", 0o644)
	same(t, m, d, env, "llm_db_query", "SELECT current_database()", "-c", "pgn")
	mustWrite(t, filepath.Join(d, ".target-database"), "host=evil\n", 0o644)
	same(t, m, d, env, "llm_db_query", "SELECT current_database()", "-c", "pgn")
	same(t, m, d, env, "llm_db_query", "SELECT current_database()", "-c", "pgn", "-d", "app")
	same(t, m, d, env, "llm_db_query", "SELECT current_database()")
	// The statement timeout from GASH_DB_TIMEOUT (a non-number means 30).
	for _, v := range []string{"7", "", "x", "007"} {
		e := baseEnv(home, map[string]string{"GASH_DB_TIMEOUT": v})
		same(t, m, d, e, "llm_db_query", "SHOW statement_timeout")
	}
}

// TestPgsqlErrorsKeepTheContract: pgsql_error details come from pgx
// (LEDGER DB-07); code, action, hint and query_time stay.
func TestPgsqlErrorsKeepTheContract(t *testing.T) {
	port := pgServer(t)
	d, home := pgHome(t, port)
	m := frozen(New())
	for _, tc := range []struct {
		env  map[string]string
		args []string
	}{
		{nil, []string{"SELECT * FROM nope"}},
		{nil, []string{"DESC users"}},
		{nil, []string{"SELECT nextval('users_id_seq')"}},
		{nil, []string{"SELECT set_config('x.y', '1', false) IS NULL"}},
		{map[string]string{"GASH_DB_TIMEOUT": "1"}, []string{"SELECT count(*) FROM generate_series(1, 1000000000)"}},
		{nil, []string{"SELECT 1", "-c", "down"}},
		{nil, []string{"SELECT 1", "-c", "pgn", "-d", "nosuchdb"}},
	} {
		env := baseEnv(home, tc.env)
		want := run2x(t, d, env, "llm_db_query", tc.args...)
		got := runGo(t, m, d, env, "llm_db_query", tc.args...)
		re := `^\{"error":"(pgsql_error|write_operation_blocked)","details":"(\\.|[^"\\])+","action":"(RETRY|FATAL)","recoverable":(true|false),"hint":"[^"]+","query_time":"0µs"\}` + "\n$"
		require.Regexp(t, re, want.Stderr, tc.args)
		require.Regexp(t, re, got.Stderr, tc.args)
		require.Equal(t, strings.SplitN(want.Stderr, ",", 2)[0], strings.SplitN(got.Stderr, ",", 2)[0], tc.args)
		require.Equal(t, want.RC, got.RC)
		require.Empty(t, got.Stdout)
		require.NotContains(t, got.Stderr, "s@cret")
	}
}
