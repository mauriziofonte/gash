// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/registry"
)

// llm_dotenv (plan V3): whether keys are set in a dotenv file and the
// values of the ones that are not secret, without printing a secret into
// the agent's context. A key whose name llm_env leaves out (PASSWORD,
// TOKEN, KEY, SECRET, ...) is "***(set)", or "" when its value is empty;
// any other value goes through llm_env's redaction (URL passwords,
// private key blocks and known token formats masked); a missing key is
// null. The file is read as python-dotenv and docker compose read it,
// except that ${VAR} is never expanded (an expansion could copy a secret
// into a key that is not secret-named).

// dotenvMaxBytes is the largest dotenv file read.
const dotenvMaxBytes = 1 << 20

// dotenvSet is the value shown for a secret-named key that is set.
const dotenvSet = "***(set)"

// envFileName reports whether a base name is a dotenv file name: .env,
// .env.*, *.env or .envrc (the names llm_dotenv reads although the
// secret-file rule refuses them to every other reader).
func envFileName(name string) bool {
	name = strings.ToLower(name)
	return name == ".env" || name == ".envrc" || strings.HasPrefix(name, ".env.") || strings.HasSuffix(name, ".env")
}

// dotenvAllowed applies the secret-file rule with one exception: a dotenv
// file by name (and by its symlink target's name) outside the secret
// directories (.ssh, .aws, ...). A key file or a credentials file stays
// refused.
func (m *Module) dotenvAllowed(c *invoke.Call, file, safe string) bool {
	g := m.guard(c)
	if !g.IsSecretFile(file) {
		return true
	}
	for _, p := range []string{file, safe} {
		if !envFileName(filepath.Base(p)) || pathguard.SecretName(filepath.Join(filepath.Dir(p), "x")) {
			return false
		}
	}
	return true
}

// dotenv is llm_dotenv.
func (m *Module) dotenv(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	var p pathTaker
	var keys []string
	present := false
	for _, w := range c.Args {
		switch {
		case w == "--present":
			present = true
		case strings.HasPrefix(w, "-"):
			return fail(c, "unknown_option", w, registry.OptionsHint(c.Name))
		case !p.set:
			p.path, p.set = w, true
		default:
			if !dotenvKey(w) {
				return fail(c, "invalid_env_name", w, "A KEY is a variable name: letters, digits, _, . and -, not starting with a digit")
			}
			keys = append(keys, w)
		}
	}
	if !p.set {
		return fail(c, "missing_file", "", "Provide a dotenv file path, then the KEYs to look up")
	}
	safe, ok := m.validatePath(c, p.path)
	if !ok {
		return 1
	}
	if !m.dotenvAllowed(c, p.path, safe) {
		return fail(c, "secret_file_blocked", p.path, "llm_dotenv reads dotenv files (.env, .env.*, *.env, .envrc), not keys or credential files")
	}
	st, err := os.Stat(safe)
	if err != nil || !st.Mode().IsRegular() {
		return fail(c, "file_not_found", safe, "Check the file path and try again")
	}
	if st.Size() > dotenvMaxBytes {
		return fail(c, "file_too_large", safe+" is "+strconv.FormatInt(st.Size(), 10)+" bytes", "A dotenv file over 1 MiB is not read")
	}
	data, err := m.readFile(safe)
	if err != nil {
		return fail(c, "path_unreadable", safe, "The file could not be read")
	}
	names, vals := parseDotenv(string(data))
	b := []byte{}
	if len(keys) == 0 {
		// No KEY: the names the file sets, in file order; never a value.
		b = append(b, '[')
		for i, n := range names {
			if i > 0 {
				b = append(b, ',')
			}
			b = contract.AppendString(b, n, contract.EscapeJQ)
		}
		_, _ = c.Stdout.Write(append(b, ']', '\n'))
		return 0
	}
	b = append(b, '{')
	seen := map[string]bool{}
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		if len(b) > 1 {
			b = append(b, ',')
		}
		b = append(contract.AppendString(b, k, contract.EscapeJQ), ':')
		v, set := vals[k]
		switch {
		case present:
			b = strconv.AppendBool(b, set)
		case !set:
			b = append(b, "null"...)
		case envSecretRE.MatchString(strings.ToUpper(k)):
			if v == "" {
				b = append(b, `""`...)
			} else {
				b = contract.AppendString(b, dotenvSet, contract.EscapeJQ)
			}
		default:
			b = contract.AppendString(b, envRedact(v), contract.EscapeJQ)
		}
	}
	_, _ = c.Stdout.Write(append(b, '}', '\n'))
	return 0
}

// parseDotenv reads dotenv text: KEY=VALUE lines (an optional "export "
// before the key, blanks around "=" allowed), # comment lines, values
// unquoted (trimmed, a blank then # starts a comment), single-quoted
// (literal; \' and \\ escaped) or double-quoted (\n \r \t \" \\ \$
// escaped); a quoted value may span lines, and what follows its closing
// quote on that line is ignored. A line without "=" is skipped; a key set
// twice keeps its last value and its first place. ${VAR} stays as written.
func parseDotenv(text string) ([]string, map[string]string) {
	all := strings.ReplaceAll(strings.TrimPrefix(text, "\xef\xbb\xbf"), "\r\n", "\n")
	var names []string
	vals := map[string]string{}
	pos := 0
	for pos < len(all) {
		end := len(all)
		if i := strings.IndexByte(all[pos:], '\n'); i >= 0 {
			end = pos + i
		}
		line := all[pos:end]
		next := min(end+1, len(all))
		s := strings.TrimLeft(line, " \t")
		if t, ok := strings.CutPrefix(s, "export"); ok && t != "" && (t[0] == ' ' || t[0] == '\t') {
			s = strings.TrimLeft(t, " \t")
		}
		eq := strings.IndexByte(s, '=')
		key := ""
		if eq > 0 && s[0] != '#' {
			key = strings.TrimRight(s[:eq], " \t")
		}
		if !dotenvKey(key) {
			pos = next
			continue
		}
		v := strings.TrimLeft(s[eq+1:], " \t")
		var value string
		if v != "" && (v[0] == '"' || v[0] == '\'') {
			at := end - len(v) // v ends where the line ends
			if val, used, ok := dotenvQuoted(all[at:]); ok {
				value = val
				next = len(all)
				if i := strings.IndexByte(all[at+used:], '\n'); i >= 0 {
					next = at + used + i + 1
				}
			} else {
				value = strings.TrimSpace(v[1:]) // an unterminated quote: the rest of the line
			}
		} else {
			for i := 1; i < len(v); i++ {
				if v[i] == '#' && (v[i-1] == ' ' || v[i-1] == '\t') {
					v = v[:i]
					break
				}
			}
			value = strings.TrimRight(v, " \t")
		}
		if _, ok := vals[key]; !ok {
			names = append(names, key)
		}
		vals[key] = value
		pos = next
	}
	return names, vals
}

// dotenvKey reports whether s is a key python-dotenv takes: letters,
// digits (not first), _, . and -.
func dotenvKey(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || c == '.' || c == '-' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

// dotenvQuoted reads the quoted value at the start of s: its text, how
// many bytes of s it took (through the closing quote) and whether the
// quote closes.
func dotenvQuoted(s string) (string, int, bool) {
	q := s[0]
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case c == q:
			return b.String(), i + 1, true
		case c == '\\' && i+1 < len(s):
			n := s[i+1]
			switch {
			case q == '\'' && (n == '\'' || n == '\\'):
				b.WriteByte(n)
				i++
			case q == '"':
				switch n {
				case 'n':
					b.WriteByte('\n')
				case 'r':
					b.WriteByte('\r')
				case 't':
					b.WriteByte('\t')
				case '"', '\\', '$', '\'':
					b.WriteByte(n)
				default:
					b.WriteByte('\\')
					b.WriteByte(n)
				}
				i++
			default:
				b.WriteByte(c)
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", 0, false
}
