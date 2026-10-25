// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package config

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/mauriziofonte/gash/internal/lazyre"
)

// DBURL is a DB: URL split the way __gash_parse_db_url splits it.
type DBURL struct {
	Driver   string // text before the first "://" (the whole URL when there is none)
	User     string // as written: never decoded
	Password string // still percent-encoded: decode it with DecodePassword
	Host     string // an IPv6 host without its brackets
	Port     string // 3306 (mysql, mariadb) or 5432 (pgsql) when the host part names no port
	Database string // everything after the first "/" of the host part
}

// ParseDBURL ports __gash_parse_db_url rule for rule: the user part ends at
// the last "@", user and password are split at the first ":", host and
// database at the first "/", host and port at the first ":", and the
// default port applies only when the host part is non-empty and names no
// port. A host part that starts with "[" is a bracketed IPv6 host: the host
// is the text inside the brackets and the port follows "]:" (3.0: CORE-11
// fixed; 2.x split it at its first ":"). It never fails: CheckDBURL reports
// a bracket that is not closed, which is then split as 2.x split it.
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

	if host, port, ok := bracketHost(hostinfo); ok {
		u.Host, u.Port = host, port
		if !strings.HasPrefix(hostinfo[len(host)+2:], ":") {
			u.Port = defaultPort(u.Driver)
		}
		return u
	}
	if hostinfo != "" {
		if i := strings.IndexByte(hostinfo, ':'); i >= 0 {
			u.Host = hostinfo[:i]
			u.Port = hostinfo[i+1:]
		} else {
			u.Host = hostinfo
			u.Port = defaultPort(u.Driver)
		}
	}
	return u
}

// defaultPort is the port of a driver when the URL names none.
func defaultPort(driver string) string {
	switch driver {
	case "mysql", "mariadb":
		return "3306"
	case "pgsql":
		return "5432"
	}
	return ""
}

// bracketHost splits "[addr]" and "[addr]:port": ok is false unless
// hostinfo starts with "[" and its "]" ends it or is followed by ":".
func bracketHost(hostinfo string) (host, port string, ok bool) {
	if !strings.HasPrefix(hostinfo, "[") {
		return "", "", false
	}
	end := strings.IndexByte(hostinfo, ']')
	if end < 0 {
		return "", "", false
	}
	after := hostinfo[end+1:]
	if after != "" && after[0] != ':' {
		return "", "", false
	}
	return hostinfo[1:end], strings.TrimPrefix(after, ":"), true
}

// ErrNULPassword is CheckDBURL's error for a password holding "%00": no
// client can send a NUL byte (LEDGER CORE-13).
var ErrNULPassword = errors.New("The password holds %00 (a NUL byte), which no client can send: change the password")

// ErrIPv6Host is CheckDBURL's error for a host part that starts with "["
// without a matching "]" (or with text other than ":PORT" after it).
var ErrIPv6Host = errors.New("Write IPv6 hosts as [addr]:port")

// CheckDBURL reports a URL that cannot be used as written: a host part
// opening an IPv6 bracket that is not closed (LEDGER CORE-11), or a
// password that decodes to a NUL byte (LEDGER CORE-13). The callers fail
// with invalid_db_url and the error text as hint.
func CheckDBURL(url string) error {
	if strings.Contains(DecodePassword(ParseDBURL(url).Password), "\x00") {
		return ErrNULPassword
	}
	rest := url
	if i := strings.Index(url, "://"); i >= 0 {
		rest = url[i+3:]
	}
	if i := strings.LastIndexByte(rest, '@'); i >= 0 {
		rest = rest[i+1:]
	}
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	if strings.HasPrefix(rest, "[") {
		if _, _, ok := bracketHost(rest); !ok {
			return ErrIPv6Host
		}
	}
	return nil
}

// URLDecode is __gash_url_decode: every "%XX" with two hex digits becomes
// that byte, anything else is kept as written. "%00" decodes to nothing
// (a Bash variable cannot hold a NUL byte).
func URLDecode(s string) string { return urlDecode(s, false) }

// urlDecode decodes every "%XX" with two hex digits; keepNUL keeps the NUL
// byte of "%00".
func urlDecode(s string, keepNUL bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			hi, ok1 := unhex(s[i+1])
			lo, ok2 := unhex(s[i+2])
			if ok1 && ok2 {
				if v := hi<<4 | lo; v != 0 || keepNUL {
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

// DecodePassword decodes a DB: password into the value as a whole: trailing
// newlines ("%0A" at the end) are kept and "%00" is a NUL byte, which
// CheckDBURL refuses (3.0: CORE-13 fixed; 2.x captured the decoder with
// $(...), so the newlines were lost and "%00" vanished, and the client was
// given another password).
func DecodePassword(encoded string) string {
	return urlDecode(encoded, true)
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
var dbListMask = lazyre.MustCompile(`(?s)^([^:]+://[^:@]*:).+(@[^@]*)$`)

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
// DRIVER://USER@HOST:PORT/DB (no password; an IPv6 HOST in brackets).
func (u DBURL) Display() string {
	host := u.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return u.Driver + "://" + u.User + "@" + host + ":" + u.Port + "/" + u.Database
}
