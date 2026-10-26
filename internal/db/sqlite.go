// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"modernc.org/sqlite"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
)

// How sqliteRun reads a statement.
const (
	sqliteSelect  = iota // a wrapped query: rows, time-declared columns kept as text
	sqliteOther          // a statement that is not wrapped (PRAGMA): rows
	sqliteExplain        // a user EXPLAIN statement: the shell printed text
	sqliteEQP            // EXPLAIN QUERY PLAN of llm_db_explain: the shell's text tree
)

// sqliteLimitAttached is SQLITE_LIMIT_ATTACHED: set to 0, no ATTACH can
// run on the connection (the -safe equivalent of the sqlite3 shell).
const sqliteLimitAttached = 7

// sqliteDSN opens path read-only (mode=ro, query_only), in defensive mode
// and with trusted_schema off, as `sqlite3 -readonly -safe` did.
func sqliteDSN(path string) string {
	var b strings.Builder
	b.WriteString("file:")
	for i := 0; i < len(path); i++ {
		c := path[i]
		if c == '/' || c == '-' || c == '.' || c == '_' || c == '~' ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	b.WriteString("?mode=ro&_defensive=1&_query_only=1&_pragma=trusted_schema(0)")
	return b.String()
}

// sqliteError is the sqlite_error envelope of a driver error.
func sqliteError(err error) output {
	return failed(contract.New("sqlite_error", cleanError(err.Error()), "Fix the SQL and retry"))
}

// cleanError is __llm_clean_error_msg: newlines and blank runs become one
// space, the ends are trimmed.
func cleanError(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\v' || r == '\f' || r == '\r'
	}), " ")
}

// sqliteRun is __llm_sqlite_run FILE SQL: the rows as the sqlite3 shell's
// -json output read by jq, or the text the shell printed for an EXPLAIN.
func (m *Module) sqliteRun(c *invoke.Call, file, query string, mode int) output {
	if o, ok := m.sqliteOnHost(c, file, query, mode); ok {
		return o
	}
	ctx := ctxOf(c)
	db, err := m.openSQLite(sqliteDSN(file))
	if err != nil {
		return sqliteError(err)
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return sqliteError(err)
	}
	defer conn.Close()
	if _, err := sqlite.Limit(conn, sqliteLimitAttached, 0); err != nil {
		return sqliteError(err)
	}
	defer m.startQuery(ctx)()
	info, err := columnInfo(conn, query)
	if err != nil {
		return sqliteError(err)
	}
	switch mode {
	case sqliteExplain:
		// An EXPLAIN QUERY PLAN is the plan tree, which the envelope
		// wraps as [{plan}]; a bytecode EXPLAIN comes back as its rows
		// (addr, opcode, p1..p5, comment). 2.x piped the sqlite3 shell's
		// text table of either into jq, which refused it (3.0: LEDGER
		// DB-14 fixed).
		if isEQP(info) {
			return m.sqlitePlan(c, conn, query)
		}
	case sqliteEQP:
		return m.sqlitePlan(c, conn, query)
	case sqliteSelect:
		if hasTimeDecl(info) {
			query = textColumns(query, info)
		}
	}
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return sqliteError(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return sqliteError(err)
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	var out []jv
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return sqliteError(err)
		}
		var o object
		for i, col := range cols {
			o.set(col, sqliteValue(vals[i]))
		}
		out = append(out, o.value())
	}
	if err := rows.Err(); err != nil {
		return sqliteError(err)
	}
	return values(jvArray(out))
}

// sqliteValue is a column value as the sqlite3 shell's -json mode printed
// it: integers, REALs in its "%!.20g" text, text and blobs as strings.
func sqliteValue(v any) jv {
	switch x := v.(type) {
	case nil:
		return jvNull
	case int64:
		return jvNumber(strconv.FormatInt(x, 10))
	case float64:
		return jvNumber(sqliteReal(x))
	case string:
		return jvString(x)
	case []byte:
		return jvString(string(x))
	case time.Time:
		return jvString(x.Format(time.RFC3339Nano))
	}
	return jvString(fmt.Sprint(v))
}

func columnInfo(conn *sql.Conn, query string) ([]sqlite.ColumnInfo, error) {
	var info []sqlite.ColumnInfo
	err := conn.Raw(func(dc any) error {
		ci, ok := dc.(interface {
			ColumnInfo(string) ([]sqlite.ColumnInfo, error)
		})
		if !ok {
			return errors.New("sqlite: the driver gives no column information")
		}
		var err error
		info, err = ci.ColumnInfo(query)
		return err
	})
	return info, err
}

// hasTimeDecl tells whether a column is declared DATE, DATETIME or
// TIMESTAMP, which the driver would turn from text into a time.
func hasTimeDecl(info []sqlite.ColumnInfo) bool {
	for _, ci := range info {
		switch strings.ToUpper(ci.DeclType) {
		case "DATE", "DATETIME", "TIMESTAMP":
			return true
		}
	}
	return false
}

// textColumns rewrites a wrapped query so that no result column has a
// declared type (+x is a no-op on every value), keeping the column names
// and order: the stored text reaches the output unchanged.
func textColumns(query string, info []sqlite.ColumnInfo) string {
	var b strings.Builder
	b.WriteString("WITH __gash_t(")
	for i := range info {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "c%d", i)
	}
	b.WriteString(") AS (\n")
	b.WriteString(query)
	b.WriteString("\n) SELECT ")
	for i, ci := range info {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "+c%d AS \"%s\"", i, strings.ReplaceAll(ci.Name, `"`, `""`))
	}
	b.WriteString(" FROM __gash_t")
	return b.String()
}

func isEQP(info []sqlite.ColumnInfo) bool {
	want := []string{"id", "parent", "notused", "detail"}
	if len(info) != len(want) {
		return false
	}
	for i, ci := range info {
		if ci.Name != want[i] {
			return false
		}
	}
	return true
}

type eqpRow struct {
	id, parent int64
	text       string
}

// sqlitePlan runs an EXPLAIN QUERY PLAN and renders it as the sqlite3
// shell did (LEDGER DB-08); no plan rows print nothing, which 2.x read as [].
func (m *Module) sqlitePlan(c *invoke.Call, conn *sql.Conn, query string) output {
	rows, err := conn.QueryContext(ctxOf(c), query)
	if err != nil {
		return sqliteError(err)
	}
	defer rows.Close()
	var plan []eqpRow
	for rows.Next() {
		var r eqpRow
		var notused any
		var detail sql.NullString
		if err := rows.Scan(&r.id, &r.parent, &notused, &detail); err != nil {
			return sqliteError(err)
		}
		r.text = detail.String
		plan = append(plan, r)
	}
	if err := rows.Err(); err != nil {
		return sqliteError(err)
	}
	if len(plan) == 0 {
		return values(jvArray(nil))
	}
	return textOut(renderEQP(plan))
}

// renderEQP is the sqlite3 shell's eqp_render: "QUERY PLAN", then each
// row under its parent with |-- (a sibling follows) or `-- (the last
// one), its children indented by "|  " or "   ", at most 31 levels deep
// (the shell's 100-byte prefix buffer).
func renderEQP(plan []eqpRow) string {
	var b strings.Builder
	b.WriteString("QUERY PLAN\n")
	var level func(parent int64, prefix string)
	level = func(parent int64, prefix string) {
		var kids []eqpRow
		for _, r := range plan {
			if r.parent == parent {
				kids = append(kids, r)
			}
		}
		for i, r := range kids {
			last := i == len(kids)-1
			conn, indent := "|--", "|  "
			if last {
				conn, indent = "`--", "   "
			}
			b.WriteString(prefix + conn + r.text + "\n")
			if len(prefix) < 100-7 {
				level(r.id, prefix+indent)
			}
		}
	}
	level(0, "")
	return b.String()
}
