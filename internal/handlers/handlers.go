// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package handlers wires every module's handlers into one table. It is
// the only place that knows the list of modules: a new module adds one
// line to Table.
package handlers

import (
	"github.com/mauriziofonte/gash/internal/agent"
	"github.com/mauriziofonte/gash/internal/ai"
	"github.com/mauriziofonte/gash/internal/chrome"
	"github.com/mauriziofonte/gash/internal/db"
	"github.com/mauriziofonte/gash/internal/dev"
	"github.com/mauriziofonte/gash/internal/docker"
	"github.com/mauriziofonte/gash/internal/files"
	"github.com/mauriziofonte/gash/internal/fsscan"
	"github.com/mauriziofonte/gash/internal/gitx"
	"github.com/mauriziofonte/gash/internal/hte/htecmd"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llm"
	"github.com/mauriziofonte/gash/internal/logs"
	"github.com/mauriziofonte/gash/internal/remote"
	"github.com/mauriziofonte/gash/internal/runcmd"
	"github.com/mauriziofonte/gash/internal/selfmgmt"
	"github.com/mauriziofonte/gash/internal/ssh"
	"github.com/mauriziofonte/gash/internal/sysinfo"
	"github.com/mauriziofonte/gash/internal/system"
	"github.com/mauriziofonte/gash/internal/wincmd"
	"github.com/mauriziofonte/gash/internal/wslcmd"
)

// Table returns the table of every ported function and compat operation.
// extra registers more (the cli adds its compat operations); it runs
// after the modules.
func Table(extra ...func(*invoke.Table)) *invoke.Table {
	t := invoke.NewTable()
	registerCore(t)
	registerConfig(t)
	registerHelp(t)
	registerOutput(t)
	selfmgmt.Register(t)
	llm.Register(t)
	agent.Register(t)
	db.Register(t)
	dev.Register(t)
	ai.Register(t)
	docker.Register(t)
	files.Register(t)
	chrome.Register(t)
	fsscan.Register(t)
	gitx.Register(t)
	remote.Register(t)
	ssh.Register(t)
	sysinfo.Register(t)
	system.Register(t)
	htecmd.Register(t)
	logs.Register(t)
	wslcmd.Register(t)
	wincmd.Register(t)
	runcmd.Register(t)
	for _, r := range extra {
		r(t)
	}
	return t
}
