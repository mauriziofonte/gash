// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package shellinit

import (
	"strings"
	"testing"
)

func hteToml(autostart string) string {
	return "listen = [\"127.0.0.1\", \"::1\"]\ndefault_php = \"\"\nmodules = []\nautostart = \"" + autostart +
		"\"\n\n[ports]\nhttp = 8080\nhttps = 8443\n"
}

// TestHTEAutostart: the layer starts the localweb stack only when
// localweb.toml says autostart = "shell" (design hte.md 5), from the XDG
// config directory; a tree of Gash 4.0 not moved yet (hte.toml) too.
func TestHTEAutostart(t *testing.T) {
	const line = `("$__GASH_BIN" localweb __autostart </dev/null >/dev/null 2>/home/u/.local/state/gash/localweb/logs/autostart.log &) 2>/dev/null || true`
	for _, tc := range []struct {
		name, file, content string
		env                 map[string]string
		want                bool
		logs                string // the logs directory, when not the default
		line                string // the start line, when not the default
	}{
		{name: "no localweb.toml"},
		{name: "shell", file: "/home/u/.config/gash/localweb/localweb.toml", content: hteToml("shell"), want: true},
		{name: "shell without the logs directory", file: "/home/u/.config/gash/localweb/localweb.toml", content: hteToml("shell"), want: true,
			line: `("$__GASH_BIN" localweb __autostart </dev/null >/dev/null 2>/dev/null &) 2>/dev/null || true`},
		{name: "systemd", file: "/home/u/.config/gash/localweb/localweb.toml", content: hteToml("systemd")},
		{name: "none", file: "/home/u/.config/gash/localweb/localweb.toml", content: hteToml("none")},
		{name: "invalid file", file: "/home/u/.config/gash/localweb/localweb.toml", content: "autostart = \"shell\"\n"},
		{name: "XDG_CONFIG_HOME", file: "/xdg/gash/localweb/localweb.toml", content: hteToml("shell"),
			env: map[string]string{"XDG_CONFIG_HOME": "/xdg"}, want: true},
		{name: "XDG_CONFIG_HOME elsewhere", file: "/home/u/.config/gash/localweb/localweb.toml", content: hteToml("shell"),
			env: map[string]string{"XDG_CONFIG_HOME": "/xdg"}},
		{name: "Gash 4.0 tree", file: "/home/u/.config/gash/hte/hte.toml", content: hteToml("shell"), want: true,
			logs: "/home/u/.local/state/gash/hte/logs",
			line: `("$__GASH_BIN" localweb __autostart </dev/null >/dev/null 2>/home/u/.local/state/gash/hte/logs/autostart.log &) 2>/dev/null || true`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness()
			switch {
			case tc.logs != "":
				h.withDir(tc.logs)
			case tc.line == "":
				h.withDir("/home/u/.local/state/gash/localweb/logs")
				tc.line = line
			}
			if tc.file != "" {
				h.withFile(tc.file, tc.content)
			}
			for k, v := range tc.env {
				h.env[k] = v
			}
			out := h.emit(t)
			if got := strings.Count(out, "localweb __autostart"); got != map[bool]int{true: 1, false: 0}[tc.want] {
				t.Fatalf("%d autostart lines in:\n%s", got, out)
			}
			if !tc.want {
				return
			}
			if !strings.Contains(out, tc.line+"\n") || !strings.Contains(out, `if [[ $- == *i* && -z "${__GASH_LOCALWEB_AUTOSTART-}" && -n "${__GASH_BIN-}" ]]; then`) {
				t.Errorf("autostart block:\n%s", out)
			}
			// The layer still ends with a status-0 line after it.
			if i, j := strings.Index(out, "localweb __autostart"), strings.LastIndex(out, "\ntrue\n"); j < i {
				t.Errorf("the autostart block comes after the final true:\n%s", out)
			}
		})
	}
}
