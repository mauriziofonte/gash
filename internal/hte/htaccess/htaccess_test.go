// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package htaccess

import (
	"errors"
	"io/fs"
	"testing"
)

func TestLedger_HTE26(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want []int // lines of the redirects
	}{
		{"hte-cli form", "RewriteEngine On\nRewriteCond %{HTTPS} off\nRewriteRule ^(.*)$ https://%{HTTP_HOST}%{REQUEST_URI} [L,R=301]\n", []int{3}},
		{"HTTPS !=on", "RewriteCond %{HTTPS} !=on\nRewriteRule ^ https://%{HTTP_HOST}%{REQUEST_URI} [R=301,L]\n", []int{2}},
		{"port 80", "RewriteCond %{SERVER_PORT} 80\nRewriteRule ^(.*)$ https://example.test/$1 [R,L]\n", []int{2}},
		{"X-Forwarded-Proto", "RewriteCond %{HTTP:X-Forwarded-Proto} !https\nRewriteCond %{HTTPS} off\nRewriteRule ^ https://%{HTTP_HOST}%{REQUEST_URI} [L,R=301]\n", []int{3}},
		{"X-Forwarded-Proto alone", "RewriteCond %{HTTP:X-Forwarded-Proto} =http\nRewriteRule .* https://%{HTTP_HOST}%{REQUEST_URI} [L,R=301]\n", []int{2}},
		{"Laravel-style with an unrelated cond first", "RewriteCond %{REQUEST_FILENAME} !-d\nRewriteCond %{HTTPS} off\nRewriteRule ^ https://%{HTTP_HOST}%{REQUEST_URI} [L,R=301]\n", []int{3}},
		{"quoted", "RewriteCond \"%{HTTPS}\" \"off\"\nRewriteRule \"^\" \"https://%{HTTP_HOST}%{REQUEST_URI}\" [R=301,L]\n", []int{2}},
		{"Redirect permanent", "Redirect permanent / https://shop.test/\n", []int{1}},
		{"RedirectMatch", "RedirectMatch 301 ^/(.*)$ https://shop.test/$1\n", []int{1}},
		{"case", "rewritecond %{https} off\nrewriterule ^ HTTPS://%{HTTP_HOST}/ [R]\n", []int{2}},
		{"unrelated cond, later unrelated rule", "RewriteCond %{HTTPS} off\nRewriteRule ^ - [E=PLAIN:1]\nRewriteRule ^old$ https://cdn.test/new [R=302,L]\n", nil},
		{"front controller", "RewriteEngine On\nRewriteCond %{REQUEST_FILENAME} !-f\nRewriteRule ^ index.php [L]\n", nil},
		{"commented", "# RewriteCond %{HTTPS} off\n# RewriteRule ^ https://%{HTTP_HOST}%{REQUEST_URI} [R=301,L]\n", nil},
		{"redirect to http", "Redirect / http://other.test/\n", nil},
		{"empty", "", nil},
	} {
		got := Find([]byte(tc.text), "/srv/a/.htaccess")
		var lines []int
		for _, r := range got {
			lines = append(lines, r.Line)
			if r.File != "/srv/a/.htaccess" || r.Text == "" || r.Warning() == "" {
				t.Errorf("%s: %+v", tc.name, r)
			}
		}
		if len(lines) != len(tc.want) || (len(lines) > 0 && lines[0] != tc.want[0]) {
			t.Errorf("%s: lines %v, want %v", tc.name, lines, tc.want)
		}
	}
}

func TestCheck(t *testing.T) {
	files := map[string]string{"/srv/a/.htaccess": "Redirect / https://a.test/\n"}
	read := func(p string) ([]byte, error) {
		if p == "/srv/locked/.htaccess" {
			return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrPermission}
		}
		if s, ok := files[p]; ok {
			return []byte(s), nil
		}
		return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
	}
	if r, err := Check("/srv/a", read); err != nil || len(r) != 1 || r[0].Line != 1 {
		t.Fatalf("%v %v", r, err)
	}
	if r, err := Check("/srv/none", read); err != nil || r != nil {
		t.Fatalf("missing: %v %v", r, err)
	}
	if _, err := Check("/srv/locked", read); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("unreadable: %v", err)
	}
}
