// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// cardAlias is one line the awk program of __gash_card_aliases
// (lib/modules/gash.sh) printed for an alias file: NAME TAB VALUE.
type cardAlias struct{ name, value string }

var (
	cardComment  = regexp.MustCompile(`^[ \t]*#`)
	cardCommentP = regexp.MustCompile(`^[ \t]*#[ \t]*`)
	cardFunction = regexp.MustCompile(`^[ \t]*function [A-Za-z0-9_.-]+ \{`)
	cardFuncP    = regexp.MustCompile(`^[ \t]*function `)
	cardAliasRe  = regexp.MustCompile(`^[ \t]*alias [^$"'=]+=`)
	cardAliasP   = regexp.MustCompile(`^[ \t]*alias `)
	cardTrailing = regexp.MustCompile(`[ \t]+#.*$`)
	cardDocker   = regexp.MustCompile(`__gash_add_docker_alias "[^"]*" "[^"]*" "[^"]*"`)
	cardLoadVar  = regexp.MustCompile(` ?\$__gash_[A-Za-z_]+`)
	cardPHPVar   = regexp.MustCompile(`\$__gash_(php_latest|bin)`)
)

// cardAliasesOf runs the awk program of __gash_card_aliases over the text
// of an alias file: functions with the comment above them, aliases with
// their value (unquoted, load-time variables shown by what they hold),
// the docker aliases of __gash_add_docker_alias; each name once.
func cardAliasesOf(text string) []cardAlias {
	var out []cardAlias
	seen := map[string]bool{}
	comment := ""
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		switch {
		case cardComment.MatchString(line):
			comment = cardCommentP.ReplaceAllString(line, "")
			continue
		case cardFunction.MatchString(line):
			n := cardFuncP.ReplaceAllString(line, "")
			if i := strings.IndexByte(n, ' '); i >= 0 {
				n = n[:i]
			}
			if !seen[n] && n != "help" {
				seen[n] = true
				out = append(out, cardAlias{n, comment})
			}
			comment = ""
			continue
		case cardAliasRe.MatchString(line):
			l := cardAliasP.ReplaceAllString(line, "")
			n := l
			if i := strings.IndexByte(n, '='); i >= 0 {
				n = n[:i]
			}
			v := l
			if i := strings.IndexByte(v, '='); i >= 0 {
				v = v[i+1:]
			}
			if loc := cardTrailing.FindStringIndex(v); loc != nil {
				v = v[:loc[0]]
			}
			v = unq(v)
			v = strings.ReplaceAll(v, "$__gash_hte", "sudo phpX.Y hte-cli")
			v = strings.ReplaceAll(v, "$__gash_composer1", "composer1")
			v = strings.ReplaceAll(v, "$__gash_composer", "composer")
			v = cardPHPVar.ReplaceAllString(v, "phpX.Y")
			v = cardLoadVar.ReplaceAllString(v, "")
			if strings.Contains(n, "$") || v == "" {
				continue
			}
			if !seen[n] {
				seen[n] = true
				out = append(out, cardAlias{n, v})
			}
			comment = ""
			continue
		case cardDocker.MatchString(line):
			q := strings.Split(line, `"`)
			name, value := quoted(q, 3), quoted(q, 5)
			if !seen[name] {
				seen[name] = true
				out = append(out, cardAlias{name, value})
			}
			continue
		}
		comment = ""
	}
	return out
}

// quoted is awk's q[i+1] of split($0, q, "\""): "" past the end.
func quoted(q []string, i int) string {
	if i < len(q) {
		return q[i]
	}
	return ""
}

// unq is the awk unq: one pair of surrounding double or single quotes
// removed.
func unq(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

// scanCardAliases reads the alias files the card topics list. A file 2.x
// could not read gives no entries, as __gash_card_aliases returned
// nothing for it.
func (c *catalog) scanCardAliases(root string) error {
	c.cardAliases = map[string][]cardAlias{}
	for _, t := range c.topics {
		for _, f := range strings.Fields(t[2]) {
			if _, done := c.cardAliases[f]; done {
				continue
			}
			text, err := os.ReadFile(filepath.Join(root, "lib/aliases", f+".sh"))
			if errors.Is(err, fs.ErrNotExist) {
				c.cardAliases[f] = nil
				continue
			}
			if err != nil {
				return err
			}
			c.cardAliases[f] = cardAliasesOf(string(text))
		}
	}
	return nil
}
