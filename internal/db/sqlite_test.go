// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const fixtureSQL = `CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT, born DATETIME, score REAL, pic BLOB, note);
INSERT INTO users VALUES (1,'alice','2024-01-02 03:04:05.500',0.1,x'414243',NULL),(2,'bob',NULL,1.0,NULL,7),
 (3,'carol "c" \','2001-09-09',1e300,x'c3a9',2.5e-10),(4,'dave
two lines',1700000000,-0.0,x'',''),(5,'é ☃ 😀',NULL,123456789.123456789,NULL,'tab	in');
CREATE INDEX ix ON users(name);
CREATE TABLE odd("a b" TEXT, "x""y" INT, d DATE, ts TIMESTAMP);
INSERT INTO odd VALUES ('1',2,'2020-02-02','2020-02-02T10:00:00Z'),('3',4,'not a date',12);
CREATE TABLE empty_t(x);`

func TestSQLiteMatches2x(t *testing.T) {
	d := physTemp(t)
	home := filepath.Join(d, "home")
	require.NoError(t, os.Mkdir(home, 0o755))
	db := sqliteFixture(t, d, fixtureSQL)
	mustWrite(t, filepath.Join(d, "plain.txt"), "not sqlite\n", 0o644)
	m := frozen(New())
	env := baseEnv(home, nil)
	for _, args := range sqliteCases(d, db) {
		same(t, m, d, env, args[0], args[1:]...)
	}
}

// sqliteCases are the llm_db_* calls on the fixture db (in dir d) that
// the 2.x comparison and the --format json golden test (rows_test.go) run.
func sqliteCases(d, db string) [][]string {
	return [][]string{
		{"llm_db_tables", "-f", db},
		{"llm_db_tables", "extra", "-f", db},
		{"llm_db_schema", "users", "-f", db},
		{"llm_db_schema", "odd", "-f", db},
		{"llm_db_schema", "nope", "-f", db},
		{"llm_db_sample", "users", "-f", db},
		{"llm_db_sample", "users", "-f", db, "-n", "1"},
		{"llm_db_sample", "odd", "-f", db, "--limit", "0"},
		{"llm_db_sample", "empty_t", "-f", db},
		{"llm_db_query", "SELECT * FROM users", "-f", db},
		{"llm_db_query", "SELECT * FROM users -- trailing comment", "-f", db},
		{"llm_db_query", "SELECT * FROM users;", "-f", db, "-r", "2"},
		{"llm_db_query", "SELECT * FROM odd", "-f", db},
		{"llm_db_query", "SELECT d, upper(d) AS u, ts FROM odd", "-f", db},
		{"llm_db_query", "SELECT 1 AS a, 2 AS a, 3 AS A", "-f", db},
		{"llm_db_query", "VALUES (1),(2.5),('x')", "-f", db},
		{"llm_db_query", "WITH a AS (SELECT 1 AS n) SELECT * FROM a", "-f", db},
		{"llm_db_query", "SELECT * FROM users WHERE id > 9", "-f", db},
		{"llm_db_query", "PRAGMA table_info(users)", "-f", db},
		{"llm_db_query", "PRAGMA index_list(users)", "-f", db},
		{"llm_db_query", "SELECT 9223372036854775807 AS big, -9223372036854775808 AS small, 1e19 AS r19, 1e20 AS r20, 9e999 AS inf", "-f", db},
		{"llm_db_query", "SELECT id FROM users", "-f", db, "--limit", "3"},
		{"llm_db_query", "SELECT id FROM users", "-f", db, "-n", "x"},
		{"llm_db_query", "SELECT id FROM users", "-f", db, "-r", "1000000000"},
		{"llm_db_query", "EXPLAIN QUERY PLAN SELECT * FROM users WHERE name = 1", "-f", db},
		{"llm_db_query", "EXPLAIN SELECT 1", "-f", db},
		{"llm_db_query", "EXPLAIN QUERY PLAN SELECT 1", "-f", db},
		{"llm_db_explain", "SELECT * FROM users WHERE name = 1 AND id IN (SELECT id FROM users)", "-f", db},
		{"llm_db_explain", "SELECT * FROM users u JOIN odd o ON o.\"x\"\"y\" = u.id ORDER BY u.name", "-f", db},
		{"llm_db_explain", "SELECT 1", "-f", db},
		{"llm_db_explain", "WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x<3) SELECT * FROM c", "-f", db},
		{"llm_db_explain", "PRAGMA table_info(users)", "-f", db},
		{"llm_db_explain", "SELECT 1", "-f", db, "--analyze"},
		// Argument and guard errors.
		{"llm_db_query", "SELECT 1", "--bogus"},
		{"llm_db_query", "SELECT", "1"},
		{"llm_db_query", "", "SELECT 1", "-f", db},
		{"llm_db_query", "-f", db},
		{"llm_db_query"},
		{"llm_db_query", "SELECT 1", "-c"},
		{"llm_db_query", "SELECT 1", "-d --database", "x", "-f", db},
		{"llm_db_tables", "-c"},
		{"llm_db_tables", "--limit", "3", "-f", db},
		{"llm_db_sample", "-f", db},
		{"llm_db_sample", "bad-name", "-f", db},
		{"llm_db_sample", "users", "-f", db, "-n"},
		{"llm_db_sample", "users", "-f", db, "-n", "-1"},
		{"llm_db_schema", "1x", "-f", db},
		{"llm_db_explain", "-f", db},
		{"llm_db_explain", "SELECT 1", "-f", db, "-r", "3"},
		{"llm_db_query", "DELETE FROM users", "-f", db},
		{"llm_db_query", "SELECT 1; SELECT 2", "-f", db},
		{"llm_db_query", "SELECT 1\n.shell id", "-f", db},
		{"llm_db_query", "ATTACH 'x.db' AS x", "-f", db},
		{"llm_db_query", "SELECT 1", "-f", filepath.Join(d, "missing.db")},
		{"llm_db_query", "SELECT 1", "-f", filepath.Join(d, "plain.txt")},
		{"llm_db_query", "SELECT 1", "-f", "/etc/shadow"},
		{"llm_db_query", "SELECT 1", "-f", d},
		{"llm_db_query", "SELECT 1", "-f", ""},
		{"llm_db_query", "SELECT 1"},
		{"llm_db_query", "SELECT 1", "-c", "nope"},
		{"llm_db_query", "SELECT 1", "-h"},
		{"llm_db_query", "SELECT 1", "-d", "--help"},
		{"llm_db_tables", "--help"},
		{"llm_db_schema", "-h"},
		{"llm_db_sample", "x", "-h"},
		{"llm_db_explain", "-h", "SELECT 1"},
	}
}

// TestSQLiteErrorsKeepTheContract: the sqlite_error details come from the
// driver (LEDGER DB-07); the code, action and hint stay.
func TestSQLiteErrorsKeepTheContract(t *testing.T) {
	d := physTemp(t)
	db := sqliteFixture(t, d, fixtureSQL)
	m := frozen(New())
	env := baseEnv(d, nil)
	for _, q := range []string{"SELECT * FROM nope", "SELECT nope FROM users", "PRAGMA user_version = 5", "SELECT abs(-9223372036854775808)", "SELECT `a\\`"} {
		want := run2x(t, d, env, "llm_db_query", q, "-f", db)
		got := runGo(t, m, d, env, "llm_db_query", q, "-f", db)
		require.Equal(t, want.RC, got.RC, q)
		require.Empty(t, got.Stdout)
		require.Regexp(t, `^\{"error":"sqlite_error","details":"(\\.|[^"\\])+","action":"RETRY","recoverable":true,"hint":"Fix the SQL and retry","query_time":"0µs"\}`+"\n$", got.Stderr, q)
		require.Regexp(t, `^\{"error":"sqlite_error","details":"(\\.|[^"\\])+","action":"RETRY","recoverable":true,"hint":"Fix the SQL and retry","query_time":"0µs"\}`+"\n$", want.Stderr, q)
	}
}
