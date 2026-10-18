// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"os/user"
	"regexp"
	"strings"

	"github.com/mauriziofonte/gash/internal/pathguard"
)

// Settings are the three llm_exec settings, which 2.x reads from ~/.gash_env
// only (never the environment). The config worker owns the file reading and
// passes the raw values here.
type Settings struct {
	Exec      string // GASH_LLM_EXEC ("" or "unrestricted")
	ExecAllow string // GASH_LLM_EXEC_ALLOW (raw, e.g. "make npm")
	Sandbox   string // GASH_LLM_SANDBOX ("" or "1")
}

// Environ is the process context llm_exec needs. Vars is the caller's
// environment (the parent's exported variables); FS and LookupUser default to
// the OS when nil.
type Environ struct {
	Cwd        string
	Home       string
	RootHome   string
	BashPath   string
	Vars       map[string]string
	FS         pathguard.FS
	LookupUser func(name string) (home string, ok bool)
}

func (e Environ) fs() pathguard.FS {
	if e.FS != nil {
		return e.FS
	}
	return pathguard.OSFS{}
}

func (e Environ) bash() string {
	if e.BashPath != "" {
		return e.BashPath
	}
	return "bash"
}

// Plan is what to run: the full argv and the exact environment to run it with
// (the caller sets the process environment to Env, i.e. does not inherit).
type Plan struct {
	Argv []string
	Env  []string
}

var allowWordRE = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

// parseAllow splits GASH_LLM_EXEC_ALLOW on comma, colon and whitespace and
// keeps the words that are plain command names, in order (exec.sh:1132-1134).
func parseAllow(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ':' || r == ' ' || r == '\t' || r == '\n'
	})
	var out []string
	for _, w := range fields {
		if allowWordRE.MatchString(w) {
			out = append(out, w)
		}
	}
	return out
}

// Build validates a single command line under the given settings and returns
// the argv and environment to run (bash --noprofile --norc -c <script> plus
// the 2.x wrapper functions), without running anything. On refusal it returns
// a *Refusal with the exact 2.x code, details, action and hint.
//
// Restricted mode (the default) renders the validated parse as a canonical
// single-quoted script (D-1 A). Unrestricted mode (the user's file-only
// opt-in) returns the raw line for bash -c after the blocklist only.
func Build(line string, s Settings, env Environ, pr Probe) (*Plan, error) {
	if pr == nil {
		pr = OSProbe{}
	}

	// Blank line (only whitespace).
	if strings.IndexFunc(line, func(r rune) bool { return !isSpaceByte(r) }) < 0 {
		return nil, refuse("empty_command", "", "RETRY", "Provide a command to execute")
	}

	if err := validateCommand(line); err != nil {
		return nil, err
	}

	extra := parseAllow(s.ExecAllow)
	extraSet := map[string]bool{}
	for _, w := range extra {
		extraSet[w] = true
	}

	mode := s.Exec

	canonical := line
	if mode != "unrestricted" {
		lc := newLineChecker(env, pr, extraSet)
		script, err := lc.checkAndRender(line)
		if err != nil {
			return nil, err
		}
		canonical = script
	}

	if s.Sandbox == "1" && !pr.HasBwrap() {
		return nil, refuse("sandbox_unavailable", "bwrap", "STOP",
			"GASH_LLM_SANDBOX=1 needs bubblewrap (bwrap) with user namespaces; ask the user to install it or turn the setting off")
	}

	var child string
	if mode == "unrestricted" {
		child = preludeUnrestricted(extra) + "\n" + line
	} else {
		child = prelude(extra, pr) + "\n" + canonical
	}

	argv := []string{env.bash(), "--noprofile", "--norc", "-c", child}
	if s.Sandbox == "1" {
		argv = append([]string{
			"bwrap", "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc",
			"--unshare-all", "--die-with-parent", "--chdir", env.Cwd,
		}, argv...)
	}

	var envSlice []string
	if mode == "unrestricted" {
		envSlice = unrestrictedEnv(env)
	} else {
		envSlice = buildChildEnv(env)
	}

	return &Plan{Argv: argv, Env: envSlice}, nil
}

func isSpaceByte(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f'
}

// lookupUserOS resolves ~user through the passwd database.
func lookupUserOS(name string) (string, bool) {
	if u, err := user.Lookup(name); err == nil {
		return u.HomeDir, true
	}
	return "", false
}
