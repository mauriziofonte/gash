// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package migrate

import (
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/hte/legacy"
	"github.com/mauriziofonte/gash/internal/hte/render"
	"github.com/mauriziofonte/gash/internal/lazyre"
)

// rawResult is a raw vhost rewritten for the userland master.
type rawResult struct {
	conf     string
	php      string // PHP version of the socket it uses, "" when none
	certFile string // legacy certificate it named, "" when none
	keyFile  string
	ports    sitePorts // ports other than 80, 443 and the stack's
	notes    []string
}

// sitePorts are the custom ports of a legacy vhost file, kept as the
// site's extra ports (design/hte.md 10, LEDGER HTE-68).
type sitePorts struct {
	http, https []int
}

// customPorts sorts the ports of every <VirtualHost> of a file: 80 and
// the stack's HTTP port are the HTTP listeners, 443 and the stack's HTTPS
// port the HTTPS ones; any other port is an extra port of the protocol of
// its section (SSLEngine on: HTTPS). An address without a port, or a port
// used with both protocols, is an error.
func customPorts(conf *legacy.ConfFile, env Env) (sitePorts, error) {
	var sp sitePorts
	for _, vh := range conf.Vhosts {
		tls := vh.TLS()
		for i, p := range vh.Ports() {
			switch {
			case p == 0:
				return sp, fmt.Errorf("line %d: <VirtualHost %s> names no port", vh.Line, vh.Addrs[i])
			case p == 80 || p == 443 || p == env.Ports.HTTP || p == env.Ports.HTTPS:
				continue
			}
			own, other := &sp.http, sp.https
			if tls {
				own, other = &sp.https, sp.http
			}
			if slices.Contains(other, p) {
				return sp, fmt.Errorf("line %d: port %d serves both HTTP and HTTPS", vh.Line, p)
			}
			if !slices.Contains(*own, p) {
				*own = append(*own, p)
			}
		}
	}
	slices.Sort(sp.http)
	slices.Sort(sp.https)
	return sp, nil
}

var (
	vhostLineRE  = lazyre.MustCompile(`(?i)^([ \t]*)<VirtualHost[ \t]+([^>]*)>[ \t]*$`)
	sockVerRE    = lazyre.MustCompile(`php([0-9]+\.[0-9]+)-fpm`)
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
	var err error
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
	if res.ports, err = customPorts(v.Conf, env); err != nil {
		return res, err
	}
	if len(res.ports.https) > 0 && res.certFile == "" {
		return res, fmt.Errorf("an SSLEngine section on port %d names no certificate", res.ports.https[0])
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
				switch p := addrPortOf(a); {
				case p == 80 || p == env.Ports.HTTP:
					repl = httpAddrs
				case p == 443 || p == env.Ports.HTTPS:
					repl = httpsAddrs
				default:
					repl = addrs(env.Listen, p) // an extra port, from customPorts
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
