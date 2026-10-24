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

// TestHTEAutostart: the layer starts the HTE stack only when hte.toml says
// autostart = "shell" (design hte.md 5), from the XDG config directory.
func TestHTEAutostart(t *testing.T) {
	const line = `("$__GASH_BIN" hte __autostart </dev/null >/dev/null 2>/home/u/.local/state/gash/hte/logs/autostart.log &) 2>/dev/null || true`
	for _, tc := range []struct {
		name, file, content string
		env                 map[string]string
		want                bool
		line                string // the start line, when not the default
	}{
		{name: "no hte.toml"},
		{name: "shell", file: "/home/u/.config/gash/hte/hte.toml", content: hteToml("shell"), want: true},
		{name: "shell without the logs directory", file: "/home/u/.config/gash/hte/hte.toml", content: hteToml("shell"), want: true,
			line: `("$__GASH_BIN" hte __autostart </dev/null >/dev/null 2>/dev/null &) 2>/dev/null || true`},
		{name: "systemd", file: "/home/u/.config/gash/hte/hte.toml", content: hteToml("systemd")},
		{name: "none", file: "/home/u/.config/gash/hte/hte.toml", content: hteToml("none")},
		{name: "invalid file", file: "/home/u/.config/gash/hte/hte.toml", content: "autostart = \"shell\"\n"},
		{name: "XDG_CONFIG_HOME", file: "/xdg/gash/hte/hte.toml", content: hteToml("shell"),
			env: map[string]string{"XDG_CONFIG_HOME": "/xdg"}, want: true},
		{name: "XDG_CONFIG_HOME elsewhere", file: "/home/u/.config/gash/hte/hte.toml", content: hteToml("shell"),
			env: map[string]string{"XDG_CONFIG_HOME": "/xdg"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness()
			if tc.line == "" {
				h.withDir("/home/u/.local/state/gash/hte/logs")
				tc.line = line
			}
			if tc.file != "" {
				h.withFile(tc.file, tc.content)
			}
			for k, v := range tc.env {
				h.env[k] = v
			}
			out := h.emit(t)
			if got := strings.Count(out, "hte __autostart"); got != map[bool]int{true: 1, false: 0}[tc.want] {
				t.Fatalf("%d autostart lines in:\n%s", got, out)
			}
			if !tc.want {
				return
			}
			if !strings.Contains(out, tc.line+"\n") || !strings.Contains(out, `if [[ $- == *i* && -z "${__GASH_HTE_AUTOSTART-}" && -n "${__GASH_BIN-}" ]]; then`) {
				t.Errorf("autostart block:\n%s", out)
			}
			// The layer still ends with a status-0 line after it.
			if i, j := strings.Index(out, "hte __autostart"), strings.LastIndex(out, "\ntrue\n"); j < i {
				t.Errorf("the autostart block comes after the final true:\n%s", out)
			}
		})
	}
}
