// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"io"
	"os"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/ui"
)

// registerOutput binds the compat operations the generated core shims
// call (go-impl/PORT/HOWTO.md, "Compat operations").
func registerOutput(t *invoke.Table) {
	t.HandleCompat("output table", compatTable)
	t.HandleCompat("output box", compatBox)
	t.HandleCompat("output wrap", compatWrap)
	t.HandleCompat("output choose", compatChoose)
	t.HandleCompat("output download", compatDownload)
	t.HandleCompat("json str", compatJSONStr)
	t.HandleCompat("json arr", compatJSONArr)
	t.HandleCompat("env load", compatEnvLoad)
	t.HandleCompat("env perms", compatEnvPerms)
	t.HandleCompat("prereq fail", compatPrereqFail)
}

func readAll(c *invoke.Call) string {
	if c.Stdin == nil {
		return ""
	}
	b, _ := io.ReadAll(c.Stdin)
	return string(b)
}

// compatTable is `gash __compat output table ARGS...`: __gash_table with
// its rows on stdin. GASH__TABLE_COLOR=1 is the color decision of
// __gash_table_into, made in the caller's shell.
func compatTable(c *invoke.Call) int {
	s := ui.StyleOf(c.Term, false)
	if c.Env.Get("GASH__TABLE_COLOR") == "1" {
		s.Color = true
	}
	s.Table(c.Stdout, ui.ReadRows(c.Stdin), c.Args...)
	return 0
}

// compatBox is `gash __compat output box [--color ESC]`: __gash_box with
// its lines on stdin, each NUL-terminated.
func compatBox(c *invoke.Call) int {
	border := ""
	if c.Arg(0) == "--color" {
		border = c.Arg(1)
	}
	var lines []string
	if data := readAll(c); data != "" {
		lines = strings.Split(strings.TrimSuffix(data, "\x00"), "\x00")
	}
	ui.StyleOf(c.Term, false).Box(c.Stdout, border, lines...)
	return 0
}

// compatWrap is `gash __compat output wrap WIDTH`: the text on stdin,
// one wrapped line per output line.
func compatWrap(c *invoke.Call) int {
	var b strings.Builder
	for _, l := range ui.Wrap(ui.Int(c.Arg(0)), readAll(c)) {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	_, _ = io.WriteString(c.Stdout, b.String())
	return 0
}

// resultFD opens the descriptor __gash_choose reads the index from (3).
var resultFD = func() io.WriteCloser { return os.NewFile(3, "result") }

// compatChoose is `gash __compat output choose DEFAULT OPTION...`: the
// menu on the terminal, the 1-based index on fd 3; status 1 on cancel.
func compatChoose(c *invoke.Call) int {
	var opts []string
	if len(c.Args) > 1 {
		opts = c.Args[1:]
	}
	idx, ok := ui.StyleOf(c.Term, false).Choose(c.Ctx, c.Term, c.Stdin, c.Stdout, ui.Int(c.Arg(0)), opts)
	if !ok {
		return 1
	}
	w := resultFD()
	defer func() { _ = w.Close() }()
	if _, err := io.WriteString(w, strconv.Itoa(idx)); err != nil {
		return 1
	}
	return 0
}

// compatDownload is `gash __compat output download URL DEST`.
func compatDownload(c *invoke.Call) int {
	return ui.Downloader{}.Run(c.Ctx, ui.StyleOf(c.Term, false), c.Term, c.Stderr, c.Arg(0), c.Arg(1))
}

// compatJSONStr is `gash __compat json str`: the raw value on stdin, the
// JSON string on stdout (__gash_json_str).
func compatJSONStr(c *invoke.Call) int {
	_, _ = io.WriteString(c.Stdout, contract.JQString(readAll(c)))
	return 0
}

// compatJSONArr is `gash __compat json arr`: the values on stdin, each
// NUL-terminated, the JSON array on stdout (__gash_json_arr).
func compatJSONArr(c *invoke.Call) int {
	var vals []string
	if data := readAll(c); data != "" {
		vals = strings.Split(strings.TrimSuffix(data, "\x00"), "\x00")
	}
	_, _ = io.WriteString(c.Stdout, contract.JQArray(vals...))
	return 0
}

// envCache are the shell variables __gash_load_env fills, in 2.x order.
var envCache = []string{
	"__GASH_ENV_LOADED", "__GASH_ENV_SSH_KEYS", "__GASH_ENV_DB_ENTRIES",
	"__GASH_ENV_AI_PROVIDERS", "__GASH_ENV_HOST_ENTRIES", "__GASH_ENV_SETTINGS",
}

// compatEnvPerms is `gash __compat env perms FILE`: the mode and owner
// warnings of __gash_check_env_perms, for a shell whose stat reads neither
// the GNU nor the BSD format (3.4: there was no warning then).
func compatEnvPerms(c *invoke.Call) int {
	p := ui.NewPrinter(c.Stderr, c.Term, false)
	for _, w := range config.CheckPerms(c.Arg(0), c.Env.Get("HOME"), os.Geteuid()) {
		p.Warning(w.Text)
	}
	return 0
}

// compatPrereqFail is `gash __compat prereq fail TOOL NEED`: the report a
// missing tool gets everywhere (the envelope when headless), for the Bash
// helpers __gash_require_command and __gash_require_binary. Status 1.
func compatPrereqFail(c *invoke.Call) int {
	return prereq.Fail(c, c.Arg(0), c.Arg(1), false)
}

// compatEnvLoad is `gash __compat env load`: the warnings of the load on
// stderr, then the cache of __gash_load_env as quoted Bash assignments
// that the shim evaluates. Secrets travel on this pipe only.
func compatEnvLoad(c *invoke.Call) int {
	f := config.LoadCaller(c.Env.Get)
	p := ui.NewPrinter(c.Stderr, c.Term, false)
	for _, w := range f.Warnings {
		p.Warning(w.Text)
		if w.Hint != "" {
			p.Hint(w.Hint)
		}
	}
	join := func(es []config.Entry) string {
		parts := make([]string, len(es))
		for i, e := range es {
			parts[i] = e.Name + "\t" + e.Value
		}
		return strings.Join(parts, "\n")
	}
	settings := make([]string, len(f.Settings))
	for i, s := range f.Settings {
		settings[i] = s.Name + "\t" + s.Value
	}
	vals := []string{"1", join(f.SSHKeys), join(f.DB), join(f.AI), join(f.Hosts), strings.Join(settings, "\n")}
	var b strings.Builder
	for i, name := range envCache {
		q, err := syntax.Quote(vals[i], syntax.LangBash)
		if err != nil { // a NUL byte: the parser drops them, so never here
			return c.Fail(contract.New("internal_error", "cannot quote "+name, ""))
		}
		b.WriteString(name + "=" + q + "\n")
	}
	_, _ = io.WriteString(c.Stdout, b.String())
	return 0
}
