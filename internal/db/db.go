// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package db holds the llm_db_* functions of the 2.x lib/modules/llm/db.sh
// and gash_db_test of lib/core/config.sh: read-only queries on SQLite,
// MySQL/MariaDB and PostgreSQL through native drivers (modernc sqlite,
// go-sql-driver/mysql, pgx) in place of the sqlite3, mysql and psql
// clients, with the 2.x argument rules, targets, SQL guard
// (internal/sqlguard) and JSON envelope.
package db

import (
	"context"
	"database/sql"
	"errors"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	_ "modernc.org/sqlite" // the "sqlite" database/sql driver

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/sqlguard"
)

// Module is the database handler set with its clock, filesystem and
// driver seams.
type Module struct {
	now        func() time.Time
	getwd      func() (string, error)
	rootHome   func() string
	openSQLite func(dsn string) (*sql.DB, error)
	openMySQL  func(cfg *mysql.Config) (*sql.DB, error)
	connectPG  func(ctx context.Context, cfg *pgx.ConnConfig) (pgConn, error)
	// myCnfFiles are the system option files read for the MySQL socket,
	// socketDefaults the built-in socket paths (LEDGER DB-16).
	myCnfFiles     []string
	socketDefaults []string
}

// pgConn is the part of *pgx.Conn the module uses.
type pgConn interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Close(ctx context.Context) error
}

// New returns the module wired to the real drivers.
func New() *Module {
	return &Module{
		now:      time.Now,
		getwd:    syscall.Getwd,
		rootHome: pathguard.RootHome,
		openSQLite: func(dsn string) (*sql.DB, error) {
			return sql.Open("sqlite", dsn)
		},
		openMySQL: func(cfg *mysql.Config) (*sql.DB, error) {
			conn, err := mysql.NewConnector(cfg)
			if err != nil {
				return nil, err
			}
			return sql.OpenDB(conn), nil
		},
		connectPG: func(ctx context.Context, cfg *pgx.ConnConfig) (pgConn, error) {
			return pgx.ConnectConfig(ctx, cfg)
		},
		myCnfFiles:     mysqlSystemFiles,
		socketDefaults: mysqlSocketDefaults,
	}
}

// Register binds the database functions to t.
func Register(t *invoke.Table) { New().Register(t) }

// Register binds the functions of m to t.
func (m *Module) Register(t *invoke.Table) {
	t.Handle("llm_db_query", m.query)
	t.Handle("llm_db_tables", m.tables)
	t.Handle("llm_db_schema", m.schema)
	t.Handle("llm_db_sample", m.sample)
	t.Handle("llm_db_explain", m.explain)
	t.Handle("gash_db_test", m.dbTest)
}

func ctxOf(c *invoke.Call) context.Context {
	if c.Ctx != nil {
		return c.Ctx
	}
	return context.Background()
}

// helpAnywhere is __llm_needs_help: --help or -h anywhere among the
// arguments prints the help (LEDGER DB-11: -h is never a host option).
func helpAnywhere(c *invoke.Call) bool {
	for _, a := range c.Args {
		if a == "--help" || a == "-h" {
			help.NeedsOf(c, "--help")
			return true
		}
	}
	return false
}

// output is what an impl printed: JSON values, or text that is not JSON.
type output struct {
	vals   []jv
	text   string
	isText bool
	fail   *contract.Envelope
}

func failed(e contract.Envelope) output { return output{fail: &e} }

func values(v ...jv) output { return output{vals: v} }

func textOut(s string) output { return output{text: s, isText: true} }

// envOf is the envelope of a 2.x error with its action: the code's fixed
// action when they agree, the 2.x action otherwise.
func envOf(code, details, action, hint string) contract.Envelope {
	if a, ok := contract.ActionOf(code); ok && string(a) == action {
		return contract.New(code, details, hint)
	}
	return contract.Legacy(code, details, action, hint)
}

// guardFail is the envelope of an internal/sqlguard verdict.
func guardFail(err error) output {
	var g *sqlguard.Error
	if errors.As(err, &g) {
		return failed(envOf(g.Code, g.Details, g.Action, g.Hint))
	}
	return failed(contract.New("internal_error", err.Error(), ""))
}

// refusal is the envelope of a pathguard refusal.
func refusal(err error) output {
	var r *pathguard.Refusal
	if errors.As(err, &r) {
		return failed(envOf(r.Code, r.Details, r.Action, r.Hint))
	}
	return failed(contract.New("internal_error", err.Error(), ""))
}

// query is llm_db_query.
func (m *Module) query(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	xargs := explainArgs(c.Args)
	return m.envelope(c, func() output { return m.explainImpl(c, xargs) }, func() output { return m.queryImpl(c, c.Args) })
}

// explainArgs is __GASH_DB_EXPLAIN_ARGS: the arguments without the row
// cap options and the word after each.
func explainArgs(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-r", "--rows", "-n", "--limit":
			i++
		default:
			out = append(out, args[i])
		}
	}
	return out
}

func (m *Module) queryImpl(c *invoke.Call, args []string) output {
	a := dbArgs{connection: "default", rows: "100"}
	if e := parseArgs(args, "-r --rows -n --limit", &a); e != nil {
		return failed(*e)
	}
	if a.positional == "" {
		return failed(contract.New("missing_query", "", "Provide a SQL query as first argument"))
	}
	if e := validUint("rows", a.rows); e != nil {
		return failed(*e)
	}
	t, g, fail := m.guardedTarget(c, &a)
	if fail != nil {
		return *fail
	}
	return m.run(c, t, g.Kind, g.Query, a.rows)
}

// guardedTarget resolves the connection, then checks the query with the
// rules of its engine, so a refused query gets the target engine's
// verdict (LEDGER DB-12; 2.x gave the sqlite verdict whenever every engine
// refused). When the connection cannot be resolved, a query that every
// engine refuses still gets the guard error, the precheck's, first.
// Resolving reads ~/.gash_env and the SQLite header only: nothing
// connects before the guard accepts.
func (m *Module) guardedTarget(c *invoke.Call, a *dbArgs) (target, sqlguard.Result, *output) {
	t, fail := m.target(c, a)
	if fail != nil {
		if err := sqlguard.Precheck(a.positional); err != nil {
			o := guardFail(err)
			return target{}, sqlguard.Result{}, &o
		}
		return target{}, sqlguard.Result{}, fail
	}
	g, err := sqlguard.Check(sqlguard.GuardDriver(t.driver), a.positional)
	if err != nil {
		o := guardFail(err)
		return target{}, sqlguard.Result{}, &o
	}
	return t, g, nil
}

// tables is llm_db_tables.
func (m *Module) tables(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	return m.envelope(c, nil, func() output { return m.tablesImpl(c) })
}

func (m *Module) tablesImpl(c *invoke.Call) output {
	a := dbArgs{connection: "default", rows: "0"}
	if e := parseArgs(c.Args, "", &a); e != nil {
		return failed(*e)
	}
	t, fail := m.target(c, &a)
	if fail != nil {
		return *fail
	}
	var q string
	switch t.driver {
	case "sqlite":
		q = "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name"
	case "mysql", "mariadb":
		q = "SELECT table_name AS name FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY table_name"
	default:
		q = "SELECT tablename AS name FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename"
	}
	o := m.run(c, t, "SELECT", q, "10000")
	if o.fail != nil || o.isText {
		return o
	}
	// jq -c '[.[] | (.name // .NAME // .TABLE_NAME // .table_name)]'
	var out []jv
	for _, v := range o.vals {
		var names []jv
		for _, row := range v.arr {
			names = append(names, pickName(row))
		}
		out = append(out, jvArray(names))
	}
	return values(out...)
}

func pickName(row jv) jv {
	for _, k := range []string{"name", "NAME", "TABLE_NAME"} {
		if v := row.get(k); v.truthy() {
			return v
		}
	}
	return row.get("table_name")
}

// schema is llm_db_schema.
func (m *Module) schema(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	return m.envelope(c, nil, func() output { return m.schemaImpl(c) })
}

func (m *Module) schemaImpl(c *invoke.Call) output {
	a := dbArgs{connection: "default", rows: "0"}
	if e := parseArgs(c.Args, "", &a); e != nil {
		return failed(*e)
	}
	if e := checkTable(a.positional); e != nil {
		return failed(*e)
	}
	table := a.positional
	t, fail := m.target(c, &a)
	if fail != nil {
		return *fail
	}
	var q string
	switch t.driver {
	case "sqlite":
		q = `SELECT cid, name, type, "notnull", dflt_value, pk FROM pragma_table_info('` + table + `')`
	case "mysql", "mariadb":
		q = "SELECT column_name AS name, column_type AS type, is_nullable AS nullable, column_key AS `key`, column_default AS `default` FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = '" + table + "' ORDER BY ordinal_position"
	default:
		q = "SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_name = '" + table + "' AND table_schema = 'public' ORDER BY ordinal_position"
	}
	return m.run(c, t, "SELECT", q, "10000")
}

// sample is llm_db_sample.
func (m *Module) sample(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	return m.envelope(c, nil, func() output { return m.sampleImpl(c) })
}

func (m *Module) sampleImpl(c *invoke.Call) output {
	a := dbArgs{connection: "default", rows: "5"}
	if e := parseArgs(c.Args, "-n --limit", &a); e != nil {
		return failed(*e)
	}
	if e := checkTable(a.positional); e != nil {
		return failed(*e)
	}
	if e := validUint("limit", a.rows); e != nil {
		return failed(*e)
	}
	t, fail := m.target(c, &a)
	if fail != nil {
		return *fail
	}
	quoted := `"` + a.positional + `"`
	if t.driver == "mysql" || t.driver == "mariadb" {
		quoted = "`" + a.positional + "`"
	}
	return m.run(c, t, "SELECT", "SELECT * FROM "+quoted, a.rows)
}

// explain is llm_db_explain.
func (m *Module) explain(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	return m.envelope(c, nil, func() output { return m.explainImpl(c, c.Args) })
}

func (m *Module) explainImpl(c *invoke.Call, args []string) output {
	a := dbArgs{connection: "default", rows: "0"}
	if e := parseArgs(args, "--analyze", &a); e != nil {
		return failed(*e)
	}
	if a.positional == "" {
		return failed(contract.New("missing_query", "", "Provide a SQL query to analyze as first argument"))
	}
	t, g, fail := m.guardedTarget(c, &a)
	if fail != nil {
		return *fail
	}
	var plan output
	switch t.driver {
	case "sqlite":
		// sqlite3 rendered EXPLAIN QUERY PLAN as a text tree (DB-08).
		plan = m.sqliteRun(c, t.file, "EXPLAIN QUERY PLAN "+g.Query, sqliteEQP)
	case "mysql", "mariadb":
		prefix := "EXPLAIN FORMAT=JSON"
		if a.analyze {
			// MariaDB: ANALYZE FORMAT=JSON; MySQL 8.0.18+: EXPLAIN ANALYZE
			v := m.mysqlRun(c, t, true, "SELECT VERSION()", "")
			if v.fail != nil {
				return v
			}
			if containsMariaDB(v.text) {
				prefix = "ANALYZE FORMAT=JSON"
			} else {
				prefix = "EXPLAIN ANALYZE"
			}
		}
		plan = m.mysqlRun(c, t, true, prefix+" "+g.Query, "")
	default:
		prefix := "EXPLAIN (FORMAT JSON)"
		if a.analyze {
			prefix = "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)"
		}
		plan = m.pgsqlRun(c, t, true, prefix+" "+g.Query)
	}
	if plan.fail != nil {
		return plan
	}
	return planArray(plan)
}

// planArray is the end of __llm_db_explain_impl: JSON plans as arrays
// (jq -c 'if type == "array" then . else [.] end'), any other text as
// [{plan: TEXT}].
func planArray(plan output) output {
	vals, ok := plan.vals, true
	if plan.isText {
		vals, ok = jqTest(plan.text)
	} else if len(vals) == 0 || !vals[len(vals)-1].truthy() {
		ok = false
	}
	if !ok {
		var o object
		o.set("plan", jvString(trimNewlines(plan.text)))
		return values(jvArray([]jv{o.value()}))
	}
	out := make([]jv, len(vals))
	for i, v := range vals {
		if v.kind == 'a' {
			out[i] = v
		} else {
			out[i] = jvArray([]jv{v})
		}
	}
	return values(out...)
}

// jqTest is `jq -e . <<<TEXT` succeeding: TEXT is a stream of JSON
// values whose last one is neither null nor false.
func jqTest(text string) ([]jv, bool) {
	vals, err := parseJSON(text)
	if err != nil || len(vals) == 0 || !vals[len(vals)-1].truthy() {
		return nil, false
	}
	return vals, true
}

func containsMariaDB(s string) bool {
	for i := 0; i+7 <= len(s); i++ {
		if s[i:i+7] == "MariaDB" {
			return true
		}
	}
	return false
}

// trimNewlines is what a command substitution keeps of s.
func trimNewlines(s string) string {
	for len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	return s
}
