// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"context"
	"strconv"
	"time"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// dbTest is gash_db_test [CONNECTION_NAME]: SELECT 1 on a ~/.gash_env
// connection through the native drivers (LEDGER CORE-05: no client to
// find, no option files, the password to the driver only), within
// GASH_DB_TIMEOUT seconds; a failure prints the driver's error as a hint.
func (m *Module) dbTest(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	name := c.Arg(0)
	if name == "" {
		name = "default"
	}
	p := ui.NewPrinter(c.Stderr, c.Term, false)
	f := config.LoadCaller(c.Env.Get)
	url, ok := f.DBURL(name)
	if !ok {
		p.Error("Connection '" + name + "' not found in " + f.Shown)
		return 1
	}
	// LEDGER CORE-11, CORE-13: an unclosed IPv6 bracket or a NUL in the
	// password is refused before any connection.
	if err := config.CheckDBURL(url); err != nil {
		p.Error("Connection '" + name + "' has an invalid URL")
		p.Hint(err.Error())
		return 1
	}
	u := config.ParseDBURL(url)
	switch u.Driver {
	case "mysql", "mariadb", "pgsql":
	default:
		// sqlite and the +ssh and +docker forms run a client somewhere
		// else: a query is the test.
		p.Error("gash_db_test tests mysql, mariadb and pgsql connections from this machine; '" + name + "' is a " + u.Driver + " connection")
		p.Hint("Test it with a query: llm_db_query \"SELECT 1\" -c " + name + " --scalar")
		return 1
	}
	t := target{driver: u.Driver, user: u.User, pass: config.DecodePassword(u.Password), host: u.Host, port: u.Port, database: u.Database}
	p.Info("Testing connection '" + name + "' (" + u.Display() + ")...")
	ctx, cancel := context.WithTimeout(ctxOf(c), dbTimeout(c))
	defer cancel()
	var err error
	switch u.Driver {
	case "mysql", "mariadb":
		err = m.pingMySQL(ctx, c, t)
	case "pgsql":
		err = m.pingPG(ctx, t)
	}
	if err == nil {
		p.Success("Connection successful!")
		return 0
	}
	p.Error("Connection failed")
	if msg := cleanError(err.Error()); msg != "" {
		p.Hint(msg)
	}
	return 1
}

// dbTimeout is GASH_DB_TIMEOUT as a duration: whole seconds, 30 when unset
// or not a number.
func dbTimeout(c *invoke.Call) time.Duration {
	s := c.Env.Get("GASH_DB_TIMEOUT")
	n, _ := strconv.Atoi(s)
	if !digitsRE.MatchString(s) || n == 0 {
		n = 30
	}
	return time.Duration(n) * time.Second
}

func (m *Module) pingMySQL(ctx context.Context, c *invoke.Call, t target) error {
	cfg := m.mysqlConfig(c, t)
	if d, ok := ctx.Deadline(); ok {
		cfg.Timeout = time.Until(d)
	}
	db, err := m.openMySQL(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, "SELECT 1")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
	}
	return rows.Err()
}

func (m *Module) pingPG(ctx context.Context, t target) error {
	cfg, err := pgConfig(t, "", false)
	if err != nil {
		return err
	}
	if d, ok := ctx.Deadline(); ok {
		cfg.ConnectTimeout = time.Until(d)
	}
	conn, err := m.connectPG(ctx, cfg)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	rows, err := conn.Query(ctx, "SELECT 1")
	if err != nil {
		return err
	}
	for rows.Next() {
	}
	rows.Close()
	return rows.Err()
}
