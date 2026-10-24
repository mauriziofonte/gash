// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"bufio"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
)

// confirmSend is __ai_confirm_send: ask before sending data to a provider.
// GASH_AI_CONFIRM auto (default) asks before data Gash collected itself
// (kind "data"), never for what the user typed or piped (kind "query");
// 0 never asks; 1 and any other value ask before every request, a value
// other than 0, 1 or auto with a warning, once per call (AI-23: fail
// closed, case-sensitive). Without a terminal, auto and the other values
// refuse.
func (m *Module) confirmSend(c *invoke.Call, provider, kind, what string, nbytes int) bool {
	p := printer(c)
	// A model name that is not plain is refused before anyone is asked
	// about a request that would not go out (AI-12).
	if !m.modelOK(c, provider) {
		return false
	}
	mode := c.Env.Get("GASH_AI_CONFIRM")
	if mode == "" {
		mode = "auto"
	}
	if mode == "0" {
		return true
	}
	if mode != "1" && mode != "auto" {
		if _, done := m.confirmWarned.LoadOrStore(c, true); !done {
			p.Warning("GASH_AI_CONFIRM=" + sanitizeText(mode) + " is not 0, 1 or auto: asking before every request")
		}
	}
	if mode == "auto" && kind == "query" {
		return true
	}
	size := strconv.Itoa(nbytes) + " B"
	if nbytes >= 1024 {
		size = strconv.Itoa((nbytes+512)/1024) + " KB"
	}
	tty, err := m.openTTY()
	if err != nil || c.Term.Headless() {
		if tty != nil {
			_ = tty.Close()
		}
		p.Error("Not sending " + what + " (" + size + ") to " + provider + ": no terminal to confirm on. Set GASH_AI_CONFIRM=0 in ~/.gash_env to allow it")
		return false
	}
	defer func() { _ = tty.Close() }()
	style(c).AskRender(c.Stderr, "Send "+what+" ("+size+", secrets redacted) to "+provider+" ("+m.model(provider, c.Env.Get)+")?", "y/N")
	line, _ := bufio.NewReader(tty).ReadString('\n')
	return yesAnswer(strings.TrimSuffix(line, "\n"))
}

// yesAnswer is the acceptance test of __ai_confirm_send: y, Y, or a
// three-letter "yes" in any case mix.
func yesAnswer(a string) bool {
	if a == "y" || a == "Y" {
		return true
	}
	return len(a) == 3 &&
		(a[0] == 'y' || a[0] == 'Y') &&
		(a[1] == 'e' || a[1] == 'E') &&
		(a[2] == 's' || a[2] == 'S')
}
