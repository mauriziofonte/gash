// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"database/sql/driver"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFormatJSONIsTheEnvelope (plan D1): --format json prints what the
// call prints without it, byte for byte, on every fixture call of the
// 2.x comparison (errors and help included), and the envelope itself is
// pinned (golden) so neither drifts.
func TestFormatJSONIsTheEnvelope(t *testing.T) {
	d := physTemp(t)
	db := sqliteFixture(t, d, fixtureSQL)
	m := frozen(New())
	env := baseEnv(d, nil)
	for _, args := range sqliteCases(d, db) {
		want := runGo(t, m, d, env, args[0], args[1:]...)
		got := runGo(t, m, d, env, args[0], append([]string{"--format", "json"}, args[1:]...)...)
		require.Equal(t, want, got, "%q", args)
	}
	golden := map[string]result{
		"SELECT * FROM users": {`{"data":[{"id":1,"name":"alice","born":"2024-01-02 03:04:05.500","score":0.100000000000000005,"pic":"ABC","note":null},` +
			`{"id":2,"name":"bob","born":null,"score":1.0,"pic":null,"note":7},` +
			`{"id":3,"name":"carol \"c\" \\","born":"2001-09-09","score":1.000000000000000052E+300,"pic":"é","note":2.50000000000000015E-10},` +
			`{"id":4,"name":"dave\ntwo lines","born":1700000000,"score":0.0,"pic":"","note":""},` +
			`{"id":5,"name":"é ☃ 😀","born":null,"score":123456789.123456791,"pic":null,"note":"tab\tin"}],"rows":5,"query_time":"0µs"}` + "\n", "", 0},
		"SELECT 1 AS a WHERE 0": {`{"data":[],"rows":0,"query_time":"0µs"}` + "\n", "", 0},
	}
	for q, want := range golden {
		require.Equal(t, want, runGo(t, m, d, env, "llm_db_query", q, "-f", db), q)
		require.Equal(t, want, runGo(t, m, d, env, "llm_db_query", q, "-f", db, "--format", "json"), q)
	}
}

// TestRowFormats: jsonl, tsv and table print the rows (NULL as null, \N
// or an empty cell; REALs in their shortest form; tabs and newlines
// escaped in TSV) and the metadata as one JSON line on stderr, with
// "truncated": true when the -n cap cut rows.
func TestRowFormats(t *testing.T) {
	d := physTemp(t)
	db := sqliteFixture(t, d, fixtureSQL)
	m := frozen(New())
	env := baseEnv(d, nil)
	q := "SELECT id, name, score, note FROM users"
	got := runGo(t, m, d, env, "llm_db_query", q, "-f", db, "--format", "tsv")
	require.Equal(t, result{"id\tname\tscore\tnote\n" +
		"1\talice\t0.1\t\\N\n" +
		"2\tbob\t1.0\t7\n" +
		"3\tcarol \"c\" \\\\\t1e+300\t2.5e-10\n" +
		"4\tdave\\ntwo lines\t0.0\t\n" +
		"5\té ☃ 😀\t123456789.12345679\ttab\\tin\n",
		`{"query_time":"0µs","rows":5}` + "\n", 0}, got)

	got = runGo(t, m, d, env, "llm_db_query", q, "-f", db, "--format", "jsonl", "-n", "2")
	require.Equal(t, result{`{"id":1,"name":"alice","score":0.1,"note":null}` + "\n" +
		`{"id":2,"name":"bob","score":1.0,"note":7}` + "\n",
		`{"query_time":"0µs","rows":2,"truncated":true}` + "\n", 0}, got)

	got = runGo(t, m, d, env, "llm_db_query", q+" WHERE id < 3", "-f", db, "--format", "table")
	require.Equal(t, result{"id  name   score  note\n1   alice  0.1    \n2   bob    1.0    7\n",
		`{"query_time":"0µs","rows":2}` + "\n", 0}, got)

	// Exactly -n rows: nothing was cut.
	got = runGo(t, m, d, env, "llm_db_query", "SELECT id FROM users", "-f", db, "--format", "tsv", "-n", "5")
	require.Equal(t, `{"query_time":"0µs","rows":5}`+"\n", got.Stderr)
	// No row: the header still names the columns.
	got = runGo(t, m, d, env, "llm_db_query", "SELECT id, name FROM users WHERE 0", "-f", db, "--format", "tsv")
	require.Equal(t, result{"id\tname\n", `{"query_time":"0µs","rows":0}` + "\n", 0}, got)
	// A statement no cap applies to (PRAGMA) keeps every row.
	got = runGo(t, m, d, env, "llm_db_query", "PRAGMA table_info(users)", "-f", db, "--format", "tsv", "-n", "1")
	require.Equal(t, 7, strings.Count(got.Stdout, "\n"), got.Stdout)
	require.Equal(t, `{"query_time":"0µs","rows":6}`+"\n", got.Stderr)
	// The other functions take the formats too.
	got = runGo(t, m, d, env, "llm_db_tables", "-f", db, "--format", "tsv")
	require.Equal(t, result{"name\nempty_t\nodd\nusers\n", `{"query_time":"0µs","rows":3}` + "\n", 0}, got)
	got = runGo(t, m, d, env, "llm_db_sample", "odd", "-f", db, "--format", "jsonl", "-n", "1")
	require.Equal(t, result{`{"a b":"1","x\"y":2,"d":"2020-02-02","ts":"2020-02-02T10:00:00Z"}` + "\n",
		`{"query_time":"0µs","rows":1,"truncated":true}` + "\n", 0}, got)
	got = runGo(t, m, d, env, "llm_db_explain", "SELECT 1", "-f", db, "--format", "tsv")
	require.Equal(t, result{"plan\nQUERY PLAN\\n`--SCAN CONSTANT ROW\n", `{"query_time":"0µs","rows":1}` + "\n", 0}, got)
	got = runGo(t, m, d, env, "llm_db_schema", "users", "-f", db, "--format", "table")
	require.Equal(t, 0, got.RC, got.Stderr)
	require.True(t, strings.HasPrefix(got.Stdout, "cid  name   type      notnull  dflt_value  pk\n0    id     INTEGER   0                    1\n"), got.Stdout)

	// An unknown format is refused before anything runs, with query_time.
	got = runGo(t, m, d, env, "llm_db_query", "SELECT 1", "-f", db, "--format", "csv")
	require.Equal(t, result{"", `{"error":"invalid_format","details":"csv","action":"RETRY","recoverable":true,"hint":"Use --format json, jsonl, tsv or table","query_time":"0µs"}` + "\n", 1}, got)
	got = runGo(t, m, d, env, "llm_db_query", "SELECT 1", "-f", db, "--format")
	require.Contains(t, got.Stderr, `"error":"missing_value","details":"--format"`)
	// The SQL guard still refuses first, whatever the format.
	got = runGo(t, m, d, env, "llm_db_query", "DELETE FROM users", "-f", db, "--format", "tsv")
	require.Equal(t, 1, got.RC)
	require.Contains(t, got.Stderr, `"error":"write_operation_blocked"`)
	require.Empty(t, got.Stdout)
}

// TestScalar: --scalar prints the one value of a 1x1 result as a TSV
// cell; any other shape is not_a_scalar (RETRY) with the shape.
func TestScalar(t *testing.T) {
	d := physTemp(t)
	db := sqliteFixture(t, d, fixtureSQL)
	m := frozen(New())
	env := baseEnv(d, nil)
	for q, want := range map[string]string{
		"SELECT count(*) FROM users":         "5\n",
		"SELECT name FROM users WHERE id=4":  "dave\\ntwo lines\n",
		"SELECT note FROM users WHERE id=1":  "\\N\n",
		"SELECT score FROM users WHERE id=1": "0.1\n",
		"SELECT 'x' AS a":                    "x\n",
	} {
		require.Equal(t, result{want, "", 0}, runGo(t, m, d, env, "llm_db_query", q, "-f", db, "--scalar"), q)
	}
	for _, tc := range []struct {
		args  []string
		shape string
	}{
		{[]string{"llm_db_query", "SELECT id, name FROM users WHERE id=1"}, "1 rows x 2 columns"},
		{[]string{"llm_db_query", "SELECT id FROM users"}, "5 rows x 1 columns"},
		{[]string{"llm_db_query", "SELECT id FROM users", "-n", "2"}, "2+ rows x 1 columns"},
		{[]string{"llm_db_query", "SELECT id FROM users WHERE 0"}, "0 rows x 1 columns"},
		{[]string{"llm_db_tables"}, "3 rows x 1 columns"},
	} {
		got := runGo(t, m, d, env, tc.args[0], append(tc.args[1:], "-f", db, "--scalar")...)
		require.Equal(t, result{"", `{"error":"not_a_scalar","details":"` + tc.shape + `","action":"RETRY","recoverable":true,` +
			`"hint":"--scalar needs exactly one row and one column: select one column, add LIMIT 1, or drop --scalar","query_time":"0µs"}` + "\n", 1}, got, "%q", tc.args)
	}
	got := runGo(t, m, d, env, "llm_db_schema", "empty_t", "-f", db, "--scalar")
	require.Equal(t, 1, got.RC)
	require.Contains(t, got.Stderr, `"details":"1 rows x 6 columns"`)
}

// TestSQLiteCatalog (plan D3): llm_db_tables --counts, llm_db_schema
// --all and llm_db_databases on SQLite.
func TestSQLiteCatalog(t *testing.T) {
	d := physTemp(t)
	db := sqliteFixture(t, d, fixtureSQL+`CREATE TABLE "it's ""odd"""(v); INSERT INTO "it's ""odd""" VALUES (1),(2);`)
	m := frozen(New())
	env := baseEnv(d, nil)
	got := runGo(t, m, d, env, "llm_db_tables", "-f", db, "--counts")
	require.Equal(t, result{`{"data":[{"name":"empty_t","rows":0},{"name":"it's \"odd\"","rows":2},{"name":"odd","rows":2},{"name":"users","rows":5}],"rows":4,"query_time":"0µs"}` + "\n", "", 0}, got)
	got = runGo(t, m, d, env, "llm_db_tables", "--counts", "-f", db, "--format", "tsv")
	require.Equal(t, result{"name\trows\nempty_t\t0\nit's \"odd\"\t2\nodd\t2\nusers\t5\n", `{"query_time":"0µs","rows":4}` + "\n", 0}, got)
	got = runGo(t, m, d, env, "llm_db_schema", "--all", "-f", db)
	require.Equal(t, 0, got.RC, got.Stderr)
	keys := keysOf(t, got.Stdout)
	require.Len(t, keys, 1+1+4+6)
	for _, k := range keys {
		require.Equal(t, []string{"table", "cid", "name", "type", "notnull", "dflt_value", "pk"}, k)
	}
	require.True(t, strings.HasPrefix(got.Stdout, `{"data":[{"table":"empty_t","cid":0,"name":"x","type":"","notnull":0,"dflt_value":null,"pk":0},{"table":"it's \"odd\"","cid":0,"name":"v"`), got.Stdout)
	got = runGo(t, m, d, env, "llm_db_schema", "--all", "-f", db, "--format", "tsv")
	require.Equal(t, "table\tcid\tname\ttype\tnotnull\tdflt_value\tpk", strings.SplitN(got.Stdout, "\n", 2)[0])
	require.Equal(t, `{"query_time":"0µs","rows":12}`+"\n", got.Stderr)
	got = runGo(t, m, d, env, "llm_db_schema", "users", "--all", "-f", db)
	require.Contains(t, got.Stderr, `"error":"unexpected_argument","details":"users"`)
	got = runGo(t, m, d, env, "llm_db_databases", "-f", db)
	require.Equal(t, result{`{"data":["main"],"rows":1,"query_time":"0µs"}` + "\n", "", 0}, got)
	got = runGo(t, m, d, env, "llm_db_databases", "-f", db, "--scalar")
	require.Equal(t, result{"main\n", "", 0}, got)
	got = runGo(t, m, d, env, "llm_db_databases", "x", "-f", db)
	require.Contains(t, got.Stderr, `"error":"unexpected_argument","details":"x"`)
	// --counts and --all belong to their own function.
	got = runGo(t, m, d, env, "llm_db_query", "SELECT 1", "-f", db, "--counts")
	require.Contains(t, got.Stderr, `"error":"unknown_option","details":"--counts"`)
	got = runGo(t, m, d, env, "llm_db_tables", "-f", db, "--all")
	require.Contains(t, got.Stderr, `"error":"unknown_option","details":"--all"`)
}

// TestMySQLRowFormats: on MySQL/MariaDB the envelope keeps NULL as the
// string "NULL" (LEDGER DB-02) and --format json is that envelope; the
// row formats carry a real NULL, and a capped query asks for one row more
// than -n to tell whether the cap cut rows.
func TestMySQLRowFormats(t *testing.T) {
	d, _, env := mysqlHome(t)
	f := &fakeMySQL{}
	m := frozen(New())
	m.openMySQL = f.open
	answer(t, d, f, "SELECT a, b FROM t", "", []string{"a", "b"},
		[]driver.Value{nil, int64(1)}, []driver.Value{[]byte("NULL"), int64(2)}, []driver.Value{[]byte("x\ty"), int64(3)})
	envelope := `{"data":[{"a":"NULL","b":"1"},{"a":"NULL","b":"2"},{"a":"x\ty","b":"3"}],"rows":3,"query_time":"0µs"}` + "\n"
	require.Equal(t, result{envelope, "", 0}, runGo(t, m, d, env, "llm_db_query", "SELECT a, b FROM t"))
	require.Equal(t, "SET SESSION sql_select_limit = 100", f.log()[len(f.log())-2])
	require.Equal(t, result{envelope, "", 0}, runGo(t, m, d, env, "llm_db_query", "SELECT a, b FROM t", "--format", "json"))
	got := runGo(t, m, d, env, "llm_db_query", "SELECT a, b FROM t", "--format", "tsv", "-n", "2")
	require.Equal(t, "SET SESSION sql_select_limit = 3", f.log()[len(f.log())-2])
	require.Equal(t, result{"a\tb\n\\N\t1\nNULL\t2\n", `{"query_time":"0µs","rows":2,"truncated":true}` + "\n", 0}, got)
	got = runGo(t, m, d, env, "llm_db_query", "SELECT a, b FROM t", "--format", "jsonl")
	require.Equal(t, `{"a":null,"b":"1"}`+"\n"+`{"a":"NULL","b":"2"}`+"\n"+`{"a":"x\ty","b":"3"}`+"\n", got.Stdout)

	// D3 on MySQL: the estimates as numbers, the schema of every table.
	answer(t, d, f, "SELECT table_name AS name, table_rows AS `rows` FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY table_name",
		"", []string{"name", "rows"}, []driver.Value{[]byte("posts"), uint64(1200)}, []driver.Value{[]byte("v_users"), nil})
	got = runGo(t, m, d, env, "llm_db_tables", "--counts")
	require.Equal(t, result{`{"data":[{"name":"posts","rows":1200},{"name":"v_users","rows":null}],"rows":2,"query_time":"0µs"}` + "\n", "", 0}, got)
	got = runGo(t, m, d, env, "llm_db_tables", "--counts", "--format", "tsv")
	require.Equal(t, result{"name\trows\nposts\t1200\nv_users\t\\N\n", `{"query_time":"0µs","rows":2}` + "\n", 0}, got)
	all := "SELECT table_name AS `table`, column_name AS name, column_type AS type, is_nullable AS nullable, column_key AS `key`, column_default AS `default` " +
		"FROM information_schema.columns WHERE table_schema = DATABASE() ORDER BY table_name, ordinal_position"
	answer(t, d, f, all, "", []string{"table", "name", "type", "nullable", "key", "default"},
		[]driver.Value{[]byte("posts"), []byte("id"), []byte("int"), []byte("NO"), []byte("PRI"), nil},
		[]driver.Value{[]byte("users"), []byte("id"), []byte("int"), []byte("NO"), []byte("PRI"), nil})
	got = runGo(t, m, d, env, "llm_db_schema", "--all")
	require.Equal(t, result{`{"data":[{"table":"posts","name":"id","type":"int","nullable":"NO","key":"PRI","default":"NULL"},` +
		`{"table":"users","name":"id","type":"int","nullable":"NO","key":"PRI","default":"NULL"}],"rows":2,"query_time":"0µs"}` + "\n", "", 0}, got)
	answer(t, d, f, "SELECT schema_name AS name FROM information_schema.schemata ORDER BY schema_name", "", []string{"name"},
		[]driver.Value{[]byte("app")}, []driver.Value{[]byte("information_schema")})
	// A connection without a database lists them too.
	got = runGo(t, m, d, env, "llm_db_databases", "-c", "maria")
	require.Equal(t, result{`{"data":["app","information_schema"],"rows":2,"query_time":"0µs"}` + "\n", "", 0}, got)
	require.Equal(t, "", f.cfg.DBName)
	require.Equal(t, "db.example:3307", f.cfg.Addr)
}

// TestPgsqlCatalog: D1 and D3 on the PostgreSQL test cluster.
func TestPgsqlCatalog(t *testing.T) {
	port := pgServer(t)
	d, home := pgHome(t, port)
	m := frozen(New())
	env := baseEnv(home, nil)
	got := runGo(t, m, d, env, "llm_db_query", "SELECT id, name, amount, note FROM users ORDER BY id", "--format", "tsv")
	require.Equal(t, result{"id\tname\tamount\tnote\n1\talice\t12.50\t\\N\n2\tbob\t\\N\ttab\\tin\n3\tcarol \"c\" \\\\\t0.00\ttwo\\nlines\n4\té ☃ 😀\t99999999.99\t\n",
		`{"query_time":"0µs","rows":4}` + "\n", 0}, got)
	got = runGo(t, m, d, env, "llm_db_query", "SELECT count(*) FROM users", "--scalar")
	require.Equal(t, result{"4\n", "", 0}, got)
	got = runGo(t, m, d, env, "llm_db_query", "SHOW statement_timeout", "--scalar")
	require.Equal(t, result{"30s\n", "", 0}, got)
	got = runGo(t, m, d, env, "llm_db_tables", "--counts")
	require.Equal(t, 0, got.RC, got.Stderr)
	var env1 struct {
		Data []struct {
			Name string
			Rows *int64
		}
	}
	require.NoError(t, json.Unmarshal([]byte(got.Stdout), &env1), got.Stdout)
	require.Len(t, env1.Data, 2)
	require.Equal(t, "empty_t", env1.Data[0].Name)
	require.Equal(t, "users", env1.Data[1].Name)
	require.NoError(t, pgSQL("app", "ANALYZE users"))
	got = runGo(t, m, d, env, "llm_db_tables", "--counts", "--format", "tsv")
	require.Contains(t, got.Stdout, "users\t4\n")
	got = runGo(t, m, d, env, "llm_db_schema", "--all", "--format", "tsv")
	require.Equal(t, 0, got.RC, got.Stderr)
	lines := strings.Split(strings.TrimSuffix(got.Stdout, "\n"), "\n")
	require.Equal(t, "table\tcolumn_name\tdata_type\tis_nullable\tcolumn_default", lines[0])
	require.Len(t, lines, 1+1+9)
	require.Equal(t, "empty_t\tx\tinteger\tYES\t\\N", lines[1])
	got = runGo(t, m, d, env, "llm_db_databases", "-c", "pgn")
	require.Equal(t, result{`{"data":["app","other_db","postgres"],"rows":3,"query_time":"0µs"}` + "\n", "", 0}, got)
}
