// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sqlguard

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// fuzzSeeds are queries near the guard's edges: the LEDGER vectors' shapes
// (E-strings, comments, dot commands, second statements) and plain reads.
var fuzzSeeds = []string{
	"SELECT 1",
	"SELECT * FROM t WHERE a = 'x'",
	"select a from t; -- done",
	"WITH r AS (SELECT 1) SELECT * FROM r",
	"VALUES (1), (2)",
	"PRAGMA table_info(t)",
	"PRAGMA user_version",
	"EXPLAIN SELECT 1",
	"SELECT 'a'';DROP TABLE t;--'",
	"SELECT e'\\'' ; DELETE FROM t",
	"SELECT 1 /* ; */ ; SELECT 2",
	"SELECT \"a\"\"b\" FROM t",
	"SELECT `a` FROM t",
	".tables",
	"\\! id",
	"SELECT 1;\nINSERT INTO t VALUES (2)",
	"SELECT load_extension('x')",
	"SELECT * FROM t -- \n; UPDATE t SET a=1",
	"SELECT $$a$$",
	"SELECT 1 　; DROP TABLE t",
	"ſELECT 1",
	"REPLACE INTO t VALUES (1)",
	"SELECT 1 RETURNING *",
	"PRAGMA user_version = 5",
	"PRAGMA journal_mode(WAL)",
	"PRAGMA main.table_info(t)",
}

// The guard never panics, on any engine or reading, and what it accepts is
// a prefix of the input that starts with one of the allowed keywords.
func FuzzCheck(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, query string) {
		_ = Precheck(query)
		for _, d := range []string{MySQL, PgSQL, SQLite, "other"} {
			res, err := Check(d, query)
			if err != nil {
				var e *Error
				if !errors.As(err, &e) || e.Code == "" {
					t.Fatalf("Check(%s, %q): error %v is not a coded *Error", d, query, err)
				}
				continue
			}
			if !strings.HasPrefix(query, res.Query) {
				t.Fatalf("Check(%s, %q): stored query %q is not a prefix of the input", d, query, res.Query)
			}
			if !slices.Contains(allowedFirstWords, res.Kind) {
				t.Fatalf("Check(%s, %q): kind %q is not an allowed first word", d, query, res.Kind)
			}
		}
	})
}

// What the SQLite rules accept changes nothing when it runs on a writable
// database: not the schema, not the rows, not the settings, and no file
// appears next to it. The llm_db_* connection is read-only too
// (internal/db/sqlite.go); this checks the guard on its own.
func FuzzCheckSQLiteReadOnly(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}
	tmpl := filepath.Join(f.TempDir(), "tmpl.db")
	db, err := sql.Open("sqlite", tmpl)
	if err != nil {
		f.Fatal(err)
	}
	for _, q := range []string{
		"CREATE TABLE t (a INTEGER PRIMARY KEY, b TEXT)",
		"INSERT INTO t (b) VALUES ('x'), ('y')",
		"CREATE VIEW v AS SELECT b FROM t",
		"PRAGMA user_version = 7",
	} {
		if _, err := db.Exec(q); err != nil {
			f.Fatal(err)
		}
	}
	db.Close()
	data, err := os.ReadFile(tmpl)
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, query string) {
		res, err := Check(SQLite, query)
		if err != nil {
			return
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "t.db")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		db.SetMaxOpenConns(1)
		before := sqliteState(t, db, dir)
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		if rows, err := db.QueryContext(ctx, res.Query); err == nil {
			for rows.Next() {
			}
			rows.Close()
		}
		if after := sqliteState(t, db, dir); after != before {
			t.Fatalf("accepted %q changed the database:\n before %s\n after  %s", query, before, after)
		}
	})
}

// sqliteState is the schema, rows, settings and files of the fuzz database.
func sqliteState(t *testing.T, db *sql.DB, dir string) string {
	t.Helper()
	var b strings.Builder
	for _, q := range []string{
		"SELECT group_concat(type || ':' || name || ':' || ifnull(sql, ''), '|') FROM (SELECT * FROM sqlite_master ORDER BY name)",
		"SELECT group_concat(a || '=' || ifnull(b, 'NULL'), ',') FROM (SELECT * FROM t ORDER BY a)",
		"PRAGMA user_version",
		"PRAGMA application_id",
		"PRAGMA journal_mode",
		"SELECT group_concat(name, ',') FROM pragma_database_list",
	} {
		var s sql.NullString
		if err := db.QueryRow(q).Scan(&s); err != nil {
			b.WriteString("ERR(" + err.Error() + ")")
		} else {
			b.WriteString(s.String)
		}
		b.WriteString(" ; ")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "t.db" && !strings.HasPrefix(e.Name(), "t.db-journal") {
			b.WriteString("file:" + e.Name() + " ")
		}
	}
	return b.String()
}
