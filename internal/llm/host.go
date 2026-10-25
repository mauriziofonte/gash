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

// hostSpecs are the functions that take --host.
func (m *Module) hostSpecs() map[string]remote.Spec {
	return map[string]remote.Spec{
		"llm_find":   {Values: []string{"--type", "--contains", "-n", "--limit"}, PathFrom: 1, Fallback: findFallback},
		"llm_grep":   {Values: []string{"--ext", "--context", "-C", "-n", "--limit"}, PathFrom: 1, Fallback: grepFallback},
		"llm_tree":   {Values: []string{"--depth", "--max-depth"}, PathFrom: 0, Fallback: treeFallback},
		"llm_config": {Values: []string{"--max-bytes"}, PathFrom: 0, Keep: true, Fallback: configFallback},
	}
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
// Go side turns into the local envelope), gash_resolve PATH (realpath -m:
// GNU realpath when it is there, else cd -P and readlink -f), and the
// rails functions gash_forbidden and gash_secret.
const fbPrelude = `LC_ALL=C; export LC_ALL
NL='
'
TAB=$(printf '\t')
gash_err() { printf '%s err %s %s %s\n' "$N" "$1" "$2" "$3" >&2; exit 1; }
gash_resolve() {
  if gr_r=$(realpath -m -- "$1" 2>/dev/null) && [ -n "$gr_r" ]; then printf '%s\n' "$gr_r"; return 0; fi
  case $1 in /*) gr_p=$1 ;; *) gr_p=$(pwd -P)/$1 ;; esac
  gr_t=
  while [ ! -e "$gr_p" ] && [ ! -L "$gr_p" ]; do
    gr_t=/${gr_p##*/}$gr_t
    gr_p=${gr_p%/*}
    [ -n "$gr_p" ] || { gr_p=/; break; }
  done
  if [ -d "$gr_p" ]; then
    gr_p=$(cd -P -- "$gr_p" 2>/dev/null && pwd -P)
  elif gr_r=$(readlink -f -- "$gr_p" 2>/dev/null) && [ -n "$gr_r" ]; then
    gr_p=$gr_r
  else
    gr_d=$(cd -P -- "${gr_p%/*}/" 2>/dev/null && pwd -P)
    gr_p=${gr_d%/}/${gr_p##*/}
  fi
  gr_o=${gr_p%/}
  gr_i=$IFS; IFS=/; set -f
  for gr_s in $gr_t; do
    case $gr_s in ''|.) ;; *) gr_o=$gr_o/$gr_s ;; esac
  done
  set +f; IFS=$gr_i
  printf '%s\n' "${gr_o:-/}"
}
`

// fbHeader is the start of a fallback body: the prelude, the rails
// functions and the shell variables of vars (NAME, value), quoted.
func fbHeader(vars ...string) string {
	var b strings.Builder
	b.WriteString(fbPrelude)
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
	prefix := nonce + " " + kind + " "
	for _, l := range strings.Split(string(stderr), "\n") {
		if strings.HasPrefix(l, prefix) {
			return l[len(prefix):], true
		}
	}
	return "", false
}

// fbHint maps an error frame (code, key, details) to the envelope's
// details and hint; nil means fbHints by key.
type fbHint func(code, key, details string) (string, string)

// fbError turns an error frame into the local envelope, or returns false.
func fbError(stderr []byte, nonce string, hint fbHint) ([]byte, bool) {
	rest, ok := fbFrame(stderr, nonce, "err")
	if !ok {
		return nil, false
	}
	parts := strings.SplitN(rest, " ", 3)
	for len(parts) < 3 {
		parts = append(parts, "")
	}
	code, key, details := parts[0], parts[1], parts[2]
	h := fbHints[key]
	if hint != nil {
		details, h = hint(code, key, details)
	}
	if _, known := contract.ActionOf(code); !known {
		code, details, h = "internal_error", "remote fallback: "+rest, ""
	}
	var b bytes.Buffer
	_ = contract.New(code, details, h).Write(&b)
	return b.Bytes(), true
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
