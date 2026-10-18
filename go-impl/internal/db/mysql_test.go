// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

// fakeMySQL is an in-process MySQL server for the driver seam: it records
// the session's statements and answers queries from a table.
type fakeMySQL struct {
	mu      sync.Mutex
	cfg     *mysql.Config
	stmts   []string
	answers map[string]fakeAnswer
	connErr error
}

type fakeAnswer struct {
	cols []string
	rows [][]driver.Value
	err  error
	// types and scales are the column metadata (DatabaseTypeName and the
	// decimals; -1 for none), when given.
	types  []string
	scales []int64
}

func (f *fakeMySQL) open(cfg *mysql.Config) (*sql.DB, error) {
	f.mu.Lock()
	f.cfg = cfg
	f.mu.Unlock()
	return sql.OpenDB(fakeConnector{f}), nil
}

func (f *fakeMySQL) log() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.stmts...)
}

type fakeConnector struct{ f *fakeMySQL }

func (c fakeConnector) Connect(context.Context) (driver.Conn, error) {
	if c.f.connErr != nil {
		return nil, c.f.connErr
	}
	return &fakeConn{c.f}, nil
}

func (c fakeConnector) Driver() driver.Driver { return fakeDriver{} }

type fakeDriver struct{}

func (fakeDriver) Open(string) (driver.Conn, error) { return nil, errors.New("not used") }

type fakeConn struct{ f *fakeMySQL }

func (c *fakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("no prepared statements")
}
func (c *fakeConn) Close() error              { return nil }
func (c *fakeConn) Begin() (driver.Tx, error) { return nil, errors.New("no transactions") }

func (c *fakeConn) ExecContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Result, error) {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	c.f.stmts = append(c.f.stmts, q)
	return driver.RowsAffected(0), nil
}

func (c *fakeConn) QueryContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	c.f.stmts = append(c.f.stmts, q)
	a, ok := c.f.answers[q]
	if !ok {
		return nil, &mysql.MySQLError{Number: 1064, SQLState: [5]byte{'4', '2', '0', '0', '0'}, Message: "You have an error in your SQL syntax"}
	}
	if a.err != nil {
		return nil, a.err
	}
	return &fakeRows{a: a}, nil
}

type fakeRows struct {
	a fakeAnswer
	i int
}

func (r *fakeRows) Columns() []string { return r.a.cols }

func (r *fakeRows) ColumnTypeDatabaseTypeName(i int) string {
	if i < len(r.a.types) {
		return r.a.types[i]
	}
	return ""
}

func (r *fakeRows) ColumnTypePrecisionScale(i int) (int64, int64, bool) {
	if i < len(r.a.scales) && r.a.scales[i] >= 0 {
		return math.MaxInt64, r.a.scales[i], true
	}
	return 0, 0, false
}
func (r *fakeRows) Close() error { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.i >= len(r.a.rows) {
		return io.EOF
	}
	copy(dest, r.a.rows[r.i])
	r.i++
	return nil
}

// mysqlHome is a HOME with MySQL connections and a mock mysql client for
// the 2.x reference: it prints the TSV of $MOCK_DIR/answer.tsv.
func mysqlHome(t *testing.T) (dir, home string, env map[string]string) {
	t.Helper()
	dir = physTemp(t)
	home = filepath.Join(dir, "home")
	require.NoError(t, os.Mkdir(home, 0o755))
	mustWrite(t, filepath.Join(home, ".gash_env"),
		"DB:default=mysql://u:p%40ss@127.0.0.1:3306/app\nDB:maria=mariadb://u:p@db.example:3307\n", 0o600)
	bin := filepath.Join(dir, "bin")
	mock := "#!/usr/bin/env bash\ncat >\"$MOCK_DIR/stdin\"\ncat \"$MOCK_DIR/answer.tsv\"\n"
	mustWrite(t, filepath.Join(bin, "mysql"), mock, 0o755)
	mustWrite(t, filepath.Join(bin, "mariadb"), mock, 0o755)
	env = baseEnv(home, map[string]string{"PATH": bin + ":" + os.Getenv("PATH"), "MOCK_DIR": dir})
	return dir, home, env
}

// answer sets what both the mock client (as mysql --batch TSV) and the
// fake server return.
func answer(t *testing.T, dir string, f *fakeMySQL, query, tsv string, cols []string, rows ...[]driver.Value) {
	t.Helper()
	mustWrite(t, filepath.Join(dir, "answer.tsv"), tsv, 0o644)
	if f.answers == nil {
		f.answers = map[string]fakeAnswer{}
	}
	f.answers[query] = fakeAnswer{cols: cols, rows: rows}
}

func TestMySQLMatches2x(t *testing.T) {
	d, _, env := mysqlHome(t)
	f := &fakeMySQL{}
	m := frozen(New())
	m.openMySQL = f.open

	answer(t, d, f, "SELECT id, name FROM t -- note", "id\tname\n1\ta\n2\tb\n", []string{"id", "name"},
		[]driver.Value{int64(1), []byte("a")}, []driver.Value{int64(2), []byte("b")})
	same(t, m, d, env, "llm_db_query", "SELECT id, name FROM t -- note", "-r", "7")
	require.Equal(t, []string{"SET SESSION TRANSACTION READ ONLY", "SET SESSION sql_select_limit = 7", "SELECT id, name FROM t -- note"}, f.log())
	require.Equal(t, "u", f.cfg.User)
	require.Equal(t, "p@ss", f.cfg.Passwd)
	require.Equal(t, "tcp", f.cfg.Net)
	require.Equal(t, "127.0.0.1:3306", f.cfg.Addr)
	require.Equal(t, "app", f.cfg.DBName)
	stdin, err := os.ReadFile(filepath.Join(d, "stdin"))
	require.NoError(t, err)
	require.Equal(t, "SET SESSION TRANSACTION READ ONLY;\nSET SESSION sql_select_limit = 7;\nSELECT id, name FROM t -- note\n;\n", string(stdin))

	// NULL is the string "NULL" (LEDGER DB-02); every value is a string.
	answer(t, d, f, "SELECT a, b FROM t", "a\tb\nNULL\t1\n", []string{"a", "b"}, []driver.Value{nil, int64(1)})
	same(t, m, d, env, "llm_db_query", "SELECT a, b FROM t")
	// The trailing semicolon goes; -d replaces the URL database.
	answer(t, d, f, "SELECT 1", "1\n1\n", []string{"1"}, []driver.Value{[]byte("1")})
	same(t, m, d, env, "llm_db_query", "SELECT 1;", "-d", "other_db-2.x")
	require.Equal(t, "other_db-2.x", f.cfg.DBName)
	// No rows: the client printed nothing.
	answer(t, d, f, "SELECT a FROM t WHERE 0", "", []string{"a"})
	same(t, m, d, env, "llm_db_query", "SELECT a FROM t WHERE 0")

	answer(t, d, f, "SELECT table_name AS name FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY table_name",
		"name\nposts\nusers\n", []string{"name"}, []driver.Value{[]byte("posts")}, []driver.Value{[]byte("users")})
	same(t, m, d, env, "llm_db_tables")
	answer(t, d, f, "SELECT column_name AS name, column_type AS type, is_nullable AS nullable, column_key AS `key`, column_default AS `default` FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'users' ORDER BY ordinal_position",
		"name\ttype\tnullable\tkey\tdefault\nid\tint\tNO\tPRI\tNULL\n", []string{"name", "type", "nullable", "key", "default"},
		[]driver.Value{[]byte("id"), []byte("int"), []byte("NO"), []byte("PRI"), nil})
	same(t, m, d, env, "llm_db_schema", "users")
	answer(t, d, f, "SELECT * FROM `users`", "id\n1\n", []string{"id"}, []driver.Value{int64(1)})
	same(t, m, d, env, "llm_db_sample", "users", "-n", "3")
	require.Equal(t, "SET SESSION sql_select_limit = 3", f.log()[len(f.log())-2])

	// EXPLAIN FORMAT=JSON: the raw JSON plan, wrapped in an array.
	plan := "{\n  \"query_block\": {\n    \"select_id\": 1,\n    \"cost_info\": {\"query_cost\": \"0.35\"}\n  }\n}"
	answer(t, d, f, "EXPLAIN FORMAT=JSON SELECT * FROM users", plan+"\n", []string{"EXPLAIN"}, []driver.Value{[]byte(plan)})
	same(t, m, d, env, "llm_db_explain", "SELECT * FROM users")
	// --analyze on MySQL: EXPLAIN ANALYZE prints a text tree.
	answer(t, d, f, "SELECT VERSION()", "8.0.36\n", []string{"VERSION()"}, []driver.Value{[]byte("8.0.36")})
	tree := "-> Table scan on users  (cost=0.35 rows=1) (actual time=0.01..0.02 rows=1 loops=1)\n"
	f.answers["EXPLAIN ANALYZE SELECT * FROM users"] = fakeAnswer{cols: []string{"EXPLAIN"}, rows: [][]driver.Value{{[]byte(tree)}}}
	got := runGo(t, m, d, env, "llm_db_explain", "SELECT * FROM users", "--analyze")
	require.Equal(t, result{`{"data":[{"plan":"-> Table scan on users  (cost=0.35 rows=1) (actual time=0.01..0.02 rows=1 loops=1)"}],"rows":1,"query_time":"0µs"}` + "\n", "", 0}, got)
	// MariaDB: ANALYZE FORMAT=JSON.
	f.answers["SELECT VERSION()"] = fakeAnswer{cols: []string{"VERSION()"}, rows: [][]driver.Value{{[]byte("10.11.6-MariaDB-0+deb12u1")}}}
	f.answers["ANALYZE FORMAT=JSON SELECT * FROM users"] = fakeAnswer{cols: []string{"ANALYZE"}, rows: [][]driver.Value{{[]byte(`[{"r_total_time_ms": 0.5}]`)}}}
	got = runGo(t, m, d, env, "llm_db_explain", "SELECT * FROM users", "--analyze")
	require.Equal(t, result{`{"data":[{"r_total_time_ms":0.5}],"rows":1,"query_time":"0µs"}` + "\n", "", 0}, got)

	// The guard runs with MySQL's rules once the driver is known.
	same(t, m, d, env, "llm_db_query", "SELECT 'a\\b'")
	answer(t, d, f, "SELECT 1 # comment", "1\n1\n", []string{"1"}, []driver.Value{[]byte("1")})
	same(t, m, d, env, "llm_db_query", "SELECT 1 # comment")
	same(t, m, d, env, "llm_db_query", "/*!50000 SELECT 1 */")
	// A query no engine accepts: the sqlite verdict before any connection
	// (LEDGER DB-12).
	same(t, m, d, env, "llm_db_query", "SELECT [a'] ; x")
	same(t, m, d, env, "llm_db_query", "SELECT `a` ; x")
	// No database anywhere: no_database before any connection.
	same(t, m, d, env, "llm_db_query", "SELECT 1", "-c", "maria")
}

// TestLedger_DB02: MySQL values are strings and NULL is the string "NULL".
func TestLedger_DB02(t *testing.T) {
	d, _, env := mysqlHome(t)
	f := &fakeMySQL{}
	m := frozen(New())
	m.openMySQL = f.open
	answer(t, d, f, "SELECT * FROM t", "", []string{"i", "f", "s", "n", "u"},
		[]driver.Value{int64(-5), float64(1.5), []byte("NULL"), nil, uint64(18446744073709551615)})
	got := runGo(t, m, d, env, "llm_db_query", "SELECT * FROM t")
	require.Equal(t, `{"data":[{"i":"-5","f":"1.5","s":"NULL","n":"NULL","u":"18446744073709551615"}],"rows":1,"query_time":"0µs"}`+"\n", got.Stdout)
}

// TestLedger_DB03: no TSV transport: an empty single-column row counts,
// control characters and tabs in values come out as valid JSON.
func TestLedger_DB03(t *testing.T) {
	d, _, env := mysqlHome(t)
	f := &fakeMySQL{}
	m := frozen(New())
	m.openMySQL = f.open
	answer(t, d, f, "SELECT name FROM t", "", []string{"name"},
		[]driver.Value{[]byte("alice")}, []driver.Value{[]byte("")}, []driver.Value{[]byte("a\tb\nc\x01\r\x00d")})
	got := runGo(t, m, d, env, "llm_db_query", "SELECT name FROM t")
	require.Equal(t, `{"data":[{"name":"alice"},{"name":""},{"name":"a\tb\nc\u0001\r\u0000d"}],"rows":3,"query_time":"0µs"}`+"\n", got.Stdout)
	_, err := parseJSON(got.Stdout)
	require.NoError(t, err)
}

// TestLedger_DB07: driver errors keep the *_error code, RETRY and hint,
// with the driver's text cleaned.
func TestLedger_DB07(t *testing.T) {
	d, _, env := mysqlHome(t)
	f := &fakeMySQL{}
	m := frozen(New())
	m.openMySQL = f.open
	got := runGo(t, m, d, env, "llm_db_query", "SELECT nope")
	require.Equal(t, result{"", `{"error":"mysql_error","details":"Error 1064 (42000): You have an error in your SQL syntax","action":"RETRY","recoverable":true,"hint":"Fix the SQL and retry","query_time":"0µs"}` + "\n", 1}, got)
	f.connErr = errors.New("dial tcp 127.0.0.1:3306:\n  connect: connection refused")
	got = runGo(t, m, d, env, "llm_db_query", "SELECT 1")
	require.Equal(t, `{"error":"mysql_error","details":"dial tcp 127.0.0.1:3306: connect: connection refused","action":"RETRY","recoverable":true,"hint":"Fix the SQL and retry","query_time":"0µs"}`+"\n", got.Stderr)
	require.True(t, strings.HasPrefix(got.Stderr, `{"error":"mysql_error"`))
}

// TestMySQLFloatText: the FLOAT and DOUBLE text of MySQL 8.0 and MariaDB
// 11 (testdata, from mysql --batch) comes back unchanged through the
// driver's parsing and mysqlText.
func TestMySQLFloatText(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "mysql_float_text.tsv"))
	require.NoError(t, err)
	n := 0
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		require.Len(t, f, 3, line)
		dec, err := strconv.Atoi(f[1])
		require.NoError(t, err)
		col := mysqlColumn{typeName: f[0], decimals: dec}
		var v any
		if f[0] == "FLOAT" {
			x, err := strconv.ParseFloat(f[2], 32)
			require.NoError(t, err)
			v = float32(x)
		} else {
			x, err := strconv.ParseFloat(f[2], 64)
			require.NoError(t, err)
			v = x
		}
		require.Equal(t, f[2], mysqlText(v, col), line)
		n++
	}
	require.Greater(t, n, 1500)
}

// TestLedger_DB18: the driver's parsed integers lose ZEROFILL padding;
// YEAR, DOUBLE and FLOAT keep the client's text (DB-02).
func TestLedger_DB18(t *testing.T) {
	d, _, env := mysqlHome(t)
	f := &fakeMySQL{}
	m := frozen(New())
	m.openMySQL = f.open
	// The server sent 00042, 0000, 1e20, 1234570, 1.5000 and 12.30.
	f.answers = map[string]fakeAnswer{"SELECT * FROM t": {
		cols:   []string{"zf", "y", "d", "f", "dd", "dec"},
		types:  []string{"INT", "YEAR", "DOUBLE", "FLOAT", "DOUBLE", "DECIMAL"},
		scales: []int64{-1, -1, -1, -1, 4, 2},
		rows:   [][]driver.Value{{int64(42), int64(0), float64(1e20), float32(1234570), float64(1.5), []byte("12.30")}},
	}}
	got := runGo(t, m, d, env, "llm_db_query", "SELECT * FROM t")
	require.Equal(t, `{"data":[{"zf":"42","y":"0000","d":"1e20","f":"1234570","dd":"1.5000","dec":"12.30"}],"rows":1,"query_time":"0µs"}`+"\n", got.Stdout)
}
