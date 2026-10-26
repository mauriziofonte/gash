// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/pathguard"
)

// The remote path rails of the agent toolkit (plan S5): the local llm_*
// rails (no .. component, no ~user, the protected trees) plus the transfer
// tiers of this package, judged on the path as written before anything
// connects. The remote side applies them again on the resolved path: the
// shipped gash binary runs its own guard, and the POSIX fallbacks test the
// resolved path with ShForbidden.

// CheckRead is the rail of a path an llm_* --host call reads: the local
// rails and the transfer DENY tier (forbidden_path, FATAL). The ASK tier
// does not apply to reads, as it does not to local llm_* reads.
func CheckRead(p string) *contract.Envelope {
	norm, e := checkLexical(p)
	if e != nil || norm == "" {
		return e
	}
	if kind, detail := tier(norm); kind == "deny" {
		env := contract.New("forbidden_path", norm, "Remote path is in the transfer DENY list (matched: "+detail+")")
		return &env
	}
	return nil
}

// CheckWrite is the rail of a path an agent_* --host call writes: the
// local rails, the secret-file rule (secret_file_blocked, FATAL), the DENY
// tier (write_path_denied, FATAL, even with --force) and the ASK tier
// (/etc, /var, /usr/local, /root, any .ssh: sensitive_path, STOP, unless
// force), the codes agent_* gives a local target.
func CheckWrite(p string, force bool) *contract.Envelope {
	norm, e := checkLexical(p)
	if e != nil {
		return e
	}
	if norm == "" {
		env := contract.New("missing_file", "", "Name the remote file to write")
		return &env
	}
	if pathguard.SecretName(norm) {
		env := contract.New("secret_file_blocked", p, "agent_* functions never write secret files")
		return &env
	}
	kind, detail := tier(norm)
	switch {
	case kind == "deny":
		env := contract.New("write_path_denied", norm+" (matched: "+detail+")", "This path is never written, not even with --force")
		return &env
	case kind == "ask" && !force:
		env := contract.New("sensitive_path", norm+" (matched: "+detail+")", "Ask the user; with their agreement run again with --force")
		return &env
	}
	return nil
}

// checkLexical refuses what the local guard refuses on the path as
// written and returns it normalized (normalizePath: ~ and ~/ relative to
// the remote home, repeated slashes and . segments dropped); "" for an
// empty path.
func checkLexical(p string) (string, *contract.Envelope) {
	if p == "" {
		return "", nil
	}
	if strings.Contains("/"+p+"/", "/../") || p == ".." || strings.HasPrefix(p, "../") || strings.HasSuffix(p, "/..") {
		e := contract.New("path_traversal_blocked", "", "Path traversal (..) is not allowed for security reasons")
		return "", &e
	}
	if strings.HasPrefix(p, "~") && p != "~" && !strings.HasPrefix(p, "~/") {
		e := contract.New("invalid_path", p, "Only ~ and ~/ expand to your home; give another user's path in full")
		return "", &e
	}
	norm := normalizePath(p)
	if strings.HasPrefix(norm, "/") {
		for _, f := range pathguard.ForbiddenBasePaths {
			if under(norm, f) {
				e := contract.New("forbidden_path", norm, "This path is protected and cannot be accessed")
				return "", &e
			}
		}
	}
	return norm, nil
}

// tier is the transfer tier of a normalized path that no shell expands
// (the words are quoted): "allow", "deny" or "ask" and the matched entry.
func tier(norm string) (string, string) {
	kind, detail, _ := strings.Cut(ClassifyLocal(norm), ":")
	return kind, detail
}

// ShForbidden is a POSIX sh function, gash_forbidden PATH, that succeeds
// when the resolved absolute PATH is refused on the remote side: at or
// below a protected tree (the local rails, root's home included), at or
// below a DENY entry, or, with a second argument "walk", the root or a
// directory above a protected tree (a walk may not start there; "names"
// walks skip the /etc entries, as the local walkRoot does). Generated from
// the Go tables, so the two sides cannot drift.
func ShForbidden() string {
	var b strings.Builder
	b.WriteString("gash_forbidden() {\n")
	b.WriteString("  gf_rh=$(cd ~root 2>/dev/null && pwd -P) || gf_rh=/root\n")
	b.WriteString("  case $gf_rh in /root|/|'') gf_rh=/root ;; esac\n")
	b.WriteString("  case $1 in\n")
	var pats []string
	for _, p := range append(append([]string{}, pathguard.ForbiddenBasePaths...), denyPrefixes...) {
		pats = append(pats, shGlobQuote(p), shGlobQuote(p)+"/*")
	}
	b.WriteString("  " + strings.Join(pats, "|") + ") return 0 ;;\n")
	b.WriteString("  \"$gf_rh\"|\"$gf_rh\"/*) return 0 ;;\n")
	b.WriteString("  esac\n")
	b.WriteString("  [ \"$2\" = '' ] && return 1\n")
	b.WriteString("  [ \"$1\" = / ] && return 0\n")
	b.WriteString("  for gf_f in")
	for _, p := range pathguard.ForbiddenBasePaths {
		b.WriteString(" " + Quote(p))
	}
	b.WriteString(" \"$gf_rh\"; do\n")
	b.WriteString("    [ \"$2\" = names ] && case $gf_f in /etc/*) continue ;; esac\n")
	b.WriteString("    case $gf_f in \"$1\"/*) return 0 ;; esac\n")
	b.WriteString("  done\n")
	b.WriteString("  return 1\n}\n")
	return b.String()
}

// shGlobQuote quotes a literal path for a case pattern.
func shGlobQuote(p string) string { return Quote(p) }

// ShSecret is a POSIX sh function, gash_secret PATH, that succeeds when
// PATH is a secret file by name (pathguard.SecretName: the base name
// against the secret patterns, case-insensitively, the exceptions winning;
// or a secret directory as a component). Generated from the pathguard
// tables. The fallbacks test the resolved path with it, so no secret
// content ever leaves the host. The patterns ignore case themselves
// (icaseGlob): the check runs no program, so it cannot fail open on a host
// that lacks one (it used to lowercase the name with tr, and compared an
// empty string where tr was missing).
func ShSecret() string {
	var b strings.Builder
	b.WriteString("gash_secret() {\n")
	b.WriteString("  gs_p=${1%/}\n  gs_b=${gs_p##*/}\n")
	b.WriteString("  case $gs_b in " + icasePatterns(pathguard.SecretExceptions) + ") return 1 ;; esac\n")
	b.WriteString("  case $gs_b in " + icasePatterns(pathguard.SecretPatterns) + ") return 0 ;; esac\n")
	var dirs []string
	for _, d := range pathguard.SecretDirs {
		dirs = append(dirs, icaseGlob("*/"+d+"/*"))
	}
	b.WriteString("  case /$gs_p/ in " + strings.Join(dirs, "|") + ") return 0 ;; esac\n")
	b.WriteString("  return 1\n}\n")
	return b.String()
}

// icasePatterns is icaseGlob of each glob, as the alternatives of one
// case pattern.
func icasePatterns(globs []string) string {
	out := make([]string, len(globs))
	for i, g := range globs {
		out[i] = icaseGlob(g)
	}
	return strings.Join(out, "|")
}

// icaseGlob turns a glob made of literal text and * into an sh case
// pattern that ignores ASCII case: each letter a [xX] bracket, the other
// literal bytes quoted, the stars bare. ".env.*" is '.'[eE][nN][vV]'.'*.
func icaseGlob(glob string) string {
	var b strings.Builder
	lit := ""
	flush := func() {
		if lit != "" {
			b.WriteString(Quote(lit))
			lit = ""
		}
	}
	for i := 0; i < len(glob); i++ {
		c := glob[i]
		switch {
		case c == '*':
			flush()
			b.WriteByte('*')
		case 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z':
			flush()
			b.WriteByte('[')
			b.WriteByte(c | 0x20)
			b.WriteByte(c &^ 0x20)
			b.WriteByte(']')
		default:
			lit += string(c)
		}
	}
	flush()
	return b.String()
}

// ShException is a POSIX sh function, gash_exception NAME, that succeeds
// when the base name NAME is a secret exception (pathguard.SecretException:
// a template such as .env.example or a *.pub key, searched even when
// hidden). Like gash_secret it runs no program.
func ShException() string {
	return "gash_exception() {\n" +
		"  case $1 in " + icasePatterns(pathguard.SecretExceptions) + ") return 0 ;; esac\n" +
		"  return 1\n}\n"
}
