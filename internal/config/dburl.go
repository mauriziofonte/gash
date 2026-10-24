// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package config

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// DBURL is a DB: URL split the way __gash_parse_db_url splits it.
type DBURL struct {
	Driver   string // text before the first "://" (the whole URL when there is none)
	User     string // as written: never decoded
	Password string // still percent-encoded: decode it with DecodePassword
	Host     string
	Port     string // 3306 (mysql, mariadb) or 5432 (pgsql) when the host has no ":"
	Database string // everything after the first "/" of the host part
}

// ParseDBURL ports __gash_parse_db_url rule for rule: the user part ends at
// the last "@", user and password are split at the first ":", host and
// database at the first "/", host and port at the first ":" (so a bracketed
// IPv6 host is split inside its brackets, LEDGER CORE-11 reproduced), and
// the default port applies only when the host part is non-empty and has no
// ":". It never fails.
func ParseDBURL(url string) DBURL {
	var u DBURL
	u.Driver = url
	rest := url
	if i := strings.Index(url, "://"); i >= 0 {
		u.Driver = url[:i]
		rest = url[i+3:]
	}

	userinfo := ""
	if i := strings.LastIndexByte(rest, '@'); i >= 0 {
		userinfo = rest[:i]
		rest = rest[i+1:]
	}

	hostinfo := rest
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		hostinfo = rest[:i]
		u.Database = rest[i+1:]
	}

	if userinfo != "" {
		if i := strings.IndexByte(userinfo, ':'); i >= 0 {
			u.User = userinfo[:i]
			u.Password = userinfo[i+1:]
		} else {
			u.User = userinfo
		}
	}

	if hostinfo != "" {
		if i := strings.IndexByte(hostinfo, ':'); i >= 0 {
			u.Host = hostinfo[:i]
			u.Port = hostinfo[i+1:]
		} else {
			u.Host = hostinfo
			switch u.Driver {
			case "mysql", "mariadb":
				u.Port = "3306"
			case "pgsql":
				u.Port = "5432"
			}
		}
	}
	return u
}

// URLDecode is __gash_url_decode: every "%XX" with two hex digits becomes
// that byte, anything else is kept as written. "%00" decodes to nothing
// (a Bash variable cannot hold a NUL byte).
func URLDecode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			hi, ok1 := unhex(s[i+1])
			lo, ok2 := unhex(s[i+2])
			if ok1 && ok2 {
				if v := hi<<4 | lo; v != 0 {
					b.WriteByte(v)
				}
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// DecodePassword is the password as the 2.x callers see it:
// pw=$(__gash_url_decode "$pw"), so trailing newlines ("%0A" at the end)
// are removed by the command substitution.
func DecodePassword(encoded string) string {
	return strings.TrimRight(URLDecode(encoded), "\n")
}

// URLEncode is __gash_url_encode: bytes outside [A-Za-z0-9._~-] become
// %XX with upper-case hex digits, one per byte.
func URLEncode(s string) string {
	const hexdigits = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '.' || c == '_' || c == '~' || c == '-' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hexdigits[c>>4])
		b.WriteByte(hexdigits[c&15])
	}
	return b.String()
}

func unhex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// dbListMask is the DB URL mask: the user name may be empty, so a
// password-only URL (mysql://:PW@h/d) is masked too (3.0: CORE-03 fixed; the
// 2.x regex required a non-empty user name and showed that password).
var dbListMask = regexp.MustCompile(`(?s)^([^:]+://[^:@]*:).+(@[^@]*)$`)

// MaskDBURL is the URL as Gash shows it: when the userinfo has a password
// (driver://USER:PASSWORD@HOST, USER possibly empty), the text between
// "USER:" and the last "@" becomes "****"; otherwise the URL is shown
// unchanged. Every place that shows a DB URL uses it.
func MaskDBURL(url string) string {
	m := dbListMask.FindStringSubmatch(url)
	if m == nil {
		return url
	}
	return m[1] + "****" + m[2]
}

// envURLMask is the llm_env redaction regex (__llm_env_redact), anchored at
// the start of the value, the password ending at the first "/" or "@"
// (LEDGER LLM-02, reproduced in rc.A).
var envURLMask = regexp.MustCompile(`(?s)^([A-Za-z][A-Za-z0-9+.-]*://[^/:@]*):[^/@]*@(.*)$`)

// MaskEnvURL is the URL step of llm_env's value redaction: scheme://user:
// followed by "***@" and the rest, applied once to the whole value. The
// token patterns that llm_env applies afterwards are not part of it.
func MaskEnvURL(value string) string {
	m := envURLMask.FindStringSubmatch(value)
	if m == nil {
		return value
	}
	return m[1] + ":***@" + m[2]
}

// MaskAIToken is how gash_ai_list shows a token: ${token:0:8}... (the first
// eight characters, then "...").
func MaskAIToken(token string) string {
	return firstChars(token, 8) + "..."
}

// firstChars is ${s:0:n} in a UTF-8 locale: the first n characters, an
// invalid byte counting as one character and kept as it is.
func firstChars(s string, n int) string {
	i := 0
	for ; n > 0 && i < len(s); n-- {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return s[:i]
}

// Display is the connection as gash_db_test announces it:
// DRIVER://USER@HOST:PORT/DB (no password).
func (u DBURL) Display() string {
	return u.Driver + "://" + u.User + "@" + u.Host + ":" + u.Port + "/" + u.Database
}
