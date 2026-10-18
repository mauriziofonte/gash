// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

import (
	"os"
	"regexp"
	"strings"
)

// matrixRow is one table row of go-impl/PORT/MATRIX.md.
type matrixRow struct {
	section string
	cells   []string
}

// matrix holds the rows of the tables of MATRIX.md, by kind.
type matrix struct {
	functions map[string]matrixRow // function rows by name
	aliases   map[string]matrixRow
	envVars   []matrixRow
	settings  []matrixRow
}

func readMatrix(path string) (*matrix, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := &matrix{functions: map[string]matrixRow{}, aliases: map[string]matrixRow{}}
	section := ""
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "#") {
			section = strings.TrimSpace(strings.TrimLeft(l, "#"))
			continue
		}
		if !strings.HasPrefix(l, "| ") || strings.HasPrefix(l, "|---") {
			continue
		}
		cells := splitRow(l)
		row := matrixRow{section: section, cells: cells}
		switch {
		case section == "Env vars":
			if cells[0] != "Name" {
				m.envVars = append(m.envVars, row)
			}
		case section == "Settings":
			if cells[0] != "Name" {
				m.settings = append(m.settings, row)
			}
		case len(cells) >= 7 && cells[1] == "function":
			m.functions[matrixName(cells[0])] = row
		case len(cells) >= 7 && cells[1] == "alias":
			m.aliases[cells[0]] = row
		}
	}
	return m, nil
}

// splitRow splits a Markdown table row on unescaped pipes.
func splitRow(l string) []string {
	l = strings.TrimSpace(l)
	l = strings.TrimPrefix(l, "|")
	l = strings.TrimSuffix(l, "|")
	var cells []string
	var cur strings.Builder
	for i := 0; i < len(l); i++ {
		if l[i] == '\\' && i+1 < len(l) && l[i+1] == '|' {
			cur.WriteByte('|')
			i++
			continue
		}
		if l[i] == '|' {
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteByte(l[i])
	}
	return append(cells, strings.TrimSpace(cur.String()))
}

// matrixName strips a parenthetical qualifier: "nvm (GASH_NVM=lazy stub)".
func matrixName(s string) string {
	if i := strings.Index(s, " ("); i > 0 {
		return s[:i]
	}
	return s
}

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// names returns the variable names of a Name cell ("LANG LC_ALL LC_CTYPE",
// "HTTPS_PROXY/https_proxy, NO_PROXY/no_proxy", "GASH_HEADLESS (node...)").
func names(cell string) []string {
	if i := strings.Index(cell, "("); i >= 0 {
		cell = cell[:i]
	}
	var out []string
	for _, f := range strings.FieldsFunc(cell, func(c rune) bool { return c == ' ' || c == ',' || c == '/' }) {
		if identRe.MatchString(f) {
			out = append(out, f)
		}
	}
	return out
}

// passedEnv returns the env var names the stub passes (Read by "passed by
// the shim").
func (m *matrix) passedEnv() map[string]bool {
	out := map[string]bool{}
	for _, r := range m.envVars {
		if len(r.cells) > 4 && strings.Contains(r.cells[4], "passed by the shim") {
			for _, n := range names(r.cells[0]) {
				out[n] = true
			}
		}
	}
	return out
}

// allEnv returns every env var name of the Env vars table.
func (m *matrix) allEnv() map[string]bool {
	out := map[string]bool{}
	for _, r := range m.envVars {
		for _, n := range names(r.cells[0]) {
			out[n] = true
		}
	}
	return out
}

// passedSettings returns the GASH_* settings the stub passes.
func (m *matrix) passedSettings() map[string]bool {
	out := map[string]bool{}
	for _, r := range m.settings {
		if len(r.cells) > 4 && strings.Contains(r.cells[4], "passed by the shim") {
			for _, n := range names(r.cells[0]) {
				out[n] = true
			}
		}
	}
	return out
}
