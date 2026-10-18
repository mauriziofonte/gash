// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package handlers wires every module's handlers into one table. It is
// the only place that knows the list of modules: a new module adds one
// line to Table.
package handlers

import (
	"github.com/mauriziofonte/gash/internal/files"
	"github.com/mauriziofonte/gash/internal/fsscan"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/selfmgmt"
)

// Table returns the table of every ported function and compat operation.
// extra registers more (the cli adds its compat operations); it runs
// after the modules.
func Table(extra ...func(*invoke.Table)) *invoke.Table {
	t := invoke.NewTable()
	registerCore(t)
	registerOutput(t)
	selfmgmt.Register(t)
	fsscan.Register(t)
	files.Register(t)
	for _, r := range extra {
		r(t)
	}
	return t
}
