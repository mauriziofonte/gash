// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"encoding/json"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
)

// The 3.4 output options of llm_db_* (plan D1, S1): --format jsonl, tsv
// and table print the rows themselves, with the metadata (rows,
// query_time, truncated, slow_query_explain) as one JSON line on stderr
// (plan D8); --scalar prints the one value of a 1x1 result. --format json
// and no option print the envelope, byte for byte as before.

// rowsMode is how a call prints its result.
type rowsMode struct {
	format llmfmt.Format
	scalar bool
	// limit is the -n cap when the query ran with one row more, to tell
	// whether the cap cut rows; -1 when no cap applied.
	limit int
	// column names the values of a result that are not objects
	// (llm_db_tables, llm_db_databases: "name").
	column string
}

// active reports whether the result prints as rows or a scalar.
func (r rowsMode) active() bool {
	return r.scalar || (r.format != llmfmt.Default && r.format != llmfmt.JSON)
}

// wrappable reports whether the engine caps the rows of a statement of
// kind (a wrapping LIMIT, or MySQL's sql_select_limit).
func wrappable(kind string) bool {
	return kind == "SELECT" || kind == "WITH" || kind == "VALUES" || kind == "TABLE"
}

// capped is the row cap to run a statement of kind with and the output
// mode: in the row formats a capped statement fetches one row more than
// -n, so the metadata can say that the cap cut rows.
func capped(a *dbArgs, kind, column string) (string, rowsMode) {
	mode := rowsMode{format: a.format, scalar: a.scalar, limit: -1, column: column}
	if !a.rowsMode() || !wrappable(kind) {
		return a.rows, mode
	}
	n, err := strconv.Atoi(a.rows)
	if err != nil {
		return a.rows, mode
	}
	mode.limit = n
	return strconv.Itoa(n + 1), mode
}

// writeRows prints o in its row format or as a scalar. qt is the query
// time, xpl the plan of a slow query (nil when fast).
func (m *Module) writeRows(c *invoke.Call, o output, qt string, xpl *jv) int {
	cols, rows := rowTable(o)
	more := false
	if o.mode.limit >= 0 && len(rows) > o.mode.limit {
		rows, more = rows[:o.mode.limit], true
	}
	if o.mode.scalar {
		if len(rows) != 1 || len(cols) != 1 {
			n := strconv.Itoa(len(rows))
			if more {
				n += "+"
			}
			shape := n + " rows x " + strconv.Itoa(len(cols)) + " columns"
			return c.Fail(contract.New("not_a_scalar", shape,
				"--scalar needs exactly one row and one column: select one column, add LIMIT 1, or drop --scalar").With("query_time", qt))
		}
		_, _ = io.WriteString(c.Stdout, cellText(rows[0][0])+"\n")
		return 0
	}
	meta := map[string]any{"rows": len(rows), "query_time": qt}
	if more {
		// The count beyond the cap is unknown: one more row exists.
		meta["truncated"] = true
		c.NoteTruncated(1)
	}
	if xpl != nil {
		meta["slow_query_explain"] = json.RawMessage(xpl.appendJSON(nil))
	}
	llmfmt.New(llmfmt.FormatFlag).Write(c, o.mode.format, llmfmt.Table{Cols: cols, Rows: rows}, meta)
	return 0
}

// rowTable turns the values of o into columns and rows: each element of
// an array value is a row (a value that is not an array is one row); the
// columns are the keys of the object rows in first-seen order, plus
// o.mode.column for rows that are not objects. With no row, the columns
// are the ones the engine named.
func rowTable(o output) ([]string, [][]any) {
	var items []jv
	for _, v := range o.vals {
		if v.kind == 'a' {
			items = append(items, v.arr...)
			continue
		}
		items = append(items, v)
	}
	var cols []string
	at := map[string]int{}
	add := func(k string) {
		if _, ok := at[k]; !ok {
			at[k] = len(cols)
			cols = append(cols, k)
		}
	}
	for _, it := range items {
		if it.kind != 'o' {
			add(o.mode.column)
			continue
		}
		for _, k := range it.keys {
			add(k)
		}
	}
	if len(items) == 0 {
		for _, k := range o.cols {
			add(k)
		}
	}
	rows := make([][]any, len(items))
	for i, it := range items {
		row := make([]any, len(cols))
		if it.kind == 'o' {
			for j, k := range it.keys {
				row[at[k]] = anyOf(it.vals[j])
			}
		} else {
			row[at[o.mode.column]] = anyOf(it)
		}
		rows[i] = row
	}
	return cols, rows
}

// anyOf is v as an llmfmt.Table value: numbers in their row form (lit),
// arrays and objects as compact JSON.
func anyOf(v jv) any {
	switch v.kind {
	case 'z':
		return nil
	case 't':
		return true
	case 'f':
		return false
	case 'n':
		if v.lit != "" {
			return json.Number(v.lit)
		}
		return json.Number(v.num)
	case 's':
		return v.str
	}
	return json.RawMessage(v.appendJSON(nil))
}

// cellText is a value as a TSV cell: \N for NULL, escaped text otherwise.
func cellText(v any) string {
	switch x := v.(type) {
	case nil:
		return `\N`
	case string:
		return llmfmt.TSVEscape(x)
	case bool:
		return strconv.FormatBool(x)
	case json.Number:
		return x.String()
	case json.RawMessage:
		return llmfmt.TSVEscape(string(x))
	}
	return ""
}

// jvReal is a SQLite REAL: the envelope keeps the sqlite3 shell's text
// (sqliteReal), the row formats its shortest form as Python's repr
// writes it (8.6, 1e+300, 0.0), which sqlite3 -tabs and a script reading
// the envelope print too; an infinity keeps the shell's 9.0e+999.
func jvReal(x float64) jv {
	v := jvNumber(sqliteReal(x))
	switch {
	case math.IsInf(x, 0):
	case x == 0:
		v.lit = "0.0"
	default:
		v.lit = pyFloat(x)
	}
	return v
}

// pyFloat is Python's repr of a finite, non-zero float: the shortest
// digits that read back, in fixed notation from 1e-4 up to 1e16 (".0"
// for a whole number), in exponent notation (two exponent digits at
// least) outside.
func pyFloat(f float64) string {
	sign := ""
	if f < 0 {
		sign, f = "-", -f
	}
	mant, exp, _ := strings.Cut(strconv.FormatFloat(f, 'e', -1, 64), "e")
	digits := strings.Replace(mant, ".", "", 1)
	x, _ := strconv.Atoi(exp)
	decpt := x + 1
	n := len(digits)
	if decpt > -4 && decpt <= 16 {
		switch {
		case decpt <= 0:
			return sign + "0." + strings.Repeat("0", -decpt) + digits
		case decpt >= n:
			return sign + digits + strings.Repeat("0", decpt-n) + ".0"
		default:
			return sign + digits[:decpt] + "." + digits[decpt:]
		}
	}
	out := sign + digits[:1]
	if n > 1 {
		out += "." + digits[1:]
	}
	e := decpt - 1
	es := "+"
	if e < 0 {
		es, e = "-", -e
	}
	if e < 10 {
		es += "0"
	}
	return out + "e" + es + strconv.Itoa(e)
}

// The queries of llm_db_tables --counts, llm_db_schema --all and
// llm_db_databases (plan D3). Each runs on the read-only session of its
// engine; none takes user text (table names come from the catalog and are
// quoted).

// sqliteTablesQuery lists the SQLite tables, as llm_db_tables.
const sqliteTablesQuery = "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name"

// sqliteCompoundMax keeps one --counts query under SQLite's limit of 500
// terms in a compound SELECT.
const sqliteCompoundMax = 400

// tableCounts is llm_db_tables --counts: [{name, rows}] sorted by name;
// rows is exact on SQLite (count(*)), the engine's estimate on
// MySQL/MariaDB (information_schema.tables.table_rows) and PostgreSQL
// (pg_class.reltuples), null where the engine has none (a view, a table
// PostgreSQL has not analyzed yet).
func (m *Module) tableCounts(c *invoke.Call, t target) output {
	var o output
	switch t.driver {
	case "sqlite":
		names := m.run(c, t, "SELECT", sqliteTablesQuery, "10000")
		if names.fail != nil {
			return names
		}
		var list []string
		for _, v := range names.vals {
			for _, row := range v.arr {
				list = append(list, row.get("name").str)
			}
		}
		var all []jv
		for start := 0; start < len(list); start += sqliteCompoundMax {
			end := min(start+sqliteCompoundMax, len(list))
			parts := make([]string, 0, end-start)
			for _, n := range list[start:end] {
				parts = append(parts, "SELECT '"+strings.ReplaceAll(n, "'", "''")+"' AS name, (SELECT count(*) FROM \""+
					strings.ReplaceAll(n, `"`, `""`)+"\") AS \"rows\"")
			}
			part := m.run(c, t, "SELECT", strings.Join(parts, " UNION ALL "), "10000")
			if part.fail != nil {
				return part
			}
			for _, v := range part.vals {
				all = append(all, v.arr...)
			}
		}
		o = values(jvArray(all))
	case "mysql", "mariadb":
		o = m.run(c, t, "SELECT", "SELECT table_name AS name, table_rows AS `rows` FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY table_name", "10000")
	default:
		o = m.run(c, t, "SELECT", `SELECT t.tablename AS name, CASE WHEN c.reltuples < 0 THEN NULL ELSE c.reltuples::bigint END AS "rows" FROM pg_tables t `+
			`JOIN pg_namespace n ON n.nspname = t.schemaname JOIN pg_class c ON c.relnamespace = n.oid AND c.relname = t.tablename `+
			`WHERE t.schemaname = 'public' ORDER BY t.tablename`, "10000")
	}
	if o.fail != nil || o.isText {
		return o
	}
	var out []jv
	for _, v := range o.vals {
		for _, row := range v.arr {
			var r object
			r.set("name", pickName(row))
			r.set("rows", countValue(row))
			out = append(out, r.value())
		}
	}
	res := values(jvArray(out))
	res.cols = []string{"name", "rows"}
	return res
}

// countValue is the rows of a --counts row as a number, or null: MySQL
// gives the text of the number, or NULL (the string "NULL" in the
// envelope).
func countValue(row jv) jv {
	v := row.get("rows")
	if v.kind == 'z' {
		v = row.get("ROWS")
	}
	switch v.kind {
	case 'n':
		return v
	case 's':
		if v.str != "" && strings.Trim(v.str, "0123456789") == "" {
			return jvNumber(v.str)
		}
	}
	return jvNull
}

// schemaAll is llm_db_schema --all: the llm_db_schema rows of every table
// llm_db_tables lists, in one call, each with the table name first,
// sorted by table and column position.
func (m *Module) schemaAll(c *invoke.Call, t target) output {
	var q string
	switch t.driver {
	case "sqlite":
		q = `SELECT m.name AS "table", p.cid, p.name, p.type, p."notnull", p.dflt_value, p.pk FROM sqlite_master AS m ` +
			`JOIN pragma_table_info(m.name) AS p WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite_%' ORDER BY m.name, p.cid`
	case "mysql", "mariadb":
		q = "SELECT table_name AS `table`, column_name AS name, column_type AS type, is_nullable AS nullable, column_key AS `key`, column_default AS `default` " +
			"FROM information_schema.columns WHERE table_schema = DATABASE() ORDER BY table_name, ordinal_position"
	default:
		q = `SELECT table_name AS "table", column_name, data_type, is_nullable, column_default FROM information_schema.columns ` +
			`WHERE table_schema = 'public' AND table_name IN (SELECT tablename FROM pg_tables WHERE schemaname = 'public') ` +
			`ORDER BY table_name, ordinal_position`
	}
	return m.run(c, t, "SELECT", q, "1000000")
}

// databases is llm_db_databases.
func (m *Module) databases(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	return m.envelope(c, nil, func() output { return m.databasesImpl(c) })
}

// databasesImpl lists the databases of the connection, sorted: every
// schema MySQL/MariaDB shows (the system ones included), every
// PostgreSQL database that is not a template, and "main" for an SQLite
// file. A connection without a database is fine here (PostgreSQL then
// connects to "postgres").
func (m *Module) databasesImpl(c *invoke.Call) output {
	a := dbArgs{connection: "default", rows: "0", anyDatabase: true}
	if e := parseArgs(c.Args, rowFlags, &a); e != nil {
		return failed(*e)
	}
	if a.positional != "" {
		return failed(contract.New("unexpected_argument", a.positional, "llm_db_databases takes no positional argument: pick the server with -c or -f"))
	}
	t, fail := m.target(c, &a)
	if fail != nil {
		return *fail
	}
	t.rowFormat = a.rowsMode()
	var q string
	switch t.driver {
	case "sqlite":
		q = "SELECT name FROM pragma_database_list ORDER BY seq"
	case "mysql", "mariadb":
		q = "SELECT schema_name AS name FROM information_schema.schemata ORDER BY schema_name"
	default:
		if t.database == "" {
			t.database = "postgres"
		}
		q = "SELECT datname AS name FROM pg_database WHERE NOT datistemplate ORDER BY datname"
	}
	o := m.run(c, t, "SELECT", q, "10000")
	if o.fail != nil || o.isText {
		return o
	}
	var names []jv
	for _, v := range o.vals {
		for _, row := range v.arr {
			n := pickName(row)
			if !n.truthy() {
				n = row.get("SCHEMA_NAME")
			}
			names = append(names, n)
		}
	}
	res := values(jvArray(names))
	res.mode = rowsMode{format: a.format, scalar: a.scalar, limit: -1, column: "name"}
	return res
}
