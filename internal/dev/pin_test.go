// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package dev

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// TestProjectPin: the version a project pins, read as php_detect_version
// and node_detect_version read it, and the file that pins it; nothing
// when no file does (their defaults are not pins).
func TestProjectPin(t *testing.T) {
	home := t.TempDir()
	pin := func(dir, family string) (string, string) {
		t.Chdir(dir)
		e := invoke.EnvMap(baseEnv(home, map[string]string{"PWD": dir}))
		c := &invoke.Call{Env: e, Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
			Term: term.Fixed(e, false, false, false)}
		return ProjectPin(c, family, ".")
	}
	for _, tc := range []struct {
		name, family string
		files        map[string]string
		version      string
		file         string
	}{
		{"php none", "php", nil, "", ""},
		{"php target file", "php", map[string]string{".target-php-version": "8.3\n"}, "8.3", ".target-php-version"},
		{"php composer", "php", map[string]string{"composer.json": `{"require":{"php":"^8.2"}}`}, "8.2", "composer.json"},
		{"php composer without php", "php", map[string]string{"composer.json": `{"require":{"laravel/framework":"^11"}}`}, "", ""},
		{"php copilot", "php", map[string]string{".github/copilot-instructions.md": "Use PHP 8.1.\n"}, "8.1", ".github/copilot-instructions.md"},
		{"node none", "node", nil, "", ""},
		{"node nvmrc", "node", map[string]string{".nvmrc": "v22.11.0\n"}, "22", ".nvmrc"},
		{"node node-version", "node", map[string]string{".node-version": "20\n"}, "20", ".node-version"},
		{"node lts", "node", map[string]string{".nvmrc": "lts/iron\n"}, "20", ".nvmrc"},
		{"node engines", "node", map[string]string{"package.json": `{"engines":{"node":">=18"}}`}, "18", "package.json"},
		{"node package without engines", "node", map[string]string{"package.json": `{"name":"x"}`}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := t.TempDir()
			for name, content := range tc.files {
				mkfile(t, filepath.Join(d, name), content)
			}
			v, f := pin(d, tc.family)
			require.Equal(t, tc.version, v)
			if tc.file == "" {
				require.Empty(t, f)
				return
			}
			require.Equal(t, tc.file, strings.TrimPrefix(f, d+"/"))
		})
	}

	// .target-php-version is found in a parent directory too.
	d := t.TempDir()
	mkfile(t, filepath.Join(d, ".target-php-version"), "8.4\n")
	sub := filepath.Join(d, "app", "src")
	mkfile(t, filepath.Join(sub, "x.php"), "<?php\n")
	v, f := pin(sub, "php")
	require.Equal(t, "8.4", v)
	require.Equal(t, filepath.Join(d, ".target-php-version"), f)
}
