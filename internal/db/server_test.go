// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

// The db module against real servers (design/testing.md 2.4): MySQL 8.4,
// MariaDB 11 and PostgreSQL 16, through the native drivers, as an agent
// calls them (a ~/.gash_env DB: connection, the llm_db_* functions and
// gash_db_test). The servers come from the environment, as admin URLs in
// the ~/.gash_env grammar (password percent-encoded):
//
//	GASH_TEST_MYSQL_URL    mysql://root:PASS@127.0.0.1:3306
//	GASH_TEST_MARIADB_URL  mariadb://root:PASS@127.0.0.1:3307
//	GASH_TEST_PGSQL_URL    pgsql://postgres:PASS@127.0.0.1:5432
//
// Without one, its engine is skipped, so `go test ./...` needs no server;
// GASH_TEST_DB_REQUIRE=1 (CI's db-servers job) makes a missing one fail.
// The test creates the database gash_it on each server, as the admin
// user, and reads it only through the module.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/config"
)

type server struct {
	name   string // the subtest
	env    string // the variable with the admin URL
	driver string // mysql, mariadb or pgsql
}

var servers = []server{
	{"MySQL84", "GASH_TEST_MYSQL_URL", "mysql"},
	{"MariaDB11", "GASH_TEST_MARIADB_URL", "mariadb"},
	{"Postgres16", "GASH_TEST_PGSQL_URL", "pgsql"},
}

func TestServers(t *testing.T) {
	for _, s := range servers {
		t.Run(s.name, func(t *testing.T) {
			admin := os.Getenv(s.env)
			if admin == "" {
				if os.Getenv("GASH_TEST_DB_REQUIRE") == "1" {
					t.Fatalf("GASH_TEST_DB_REQUIRE=1 but %s is not set", s.env)
				}
				t.Skipf("%s is not set (a %s server for the integration tests)", s.env, s.driver)
			}
			require.True(t, strings.HasPrefix(admin, s.driver+"://"), "%s must be a %s:// URL", s.env, s.driver)
			testServer(t, s, strings.TrimSuffix(admin, "/"))
		})
	}
}

// dbFixture is the same data on every engine; meta (JSON) only on
// PostgreSQL, where to_jsonb keeps it a JSON object.
func dbFixture(driver string) []string {
	if driver == "pgsql" {
		return []string{
			"CREATE TABLE users (id int PRIMARY KEY, name text NOT NULL, amount numeric(8,2), born date, meta jsonb, note text)",
			`INSERT INTO users VALUES (1, 'alice', 1.50, '1990-01-02', '{"a": 1}', NULL), (2, 'bob', 20.00, '1985-05-06', NULL, 'x"y')`,
			"CREATE TABLE empty_t (id int)",
			"CREATE SEQUENCE seq_it",
		}
	}
	return []string{
		"CREATE TABLE users (id INT PRIMARY KEY, name VARCHAR(40) NOT NULL, amount DECIMAL(8,2), born DATE, note TEXT NULL)",
		`INSERT INTO users VALUES (1, 'alice', 1.50, '1990-01-02', NULL), (2, 'bob', 20.00, '1985-05-06', 'x"y')`,
		"CREATE TABLE empty_t (id INT)",
	}
}

// adminExec runs stmts as the admin of the URL in database db ("" for
// the server's default), waiting up to 90 s for a starting server.
func adminExec(t *testing.T, u config.DBURL, db string, stmts ...string) {
	t.Helper()
	pass := config.DecodePassword(u.Password)
	deadline := time.Now().Add(90 * time.Second)
	var err error
	for {
		err = tryAdminExec(u, pass, db, stmts)
		if err == nil || time.Now().After(deadline) || !isConnectError(err) {
			break
		}
		time.Sleep(time.Second)
	}
	require.NoError(t, err, "as %s on %s:%s", u.User, u.Host, u.Port)
}

type connectError struct{ error }

func isConnectError(err error) bool {
	_, ok := err.(connectError)
	return ok
}

func tryAdminExec(u config.DBURL, pass, db string, stmts []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if u.Driver == "pgsql" {
		if db == "" {
			db = "postgres"
		}
		cfg, err := pgx.ParseConfig("")
		if err != nil {
			return err
		}
		cfg.Host, cfg.User, cfg.Password, cfg.Database = u.Host, u.User, pass, db
		if _, err := fmt.Sscan(u.Port, &cfg.Port); err != nil {
			return err
		}
		conn, err := pgx.ConnectConfig(ctx, cfg)
		if err != nil {
			return connectError{err}
		}
		defer conn.Close(context.Background())
		for _, q := range stmts {
			if _, err := conn.Exec(ctx, q); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
		return nil
	}
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = u.User, pass, "tcp", u.Host+":"+u.Port, db
	conn, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.PingContext(ctx); err != nil {
		return connectError{err}
	}
	for _, q := range stmts {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("%s: %w", q, err)
		}
	}
	return nil
}

func createFixture(t *testing.T, u config.DBURL) {
	t.Helper()
	adminExec(t, u, "", "DROP DATABASE IF EXISTS gash_it", "CREATE DATABASE gash_it")
	adminExec(t, u, "gash_it", dbFixture(u.Driver)...)
}

// envelopeOf decodes a success envelope: data, rows, query_time, and
// whether slow_query_explain is there.
type dbEnvelope struct {
	Data      []json.RawMessage `json:"data"`
	Rows      int               `json:"rows"`
	QueryTime string            `json:"query_time"`
	Slow      *json.RawMessage  `json:"slow_query_explain"`
}

func testServer(t *testing.T, s server, adminURL string) {
	u := config.ParseDBURL(adminURL)
	createFixture(t, u)

	dir := physTemp(t)
	home := filepath.Join(dir, "home")
	require.NoError(t, os.Mkdir(home, 0o755))
	hostport := u.Host + ":" + u.Port
	// The 2.x grammar: the URL splits at its last @, user and password at
	// the first :, and only the password is percent-decoded, so "raw" (a
	// literal @ and : in the password) and "default" reach the same user.
	raw := config.DecodePassword(u.Password)
	mustWrite(t, filepath.Join(home, ".gash_env"), fmt.Sprintf(
		"DB:default=%s://%s:%s@%s/gash_it\nDB:raw=%s://%s:%s@%s/gash_it\nDB:nodb=%s://%s:%s@%s\nDB:badpass=%s://%s:wrong@%s/gash_it\n",
		s.driver, u.User, u.Password, hostport,
		s.driver, u.User, raw, hostport,
		s.driver, u.User, u.Password, hostport,
		s.driver, u.User, hostport), 0o600)
	env := baseEnv(home, nil)
	m := New()

	sch := loadDBSchemas(t)
	ok := func(t *testing.T, fn string, args ...string) dbEnvelope {
		t.Helper()
		r := runGo(t, m, dir, env, fn, args...)
		require.Equal(t, 0, r.RC, "%s %q: %s", fn, args, r.Stderr)
		require.Empty(t, r.Stderr, "%s %q", fn, args)
		validateAgainst(t, sch["db"], r.Stdout)
		var e dbEnvelope
		require.NoError(t, json.Unmarshal([]byte(r.Stdout), &e), r.Stdout)
		require.Equal(t, len(e.Data), e.Rows)
		return e
	}
	fail := func(t *testing.T, code string, fn string, args ...string) map[string]any {
		t.Helper()
		r := runGo(t, m, dir, env, fn, args...)
		require.Equal(t, 1, r.RC, "%s %q: %s%s", fn, args, r.Stdout, r.Stderr)
		require.Empty(t, r.Stdout)
		validateAgainst(t, sch["error"], r.Stderr)
		var e map[string]any
		require.NoError(t, json.Unmarshal([]byte(r.Stderr), &e))
		require.Equal(t, code, e["error"], r.Stderr)
		require.Contains(t, e, "query_time")
		return e
	}
	isMySQL := s.driver != "pgsql"

	t.Run("tables", func(t *testing.T) {
		e := ok(t, "llm_db_tables")
		require.Equal(t, []json.RawMessage{json.RawMessage(`"empty_t"`), json.RawMessage(`"users"`)}, e.Data)
		require.Equal(t, ok(t, "llm_db_tables", "-c", "raw").Data, e.Data, "a raw @ and : in the password")
	})

	t.Run("schema keys per engine", func(t *testing.T) {
		e := ok(t, "llm_db_schema", "users")
		require.Len(t, e.Data, 6-boolInt(isMySQL))
		var first map[string]any
		require.NoError(t, json.Unmarshal(e.Data[0], &first))
		keys := make([]string, 0, len(first))
		for k := range first {
			keys = append(keys, k)
		}
		want := []string{"column_default", "column_name", "data_type", "is_nullable"}
		if isMySQL {
			want = []string{"default", "key", "name", "nullable", "type"}
		}
		require.ElementsMatch(t, want, keys)
		require.Empty(t, ok(t, "llm_db_schema", "nope").Data)
	})

	t.Run("values per engine", func(t *testing.T) {
		cols := "id, name, amount, born, note"
		if !isMySQL {
			cols = "id, name, amount, born, meta, note"
		}
		r := runGo(t, m, dir, env, "llm_db_query", "SELECT "+cols+" FROM users ORDER BY id")
		require.Equal(t, 0, r.RC, r.Stderr)
		data := r.Stdout[:strings.Index(r.Stdout, `,"rows":`)]
		if isMySQL {
			// Every value the client's text, NULL the string "NULL".
			require.Equal(t, `{"data":[{"id":"1","name":"alice","amount":"1.50","born":"1990-01-02","note":"NULL"},`+
				`{"id":"2","name":"bob","amount":"20.00","born":"1985-05-06","note":"x\"y"}]`, data)
		} else {
			// to_jsonb: numbers keep the server's text, JSON stays JSON.
			// jsonb orders the keys (shorter first, then bytewise), as 2.x's
			// jsonb_agg(to_jsonb(...)) did.
			require.Equal(t, `{"data":[{"id":1,"born":"1990-01-02","meta":{"a":1},"name":"alice","note":null,"amount":1.50},`+
				`{"id":2,"born":"1985-05-06","meta":null,"name":"bob","note":"x\"y","amount":20.00}]`, data)
		}
		validateAgainst(t, sch["db"], r.Stdout)
	})

	t.Run("row limits", func(t *testing.T) {
		require.Equal(t, 1, ok(t, "llm_db_query", "SELECT * FROM users ORDER BY id", "-n", "1").Rows)
		require.Equal(t, 1, ok(t, "llm_db_query", "SELECT * FROM users ORDER BY id", "--rows", "1").Rows)
		require.Equal(t, 1, ok(t, "llm_db_sample", "users", "-n", "1").Rows)
		require.Equal(t, 2, ok(t, "llm_db_sample", "users").Rows)
		require.Equal(t, 0, ok(t, "llm_db_sample", "empty_t").Rows)
		require.Equal(t, 0, ok(t, "llm_db_query", "SELECT * FROM users WHERE id > 99").Rows)
	})

	t.Run("explain", func(t *testing.T) {
		e := ok(t, "llm_db_explain", "SELECT * FROM users WHERE id = 1")
		require.NotEmpty(t, e.Data)
		require.NotEmpty(t, ok(t, "llm_db_explain", "SELECT * FROM users WHERE id = 1", "--analyze").Data)
	})

	t.Run("read-only session", func(t *testing.T) {
		if isMySQL {
			v := "@@transaction_read_only"
			e := ok(t, "llm_db_query", "SELECT "+v+" AS ro")
			require.JSONEq(t, `{"ro":"1"}`, string(e.Data[0]))
		} else {
			e := ok(t, "llm_db_query", "SHOW default_transaction_read_only")
			require.JSONEq(t, `{"default_transaction_read_only":"on"}`, string(e.Data[0]))
			// A function that writes passes the SQL guard; the server refuses it.
			en := fail(t, "pgsql_error", "llm_db_query", "SELECT nextval('seq_it')")
			require.Contains(t, en["details"], "read-only transaction")
		}
		// The guard refuses a write before it reaches the server.
		e := fail(t, "write_operation_blocked", "llm_db_query", "DELETE FROM users")
		require.Equal(t, "FATAL", e["action"])
		require.Equal(t, 2, ok(t, "llm_db_query", "SELECT * FROM users").Rows, "nothing was deleted")
	})

	t.Run("slow query explain", func(t *testing.T) {
		// SLEEP and pg_sleep are refused by the SQL guard: a query that
		// takes its time instead.
		q := slowQuery(isMySQL)
		e := ok(t, "llm_db_query", q)
		require.NotNil(t, e.Slow, "a query of 100 ms or more carries slow_query_explain")
		require.False(t, strings.HasSuffix(e.QueryTime, "µs"), e.QueryTime)
		require.Nil(t, ok(t, "llm_db_query", "SELECT 1 AS one").Slow)
	})

	if !isMySQL {
		t.Run("statement timeout", func(t *testing.T) {
			saved := env["GASH_DB_TIMEOUT"]
			env["GASH_DB_TIMEOUT"] = "1"
			defer func() { env["GASH_DB_TIMEOUT"] = saved }()
			e := fail(t, "pgsql_error", "llm_db_query", "SELECT count(*) FROM generate_series(1, 2000000000)")
			require.Contains(t, e["details"], "statement timeout")
		})
	}

	t.Run("database selection", func(t *testing.T) {
		q, other := "SELECT DATABASE() AS d", "information_schema"
		if !isMySQL {
			q, other = "SELECT current_database() AS d", "postgres"
		}
		require.JSONEq(t, `{"d":"gash_it"}`, string(ok(t, "llm_db_query", q).Data[0]))
		require.JSONEq(t, `{"d":"`+other+`"}`, string(ok(t, "llm_db_query", q, "-d", other).Data[0]))
		// A connection without a database takes the first word of
		// .target-database in the current directory.
		mustWrite(t, filepath.Join(dir, ".target-database"), "gash_it extra\n", 0o644)
		defer os.Remove(filepath.Join(dir, ".target-database"))
		require.JSONEq(t, `{"d":"gash_it"}`, string(ok(t, "llm_db_query", q, "-c", "nodb").Data[0]))
		fail(t, "invalid_database_name", "llm_db_query", q, "-d", "a;b")
	})

	t.Run("server errors", func(t *testing.T) {
		code := "pgsql_error"
		if isMySQL {
			code = "mysql_error"
		}
		e := fail(t, code, "llm_db_query", "SELECT * FROM nope")
		require.Equal(t, "RETRY", e["action"])
		require.NotContains(t, e["details"], "\n", "the driver's message on one line")
		e = fail(t, code, "llm_db_tables", "-c", "badpass")
		require.NotContains(t, e["details"], "wrong", "the password never reaches the message")
	})

	t.Run("gash_db_test", func(t *testing.T) {
		r := runGo(t, m, dir, env, "gash_db_test")
		require.Equal(t, 0, r.RC, r.Stderr)
		require.Contains(t, r.Stderr, "Connection successful!")
		require.NotContains(t, r.Stderr+r.Stdout, raw)
		r = runGo(t, m, dir, env, "gash_db_test", "raw")
		require.Equal(t, 0, r.RC, r.Stderr)
		r = runGo(t, m, dir, env, "gash_db_test", "badpass")
		require.Equal(t, 1, r.RC, r.Stderr)
		require.NotContains(t, r.Stderr, "Connection successful!")
		require.NotContains(t, r.Stderr+r.Stdout, "wrong")
	})
}

// slowQuery takes well over 100 ms on any server (about 0.6 s on MySQL
// 8.4: a cross join of seven ten-row tables, ten million rows).
func slowQuery(isMySQL bool) string {
	if isMySQL {
		digits := "(SELECT 0 AS x" + strings.Repeat(" UNION ALL SELECT 1", 9) + ")"
		from := make([]string, 7)
		for i := range from {
			from[i] = digits + " " + string(rune('a'+i))
		}
		return "SELECT COUNT(*) AS n FROM " + strings.Join(from, ", ")
	}
	return "SELECT count(*) AS n FROM generate_series(1, 5000000)"
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// loadDBSchemas compiles testdata/schema/{db,error}.json of the checkout
// (the envelope schemas of design/architecture.md 4).
func loadDBSchemas(t *testing.T) map[string]*jsonschema.Schema {
	t.Helper()
	out := map[string]*jsonschema.Schema{}
	for _, name := range []string{"db", "error"} {
		s, err := jsonschema.NewCompiler().Compile(filepath.Join("..", "..", "testdata", "schema", name+".json"))
		require.NoError(t, err)
		out[name] = s
	}
	return out
}

func validateAgainst(t *testing.T, s *jsonschema.Schema, text string) {
	t.Helper()
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(text)))
	require.NoError(t, err, text)
	require.NoError(t, s.Validate(v), text)
}
