// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"io"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

const checkHelp = `docker_compose_check - Check for Docker Compose image updates

USAGE:
  docker_compose_check [PATH] [--json] [--no-color]

OPTIONS:
  PATH        Directory containing docker-compose.yml (default: current dir)
  --json      Output in JSON format for scripting
  --no-color  Disable ANSI colors (auto-off on pipe / NO_COLOR / GASH_HEADLESS)
  -h          Show this help

OUTPUT:
  Shows each service with current local digest, remote digest, and update status.
  Services with pinned versions show "PINNED" and won't be upgraded.

EXAMPLES:
  docker_compose_check                    Check current directory
  docker_compose_check /path/to/project   Check specific project
  docker_compose_check --json             JSON output for scripting
`

// result is one service of docker_compose_check, kept whole (LEDGER
// DOCK-08: 2.x joined the fields with '|' and split them again).
type result struct {
	name, image, local, remote, status, err string
	upgradeable                             bool
}

// isHelp is the -h/--help test of the compose functions: the first
// argument only.
func isHelp(c *invoke.Call) bool { a := c.Arg(0); return a == "-h" || a == "--help" }

func q(s string) string { return contract.Quote(s, contract.EscapeBash) }

// check is docker_compose_check [PATH] [--json] [--no-color]: per
// service the local digest (docker inspect) against the registry's, as a
// table or as the --json object; --json errors are envelopes on stderr.
// Its name and --json contract are what llm_docker_check calls.
func (m *Module) check(c *invoke.Call) int {
	if isHelp(c) {
		_, _ = io.WriteString(c.Stdout, checkHelp)
		return 0
	}
	x := m.begin(c)
	path, jsonOut, noColor := ".", false, false
	joined := " " + strings.Join(c.Args, " ") + " "
	for _, a := range c.Args {
		switch {
		case a == "--json" || a == "-j":
			jsonOut = true
		case a == "--no-color":
			noColor = true
		case strings.HasPrefix(a, "-"):
			if strings.Contains(joined, " --json ") || strings.Contains(joined, " -j ") {
				return c.Fail(contract.New("unknown_option", a, "Use a path and --json"))
			}
			x.p.Error("Unknown option: " + a)
			return 1
		default:
			path = a
		}
	}
	if jsonOut {
		if !x.have("docker") {
			return c.Fail(contract.New("docker_not_found", "Docker is not installed", "Skip the image update check"))
		}
		if x.composeCmd() == nil {
			return c.Fail(contract.New("compose_not_installed", "Docker Compose is not installed", "Skip the image update check"))
		}
	} else if !x.requireDocker() || !x.requireCompose() {
		return 1
	}
	x.colors(noColor)

	path = x.resolveDir(path)
	file := composeFile(path)
	if file == "" {
		if jsonOut {
			return c.Fail(contract.New("compose_not_found", path,
				"Pass the directory that holds docker-compose.yml, docker-compose.yaml, compose.yml or compose.yaml"))
		}
		x.p.Error("No docker-compose.yml found in " + path)
		return 1
	}
	services := x.parseServices(file)
	if services == "" {
		if jsonOut {
			return c.Fail(contract.New("no_services", file, "The compose file has no service with an image: nothing to check"))
		}
		x.p.Error("No services with images found in " + file)
		return 1
	}

	envPath := path + "/.env"
	var results []result
	var total, updates, current, pinned, errs int
	for _, line := range hereLines(services) {
		f := readFields(line, 2)
		total++
		r := result{name: f[0], image: x.resolveEnvVars(f[1], envPath)}
		img := normalizeImage(r.image)
		r.upgradeable = isUpgradeable(img.tag)
		if strings.HasPrefix(img.tag, "@sha256:") {
			r.status, r.local, r.remote = "PINNED", img.tag[1:], img.tag[1:]
			pinned++
		} else {
			r.local = x.localDigest(r.image)
			var e string
			r.remote, e = x.remoteDigest(img)
			if x.ctx().Err() != nil {
				return 130
			}
			switch {
			case r.remote == "":
				r.status, r.err = "ERROR", e
				if r.err == "" {
					r.err = "registry_unreachable"
				}
				errs++
			case r.local == "":
				r.status = "NOT_PULLED"
				updates++
			case !r.upgradeable:
				r.status = "PINNED"
				pinned++
			case r.local == r.remote:
				r.status = "CURRENT"
				current++
			default:
				r.status = "UPDATE"
				updates++
			}
		}
		results = append(results, r)
	}

	if jsonOut {
		var b strings.Builder
		b.WriteString(`{"path":` + q(path) + `,"compose_file":` + q(basename(file)) + `,"services":[`)
		for i, r := range results {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(`{"name":` + q(r.name) + `,"image":` + q(r.image))
			for _, d := range []struct{ key, v string }{{"local_digest", r.local}, {"remote_digest", r.remote}} {
				if d.v != "" {
					b.WriteString(`,"` + d.key + `":` + q(d.v))
				} else {
					b.WriteString(`,"` + d.key + `":null`)
				}
			}
			b.WriteString(`,"status":` + q(r.status) + `,"upgradeable":` + strconv.FormatBool(r.upgradeable))
			if r.err != "" {
				b.WriteString(`,"error":` + q(r.err))
			}
			b.WriteByte('}')
		}
		b.WriteString(`],"summary":{"total":` + strconv.Itoa(total) + `,"updates":` + strconv.Itoa(updates) +
			`,"current":` + strconv.Itoa(current) + `,"pinned":` + strconv.Itoa(pinned) + `,"errors":` + strconv.Itoa(errs) + "}}\n")
		_, _ = io.WriteString(c.Stdout, b.String())
		return 0
	}
	x.report(results, file, updates, current, pinned, errs)
	return 0
}

// report is the human output of docker_compose_check.
func (x *call) report(results []result, file string, updates, current, pinned, errs int) {
	w, s := x.Stdout, x.s
	var p struct{ W, S, I, E, B, M, A, R string }
	if s.Color {
		p.W, p.S, p.I, p.E, p.B, p.M, p.A, p.R = s.P.Warning, s.P.Success, s.P.Info, s.P.Error, s.P.Bold, s.P.Muted, s.P.Accent, s.P.Off
	}
	put := func(t string) { _, _ = io.WriteString(w, t) }
	put("\n")
	s.Heading(w, "Checking "+file)
	put("\n")

	// One row per service; the status takes its status color (hidden
	// column 4). The widths are 2.x's (LEDGER DOCK-30, kept).
	text := "SERVICE\tIMAGE\tSTATUS\t\n"
	for _, r := range results {
		style := "error"
		switch r.status {
		case "UPDATE", "NOT_PULLED":
			style = "warning"
		case "CURRENT":
			style = "success"
		case "PINNED":
			style = "info"
		}
		text += r.name + "\t" + r.image + "\t" + r.status + "\t" + style + "\n"
	}
	s.Table(w, ui.ReadRows(strings.NewReader(text)), "--gap", "1", "--min", "1:15", "--min", "2:40", "--max", "2:38",
		"--color", "1:accent", "--cell-color", "3:4")

	put("\n")
	put(p.B + "Summary:" + p.R + " ")
	if updates > 0 {
		put(p.W + strconv.Itoa(updates) + " update(s)" + p.R + " ")
	}
	if current > 0 {
		put(p.S + strconv.Itoa(current) + " current" + p.R + " ")
	}
	if pinned > 0 {
		put(p.I + strconv.Itoa(pinned) + " pinned" + p.R + " ")
	}
	if errs > 0 {
		put(p.E + strconv.Itoa(errs) + " error(s)" + p.R)
	}
	put("\n")

	if errs > 0 {
		put("\n")
		s.Heading(w, "Errors:")
		for _, r := range results {
			if r.status != "ERROR" {
				continue
			}
			reg := normalizeImage(r.image).registry
			msg, hint := errorText(r.err, reg)
			put("  " + p.A + r.name + p.R + " (" + r.image + ")\n")
			s.KV(w, "--indent", "4", "Registry:", reg)
			s.Mark(w, "--indent", "4", "error", msg)
			put("    " + p.M + "Hint: " + hint + p.R + "\n")
		}
	}
	if updates > 0 {
		put("\n")
		s.Print(w, "bold", "Run 'docker_compose_upgrade' to update services with mutable tags.")
	}
}

// errorText is the message and hint of an error class on a registry.
func errorText(err, reg string) (string, string) {
	switch err {
	case "timeout":
		return "Connection timed out", "Registry may be slow or unreachable"
	case "dns_error":
		return "DNS resolution failed", "Check network connectivity and registry hostname"
	case "connection_refused":
		return "Connection refused", "Registry may be down or port blocked"
	case "ssl_error":
		return "SSL/TLS handshake failed", "Check certificate validity"
	case "unauthorized":
		if reg == "ghcr.io" {
			return "Authentication required (401)", "Set GITHUB_TOKEN for authenticated access"
		}
		return "Authentication required (401)", "Registry requires authentication"
	case "forbidden":
		return "Access denied (403)", "Check credentials and repository permissions"
	case "not_found":
		return "Image not found (404)", "Check image name and tag are correct"
	case "rate_limited":
		if reg == "docker.io" {
			return "Rate limit exceeded (429)", "Docker Hub limits unauthenticated pulls. Try logging in."
		}
		return "Rate limit exceeded (429)", "Wait and retry, or authenticate for higher limits"
	case "server_error":
		return "Registry server error (5xx)", "Temporary issue, retry later"
	}
	switch reg {
	case "ghcr.io":
		return "Could not reach registry or get manifest", "Try setting GITHUB_TOKEN for authenticated access"
	case "docker.io":
		return "Could not reach registry or get manifest", "Check network or Docker Hub rate limits"
	}
	return "Could not reach registry or get manifest", "Registry may require authentication"
}
