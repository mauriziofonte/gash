// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"bufio"
	"bytes"
	"io"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
)

// aiQuery is ai_query [provider] "query": a non-interactive query. Help is
// the first argument, or the only word after a provider (LEDGER AI-24).
func (m *Module) aiQuery(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) || (providerHelp(c.Args) && help.NeedsOf(c, c.Arg(1))) {
		return 0
	}
	p := printer(c)

	// The argument grammar comes first: a provider alone is no question
	// (LEDGER AI-02), refused before stdin is read or a request is made.
	provider, query := parseQueryArgs(c.Args)
	if query == "" {
		p.Error("No query provided")
		return 1
	}

	piped := ""
	if !c.Term.StdinTTY() {
		piped = readPiped(c.Stdin)
	}
	if piped != "" {
		query = pipeWrap(piped, query)
	}

	provider, token, ok := m.resolveProvider(c, provider)
	if !ok {
		return 1
	}

	what := "your question"
	if piped != "" {
		what = "piped input and your question"
	}
	if !m.confirmSend(c, provider, "query", what, len(query)) {
		return 1
	}

	reply, rc := m.request(c, provider, token, systemPrompt,
		"Context: "+m.gatherContext(c)+"\n\n"+query, responseSchema, 1024, responseTimeout)
	if rc != 0 {
		return 1
	}
	m.formatResponse(c, reply)
	return 0
}

// parseQueryArgs is the ai_query grammar: a leading "claude" or "gemini"
// is the provider (case-sensitive, AI-17), the rest the question joined by
// one space (AI-16). A provider alone leaves the question empty (LEDGER
// AI-02: `ai_query claude` is no longer the question "claude").
func parseQueryArgs(args []string) (provider, query string) {
	if len(args) >= 1 && isProvider(args[0]) {
		return args[0], strings.Join(args[1:], " ")
	}
	return "", strings.Join(args, " ")
}

// isProvider reports whether word names a provider (case-sensitive).
func isProvider(word string) bool { return word == "claude" || word == "gemini" }

// providerHelp reports a provider followed by one word only, which is then
// help when it is -h or --help (LEDGER AI-24): `ai_query claude --help`.
func providerHelp(args []string) bool { return len(args) == 2 && isProvider(args[0]) }

// aiAsk is ai_ask [provider]: one interactive question read from the
// prompt (AI-03: one question per call). Help is the first argument, or
// the only word after a provider (LEDGER AI-24).
func (m *Module) aiAsk(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) || (providerHelp(c.Args) && help.NeedsOf(c, c.Arg(1))) {
		return 0
	}
	p := printer(c)

	provider, token, ok := m.resolveProvider(c, c.Arg(0))
	if !ok {
		return 1
	}
	// ai_ask validates the named provider explicitly before the token.
	if c.Arg(0) != "" && c.Arg(0) != "claude" && c.Arg(0) != "gemini" {
		p.Error("Unknown provider '" + c.Arg(0) + "'. Use 'claude' or 'gemini'")
		return 1
	}

	piped := ""
	if !c.Term.StdinTTY() {
		piped = readPiped(c.Stdin)
	}

	var query string
	if piped != "" {
		lines := strings.Count(piped, "\n") + 1
		p.Info("Piped input received (" + strconv.Itoa(lines) + " lines, " + strconv.Itoa(len(piped)) + " bytes)")
		query = m.readTTYLine() // AI-04: without a terminal this reads nothing
	} else {
		query = m.readStdinLine(c)
	}

	if query == "" && piped == "" {
		p.Warning("Empty query, aborting")
		return 1
	}
	if query == "" && piped != "" {
		query = "Analyze this content: explain what it does, identify any issues or errors, and suggest improvements."
	}
	if piped != "" {
		query = pipeWrap(piped, query)
	}

	what := "your question"
	if piped != "" {
		what = "piped input and your question"
	}
	if !m.confirmSend(c, provider, "query", what, len(query)) {
		return 1
	}

	reply, rc := m.request(c, provider, token, systemPrompt,
		"Context: "+m.gatherContext(c)+"\n\n"+query, responseSchema, 1024, responseTimeout)
	if rc != 0 {
		return 1
	}
	m.formatResponse(c, reply)
	return 0
}

// resolveProvider fills an empty provider from the first configured one
// and reads its token, printing the 2.x errors on failure.
func (m *Module) resolveProvider(c *invoke.Call, provider string) (string, string, bool) {
	p := printer(c)
	if provider == "" {
		f := config.LoadCaller(c.Env.Get)
		pr, ok := f.FirstAIProvider()
		if !ok {
			p.Error("No AI provider configured in ~/.gash_env")
			p.Info("Add: AI:claude=YOUR_API_KEY or AI:gemini=YOUR_API_KEY")
			return "", "", false
		}
		provider = pr
	}
	f := config.LoadCaller(c.Env.Get)
	token, ok := f.AIToken(provider)
	if !ok {
		p.Error("No token configured for '" + provider + "' in ~/.gash_env")
		return "", "", false
	}
	return provider, token, true
}

// pipeWrap is the troubleshoot-mode wrapper 2.x builds around piped input.
func pipeWrap(piped, query string) string {
	return "[PIPE INPUT - TROUBLESHOOT MODE]\n---\n" + piped + "\n---\nUser question: " + query
}

// readPiped is `head -c 4096` captured in $(...): up to 4096 bytes of
// stdin, NUL bytes dropped silently (AI-20), trailing newlines stripped,
// the "[... truncated to 4KB]" marker added when 4096 bytes remain (AI-19:
// bytes, not characters).
func readPiped(r io.Reader) string {
	if r == nil {
		return ""
	}
	buf := make([]byte, 4096)
	n, _ := io.ReadFull(r, buf)
	data := bytes.ReplaceAll(buf[:n], []byte{0}, nil)
	s := strings.TrimRight(string(data), "\n")
	if len(s) >= 4096 {
		s += "\n[... truncated to 4KB]"
	}
	return s
}

// readTTYLine reads one line from /dev/tty (the pipe case of ai_ask), ""
// when no terminal can be opened (AI-04 reproduced: the empty question
// becomes the auto-analysis prompt).
func (m *Module) readTTYLine() string {
	tty, err := m.openTTY()
	if err != nil {
		return ""
	}
	defer func() { _ = tty.Close() }()
	line, _ := bufio.NewReader(tty).ReadString('\n')
	return strings.TrimSuffix(line, "\n")
}

// readStdinLine reads one line from the call's stdin (the terminal, in the
// no-pipe case).
func (m *Module) readStdinLine(c *invoke.Call) string {
	if c.Stdin == nil {
		return ""
	}
	line, _ := bufio.NewReader(c.Stdin).ReadString('\n')
	return strings.TrimSuffix(line, "\n")
}
