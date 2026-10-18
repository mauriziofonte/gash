// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package migrate

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/hte/legacy"
	"github.com/mauriziofonte/gash/internal/hte/render"
)

// rawResult is a raw vhost rewritten for the userland master.
type rawResult struct {
	conf     string
	php      string // PHP version of the socket it uses, "" when none
	certFile string // legacy certificate it named, "" when none
	keyFile  string
	notes    []string
}

var (
	vhostLineRE  = regexp.MustCompile(`(?i)^([ \t]*)<VirtualHost[ \t]+([^>]*)>[ \t]*$`)
	sockVerRE    = regexp.MustCompile(`php([0-9]+\.[0-9]+)-fpm`)
	sslPathLines = map[string]bool{"sslcertificatefile": true, "sslcertificatekeyfile": true}
)

// rewriteRaw rewrites an imported raw vhost (design/hte.md section 10):
// ${APACHE_LOG_DIR} and the system log directories become the site's logs
// directory, <VirtualHost> addresses become the loopback listeners, FPM
// sockets become the site's own socket, and the certificate lines name
// the site's userland pair. Everything else is kept verbatim; the config
// test of --apply validates the result. What cannot be rewritten safely
// is an error that keeps the vhost in the system.
func rewriteRaw(v legacy.Vhost, domain string, env Env) (rawResult, error) {
	var res rawResult
	logDir := env.Paths.SiteLogDir(domain)
	if strings.ContainsAny(logDir, " \t\"\\") {
		return res, fmt.Errorf("the logs directory %s holds characters an unquoted log path cannot carry", logDir)
	}
	httpAddrs := addrs(env.Listen, env.Ports.HTTP)
	httpsAddrs := addrs(env.Listen, env.Ports.HTTPS)
	for _, d := range v.Conf.Outside {
		if d.Is("Listen") {
			return res, fmt.Errorf("line %d: Listen in a vhost file; ports come from hte.toml", d.Line)
		}
	}
	certLines := map[int]string{} // line → directive name
	for _, vh := range v.Conf.Vhosts {
		for _, d := range vh.Directives {
			if d.Is("Listen") {
				return res, fmt.Errorf("line %d: Listen in a vhost file; ports come from hte.toml", d.Line)
			}
			if !sslPathLines[strings.ToLower(d.Name)] {
				continue
			}
			if len(d.Args) != 1 {
				return res, fmt.Errorf("line %d: %s takes one path", d.Line, d.Name)
			}
			certLines[d.Line] = d.Name
			src := &res.certFile
			if strings.EqualFold(d.Name, "SSLCertificateKeyFile") {
				src = &res.keyFile
			}
			if *src != "" && *src != d.Args[0] {
				return res, fmt.Errorf("line %d: the vhosts name different certificate files; one site has one pair", d.Line)
			}
			*src = d.Args[0]
		}
	}
	if (res.certFile == "") != (res.keyFile == "") {
		return res, fmt.Errorf("SSLCertificateFile and SSLCertificateKeyFile must come together")
	}
	for _, sock := range v.Sockets {
		m := sockVerRE.FindStringSubmatch(path.Base(sock))
		if m == nil {
			return res, fmt.Errorf("the PHP-FPM socket %s names no PHP version", sock)
		}
		if res.php != "" && res.php != m[1] {
			return res, fmt.Errorf("the vhost uses PHP %s and %s; one site has one pool", res.php, m[1])
		}
		res.php = m[1]
	}
	quotedCert, err := render.QuoteApacheArg(env.Paths.CertFile(domain))
	if err != nil {
		return res, err
	}
	quotedKey, err := render.QuoteApacheArg(env.Paths.KeyFile(domain))
	if err != nil {
		return res, err
	}
	sock := ""
	if res.php != "" {
		sock = render.SocketPath(env.Paths.RuntimeDir, res.php, domain)
	}
	lines := strings.Split(string(v.Data), "\n")
	for i, line := range lines {
		n := i + 1
		if name, ok := certLines[n]; ok {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			arg := quotedCert
			if strings.EqualFold(name, "SSLCertificateKeyFile") {
				arg = quotedKey
			}
			lines[i] = indent + name + " " + arg
			continue
		}
		if m := vhostLineRE.FindStringSubmatch(line); m != nil {
			var out []string
			for _, a := range strings.Fields(m[2]) {
				var repl []string
				switch addrPortOf(a) {
				case 80:
					repl = httpAddrs
				case 443:
					repl = httpsAddrs
				default:
					return res, fmt.Errorf("line %d: <VirtualHost %s> listens on a port other than 80 and 443", n, a)
				}
				for _, r := range repl {
					if !slices.Contains(out, r) {
						out = append(out, r)
					}
				}
			}
			lines[i] = m[1] + "<VirtualHost " + strings.Join(out, " ") + ">"
			continue
		}
		line = strings.ReplaceAll(line, "${APACHE_LOG_DIR}", logDir)
		for _, sys := range env.Layout.LogDirs {
			line = strings.ReplaceAll(line, sys+"/", logDir+"/")
		}
		for _, s := range v.Sockets {
			line = strings.ReplaceAll(line, "proxy:unix:"+s, "proxy:unix:"+sock)
		}
		lines[i] = line
	}
	res.conf = strings.Join(lines, "\n")
	if strings.Contains(res.conf, "${") {
		res.notes = append(res.notes, "the vhost still uses ${...} variables; the userland master has no envvars")
	}
	return res, nil
}

func addrs(listen []string, port int) []string {
	out := make([]string, 0, len(listen))
	for _, a := range listen {
		if strings.Contains(a, ":") {
			a = "[" + a + "]"
		}
		out = append(out, a+":"+strconv.Itoa(port))
	}
	return out
}

// addrPortOf is the port of a VirtualHost address, 0 when it has none.
func addrPortOf(a string) int {
	i := strings.LastIndex(a, ":")
	if i < 0 || strings.HasSuffix(a, "]") {
		return 0
	}
	n, err := strconv.Atoi(a[i+1:])
	if err != nil {
		return 0
	}
	return n
}
