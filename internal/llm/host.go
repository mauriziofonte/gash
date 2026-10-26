// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

// --host ALIAS on the llm_* reads (plan S5). A function opts in with one
// line at its registration, t.Handle(name, m.hostable(name, m.fn)), and
// one entry in hostSpecs below: its value options, which positionals are
// remote paths, and its POSIX sh fallback, if any (host_*.go). The
// transport, the shipped binary, the rails and the "host" key are
// internal/remote's (host.go there).

import (
	"bytes"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/remote"
)

// hostSpecs are the functions that take --host. Most are binary only (no
// Fallback): the host runs the same gash, shipped once and cached, so the
// answer is the local one.
func (m *Module) hostSpecs() map[string]remote.Spec {
	return map[string]remote.Spec{
		// Files and search: a POSIX fallback covers the 3.0 options.
		"llm_find":   {Values: []string{"--type", "--contains", "-n", "--limit", "--sort", "--newer", "--since", "--by"}, PathFrom: 1, PathOpts: []string{"--newer"}, Fallback: findFallback},
		"llm_grep":   {Values: []string{"--ext", "--context", "-C", "-n", "--limit", "--exclude", "--by"}, PathFrom: 1, Fallback: grepFallback},
		"llm_tree":   {Values: []string{"--depth", "--max-depth", "--by", "--type", "--name", "--newer", "--since", "--top"}, PathFrom: 0, PathOpts: []string{"--newer"}, Fallback: treeFallback},
		"llm_config": {Values: []string{"--max-bytes", "--path"}, PathFrom: 0, Keep: true, Fallback: configFallback},

		// Text reads.
		"llm_stat":      {PathFrom: 0},
		"llm_read":      {Values: []string{"--lines", "--around", "-C", "--context", "--max-bytes"}, PathFrom: 0, Also: readPathForms},
		"llm_log":       {Values: []string{"-n", "--last-match", "--grep", "--since", "--level"}, PathFrom: 0},
		"llm_hash":      {Values: []string{"--algo", "--short"}, PathFrom: 0},
		"llm_count":     {Values: []string{"--pat"}, PathFrom: 0},
		"llm_assert":    {Values: []string{"--has", "--lacks", "--count"}, PathFrom: 0},
		"llm_textstats": {PathFrom: 0},
		"llm_textcheck": {Values: []string{"--rules"}, PathFrom: 0},

		// Structured data (llm_dotenv: FILE then KEY names, not paths).
		"llm_json":   {Values: []string{"--path"}, PathFrom: 0},
		"llm_csv":    {Values: []string{"--delim", "--where"}, PathFrom: 0},
		"llm_dotenv": {PathFrom: 0, PathMax: 1},

		// Git (llm_git_log --path is a pathspec inside the repository).
		"llm_git_status":   {PathFrom: 0},
		"llm_git_log":      {Values: []string{"-n", "--limit", "--since", "--until", "--grep", "--author", "--path", "--fields"}, PathFrom: 0},
		"llm_git_overview": {Values: []string{"--log", "--files"}, PathFrom: 0},

		// Processes, services, containers, the machine.
		"llm_procs":       {Values: []string{"--name", "--port", "--pid"}, PathFrom: -1},
		"llm_ports":       {Values: []string{"--port"}, PathFrom: -1},
		"llm_service":     {Values: []string{"--log"}, PathFrom: -1},
		"llm_docker_ps":   {PathFrom: -1},
		"llm_docker_logs": {Values: []string{"--since", "--grep"}, PathFrom: -1},
		"llm_docker_exec": {Values: []string{"--user", "-u"}, PathFrom: -1},
		"llm_wait": {Values: []string{"--interval", "--exists", "--gone", "--http", "--code", "--json-path", "--equals", "--port", "--cmd-ok"},
			Pairs: []string{"--file-match"}, PathOpts: []string{"--exists", "--file-match"}, PathFrom: -1},
		"llm_sysinfo": {Values: []string{"--mount"}, PathOpts: []string{"--mount"}, PathFrom: -1},

		// Project and runtime.
		"llm_lint":    {Values: []string{"--ext", "--php", "--node"}, PathFrom: 0},
		"llm_tools":   {PathFrom: -1},
		"llm_project": {PathFrom: 0},
		"llm_deps":    {PathFrom: 0},
		"llm_env":     {Values: []string{"--filter", "-f"}, PathFrom: -1},
	}
}

// readPathForms is the PATH of an llm_read PATH:A-B word (also PATH:A:B,
// PATH:A-), so the rails check the file as well as the word.
func readPathForms(word string) []string {
	for _, i := range []int{lastIndexNth(word, ':', 2), strings.LastIndexByte(word, ':')} {
		if i <= 0 {
			continue
		}
		if _, ok := parseRange(word[i+1:]); ok {
			return []string{word[:i]}
		}
	}
	return nil
}

// hostable is h with --host ALIAS; name must have a hostSpecs entry.
func (m *Module) hostable(name string, h invoke.Handler) invoke.Handler {
	spec, ok := m.hostSpecs()[name]
	if !ok {
		panic("llm: no --host spec for " + name)
	}
	return remote.Hoster{Run: m.run, Exe: m.exe}.Wrap(name, spec, h)
}

// argError is a fallback plan that refuses the arguments as the local
// function refuses them.
func argError(code, details, hint string) (*remote.Plan, bool) {
	e := contract.New(code, details, hint)
	return &remote.Plan{Err: &e}, true
}

// fbPrelude are the sh helpers of every fallback: the C locale, newline
// and tab in variables, gash_err CODE KEY DETAILS (the error frame the
// Go side turns into the local envelope), gash_resolve PATH (~ and ~/ as
// the remote home, then realpath -m: GNU realpath when it is there, else
// cd -P and readlink -f), and the rails functions gash_forbidden and
// gash_secret.
func fbPrelude() string {
	return "LC_ALL=C; export LC_ALL\nNL='\n'\nTAB=$(printf '\\t')\n" + remote.ShErr() + remote.ShResolve()
}

// fbHeader is the start of a fallback body: the prelude, gash_tools TOOL...
// (the "missing TOOL" frame for a program the host lacks, which the
// transport turns into the prerequisite envelope: a fallback names every
// program it runs before it runs it, so a missing one is never an empty
// answer), the rails functions and the shell variables of vars (NAME,
// value), quoted.
func fbHeader(vars ...string) string {
	var b strings.Builder
	b.WriteString(fbPrelude())
	b.WriteString(remote.ShTools())
	b.WriteString(remote.ShForbidden())
	b.WriteString(remote.ShSecret())
	b.WriteString(remote.ShException())
	for i := 0; i+1 < len(vars); i += 2 {
		b.WriteString(vars[i] + "=" + remote.Quote(vars[i+1]) + "\n")
	}
	return b.String()
}

// fbHints are the hints of the error frames, as the local functions give
// them.
var fbHints = map[string]string{
	"protected":    "This path is protected and cannot be accessed",
	"subdir":       "Search a subdirectory",
	"exists":       "Provide an existing file or directory",
	"dir":          "Provide a valid directory path",
	"file":         "Check the file path and try again",
	"notreadable":  "The file is not readable by this user",
	"couldnot":     "The file could not be read",
	"searchsecret": "This file contains secrets and cannot be searched",
}

// fbFrame is one "<nonce> KIND REST" line of the fallback's stderr.
func fbFrame(stderr []byte, nonce, kind string) (string, bool) {
	return remote.Frame(stderr, nonce, kind)
}

// fbHint maps an error frame (code, key, details) to the envelope's
// details and hint; nil means fbHints by key.
type fbHint func(code, key, details string) (string, string)

// fbError turns an error frame into the local envelope, or returns false.
func fbError(stderr []byte, nonce string, hint fbHint) ([]byte, bool) {
	e, ok := fbEnvelope(stderr, nonce, hint)
	if !ok {
		return nil, false
	}
	var b bytes.Buffer
	_ = e.Write(&b)
	return b.Bytes(), true
}

// fbEnvelope is the envelope of an error frame, or false.
func fbEnvelope(stderr []byte, nonce string, hint fbHint) (contract.Envelope, bool) {
	code, key, details, ok := remote.ErrFrame(stderr, nonce)
	if !ok {
		return contract.Envelope{}, false
	}
	rest := code + " " + key + " " + details
	h := fbHints[key]
	if hint != nil {
		details, h = hint(code, key, details)
	}
	if _, known := contract.ActionOf(code); !known {
		code, details, h = "internal_error", "remote fallback: "+rest, ""
	}
	return contract.New(code, details, h), true
}

// fbFinish is the Finish of a fallback whose stdout is the local output:
// an error frame becomes the envelope (exit 1), anything else on stderr is
// the remote tools' own noise and is dropped.
func fbFinish(nonce string, hint fbHint) func(remote.Host, []byte, []byte, int) ([]byte, []byte, int) {
	return func(_ remote.Host, out, errOut []byte, st int) ([]byte, []byte, int) {
		if env, ok := fbError(errOut, nonce, hint); ok {
			return nil, env, 1
		}
		return out, nil, st
	}
}
