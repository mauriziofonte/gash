// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package service

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/lazyre"
)

// Autostart modes (design/hte.md 5, recorded in the receipt).
const (
	AutostartSystemd = "systemd" // user unit plus linger
	AutostartLaunchd = "launchd" // user LaunchAgent
	AutostartShell   = "shell"   // the first interactive shell starts the stack
	AutostartNone    = "none"
)

// UnitName is the systemd user unit.
const UnitName = "gash-hte.service"

// LaunchAgentLabel is the label of the macOS LaunchAgent.
const LaunchAgentLabel = "io.github.mauriziofonte.gash.hte"

// UnitArgs are the arguments the unit and the LaunchAgent run gash with:
// the hidden command that starts or stops the masters directly. (The
// public service start delegates to systemctl when the unit is in use, so
// the unit cannot run it without recursion.)
func UnitArgs(action string) []string { return []string{"hte", "__unit", action} }

// UnitPath is the user unit under $XDG_CONFIG_HOME/systemd/user.
func UnitPath(configHome string) string {
	return filepath.Join(configHome, "systemd", "user", UnitName)
}

// LaunchAgentPath is the LaunchAgent plist under ~/Library/LaunchAgents.
func LaunchAgentPath(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents", LaunchAgentLabel+".plist")
}

func checkProgram(gash string) error {
	if !filepath.IsAbs(gash) || strings.ContainsFunc(gash, unicode.IsControl) {
		return hteerr.Newf(hteerr.PlatformUnsupported, "", "%q cannot be the program of an autostart unit", gash)
	}
	return nil
}

// systemdWord quotes one word of an Exec line: double quotes, with \, "
// escaped, % doubled (specifiers) and $ doubled (variables).
func systemdWord(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%", "$", "$$")
	return `"` + r.Replace(s) + `"`
}

func execLine(gash, action string) string {
	words := []string{systemdWord(gash)}
	for _, a := range UnitArgs(action) {
		words = append(words, systemdWord(a))
	}
	return strings.Join(words, " ")
}

// SystemdUnit is the user unit of design/hte.md 5: a oneshot that
// remains active, started at login (and at boot with linger).
func SystemdUnit(gash string) ([]byte, error) {
	if err := checkProgram(gash); err != nil {
		return nil, err
	}
	return fmt.Appendf(nil, `# Managed by gash hte install; gash hte uninstall removes it.
[Unit]
Description=Gash HTE userland web stack (Apache and PHP-FPM)

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=%s
ExecStop=%s
TimeoutStartSec=120

[Install]
WantedBy=default.target
`, execLine(gash, "start"), execLine(gash, "stop")), nil
}

// AgentEnv is the environment of the LaunchAgent: the XDG directories
// install resolved, so that the agent, which launchd starts without them,
// finds the same tree (XDG_RUNTIME_DIR only when it was set).
func AgentEnv(configHome, stateHome, dataHome, runtimeDir string) []string {
	env := []string{"XDG_CONFIG_HOME=" + configHome, "XDG_DATA_HOME=" + dataHome, "XDG_STATE_HOME=" + stateHome}
	if runtimeDir != "" {
		env = append(env, "XDG_RUNTIME_DIR="+runtimeDir)
	}
	return env
}

var agentKeyRE = lazyre.MustCompile(`^XDG_[A-Z_]+$`)

// LaunchAgent is the macOS user agent: it runs the start at login, with
// env (KEY=VALUE, absolute paths) as its environment variables.
func LaunchAgent(gash string, env []string) ([]byte, error) {
	if err := checkProgram(gash); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	esc := func(s string) string {
		var e bytes.Buffer
		_ = xml.EscapeText(&e, []byte(s))
		return e.String()
	}
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<!-- Managed by gash hte install; gash hte uninstall removes it. -->
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>` + LaunchAgentLabel + `</string>
  <key>ProgramArguments</key>
  <array>
`)
	for _, a := range append([]string{gash}, UnitArgs("start")...) {
		b.WriteString("    <string>" + esc(a) + "</string>\n")
	}
	b.WriteString("  </array>\n")
	if len(env) > 0 {
		b.WriteString("  <key>EnvironmentVariables</key>\n  <dict>\n")
		for _, kv := range env {
			k, val, _ := strings.Cut(kv, "=")
			if !agentKeyRE.MatchString(k) || checkProgram(val) != nil {
				return nil, hteerr.Newf(hteerr.PlatformUnsupported, "", "%q cannot be in the environment of the LaunchAgent", kv)
			}
			b.WriteString("    <key>" + k + "</key>\n    <string>" + esc(val) + "</string>\n")
		}
		b.WriteString("  </dict>\n")
	}
	b.WriteString(`  <key>RunAtLoad</key>
  <true/>
</dict>
</plist>
`)
	return b.Bytes(), nil
}
