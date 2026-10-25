// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import "strings"

// The POSIX sh pieces every remote script of the agent toolkit shares
// (the --host fallbacks, llm_same, agent_put): the error frame, path
// resolution and the transfer tier on the resolved path. A script sets N
// to its nonce first; frames are "<nonce> KIND REST" lines on stderr.

// ShErr is gash_err CODE KEY DETAILS: the error frame, then exit 1 (the
// scripts run their checks in a subshell, so the trailer still follows).
func ShErr() string {
	return `gash_err() { printf '%s err %s %s %s\n' "$N" "$1" "$2" "$3" >&2; exit 1; }` + "\n"
}

// ShResolve is gash_resolve PATH: ~ and ~/ as the remote home, then
// realpath -m (GNU realpath where it is there, else cd -P and readlink -f
// on the longest existing prefix, the rest appended without . segments).
func ShResolve() string {
	return `gash_resolve() {
  case $1 in '~') set -- "$HOME" ;; '~/'*) set -- "$HOME/${1#'~/'}" ;; esac
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
}

// ShTier is gash_tier PATH: the transfer tier of a resolved absolute PATH
// as classifyPath reads it (deny:ENTRY, ask:ENTRY or allow), generated
// from the same tables, so a symlink on the host cannot carry a write
// into a tier its spelling avoided.
func ShTier() string {
	var b strings.Builder
	b.WriteString("gash_tier() {\n")
	b.WriteString("  [ \"$1\" = / ] && { echo deny:/; return; }\n")
	b.WriteString("  for gt_p in " + QuoteAll(denyPrefixes) + "; do\n")
	b.WriteString("    case $1 in \"$gt_p\"|\"$gt_p\"/*) echo \"deny:$gt_p\"; return ;; esac\n  done\n")
	b.WriteString("  gt_l=$(printf '/%s/' \"$1\" | tr ABCDEFGHIJKLMNOPQRSTUVWXYZ abcdefghijklmnopqrstuvwxyz)\n")
	for _, comp := range askComponents {
		b.WriteString("  case $gt_l in *" + Quote("/"+asciiLower(comp)+"/") + "*) echo " + Quote("ask:"+comp) + "; return ;; esac\n")
	}
	b.WriteString("  for gt_p in " + QuoteAll(deleteSafePrefixes) + "; do\n")
	b.WriteString("    case $1 in \"$gt_p\"|\"$gt_p\"/*) echo allow; return ;; esac\n  done\n")
	b.WriteString("  for gt_p in " + QuoteAll(askPrefixes) + "; do\n")
	b.WriteString("    case $1 in \"$gt_p\"|\"$gt_p\"/*) echo \"ask:$gt_p\"; return ;; esac\n  done\n")
	b.WriteString("  echo allow\n}\n")
	return b.String()
}

// Frame is the REST of the first "<nonce> KIND REST" line of stderr.
func Frame(stderr []byte, nonce, kind string) (string, bool) {
	prefix := nonce + " " + kind + " "
	for _, l := range strings.Split(string(stderr), "\n") {
		if strings.HasPrefix(l, prefix) {
			return l[len(prefix):], true
		}
	}
	return "", false
}

// ErrFrame is the CODE, KEY and DETAILS of a gash_err frame.
func ErrFrame(stderr []byte, nonce string) (code, key, details string, ok bool) {
	rest, ok := Frame(stderr, nonce, "err")
	if !ok {
		return "", "", "", false
	}
	parts := strings.SplitN(rest, " ", 3)
	for len(parts) < 3 {
		parts = append(parts, "")
	}
	return parts[0], parts[1], parts[2], true
}
