// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
)

// run is __llm_db_run KIND SQL CAP: the guarded query on the target, its
// rows capped at CAP by the engine (a wrapping query or sql_select_limit).
func (m *Module) run(c *invoke.Call, t target, kind, query, rowCap string) output {
	wrap := wrappable(kind)
	switch t.driver {
	case "sqlite":
		mode := sqliteOther
		switch {
		case wrap:
			query = "SELECT * FROM (\n" + query + "\n) LIMIT " + rowCap
			mode = sqliteSelect
		case kind == "EXPLAIN":
			mode = sqliteExplain
		}
		return m.sqliteRun(c, t.file, query, mode)
	case "mysql", "mariadb":
		return m.mysqlRun(c, t, false, query, rowCap)
	}
	if !wrap {
		return m.pgsqlRun(c, t, false, query)
	}
	// PostgreSQL serializes the rows itself: native types, and tabs or
	// newlines inside values survive. The envelope keeps jsonb's key
	// order (2.x); the row formats keep the query's column order.
	agg, obj, typ := "jsonb_agg", "to_jsonb", "jsonb"
	if t.rowFormat {
		agg, obj, typ = "json_agg", "to_json", "json"
	}
	o := m.pgsqlRun(c, t, true, "SELECT COALESCE("+agg+"("+obj+"(__gash_row)), '[]'::"+typ+") FROM (SELECT * FROM (\n"+
		query+"\n) __gash_q LIMIT "+rowCap+") __gash_row")
	if o.fail != nil {
		return o
	}
	if vals, err := parseJSON(o.text); err == nil {
		return values(vals...)
	}
	return o
}

// quietLogger drops the driver's own log lines (2.x printed only the
// error envelope).
type quietLogger struct{}

func (quietLogger) Print(...any) {}

// mysqlConfig is the connection of t: TCP to HOST:PORT, or for host
// localhost the Unix socket the mysql client used (LEDGER DB-16); the
// database by name, the password to the driver only (never argv, a file
// or a child).
func (m *Module) mysqlConfig(c *invoke.Call, t target) *mysql.Config {
	cfg := mysql.NewConfig()
	cfg.User = t.user
	cfg.Passwd = t.pass
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(t.host, t.port)
	if t.host == "localhost" {
		cfg.Net = "unix"
		cfg.Addr = m.mysqlSocket(c.Env.Get("HOME"))
	}
	cfg.DBName = t.database
	cfg.Logger = quietLogger{}
	return cfg
}

func mysqlError(err error) output {
	return failed(contract.New("mysql_error", cleanError(err.Error()), "Fix the SQL and retry"))
}

// mysqlRun is __llm_mysql_run: a read-only session (and sql_select_limit
// when rowCap is set), then the query. raw is the --raw --skip-column-names
// text (tab-separated columns, one row per line); otherwise the rows are
// objects of strings, each the client's text, NULL as "NULL" (LEDGER
// DB-02).
func (m *Module) mysqlRun(c *invoke.Call, t target, raw bool, query, rowCap string) output {
	if t.via != nil {
		return m.sshMySQL(c, t, raw, query, rowCap)
	}
	ctx := ctxOf(c)
	db, err := m.openMySQL(m.mysqlConfig(c, t))
	if err != nil {
		return mysqlError(err)
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return mysqlError(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "SET SESSION TRANSACTION READ ONLY"); err != nil {
		return mysqlError(err)
	}
	if rowCap != "" {
		if _, err := conn.ExecContext(ctx, "SET SESSION sql_select_limit = "+rowCap); err != nil {
			return mysqlError(err)
		}
	}
	defer m.startQuery(ctx)()
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return mysqlError(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return mysqlError(err)
	}
	cts, err := rows.ColumnTypes()
	if err != nil {
		return mysqlError(err)
	}
	meta := mysqlColumns(cts)
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	var out []jv
	var lines []string
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return mysqlError(err)
		}
		if raw {
			fields := make([]string, len(cols))
			for i := range cols {
				fields[i] = mysqlText(vals[i], meta[i])
			}
			lines = append(lines, strings.Join(fields, "\t"))
			continue
		}
		var o object
		for i, col := range cols {
			if vals[i] == nil && t.rowFormat {
				o.set(col, jvNull)
				continue
			}
			o.set(col, jvString(mysqlText(vals[i], meta[i])))
		}
		out = append(out, o.value())
	}
	if err := rows.Err(); err != nil {
		return mysqlError(err)
	}
	if raw {
		return textOut(trimNewlines(strings.Join(lines, "\n")))
	}
	res := values(jvArray(out))
	res.cols = cols
	return res
}

var digitsRE = lazyre.MustCompile(`^[0-9]+$`)

// pgQuote quotes a libpq keyword/value connection string value.
func pgQuote(v string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v) + "'"
}

// pgConfig is the connection of t as psql got it from 2.x: -h, -p, -U,
// the password and database through their own fields (never argv), the
// read-only session, standard strings and the statement timeout in the
// startup parameters (2.x: PGOPTIONS), a 10 s connect timeout.
func pgConfig(t target, timeout string, readOnly bool) (*pgx.ConnConfig, error) {
	var parts []string
	add := func(k, v string) { parts = append(parts, k+"="+pgQuote(v)) }
	if t.host != "" {
		add("host", t.host)
	}
	if t.port != "" {
		add("port", t.port)
	}
	if t.user != "" {
		add("user", t.user)
	}
	add("password", t.pass)
	if t.database != "" {
		add("dbname", t.database)
	}
	cfg, err := pgx.ParseConfig(strings.Join(parts, " "))
	if err != nil {
		return nil, err
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	if readOnly {
		delete(cfg.RuntimeParams, "options")
		cfg.RuntimeParams["default_transaction_read_only"] = "on"
		cfg.RuntimeParams["standard_conforming_strings"] = "on"
		cfg.RuntimeParams["statement_timeout"] = timeout + "s"
		cfg.ConnectTimeout = 10 * time.Second
	}
	return cfg, nil
}

func pgsqlError(err error) output {
	return failed(contract.New("pgsql_error", cleanError(err.Error()), "Fix the SQL and retry"))
}

// pgsqlRun is __llm_pgsql_run: json is the -At output of a query that
// returns one JSON value (the text, "[]" when empty); otherwise the rows
// are objects of strings in the server's text format, NULL as "".
func (m *Module) pgsqlRun(c *invoke.Call, t target, json bool, query string) output {
	if t.via != nil {
		return m.sshPG(c, t, json, query)
	}
	timeout := c.Env.Get("GASH_DB_TIMEOUT")
	if !digitsRE.MatchString(timeout) {
		timeout = "30"
	}
	cfg, err := pgConfig(t, timeout, true)
	if err != nil {
		return pgsqlError(err)
	}
	ctx := ctxOf(c)
	conn, err := m.connectPG(ctx, cfg)
	if err != nil {
		return pgsqlError(err)
	}
	defer conn.Close(context.Background())
	defer m.startQuery(ctx)()
	rows, err := conn.Query(ctx, query)
	if err != nil {
		return pgsqlError(err)
	}
	fields := rows.FieldDescriptions()
	var recs [][]*string
	for rows.Next() {
		raw := rows.RawValues()
		rec := make([]*string, len(raw))
		for i, b := range raw {
			if b != nil {
				s := string(b)
				rec[i] = &s
			}
		}
		recs = append(recs, rec)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return pgsqlError(err)
	}
	if json {
		lines := make([]string, len(recs))
		for i, rec := range recs {
			f := make([]string, len(rec))
			for j, v := range rec {
				if v != nil {
					f[j] = *v
				}
			}
			lines[i] = strings.Join(f, "|")
		}
		out := trimNewlines(strings.Join(lines, "\n"))
		if out == "" {
			out = "[]"
		}
		return textOut(out)
	}
	var out []jv
	for _, rec := range recs {
		var o object
		for i, f := range fields {
			v := ""
			if i < len(rec) && rec[i] != nil {
				v = *rec[i]
			} else if t.rowFormat {
				o.set(f.Name, jvNull)
				continue
			}
			o.set(f.Name, jvString(v))
		}
		out = append(out, o.value())
	}
	res := values(jvArray(out))
	for _, f := range fields {
		res.cols = append(res.cols, f.Name)
	}
	return res
}
