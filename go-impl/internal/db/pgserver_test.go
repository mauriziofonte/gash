// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

// A private PostgreSQL cluster for the tests: initdb and postgres from
// the system's PostgreSQL 16, run as nobody (postgres refuses root) in a
// temporary directory, listening on 127.0.0.1 only, trust authentication.
var (
	pgOnce sync.Once
	pgPort int
	pgErr  error
	pgDir  string
	pgProc *exec.Cmd
)

const pgBin = "/usr/lib/postgresql/16/bin"

func TestMain(m *testing.M) {
	code := m.Run()
	if pgProc != nil {
		_ = pgProc.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() { _ = pgProc.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = pgProc.Process.Kill()
		}
	}
	if pgDir != "" {
		_ = os.RemoveAll(pgDir)
	}
	os.Exit(code)
}

// pgServer returns the port of the test cluster, or skips.
func pgServer(t *testing.T) int {
	t.Helper()
	pgOnce.Do(func() { pgErr = startPG() })
	if pgErr != nil {
		t.Skipf("no PostgreSQL test server: %v", pgErr)
	}
	return pgPort
}

func startPG() error {
	if _, err := os.Stat(filepath.Join(pgBin, "postgres")); err != nil {
		return err
	}
	runAs := []string{}
	if os.Geteuid() == 0 {
		runAs = []string{"runuser", "-u", "nobody", "--"}
	}
	dir, err := os.MkdirTemp("", "gash-db-pg")
	if err != nil {
		return err
	}
	pgDir = dir
	if err := os.Chmod(dir, 0o777); err != nil {
		return err
	}
	data := filepath.Join(dir, "data")
	initdb := append(append([]string{}, runAs...), filepath.Join(pgBin, "initdb"), "-D", data, "-U", "postgres", "--auth=trust", "-E", "UTF8", "--locale=C.UTF-8")
	if out, err := exec.Command(initdb[0], initdb[1:]...).CombinedOutput(); err != nil {
		return fmt.Errorf("initdb: %v: %s", err, out)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	pgPort = l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	args := append(append([]string{}, runAs...), filepath.Join(pgBin, "postgres"), "-D", data, "-k", dir, "-h", "127.0.0.1",
		"-p", strconv.Itoa(pgPort), "-c", "fsync=off")
	pgProc = exec.Command(args[0], args[1:]...)
	pgProc.Stdout, pgProc.Stderr = nil, nil
	if err := pgProc.Start(); err != nil {
		pgProc = nil
		return err
	}
	for i := 0; i < 100; i++ {
		if exec.Command("pg_isready", "-q", "-h", "127.0.0.1", "-p", strconv.Itoa(pgPort)).Run() == nil {
			if err := pgSQL("postgres", "CREATE DATABASE app"); err != nil {
				return err
			}
			if err := pgSQL("postgres", "CREATE DATABASE other_db"); err != nil {
				return err
			}
			return pgSQL("app", fixturePG)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("postgres did not start")
}

// pgSQL runs SQL as the superuser through psql.
func pgSQL(db, sqlText string) error {
	cmd := exec.Command("psql", "-X", "-q", "-v", "ON_ERROR_STOP=1", "-h", "127.0.0.1", "-p", strconv.Itoa(pgPort), "-U", "postgres", "-d", db, "-c", sqlText)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "PGCONNECT_TIMEOUT=10"}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("psql: %v: %s", err, out)
	}
	return nil
}

const fixturePG = `CREATE TABLE users(id serial PRIMARY KEY, name text, born timestamp, score double precision,
  amount numeric(10,2), ok boolean, tags text[], meta jsonb, note text);
INSERT INTO users(name, born, score, amount, ok, tags, meta, note) VALUES
  ('alice', '2024-01-02 03:04:05.5', 0.1, 12.50, true, '{a,b}', '{"k": [1, 2.50, null], "s": "x\u00e9"}', NULL),
  ('bob', NULL, 1e300, NULL, false, '{}', 'null', 'tab	in'),
  ('carol "c" \', '2001-09-09', -0.0, 0.00, NULL, NULL, '{"b": 1, "a": 2}', 'two
lines'),
  ('é ☃ 😀', NULL, 1e-7, 99999999.99, true, '{"x y"}', '[1e2, 0.0000001]', '');
CREATE TABLE empty_t(x int);`
