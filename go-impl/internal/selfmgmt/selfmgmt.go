// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package selfmgmt holds the functions that manage Gash itself (the
// lib/modules/gash.sh module of 2.x).
package selfmgmt

import (
	"github.com/mauriziofonte/gash/internal/invoke"
)

// Module is the selfmgmt handler set with its process and file seams.
type Module struct {
	proc     procs
	readFile func(string) ([]byte, error)
	passwd   string
}

// New returns the module wired to the real system.
func New() *Module {
	return &Module{proc: osProcs{}, readFile: readFileOS, passwd: "/etc/passwd"}
}

// Register binds the selfmgmt functions to t.
func Register(t *invoke.Table) { New().Register(t) }

// Register binds the functions of m to t.
func (m *Module) Register(t *invoke.Table) {
	t.Handle("gash_username", m.username)
}
