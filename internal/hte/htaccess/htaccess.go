// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package htaccess finds HTTPS redirects in a document root's .htaccess
// (design/hte.md section 3, LEDGER HTE-26): site create, modify, show,
// doctor and migrate warn about them, because such a redirect forces
// HTTPS whatever force_https and its loopback bypass say. Finding one is
// never an error.
package htaccess

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Redirect is one HTTPS redirect of an .htaccess.
type Redirect struct {
	File string `json:"file"`
	Line int    `json:"line"` // the line of the RewriteRule or Redirect
	Text string `json:"text"` // that line, trimmed
}

// Warning is the text of the warning for r.
func (r Redirect) Warning() string {
	return r.File + ":" + strconv.Itoa(r.Line) + " redirects to HTTPS on its own (" + r.Text + "); force_https and its loopback bypass do not apply to it"
}

// schemeTests are the RewriteCond test strings that look at the scheme or
// the port of the request: hte-cli's forms plus X-Forwarded-Proto.
var schemeTests = []string{
	"%{https}", "%{server_port}", "%{request_scheme}", "%{env:https}",
	"%{http:x-forwarded-proto}", "%{http:x-forwarded-ssl}", "http_x_forwarded_proto",
}

func testsScheme(cond string) bool {
	c := strings.ToLower(cond)
	for _, s := range schemeTests {
		if strings.Contains(c, s) {
			return true
		}
	}
	return false
}

func fields(line string) []string {
	f := strings.Fields(line)
	for i, w := range f {
		f[i] = strings.Trim(w, `"'`)
	}
	return f
}

// Find lists the HTTPS redirects of an .htaccess: a Redirect,
// RedirectMatch, RedirectPermanent or RedirectTemp to https://, and a
// RewriteRule to https:// one of whose own conditions (the RewriteCond
// lines since the previous RewriteRule, as mod_rewrite attaches them)
// tests the scheme or port.
func Find(data []byte, file string) []Redirect {
	var out []Redirect
	var conds []string
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := fields(line)
		if len(f) == 0 {
			continue
		}
		switch strings.ToLower(f[0]) {
		case "rewritecond":
			if len(f) > 1 {
				conds = append(conds, f[1])
			}
		case "rewriterule":
			if len(f) > 2 && strings.HasPrefix(strings.ToLower(f[2]), "https://") {
				for _, c := range conds {
					if testsScheme(c) {
						out = append(out, Redirect{File: file, Line: i + 1, Text: line})
						break
					}
				}
			}
			conds = nil
		case "redirect", "redirectmatch", "redirectpermanent", "redirecttemp":
			if strings.HasPrefix(strings.ToLower(f[len(f)-1]), "https://") {
				out = append(out, Redirect{File: file, Line: i + 1, Text: line})
			}
		}
	}
	return out
}

// Check reads <docroot>/.htaccess with read (os.ReadFile when nil). A
// missing file has no redirect; an unreadable one is returned as err for
// the caller to report as a warning.
func Check(docroot string, read func(string) ([]byte, error)) ([]Redirect, error) {
	if read == nil {
		read = os.ReadFile
	}
	file := filepath.Join(docroot, ".htaccess")
	data, err := read(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return Find(data, file), nil
}
