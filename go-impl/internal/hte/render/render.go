// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package render turns the declarative state into one generation of
// Apache and PHP-FPM configuration (design/hte.md sections 2 to 4). Render
// is a pure function of its Input: the same state renders byte-identical
// files, and nothing is read from disk. The generation layout on disk
// (gen-<n>, the current and previous symlinks) is in layout.go.
package render

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/profiles"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

var templates = template.Must(template.New("").Funcs(template.FuncMap{
	"q":    QuoteApacheArg,
	"iq":   quoteINI,
	"join": strings.Join,
}).ParseFS(templateFS, "templates/*.tmpl"))

// BaseModules is the userland module list of design/hte.md section 3 in
// load order (dependencies first). log_config, logio and unixd are built
// into Debian's apache2 and loadable modules elsewhere; ResolveModules in
// internal/hte/discover drops the built-in ones.
var BaseModules = []string{
	"mpm_event", "unixd", "log_config", "logio",
	"authn_core", "authn_file", "authz_core", "authz_host", "authz_user", "auth_basic", "access_compat",
	"alias", "dir", "env", "filter", "deflate", "brotli", "headers", "mime", "negotiation",
	"reqtimeout", "setenvif", "rewrite", "proxy", "proxy_fcgi", "proxy_http", "socache_shmcb",
	"ssl", "http2",
}

// MaxSocketPath is the longest Unix socket path that fits sun_path on
// every platform (104 bytes on macOS, including the NUL).
const MaxSocketPath = 103

// Module is one LoadModule line.
type Module struct {
	Name string // "ssl": LoadModule ssl_module
	Path string // absolute path of the .so
}

// Apache are the facts of the platform's Apache.
type Apache struct {
	Modules   []Module // LoadModule lines, built-in modules already left out
	MimeTypes string   // the platform's mime.types
}

// INIValue is one setting of a PHP overlay: the ini name and the value
// text exactly as written in the file (quotes included), so that it means
// in the pool what it means in php.ini.
type INIValue struct {
	Key   string
	Value string
}

// PHPVersion are the facts of one PHP version in use.
type PHPVersion struct {
	// SessionDir is set when the platform's session.save_path is not
	// writable by the user (LEDGER HTE-41).
	SessionDir string
	// Overlay is the parsed global overlay php/<X.Y>.ini, in file order.
	Overlay []INIValue
}

// Input is everything a generation is rendered from.
type Input struct {
	GenDir string      // absolute directory of this generation: <render>/gen-<n>
	Paths  state.Paths // the userland layout
	Apache Apache
	Ports  state.Ports
	// Listen are the loopback addresses in use: 127.0.0.1, plus ::1 where
	// IPv6 loopback exists.
	Listen []string
	// RedirectHTTPSPort is the HTTPS port clients reach (443 for standard
	// ports and the macOS pf redirect); 0 means Ports.HTTPS.
	RedirectHTTPSPort int
	// PHP holds the facts of every installed version, keyed by "X.Y".
	PHP   map[string]PHPVersion
	Sites []state.Site
}

// File is one rendered file, Path relative to the generation directory.
type File struct {
	Path string
	Data []byte
}

// Generation is the set of rendered files, sorted by path.
type Generation struct {
	Files []File
}

// File returns the content of the file at rel, or nil.
func (g *Generation) File(rel string) []byte {
	for _, f := range g.Files {
		if f.Path == rel {
			return f.Data
		}
	}
	return nil
}

// Versions returns the PHP versions that get an FPM master.
func (g *Generation) Versions() []string {
	var out []string
	for _, f := range g.Files {
		if v, ok := strings.CutSuffix(f.Path, "/php-fpm.conf"); ok {
			out = append(out, strings.TrimPrefix(v, "fpm/"))
		}
	}
	return out
}

// QuoteApacheArg quotes one argument of an Apache directive: double quotes
// around it, with " and \ escaped by a backslash (ap_getword_conf). A
// control character, newline included, cannot be quoted and is an error:
// validation refuses it before rendering, so this is the last barrier
// against injected directives.
func QuoteApacheArg(s string) (string, error) {
	if !utf8.ValidString(s) || strings.ContainsFunc(s, unicode.IsControl) {
		return "", fmt.Errorf("render: %q cannot be written into an Apache configuration", s)
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`, nil
}

// quoteINI double-quotes a path value of a PHP-FPM file. The FPM INI
// parser has no escapes inside double quotes and expands ${NAME}, so ", \
// and $ are refused along with control characters.
func quoteINI(s string) (string, error) {
	if !utf8.ValidString(s) || strings.ContainsFunc(s, unicode.IsControl) || strings.ContainsAny(s, "\"\\$") {
		return "", fmt.Errorf("render: %q cannot be written into a PHP-FPM configuration", s)
	}
	return `"` + s + `"`, nil
}

// unsafePathChars are refused in the base directories (home, XDG and
// generation paths) that end up in configuration files: quoting and
// expansion (" \ $), Include globs (* ? [), and FPM comments (;).
const unsafePathChars = "\"\\$*?[;"

func checkBasePath(what, p string) error {
	if p == "" || !filepath.IsAbs(p) {
		return hteerr.Newf(hteerr.PlatformUnsupported, "", "the %s %q is not an absolute path", what, p)
	}
	if !utf8.ValidString(p) || strings.ContainsFunc(p, unicode.IsControl) || strings.ContainsAny(p, unsafePathChars) {
		return hteerr.Newf(hteerr.PlatformUnsupported,
			"use a home and XDG directories without the characters \" \\ $ * ? [ ; and control characters",
			"the %s %q contains characters Apache or PHP-FPM would interpret", what, p)
	}
	return nil
}

// SocketPath is the FPM socket of a site: <run>/<X.Y>-<10 hex of
// sha256(domain)>.sock, short enough for sun_path (LEDGER HTE-35).
func SocketPath(runDir, version, domain string) string {
	sum := sha256.Sum256([]byte(domain))
	return filepath.Join(runDir, version+"-"+hex.EncodeToString(sum[:])[:10]+".sock")
}

// Rel paths of the generation layout.
func siteConfRel(domain string) string      { return "sites/" + domain + ".conf" }
func fpmConfRel(version string) string      { return "fpm/" + version + "/php-fpm.conf" }
func poolRel(version, domain string) string { return "fpm/" + version + "/pools/" + domain + ".conf" }
func (in Input) abs(rel string) string      { return filepath.Join(in.GenDir, rel) }
func (in Input) redirectPort() int          { return cmp.Or(in.RedirectHTTPSPort, in.Ports.HTTPS) }
func addrList(addrs []string, port int) []string {
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if strings.Contains(a, ":") {
			a = "[" + a + "]"
		}
		out = append(out, a+":"+strconv.Itoa(port))
	}
	return out
}

func (in Input) validate() error {
	for _, p := range []struct{ what, path string }{
		{"generation directory", in.GenDir}, {"configuration directory", in.Paths.ConfigDir},
		{"state directory", in.Paths.StateDir}, {"data directory", in.Paths.DataDir},
		{"runtime directory", in.Paths.RuntimeDir}, {"mime.types file", in.Apache.MimeTypes},
	} {
		if err := checkBasePath(p.what, p.path); err != nil {
			return err
		}
	}
	for _, m := range in.Apache.Modules {
		if err := checkBasePath("module", m.Path); err != nil {
			return err
		}
	}
	settings := state.Settings{Ports: in.Ports, Listen: in.Listen, Autostart: "none"}
	if err := settings.Validate(); err != nil {
		return err
	}
	if p := in.redirectPort(); p < 1 || p > 65535 {
		return hteerr.Newf(hteerr.InvalidState, "", "redirect port %d is not a TCP port", p)
	}
	for i, s := range in.Sites {
		if err := s.Validate(); err != nil {
			return err
		}
		if err := state.CheckUnique(s, in.Sites[i+1:]); err != nil {
			return err
		}
		if !s.Enabled || !s.HasPool() {
			continue
		}
		if _, ok := in.PHP[s.PHP]; !ok {
			return hteerr.Newf(hteerr.PHPVersionNotInstalled, "install it or change the site with gash hte site modify --php",
				"site %s uses PHP %s, which is not installed", s.Domain, s.PHP)
		}
		if sock := SocketPath(in.Paths.RuntimeDir, s.PHP, s.Domain); len(sock) > MaxSocketPath {
			return hteerr.Newf(hteerr.PlatformUnsupported, "set XDG_RUNTIME_DIR to a shorter directory",
				"the FPM socket path %s is longer than %d bytes", sock, MaxSocketPath)
		}
	}
	for v, facts := range in.PHP {
		if facts.SessionDir != "" {
			if err := checkBasePath("PHP "+v+" session directory", facts.SessionDir); err != nil {
				return err
			}
		}
		for _, kv := range facts.Overlay {
			if err := state.CheckINI(kv.Key, kv.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

// Render produces one generation from in.
func Render(in Input) (*Generation, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	sites := slices.Clone(in.Sites)
	slices.SortFunc(sites, func(a, b state.Site) int { return strings.Compare(a.Domain, b.Domain) })

	var files []File
	var siteConfs []string
	pools := map[string][]string{} // version → absolute pool paths
	for _, s := range sites {
		if !s.Enabled {
			continue
		}
		vhost, err := in.vhost(s)
		if err != nil {
			return nil, err
		}
		files = append(files, File{siteConfRel(s.Domain), vhost})
		siteConfs = append(siteConfs, in.abs(siteConfRel(s.Domain)))
		if !s.HasPool() {
			continue
		}
		pool, err := in.pool(s)
		if err != nil {
			return nil, err
		}
		files = append(files, File{poolRel(s.PHP, s.Domain), pool})
		pools[s.PHP] = append(pools[s.PHP], in.abs(poolRel(s.PHP, s.Domain)))
	}
	for _, v := range slices.Sorted(maps.Keys(pools)) {
		conf, err := execute("php-fpm.conf.tmpl", map[string]any{
			"Version":  v,
			"PidFile":  filepath.Join(in.Paths.RuntimeDir, "php-fpm-"+v+".pid"),
			"ErrorLog": filepath.Join(in.Paths.LogsDir(), "php-fpm-"+v+".log"),
			"Pools":    pools[v],
		})
		if err != nil {
			return nil, err
		}
		files = append(files, File{fpmConfRel(v), conf})
	}
	var listen []string
	listen = append(listen, addrList(in.Listen, in.Ports.HTTP)...)
	listen = append(listen, addrList(in.Listen, in.Ports.HTTPS)...)
	httpd, err := execute("httpd.conf.tmpl", map[string]any{
		"RunDir":       in.Paths.RuntimeDir,
		"PidFile":      filepath.Join(in.Paths.RuntimeDir, "httpd.pid"),
		"MutexArg":     "file:" + in.Paths.RuntimeDir,
		"ErrorLog":     filepath.Join(in.Paths.LogsDir(), "error.log"),
		"Modules":      in.Apache.Modules,
		"Listen":       listen,
		"MimeTypes":    in.Apache.MimeTypes,
		"SessionCache": "shmcb:" + filepath.Join(in.Paths.RuntimeDir, "ssl_scache") + "(512000)",
		"Overlay":      in.Paths.ApacheOverlay(),
		"SiteConfs":    siteConfs,
	})
	if err != nil {
		return nil, err
	}
	files = append(files, File{"httpd.conf", httpd})
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return &Generation{Files: files}, nil
}

func execute(name string, data any) ([]byte, error) {
	var b bytes.Buffer
	if err := templates.ExecuteTemplate(&b, name, data); err != nil {
		return nil, hteerr.Newf(hteerr.InvalidState, "", "rendering %s: %v", name, err)
	}
	return b.Bytes(), nil
}

// vhost renders sites/<domain>.conf: the raw configuration verbatim, or
// the managed HTTP vhost and, with HTTPS, the HTTPS vhost.
func (in Input) vhost(s state.Site) ([]byte, error) {
	if s.Kind == state.KindRaw {
		conf := s.RawConf
		if !strings.HasSuffix(conf, "\n") {
			conf += "\n"
		}
		return []byte(conf), nil
	}
	logs := in.Paths.SiteLogDir(s.Domain)
	dir := s.Docroot + "/"
	if s.Docroot == "/" {
		dir = "/"
	}
	httpProto, httpsProto := "http/1.1", "http/1.1"
	if s.HTTP2 {
		httpProto, httpsProto = "h2c http/1.1", "h2 http/1.1"
	}
	target := "https://%{HTTP_HOST}%{REQUEST_URI}"
	if p := in.redirectPort(); p != 443 {
		target = "https://%{SERVER_NAME}:" + strconv.Itoa(p) + "%{REQUEST_URI}"
	}
	handler := ""
	if s.HasPool() {
		handler = "proxy:unix:" + SocketPath(in.Paths.RuntimeDir, s.PHP, s.Domain) + "|fcgi://localhost/"
	}
	return execute("vhost.conf.tmpl", map[string]any{
		"Domain":         s.Domain,
		"Aliases":        s.Aliases,
		"Docroot":        s.Docroot,
		"DirectoryPath":  dir,
		"HTTPAddrs":      strings.Join(addrList(in.Listen, in.Ports.HTTP), " "),
		"HTTPSAddrs":     strings.Join(addrList(in.Listen, in.Ports.HTTPS), " "),
		"ErrorLog":       filepath.Join(logs, "error.log"),
		"AccessLog":      filepath.Join(logs, "access.log"),
		"BytesLog":       filepath.Join(logs, "bytes.log"),
		"SSLAccessLog":   filepath.Join(logs, "ssl-access.log"),
		"SSLBytesLog":    filepath.Join(logs, "ssl-bytes.log"),
		"HTTPProtocols":  httpProto,
		"HTTPSProtocols": httpsProto,
		"Handler":        handler,
		"ForceHTTPS":     s.HTTPS && s.ForceHTTPS,
		"Bypass":         s.ForceHTTPSBypassLoopback,
		"RedirectTarget": target,
		"HTTPS":          s.HTTPS,
		"CertFile":       in.Paths.CertFile(s.Domain),
		"KeyFile":        in.Paths.KeyFile(s.Domain),
		"Overlay":        in.Paths.SiteOverlay(s.Domain),
	})
}

// pool renders fpm/<X.Y>/pools/<domain>.conf: gash's header, the profile
// block of hte-cli with the log paths moved to the logs directory, the
// session directory where needed, the global overlay of the version, then
// the site's php_ini.
func (in Input) pool(s state.Site) ([]byte, error) {
	facts := in.PHP[s.PHP]
	logs := in.Paths.SiteLogDir(s.Domain)
	sock, err := quoteINI(SocketPath(in.Paths.RuntimeDir, s.PHP, s.Domain))
	if err != nil {
		return nil, err
	}
	opts := profiles.Options{
		Docroot:  s.Docroot,
		SlowLog:  filepath.Join(logs, "php-slow.log"),
		ErrorLog: filepath.Join(logs, "php-error.log"),
	}
	if facts.SessionDir != "" {
		opts.OpenBasedirExtra = []string{facts.SessionDir}
	}
	lines := []string{
		"[" + s.Domain + "]",
		"; Managed by gash hte. Profile: " + s.Profile,
		"listen = " + sock,
		"listen.mode = 0660",
		"security.limit_extensions = .php",
		"",
	}
	for _, l := range profiles.Lines(profiles.Name(s.Profile), opts) {
		switch {
		case l.IsBlank():
			lines = append(lines, "")
		case l.Comment != "":
			lines = append(lines, "; "+l.Comment)
		case l.Path:
			v, err := quoteINI(l.Value)
			if err != nil {
				return nil, err
			}
			lines = append(lines, l.Key+" = "+v)
		default:
			lines = append(lines, l.Key+" = "+l.Value)
		}
	}
	if facts.SessionDir != "" {
		v, err := quoteINI(facts.SessionDir)
		if err != nil {
			return nil, err
		}
		lines = append(lines, "", "; Sessions: the platform's session.save_path is not writable by the user",
			"php_admin_value[session.save_path] = "+v)
	}
	if len(facts.Overlay) > 0 {
		lines = append(lines, "", "; Global overlay "+in.Paths.PHPOverlay(s.PHP))
		for _, kv := range facts.Overlay {
			lines = append(lines, iniDirective(kv.Key, kv.Value))
		}
	}
	if len(s.PHPIni) > 0 {
		lines = append(lines, "", "; php_ini of "+in.Paths.SiteFile(s.Domain))
		for _, k := range slices.Sorted(maps.Keys(s.PHPIni)) {
			lines = append(lines, iniDirective(k, s.PHPIni[k]))
		}
	}
	return execute("pool.conf.tmpl", lines)
}

// iniDirective writes an override with the directive choice of
// PhpProfiles.php:233-246 (php_admin_flag for on/off, php_admin_value
// otherwise) and the value text verbatim.
func iniDirective(key, value string) string {
	d, v := profiles.AdminDirective(key, value)
	return d + " = " + v
}
