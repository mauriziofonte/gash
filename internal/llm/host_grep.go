// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/remote"
)

// grepFallback is the POSIX sh llm_grep for a host without the gash
// binary: grep -E over the files the local engine would search (the walk's
// order, noise directories, hidden entries, secret files and --ext as the
// local rules read them, binary files skipped), with the local line
// format, context separators and limit. It answers only where that is the
// local answer: a pattern in the syntax ripgrep and POSIX ERE share (no
// \d \w \s \b, no (?flags), no lazy or counted repetition, no backslash in
// a bracket, ASCII only around metacharacters), no ignore file in the tree
// or above it, no global git excludes, no explicit binary FILE; elsewhere
// the remote side asks for the binary (remote_binary_unavailable). In the
// C locale "." and [...] match a byte, and a UTF-8 or UTF-16 BOM is
// searched as it is.
func grepFallback(_ *invoke.Call, nonce string, args []string) (*remote.Plan, bool) {
	pattern, extensions, context, limit := "", "", "0", "100"
	havePattern, havePath := false, false
	path := "."
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--ext", "--context", "-C", "-n", "--limit":
			if i+1 >= len(args) {
				return argError("missing_value", a, a+" needs a value")
			}
			switch a {
			case "--ext":
				extensions = args[i+1]
			case "--context", "-C":
				context = args[i+1]
			default:
				limit = args[i+1]
			}
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			return nil, false
		}
		switch {
		case !havePattern:
			pattern, havePattern = a, true
		case havePath:
			return argError("unexpected_argument", a, "Pass at most one PATH")
		default:
			path, havePath = a, true
		}
	}
	if pattern == "" {
		return argError("missing_pattern", "", "Provide a search pattern as first argument")
	}
	if !uintRE.MatchString(limit) {
		return argError("invalid_limit", "Value '"+limit+"' is not a non-negative integer below 1000000000", "Provide a plain integer for limit")
	}
	if !uintRE.MatchString(context) {
		return argError("invalid_context", "Value '"+context+"' is not a non-negative integer below 1000000000", "Provide a plain integer for context")
	}
	if _, err := compileRg(pattern); err != nil {
		return argError("invalid_regex", pattern, "Patterns use ripgrep's regex syntax: "+err.Error())
	}
	if !ereShared(pattern) {
		return nil, false
	}
	exts := splitExts(extensions)
	for _, e := range exts {
		if !plainExt.MatchString(e) {
			return nil, false
		}
	}
	ctx := ""
	if atoi(context) > 0 {
		ctx = "-C " + strconv.Itoa(atoi(context))
	}
	script := fbHeader("P", path, "PAT", pattern, "EXTS", strings.Join(exts, " "), "CTX", ctx, "L", limit) + `
gash_need() { printf '%s need %s\n' "$N" "$1" >&2; exit 0; }
abs=$(gash_resolve "$P") || exit 1
gash_forbidden "$abs" && gash_err forbidden_path protected "$abs"
[ -e "$abs" ] || gash_err path_not_found exists "$abs"
gash_secret "$abs" && gash_err secret_file_blocked searchsecret "$abs"
gash_forbidden "$abs" content && gash_err forbidden_path subdir "$abs"
[ "$L" -eq 0 ] && exit 0
if [ ! -d "$abs" ]; then
  gash_tools tr wc grep head
  [ "$(tr -d '\000' <"$abs" | wc -c)" -eq "$(wc -c <"$abs")" ] || gash_need "$abs is a binary file"
  grep -n $CTX -E -e "$PAT" -- "$abs" 2>/dev/null | head -n "$L"
  exit 0
fi
gash_tools find sort tr grep head
d=$abs
while :; do
  for f in .gitignore .ignore .rgignore .git; do [ -e "$d/$f" ] || [ -L "$d/$f" ] && gash_need "an ignore file: $d/$f"; done
  [ "$d" = / ] && break
  d=${d%/*}; [ -n "$d" ] || d=/
done
x=$(find "$abs" \( -name node_modules -o -name vendor \) -prune -o \( -name .gitignore -o -name .ignore -o -name .rgignore -o -name .git \) -print 2>/dev/null | head -n 1)
[ -n "$x" ] && gash_need "an ignore file: $x"
for g in "${XDG_CONFIG_HOME:-$HOME/.config}/git/ignore" "$HOME/.config/git/ignore"; do
  [ -e "$g" ] && gash_need "global git excludes: $g"
done
grep -qi '^[[:space:]]*excludesfile' "$HOME/.gitconfig" 2>/dev/null && gash_need "global git excludes (core.excludesFile)"
first=1 skip=
find "$abs" -mindepth 1 \( -name node_modules -o -name vendor -o -name .git -o -name "*$NL*" \) -prune -o -print 2>/dev/null |
  tr / '\001' | sort | tr '\001' / |
  while IFS= read -r p; do
    if [ -n "$skip" ]; then
      case $p in "$skip"/*) continue ;; esac
      skip=
    fi
    b=${p##*/}
    if [ -d "$p" ] && [ ! -L "$p" ]; then
      case $b in .*) skip=$p; continue ;; esac
      gash_secret "$b" && skip=$p
      continue
    fi
    { [ -f "$p" ] && [ ! -L "$p" ]; } || continue
    if [ -n "$EXTS" ]; then
      m=0
      for e in $EXTS; do case $b in *."$e") m=1 ;; esac; done
      [ "$m" = 1 ] || continue
    else
      case $b in .*) gash_exception "$b" || continue ;; esac
    fi
    gash_secret "$p" && continue
    o=$(grep -I -H -n $CTX -E -e "$PAT" -- "$p" 2>/dev/null) || continue
    [ -n "$CTX" ] && [ "$first" = 0 ] && printf '%s\n' --
    printf '%s\n' "$o"
    first=0
  done | head -n "$L"
exit 0`
	finish := func(h remote.Host, out, errOut []byte, st int) ([]byte, []byte, int) {
		if env, ok := fbError(errOut, nonce, nil); ok {
			return nil, env, 1
		}
		if why, ok := fbFrame(errOut, nonce, "need"); ok {
			var b bytes.Buffer
			_ = contract.New("remote_binary_unavailable", "llm_grep on "+h.Alias+": the POSIX fallback cannot give the local answer here ("+why+")",
				"Install gash on the host, or search a tree without ignore files with a simpler pattern").Write(&b)
			return nil, b.Bytes(), 1
		}
		return out, nil, st
	}
	return &remote.Plan{Script: script, Finish: finish}, true
}

var plainExt = lazyre.MustCompile(`^[A-Za-z0-9_+-][A-Za-z0-9_.+-]*$`)

// ereShared reports whether re means the same in ripgrep's syntax and in
// POSIX ERE (grep -E, C locale): literal ASCII, the operators . [ ] * + ?
// ^ $ | ( ), and a backslash before punctuation outside brackets. Counted
// repetition, flags, lazy quantifiers, escape classes, nested classes and
// class set operations, and a backslash inside brackets differ between the
// two and are refused, as is a non-ASCII byte in a pattern that has any
// metacharacter.
func ereShared(re string) bool {
	meta := strings.ContainsAny(re, `.[]*+?^$|()\{}`)
	inBracket := false
	for i := 0; i < len(re); i++ {
		c := re[i]
		next := byte(0)
		if i+1 < len(re) {
			next = re[i+1]
		}
		switch {
		case c >= 0x80:
			if meta {
				return false
			}
		case inBracket:
			switch {
			case c == '\\':
				return false
			case c == '[' && next == ':':
				j := strings.Index(re[i:], ":]")
				if j < 0 {
					return false
				}
				i += j + 1
			case c == '[':
				return false
			case c == ']':
				inBracket = false
			case (c == '&' || c == '-' || c == '~') && next == c:
				return false
			}
		case c == '{' || c == '}':
			return false
		case c == '\\':
			if next == 0 || next >= 0x80 || next == ' ' || ('a' <= next && next <= 'z') || ('A' <= next && next <= 'Z') || ('0' <= next && next <= '9') {
				return false
			}
			i++
		case c == '[':
			inBracket = true
			// A leading ^ and a ] right after it (or after [) are literal.
			if next == '^' {
				i++
				if i+1 < len(re) && re[i+1] == ']' {
					i++
				}
			} else if next == ']' {
				i++
			}
		case c == '(':
			if next == '?' {
				return false
			}
		case c == '*' || c == '+' || c == '?':
			if next == '?' || next == '+' {
				return false
			}
		}
	}
	return !inBracket
}
