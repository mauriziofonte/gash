// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package handlers wires every module's handlers into one table. It is
// the only place that knows the list of modules: a new module adds one
// line to Table.
package handlers

import (
	"github.com/mauriziofonte/gash/internal/chrome"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/selfmgmt"
	"github.com/mauriziofonte/gash/internal/ssh"
	"github.com/mauriziofonte/gash/internal/sysinfo"
	"github.com/mauriziofonte/gash/internal/system"
)

// Table returns the table of every ported function and compat operation.
// extra registers more (the cli adds its compat operations); it runs
// after the modules.
func Table(extra ...func(*invoke.Table)) *invoke.Table {
	t := invoke.NewTable()
	registerCore(t)
	registerOutput(t)
	selfmgmt.Register(t)
	system.Register(t)
	ssh.Register(t)
	chrome.Register(t)
	sysinfo.Register(t)
	for _, r := range extra {
		r(t)
	}
	return t
}
