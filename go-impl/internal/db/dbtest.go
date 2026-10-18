// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"context"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// dbTest is gash_db_test [CONNECTION_NAME]: SELECT 1 on a ~/.gash_env
// connection through the native drivers (LEDGER CORE-05: no client to
// find, no option files, the password to the driver only).
func (m *Module) dbTest(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	name := c.Arg(0)
	if name == "" {
		name = "default"
	}
	p := ui.NewPrinter(c.Stderr, c.Term, false)
	url, ok := config.LoadCaller(c.Env.Get).DBURL(name)
	if !ok {
		p.Error("Connection '" + name + "' not found in ~/.gash_env")
		return 1
	}
	u := config.ParseDBURL(url)
	t := target{driver: u.Driver, user: u.User, pass: config.DecodePassword(u.Password), host: u.Host, port: u.Port, database: u.Database}
	p.Info("Testing connection '" + name + "' (" + u.Display() + ")...")
	switch u.Driver {
	case "mysql", "mariadb":
		ok = m.pingMySQL(c, t)
	case "pgsql":
		ok = m.pingPG(c, t)
	default:
		p.Error("Unknown driver: " + u.Driver)
		return 1
	}
	if ok {
		p.Success("Connection successful!")
		return 0
	}
	p.Error("Connection failed")
	return 1
}

func (m *Module) pingMySQL(c *invoke.Call, t target) bool {
	ctx := ctxOf(c)
	db, err := m.openMySQL(mysqlConfig(t))
	if err != nil {
		return false
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, "SELECT 1")
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
	}
	return rows.Err() == nil
}

func (m *Module) pingPG(c *invoke.Call, t target) bool {
	cfg, err := pgConfig(t, "", false)
	if err != nil {
		return false
	}
	ctx := ctxOf(c)
	conn, err := m.connectPG(ctx, cfg)
	if err != nil {
		return false
	}
	defer conn.Close(context.Background())
	rows, err := conn.Query(ctx, "SELECT 1")
	if err != nil {
		return false
	}
	for rows.Next() {
	}
	rows.Close()
	return rows.Err() == nil
}
