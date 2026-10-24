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
	BashPath   string // the bash to run; see Build
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
// Wrap, Bash and Script are the same argv as runner.Bash takes it: the
// bwrap prefix (nil without the sandbox), the bash program and the -c
// script.
type Plan struct {
	Argv   []string
	Env    []string
	Wrap   []string
	Bash   string
	Script string
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
//
// 2.x runs "${BASH:-bash}", the absolute path of the user's bash, and bash
// starts a syntax error message with that path. Pass the absolute path in
// Environ.BashPath (exec.LookPath("bash"), say) for the same messages; when
// it is empty, argv[0] is "bash".
func Build(line string, s Settings, env Environ, pr Probe) (*Plan, error) {
	if pr == nil {
		pr = noProbe{}
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

	// 2.x joins "$(__llm_exec_prelude ...)" and the line with one newline;
	// the command substitution drops the prelude's trailing newlines, so the
	// line starts on the line right after the last prelude line.
	var child string
	if mode == "unrestricted" {
		child = strings.TrimRight(preludeUnrestricted(extra), "\n") + "\n" + line
	} else {
		child = strings.TrimRight(prelude(extra, pr), "\n") + "\n" + canonical
	}

	argv := []string{env.bash(), "--noprofile", "--norc", "-c", child}
	var wrap []string
	if s.Sandbox == "1" {
		wrap = []string{
			"bwrap", "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc",
			"--unshare-all", "--die-with-parent", "--chdir", env.Cwd,
		}
		argv = append(append([]string{}, wrap...), argv...)
	}

	var envSlice []string
	if mode == "unrestricted" {
		envSlice = unrestrictedEnv(env)
	} else {
		envSlice = buildChildEnv(env)
	}

	return &Plan{Argv: argv, Env: envSlice, Wrap: wrap, Bash: env.bash(), Script: child}, nil
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
