// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package ai ports the 2.x AI chat module (lib/modules/ai.sh and its
// ai/ parts): ai_ask, ai_query and ai_sysinfo. The shared HTTP client,
// the request bodies, the retry policy and the error texts are the 2.x
// ones over net/http (LEDGER AI-13, AI-14, AI-15): no curl and no jq, the
// key header set in-process, proxies from HTTPS_PROXY/NO_PROXY and the CA
// from the system pool or SSL_CERT_FILE/SSL_CERT_DIR. The LEDGER defects
// of the 2.x behavior (AI-01..AI-12) are reproduced; AI-16..AI-24 are the
// forced-divergence and operator decisions kept as 2.x.
package ai

import (
	"net/http"
	"os"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/sysinfo"
	"github.com/mauriziofonte/gash/internal/ui"
)

// Endpoints, models and timeouts (lib/modules/ai/prompts.sh).
const (
	claudeAPI   = "https://api.anthropic.com/v1/messages"
	claudeModel = "claude-haiku-5-5"
	geminiBase  = "https://generativelanguage.googleapis.com/v1beta/models"
	geminiModel = "gemini-2.5-flash"

	connectTimeout  = 5 * time.Second
	responseTimeout = 60 * time.Second

	sysinfoMaxTokens       = 8192
	sysinfoResponseTimeout = 120 * time.Second

	maxRetries = 3
)

// Module is the AI handler with its seams: the sysroot (/etc/os-release
// for the context distro), the runner (git branch of the context), the
// sysinfo module (ai_sysinfo collectors), the HTTP transport and the
// endpoints (overridden to a test server), a clock and a sleep.
type Module struct {
	sys     platform.Sysroot
	run     runner.Runner
	si      *sysinfo.Module
	client  *http.Client
	claude  string
	gemini  string
	now     func() time.Time
	sleep   func(time.Duration)
	getwd   func() (string, error)
	readDir func(string) ([]os.DirEntry, error)
	openTTY func() (*os.File, error)
	// pipeWait bounds the wait for piped input (LEDGER AI-05).
	pipeWait time.Duration
}

// New returns the module wired to the real system.
func New() *Module {
	return &Module{
		sys:     platform.Real(),
		run:     runner.Exec{},
		si:      sysinfo.New(),
		claude:  claudeAPI,
		gemini:  geminiBase,
		now:     time.Now,
		sleep:   time.Sleep,
		getwd:   os.Getwd,
		readDir: os.ReadDir,
		openTTY: func() (*os.File, error) { return os.OpenFile("/dev/tty", os.O_RDWR, 0) },

		pipeWait: pipedWait,
	}
}

// Register binds the AI functions to t.
func Register(t *invoke.Table) { New().Register(t) }

// Register binds the functions of m to t.
func (m *Module) Register(t *invoke.Table) {
	t.Handle("ai_ask", m.aiAsk)
	t.Handle("ai_query", m.aiQuery)
	t.Handle("ai_sysinfo", m.aiSysinfo)
}

// printer is the stderr printer of a call, with color on a color terminal.
func printer(c *invoke.Call) ui.Printer { return ui.NewPrinter(c.Stderr, c.Term, false) }

// style is the output style of a call.
func style(c *invoke.Call) ui.Style { return ui.StyleOf(c.Term, false) }
