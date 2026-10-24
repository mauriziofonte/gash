// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// TestLedger_DB05: with no jq anywhere on PATH the binary still prints the
// {data, rows, query_time} envelope (2.x printed the raw rows).
func TestLedger_DB05(t *testing.T) {
	d := physTemp(t)
	db := sqliteFixture(t, d, "CREATE TABLE u(id INTEGER); INSERT INTO u VALUES (1);")
	empty := filepath.Join(d, "empty-bin")
	require.NoError(t, os.Mkdir(empty, 0o755))
	m := frozen(New())
	env := baseEnv(d, map[string]string{"PATH": empty})
	got := runGo(t, m, d, env, "llm_db_query", "SELECT 1 AS id", "-f", db)
	require.Equal(t, result{`{"data":[{"id":1}],"rows":1,"query_time":"0µs"}` + "\n", "", 0}, got)
	got = runGo(t, m, d, env, "llm_db_tables", "-f", db)
	require.Equal(t, result{`{"data":["u"],"rows":1,"query_time":"0µs"}` + "\n", "", 0}, got)
}

var queryTimeRE = regexp.MustCompile(`"query_time":"([0-9]+)\.([0-9])ms"`)

// TestLedger_DB06_2x: 2.x started the clock before the client connected,
// so a slow connect with a fast query tripped the 100 ms rule and added
// slow_query_explain (rc.A pinned it with a fake server whose connect took
// 150 ms of the module's clock). 3.0: DB-06 fixed, the same connect counts
// nothing: query_time is the query's own, and no plan is added.
func TestLedger_DB06_2x(t *testing.T) {
	d, _, env := mysqlHome(t)
	plan := `{"query_block":{"select_id":1}}`
	f := &fakeMySQL{answers: map[string]fakeAnswer{
		"SELECT id FROM t":                     {cols: []string{"id"}, rows: [][]driver.Value{{int64(1)}}},
		"EXPLAIN FORMAT=JSON SELECT id FROM t": {cols: []string{"EXPLAIN"}, rows: [][]driver.Value{{[]byte(plan)}}},
	}}
	m := New()
	clock := time.Unix(1792007131, 0)
	m.now = func() time.Time { return clock }
	m.openMySQL = func(cfg *mysql.Config) (*sql.DB, error) {
		clock = clock.Add(150 * time.Millisecond)
		return f.open(cfg)
	}
	got := runGo(t, m, d, env, "llm_db_query", "SELECT id FROM t", "-r", "5")
	require.Equal(t, result{`{"data":[{"id":"1"}],"rows":1,"query_time":"0µs"}` + "\n", "", 0}, got)
	require.Equal(t, []string{"SET SESSION TRANSACTION READ ONLY", "SET SESSION sql_select_limit = 5", "SELECT id FROM t"}, f.log())
}

// TestLedger_DB06_fix: the 100 ms rule and query_time see the query alone:
// a slow query trips it (the plan runs without the row cap), a slow
// connect plus a query under the threshold does not, and a call that runs
// no query (a guard refusal) counts its whole time.
func TestLedger_DB06_fix(t *testing.T) {
	d, _, env := mysqlHome(t)
	plan := `{"query_block":{"select_id":1}}`
	f := &fakeMySQL{answers: map[string]fakeAnswer{
		"SELECT id FROM t":                     {cols: []string{"id"}, rows: [][]driver.Value{{int64(1)}}},
		"EXPLAIN FORMAT=JSON SELECT id FROM t": {cols: []string{"EXPLAIN"}, rows: [][]driver.Value{{[]byte(plan)}}},
	}}
	m := New()
	clock := time.Unix(1792007131, 0)
	m.now = func() time.Time { return clock }
	connect := 2 * time.Second
	query := 150 * time.Millisecond
	m.openMySQL = func(cfg *mysql.Config) (*sql.DB, error) {
		clock = clock.Add(connect)
		return f.open(cfg)
	}
	f.onQuery = func(q string) {
		if q == "SELECT id FROM t" {
			clock = clock.Add(query)
		}
	}
	got := runGo(t, m, d, env, "llm_db_query", "SELECT id FROM t", "-r", "5")
	require.Equal(t, result{`{"data":[{"id":"1"}],"rows":1,"query_time":"150.0ms","slow_query_explain":[` + plan + `]}` + "\n", "", 0}, got)
	require.Equal(t, []string{"SET SESSION TRANSACTION READ ONLY", "SET SESSION sql_select_limit = 5", "SELECT id FROM t",
		"SET SESSION TRANSACTION READ ONLY", "EXPLAIN FORMAT=JSON SELECT id FROM t"}, f.log())

	query = 99999 * time.Microsecond
	got = runGo(t, m, d, env, "llm_db_query", "SELECT id FROM t")
	require.Equal(t, result{`{"data":[{"id":"1"}],"rows":1,"query_time":"99.9ms"}` + "\n", "", 0}, got)

	got = runGo(t, m, d, env, "llm_db_query", "DELETE FROM t")
	require.Equal(t, 1, got.RC)
	require.Contains(t, got.Stderr, `"query_time":"0µs"`)
}

// cliEQP is what the sqlite3 shell makes of EXPLAIN QUERY PLAN QUERY: the
// plan rows (.explain off, .mode json) and its text tree.
func cliEQP(t *testing.T, db, query string) ([]eqpRow, string) {
	t.Helper()
	cmd := exec.Command("sqlite3", db)
	cmd.Stdin = strings.NewReader(".explain off\n.mode json\nEXPLAIN QUERY PLAN " + query + ";\n")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	var raw []struct {
		ID     int64  `json:"id"`
		Parent int64  `json:"parent"`
		Detail string `json:"detail"`
	}
	require.NoError(t, json.Unmarshal(out, &raw), string(out))
	rows := make([]eqpRow, len(raw))
	for i, r := range raw {
		rows[i] = eqpRow{id: r.ID, parent: r.Parent, text: r.Detail}
	}
	tree, err := exec.Command("sqlite3", db, "EXPLAIN QUERY PLAN "+query).CombinedOutput()
	require.NoError(t, err, string(tree))
	return rows, string(tree)
}

// TestLedger_DB08: the plan tree rebuilt from the plan rows is the sqlite3
// shell's own rendering of the same rows, the depth limit included, and
// llm_db_explain wraps it as [{plan}] with the table name in it.
func TestLedger_DB08(t *testing.T) {
	d := physTemp(t)
	db := sqliteFixture(t, d, "CREATE TABLE u(id INTEGER PRIMARY KEY, name TEXT, g INT); CREATE INDEX ix ON u(name);")
	var chain strings.Builder
	chain.WriteString("WITH c0 AS MATERIALIZED (SELECT g FROM u ORDER BY random())")
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&chain, ", c%d AS MATERIALIZED (SELECT g FROM c%d ORDER BY random() LIMIT 3)", i, i-1)
	}
	chain.WriteString(" SELECT * FROM c40")
	for _, q := range []string{
		"SELECT * FROM u WHERE id = 1",
		"SELECT * FROM u WHERE name = 'x' ORDER BY g",
		"SELECT * FROM u WHERE id IN (SELECT g FROM u WHERE name IN (SELECT name FROM u GROUP BY name)) UNION SELECT * FROM u ORDER BY 2",
		"SELECT (SELECT count(*) FROM u b WHERE b.g = a.id) FROM u a LEFT JOIN u c ON c.g = a.g",
		"WITH RECURSIVE r(x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM r WHERE x < 3) SELECT * FROM r, u",
		chain.String(),
	} {
		rows, tree := cliEQP(t, db, q)
		require.Equal(t, tree, renderEQP(rows), q)
	}

	m := frozen(New())
	got := runGo(t, m, d, baseEnv(d, nil), "llm_db_explain", "SELECT * FROM u WHERE id = 1", "-f", db)
	var env struct {
		Data []struct{ Plan string } `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(got.Stdout), &env), got.Stdout)
	require.Len(t, env.Data, 1)
	_, tree := cliEQP(t, db, "SELECT * FROM u WHERE id = 1")
	require.Equal(t, strings.TrimRight(tree, "\n"), env.Data[0].Plan)
	require.Regexp(t, "u", env.Data[0].Plan)
}

// TestLedger_DB09: pgx sends the text to the server, so a psql
// meta-command is SQL there and fails; nothing runs on the client. The
// guard is bypassed here only by calling the transport directly.
func TestLedger_DB09(t *testing.T) {
	port := pgServer(t)
	d, home := pgHome(t, port)
	empty := filepath.Join(d, "empty-bin")
	require.NoError(t, os.Mkdir(empty, 0o755))
	env := baseEnv(home, map[string]string{"PATH": empty})
	marker := filepath.Join(d, "marker")
	m := frozen(New())
	tg := target{driver: "pgsql", user: "postgres", pass: "s@cret", host: "127.0.0.1", port: strconv.Itoa(port), database: "app"}
	e := invoke.EnvMap(env)
	c := &invoke.Call{Ctx: context.Background(), Env: e, Term: term.Fixed(e, false, false, false)}
	for _, q := range []string{"\\! touch " + marker, "SELECT 1\n\\! touch " + marker + "\n", "\\copy (SELECT 1) TO '" + marker + "'"} {
		for _, asJSON := range []bool{false, true} {
			o := m.pgsqlRun(c, tg, asJSON, q)
			require.NotNil(t, o.fail, q)
			require.Equal(t, "pgsql_error", o.fail.Code, q)
			require.Contains(t, o.fail.Details, "syntax error", q)
		}
	}
	_, err := os.Stat(marker)
	require.True(t, os.IsNotExist(err), "a meta-command ran on the client")

	// 3.0: DB-01 fixed. The DB-01 vector passed the guard in 2.x and
	// reached the server as SQL, which refused it; the guard now refuses
	// its backslash line before any connection.
	got := runGo(t, m, d, env, "llm_db_query", "SELECT 1 WHERE somee'\\'\n\\1\n'")
	require.Equal(t, 1, got.RC)
	require.Regexp(t, `^\{"error":"client_command_blocked","details":"Backslash outside a string literal","action":"FATAL"`, got.Stderr)
	got = runGo(t, m, d, env, "llm_db_query", "SELECT 1 AS one")
	require.Equal(t, result{`{"data":[{"one":1}],"rows":1,"query_time":"0µs"}` + "\n", "", 0}, got)
}

// keysOf is the key list of each object of .data.
func keysOf(t *testing.T, stdout string) [][]string {
	t.Helper()
	vals, err := parseJSON(stdout)
	require.NoError(t, err, stdout)
	require.Len(t, vals, 1, stdout)
	var data jv
	for i, k := range vals[0].keys {
		if k == "data" {
			data = vals[0].vals[i]
		}
	}
	var out [][]string
	for _, row := range data.arr {
		out = append(out, row.keys)
	}
	return out
}

// TestLedger_DB10: llm_db_schema keeps each engine's own key set.
func TestLedger_DB10(t *testing.T) {
	d := physTemp(t)
	db := sqliteFixture(t, d, "CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT);")
	m := frozen(New())
	got := runGo(t, m, d, baseEnv(d, nil), "llm_db_schema", "users", "-f", db)
	sqliteKeys := []string{"cid", "name", "type", "notnull", "dflt_value", "pk"}
	require.Equal(t, [][]string{sqliteKeys, sqliteKeys}, keysOf(t, got.Stdout))

	md, _, menv := mysqlHome(t)
	f := &fakeMySQL{}
	m.openMySQL = f.open
	q := "SELECT column_name AS name, column_type AS type, is_nullable AS nullable, column_key AS `key`, column_default AS `default` FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'users' ORDER BY ordinal_position"
	answer(t, md, f, q, "", []string{"name", "type", "nullable", "key", "default"},
		[]driver.Value{[]byte("id"), []byte("int"), []byte("NO"), []byte("PRI"), nil})
	got = runGo(t, m, md, menv, "llm_db_schema", "users")
	require.Equal(t, [][]string{{"name", "type", "nullable", "key", "default"}}, keysOf(t, got.Stdout))
	require.Equal(t, q, f.log()[len(f.log())-1])

	port := pgServer(t)
	pd, home := pgHome(t, port)
	got = runGo(t, m, pd, baseEnv(home, nil), "llm_db_schema", "users")
	require.Equal(t, 0, got.RC, got.Stderr)
	for _, k := range keysOf(t, got.Stdout) {
		// jsonb orders the keys (shorter first), as in 2.x.
		require.Equal(t, []string{"data_type", "column_name", "is_nullable", "column_default"}, k)
	}
}

// TestLedger_DB11: -h and --help anywhere print the help of every
// llm_db_* function, also where an option expects a value.
func TestLedger_DB11(t *testing.T) {
	d := physTemp(t)
	m := frozen(New())
	env := baseEnv(d, nil)
	for _, fn := range []string{"llm_db_query", "llm_db_tables", "llm_db_schema", "llm_db_sample", "llm_db_explain"} {
		for _, args := range [][]string{
			{"-h"},
			{"--help"},
			{"x", "-h"},
			{"-c", "-h"},
			{"-h", "127.0.0.1"},
			{"x", "-f", "nope.db", "--help", "-d"},
		} {
			got := runGo(t, m, d, env, fn, args...)
			require.Equal(t, 0, got.RC)
			require.Contains(t, got.Stdout, fn)
			require.Empty(t, got.Stderr)
		}
	}
}

// TestLedger_CORE05: gash_db_test pings through the native drivers with
// the 2.x lines and exit codes (the 2.x reference runs a mock mysql client
// and the real psql); no client is looked up or started.
func TestLedger_CORE05(t *testing.T) {
	d, home, env := mysqlHome(t)
	mustWrite(t, filepath.Join(d, "bin", "mysql"), "#!/usr/bin/env bash\nexit \"${MOCK_RC:-0}\"\n", 0o755)
	mustWrite(t, filepath.Join(home, ".gash_env"),
		"DB:default=mysql://u:p%40ss@127.0.0.1:3306/app\nDB:noport=mariadb://u:p@db.example/app\nDB:odd=redis://u@h:1/x\n", 0o600)
	f := &fakeMySQL{answers: map[string]fakeAnswer{"SELECT 1": {cols: []string{"1"}, rows: [][]driver.Value{{int64(1)}}}}}
	m := frozen(New())
	m.openMySQL = f.open
	same(t, m, d, env, "gash_db_test")
	require.Equal(t, "p@ss", f.cfg.Passwd)
	require.Equal(t, "127.0.0.1:3306", f.cfg.Addr)
	require.Equal(t, "app", f.cfg.DBName)
	same(t, m, d, env, "gash_db_test", "noport")
	require.Equal(t, "db.example:3306", f.cfg.Addr)
	for _, name := range []string{"default", "odd", "nope", "-h"} {
		same(t, m, d, env, "gash_db_test", name)
	}

	// A failed connection: the same lines as a failing client.
	f.connErr = &mysql.MySQLError{Number: 1045, Message: "Access denied"}
	fenv := baseEnv(home, map[string]string{"PATH": env["PATH"], "MOCK_DIR": d, "MOCK_RC": "1"})
	got := same(t, m, d, fenv, "gash_db_test")
	require.Equal(t, 1, got.RC)

	// No client on PATH: the native driver still connects.
	f.connErr = nil
	empty := filepath.Join(d, "empty-bin")
	require.NoError(t, os.Mkdir(empty, 0o755))
	got = runGo(t, m, d, baseEnv(home, map[string]string{"PATH": empty}), "gash_db_test")
	require.Equal(t, 0, got.RC)
	require.Contains(t, got.Stderr, "Connection successful!")
	require.NotContains(t, got.Stderr, "not found")

	// PostgreSQL against the test cluster, 2.x through the real psql.
	port := pgServer(t)
	pd, phome := pgHome(t, port)
	penv := baseEnv(phome, nil)
	for _, name := range []string{"default", "pgn", "down"} {
		same(t, m, pd, penv, "gash_db_test", name)
	}
	got = runGo(t, m, pd, baseEnv(phome, map[string]string{"PATH": empty}), "gash_db_test")
	require.Equal(t, 0, got.RC)
	require.Contains(t, got.Stderr, "Connection successful!")
}

// TestLedger_DB14_2x: 2.x failed an EXPLAIN on SQLite through
// llm_db_query, its text table refused by jq: the jq message, exit 5,
// nothing on stdout. 3.0: DB-14 fixed, each is an envelope on stdout.
func TestLedger_DB14_2x(t *testing.T) {
	d := physTemp(t)
	home := filepath.Join(d, "home")
	require.NoError(t, os.Mkdir(home, 0o755))
	db := sqliteFixture(t, d, "CREATE TABLE u(id INTEGER PRIMARY KEY, name TEXT);")
	m := frozen(New())
	env := baseEnv(home, nil)
	for _, q := range []string{"EXPLAIN SELECT 1", "EXPLAIN QUERY PLAN SELECT * FROM u WHERE id = 1", "explain select name from u"} {
		got := runGo(t, m, d, env, "llm_db_query", q, "-f", db)
		require.Equal(t, 0, got.RC, q)
		require.Empty(t, got.Stderr, q)
		require.True(t, json.Valid([]byte(got.Stdout)), got.Stdout)
	}
}

// TestLedger_DB14_fix: llm_db_query wraps SQLite's EXPLAIN QUERY PLAN as
// [{plan: TEXT}], the tree llm_db_explain gives, and returns a bytecode
// EXPLAIN as its rows.
func TestLedger_DB14_fix(t *testing.T) {
	d := physTemp(t)
	db := sqliteFixture(t, d, "CREATE TABLE u(id INTEGER PRIMARY KEY, name TEXT);")
	m := frozen(New())
	env := baseEnv(d, nil)
	q := "SELECT * FROM u WHERE id = 1"
	got := runGo(t, m, d, env, "llm_db_query", "EXPLAIN QUERY PLAN "+q, "-f", db)
	want := runGo(t, m, d, env, "llm_db_explain", q, "-f", db)
	require.Equal(t, 0, got.RC, got.Stderr)
	require.Equal(t, want, got)
	require.Regexp(t, `^\{"data":\[\{"plan":"QUERY PLAN\\n`+"`"+`--SEARCH u USING INTEGER PRIMARY KEY \(rowid=\?\)"\}\],"rows":1,"query_time":"0µs"\}\n$`, got.Stdout)

	got = runGo(t, m, d, env, "llm_db_query", "EXPLAIN SELECT 1", "-f", db)
	require.Equal(t, 0, got.RC, got.Stderr)
	rows := keysOf(t, got.Stdout)
	require.NotEmpty(t, rows)
	require.Equal(t, []string{"addr", "opcode", "p1", "p2", "p3", "p4", "p5", "comment"}, rows[0])
	require.Contains(t, got.Stdout, `"opcode":"Init"`)
}

// TestLedger_DB15: SQLite text that is not UTF-8, or holds a NUL, comes
// back whole as valid JSON: one U+FFFD per invalid sequence as jq 1.7
// decodes it, \u0000 for NUL (2.x: ￿ffff escapes, text cut at NUL).
func TestLedger_DB15(t *testing.T) {
	d := physTemp(t)
	db := sqliteFixture(t, d, "CREATE TABLE b(v TEXT); INSERT INTO b VALUES "+
		"(CAST(x'ff00' AS TEXT)), (CAST(x'e28278' AS TEXT)), ('a' || char(0) || 'b'), "+
		"(CAST(x'c0af' AS TEXT)), (CAST(x'eda080' AS TEXT)), (CAST(x'f4908080' AS TEXT)), (CAST(x'e282ac' AS TEXT));")
	m := frozen(New())
	env := baseEnv(d, nil)
	got := runGo(t, m, d, env, "llm_db_query", "SELECT v FROM b", "-f", db)
	require.Equal(t, 0, got.RC, got.Stderr)
	require.True(t, json.Valid([]byte(got.Stdout)), got.Stdout)
	require.Equal(t, `{"data":[{"v":"�\u0000"},{"v":"�x"},{"v":"a\u0000b"},{"v":"��"},`+
		`{"v":"�"},{"v":"�"},{"v":"€"}],"rows":7,"query_time":"0µs"}`+"\n",
		strings.NewReplacer("�", `�`, "€", `€`).Replace(got.Stdout))
}

// TestLedger_DB17: table and database names take ASCII letters and digits
// only, whatever the locale (2.x's bracket ranges followed collation).
func TestLedger_DB17(t *testing.T) {
	d, _, env := mysqlHome(t)
	db := sqliteFixture(t, d, "CREATE TABLE ok_1(id INTEGER);")
	m := frozen(New())
	m.openMySQL = (&fakeMySQL{}).open
	for _, loc := range []string{"C", "C.UTF-8", "en_US.UTF-8", "tr_TR.UTF-8"} {
		env["LC_ALL"], env["LANG"] = loc, loc
		for _, name := range []string{"té", "ß", "ıd", "ａ", "Ǆ", "á", "٣"} {
			got := runGo(t, m, d, env, "llm_db_schema", name, "-f", db)
			require.Equal(t, 1, got.RC, "%s %q", loc, name)
			require.Contains(t, got.Stderr, `"error":"invalid_table_name"`, "%s %q", loc, name)
			got = runGo(t, m, d, env, "llm_db_tables", "-d", name)
			require.Contains(t, got.Stderr, `"error":"invalid_database_name"`, "%s %q", loc, name)
			mustWrite(t, filepath.Join(d, ".target-database"), name+"\n", 0o644)
			got = runGo(t, m, d, env, "llm_db_tables", "-c", "maria")
			require.Contains(t, got.Stderr, `"error":"invalid_database_name"`, "%s %q", loc, name)
		}
		got := runGo(t, m, d, env, "llm_db_schema", "ok_1", "-f", db)
		require.Equal(t, 0, got.RC, got.Stderr)
	}
}
