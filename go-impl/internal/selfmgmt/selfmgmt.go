// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package selfmgmt holds the functions that manage Gash itself (the
// lib/modules/gash.sh module of 2.x).
package selfmgmt

import (
	"math/rand/v2"
	"os"

	"github.com/mauriziofonte/gash/internal/buildinfo"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
)

// Module is the selfmgmt handler set with its process and file seams.
type Module struct {
	run runner.Runner
	// lookPathFn replaces the PATH search in tests.
	lookPathFn func(name string) (string, bool)
	readFile   func(string) ([]byte, error)
	passwd     string
	// intn picks the quote: a uniform int in [0, n).
	intn func(n int) int
	// version and executable are the binary's own (gash_doctor).
	version    string
	executable func() (string, error)
}

// New returns the module wired to the real system.
func New() *Module {
	return &Module{
		run: runner.Exec{}, readFile: readFileOS, passwd: "/etc/passwd", intn: rand.IntN,
		version: buildinfo.Version, executable: os.Executable,
	}
}

// Register binds the selfmgmt functions to t.
func Register(t *invoke.Table) { New().Register(t) }

// Register binds the functions of m to t.
func (m *Module) Register(t *invoke.Table) {
	t.Handle("gash", m.card)
	t.Handle("gash_username", m.username)
	t.Handle("gash_inspiring_quote", m.quote)
	t.Handle("gash_migrate", m.migrate)
	t.Handle("gash_env_init", m.envInit)
	t.Handle("gash_doctor", m.doctor)
}
