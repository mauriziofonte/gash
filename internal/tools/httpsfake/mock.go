// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

// reply is a canned response of tests/mocks/bin/curl, as HTTP.
type reply struct {
	status int
	header [][2]string
	body   string
	// hang keeps the request unanswered until the client gives up: the
	// mock's exit 28 (operation timed out).
	hang bool
	// drop closes the connection without an answer: an unreachable route
	// (the mock's exit 6).
	drop bool
	// length is the Content-Length of a routed answer, which a HEAD
	// answer announces without the body; 0 for the canned answers.
	length int64
}

// request is what the mock looked at: the method (-I is HEAD), the URL
// and the body.
type request struct {
	method, url, body string
}

// respond is tests/mocks/bin/curl ported rule for rule: env is the
// environment of the process that made the request (the MOCK_CURL_*
// knobs of the test). The routes of MOCK_CURL_ROUTES come first.
func respond(env map[string]string, r request) reply {
	if path := env["MOCK_CURL_ROUTES"]; path != "" {
		rt, ok, err := findRoute(path, r.method, r.url)
		if err != nil {
			return routeFailure(err)
		}
		if ok {
			return rt.reply(path, r.method)
		}
	}
	head := r.method == "HEAD"
	url := r.url
	resp := ""
	code := 200
	ctype := "application/json"
	var extra [][2]string

	switch {
	case env["MOCK_CURL_ERROR"] != "":
		code = atoiOr(env["MOCK_CURL_ERROR_CODE"], 500)
		resp = `{"error":{"message":"Simulated error"}}`
	case env["MOCK_CURL_TIMEOUT"] != "":
		return reply{hang: true}

	// Docker registry endpoints.
	case strings.Contains(url, "auth.docker.io/token"):
		switch {
		case env["MOCK_CURL_REGISTRY_AUTH_FAIL"] == "1":
			code, resp = 500, `{"error":"internal_error"}`
		case env["MOCK_CURL_REGISTRY_UNAUTHORIZED"] == "1":
			code, resp = 401, `{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}`
		default:
			resp = `{"token":"mock-dockerhub-token-abc123","access_token":"mock-dockerhub-token-abc123"}`
		}
	case strings.Contains(url, "ghcr.io/token"):
		if env["MOCK_CURL_REGISTRY_AUTH_FAIL"] == "1" {
			code, resp = 500, `{"error":"internal_error"}`
		} else {
			resp = `{"token":"mock-ghcr-token-xyz789"}`
		}
	case manifestURL.MatchString(url):
		switch {
		case env["MOCK_CURL_REGISTRY_TIMEOUT"] == "1":
			return reply{hang: true}
		case env["MOCK_CURL_REGISTRY_RATE_LIMITED"] == "1":
			code, resp = 429, `{"errors":[{"code":"TOOMANYREQUESTS","message":"rate limit exceeded"}]}`
		case env["MOCK_CURL_REGISTRY_NOT_FOUND"] == "1":
			code, resp = 404, `{"errors":[{"code":"MANIFEST_UNKNOWN","message":"manifest unknown"}]}`
		case env["MOCK_CURL_REGISTRY_UNAUTHORIZED"] == "1":
			code, resp = 401, `{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}`
		case head:
			// curl -I printed these header lines; over HTTP they are the
			// response headers of the HEAD request.
			return reply{status: 200, header: [][2]string{
				{"Content-Type", "application/vnd.docker.distribution.manifest.v2+json"},
				{"Docker-Content-Digest", "sha256:newdigest456abc123def456abc123def456abc123def456abc123def456newdig"},
			}}
		default:
			resp = `{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json"}`
		}

	// AI API endpoints.
	case strings.Contains(url, "api.anthropic.com"):
		inner := strings.ReplaceAll(innerJSON(responseType(r.body)), `"`, `\"`)
		resp = `{"content":[{"type":"text","text":"` + inner + `"}],"model":"claude-haiku-4-5","stop_reason":"end_turn"}`
	case strings.Contains(url, "generativelanguage.googleapis.com"):
		inner := strings.ReplaceAll(innerJSON(responseType(r.body)), `"`, `\"`)
		resp = `{"candidates":[{"content":{"parts":[{"text":"` + inner + `"}]}}]}`
	default:
		code = 404
		resp = `{"error":"Unknown endpoint","url":"` + url + `"}`
	}

	// The AI knobs: a verbatim reply, and one busy answer per state file.
	if strings.Contains(url, "api.anthropic.com") || strings.Contains(url, "generativelanguage.googleapis.com") {
		if v := env["MOCK_CURL_AI_RESPONSE"]; v != "" {
			resp = v
		}
		if env["MOCK_CURL_AI_BUSY_ONCE"] != "" && env["MOCK_CURL_STATE"] != "" {
			if _, err := os.Stat(env["MOCK_CURL_STATE"]); err != nil {
				_ = os.WriteFile(env["MOCK_CURL_STATE"], nil, 0o600)
				code = atoiOr(env["MOCK_CURL_AI_BUSY_ONCE"], 500)
				resp = `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`
				extra = append(extra, [2]string{"Retry-After", "1"})
			}
		}
	}
	h := append([][2]string{{"Content-Type", ctype}}, extra...)
	if head {
		return reply{status: code, header: h}
	}
	return reply{status: code, header: h, body: resp + "\n"}
}

var manifestURL = regexp.MustCompile(`/v2/.*/manifests/`)

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n >= 100 && n <= 999 {
		return n
	}
	return def
}

// lowerASCII is tr '[:upper:]' '[:lower:]'.
func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// userQuery is the mock's sed -n 's/.*}\\n\\n\([^"]*\)".*/\1/p' | head -1:
// the text after the last `}\n\n` (as JSON escapes) of the first line
// that has one followed by a quote.
var userQueryRe = regexp.MustCompile(`^.*\}\\n\\n([^"]*)".*$`)

func userQuery(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if m := userQueryRe.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	}
	return ""
}

// responseType is the mock's detect_response_type.
func responseType(body string) string {
	lb := lowerASCII(body)
	switch {
	case strings.Contains(lb, "section:") && strings.Contains(lb, "initial findings"):
		return "drilldown"
	case strings.Contains(lb, "initial server analysis") && strings.Contains(lb, "user question"):
		return "drilldown"
	case strings.Contains(lb, "analyze this system enumeration"):
		return "sysinfo"
	}
	q := lowerASCII(userQuery(body))
	switch {
	case strings.Contains(q, "[pipe input"):
		return "troubleshoot"
	case strings.Contains(q, "how to") || strings.Contains(q, "come faccio") || strings.Contains(q, "how do i"):
		return "command"
	case strings.Contains(q, "what is") || strings.Contains(q, "cos'è") || strings.Contains(q, "explain") || strings.Contains(q, "spiega"):
		return "explanation"
	case strings.Contains(q, "write a script") || strings.Contains(q, "scrivi uno script") || strings.Contains(q, "write code"):
		return "code"
	}
	return "fallback"
}

// innerJSON is the mock's generate_inner_json.
func innerJSON(kind string) string {
	switch kind {
	case "troubleshoot":
		return `{"type":"troubleshoot","issue":"PHP Fatal error - undefined variable","suggestion":"Check variable initialization before use","text":"Analysis complete"}`
	case "command":
		return `{"type":"command","command":"ls -lhS","text":"Lists files sorted by size in descending order."}`
	case "explanation":
		return `{"type":"explanation","text":"This is a conceptual explanation of the topic."}`
	case "code":
		return `{"type":"code","text":"Creates a backup script.","code":"#!/bin/bash\\ntar -czf backup.tar.gz ~/","lang":"bash"}`
	case "sysinfo":
		return `{"hostname":"testhost","platform":"Debian 12 | 6.1.0-amd64 | bare-metal","sections":[{"title":"Services Detected","findings":[{"severity":"ok","text":"Apache2 running with 3 vhosts"},{"severity":"info","text":"PHP 8.1 FPM with 2 pools"}]},{"title":"Security Posture","findings":[{"severity":"warning","text":"No firewall detected"},{"severity":"info","text":"SSH key-only auth enabled"}]},{"title":"Recommendations","findings":[{"severity":"warning","text":"Install ufw or firewalld for perimeter protection"}]}]}`
	case "drilldown":
		return `{"title":"Services Detected","summary":"Detailed analysis of detected services on this server.","components":[{"name":"Apache2","status":"running","config_highlights":["3 vhosts in sites-enabled","MPM: event","Listen: *:80, *:443"],"issues":[{"severity":"ok","text":"ProxyPassMatch configured for PHP-FPM"}]},{"name":"PHP-FPM 8.1","status":"running","config_highlights":["pool www: pm=dynamic, max_children=5","memory_limit=256M"],"issues":[{"severity":"warning","text":"max_children=5 is low for production"}]},{"name":"Viper-backup","status":"running","config_highlights":["Custom systemd service","ExecStart=/opt/viper/backup.sh","WorkingDirectory=/opt/viper"],"issues":[{"severity":"info","text":"Backup orchestration service running as root"}]}]}`
	}
	return `{"type":"fallback","text":"I am a bash/linux terminal assistant. How can I help?"}`
}
