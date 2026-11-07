// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/selfmgmt"
)

// gash_card_spec.sh "gash all lists every registered function", 3.5:
// every visible command of the tree is a row of its namespace page in
// gash all, and the overview names every namespace.
func TestCardListsTheRegistry(t *testing.T) {
	env := map[string]string{"LC_ALL": "C", "COLUMNS": "200"}
	all, _, rc := run(t, "gash", env, false, "all", "--no-color")
	require.Equal(t, 0, rc)
	n := 0
	for i := range registry.Tree() {
		cmd := &registry.Tree()[i]
		if cmd.Hidden || len(cmd.Path) < 2 {
			continue
		}
		require.Regexp(t, `(?m)^  `+regexp.QuoteMeta(strings.Join(cmd.Path[1:], " "))+`( |$)`, all, cmd.Name())
		n++
	}
	require.Greater(t, n, 100)
	overview, _, rc := run(t, "gash", env, false, "--no-color")
	require.Equal(t, 0, rc)
	for _, ns := range registry.Namespaces() {
		require.Regexp(t, `(?m)^    `+ns.Name+` +`+regexp.QuoteMeta(ns.Summary)+`$`, overview)
	}
}

// LEDGER SELF-12: from a shell that is not headless the pages list only
// the shortcuts the shell defines (3.0: SELF-12 fixed; rc.A listed gp and
// gc although the shadow rule had dropped them). Without the live flag
// (headless, a script) the pages still show every registered shortcut.
func TestLedger_SELF12(t *testing.T) {
	var live strings.Builder
	for _, a := range registry.CardAliases("git") {
		if a.Name != "gp" && a.Name != "gc" && a.Name != a.Value {
			live.WriteString(a.Name + "\t" + a.Value + "\n")
		}
	}
	env := map[string]string{"LC_ALL": "C", "COLUMNS": "200", selfmgmt.CardAliasesVar: live.String(), selfmgmt.CardLiveVar: "1"}
	card, _, _ := run(t, "gash", env, false, "git")
	require.NotRegexp(t, `(?m)  gp +git pull( |$)`, card)
	require.NotRegexp(t, `(?m)  gc +git commit( |$)`, card)
	require.Regexp(t, `(?m)  gst +git status( |$)`, card)

	// A shell with no alias defined: no pack shortcut listed.
	env[selfmgmt.CardAliasesVar] = ""
	card, _, _ = run(t, "gash", env, false, "git")
	require.NotRegexp(t, `(?m)  gst +git status( |$)`, card)

	// Headless or a script (no live flag): every registered alias, the
	// values of defined ones from the shell.
	for _, live := range []string{"", "gst\tgit status --short\n"} {
		env := map[string]string{"LC_ALL": "C", "COLUMNS": "200", selfmgmt.CardAliasesVar: live}
		card, _, _ := run(t, "gash", env, false, "git")
		require.Regexp(t, `(?m)  gp +git pull( |$)`, card)
		require.Regexp(t, `(?m)  gc +git commit( |$)`, card)
		if live != "" {
			require.Regexp(t, `(?m)  gst +git status --short( |$)`, card)
		}
	}
}

// LEDGER DEV-21: the runtime page (the former dev topic) shows the php
// pack's families with their conditions; a shell that defines php83 and
// php74 gets those listed in place of phpXY, and no condition notes.
func TestLedger_DEV21(t *testing.T) {
	env := map[string]string{"LC_ALL": "C", "COLUMNS": "200"}
	card, _, _ := run(t, "gash", env, false, "dev")
	require.Regexp(t, `(?m)^  phpXY +phpX\.Y `, card)
	require.Contains(t, card, "\n  phpXY: for each installed phpX.Y\n")
	require.Contains(t, card, "\n  php, composer: with GASH_REPLACE_TOOLS=1\n")
	require.NotContains(t, card, "hte")

	env[selfmgmt.CardAliasesVar] = "php83\t/usr/bin/php8.3 -d x\nphp74\t/usr/bin/php7.4 -d x\ncomposer83\t/usr/bin/php8.3 c\nphpfoo\tx\nphp8x\ty\n"
	env[selfmgmt.CardLiveVar] = "1"
	card, _, _ = run(t, "gash", env, false, "dev")
	require.Regexp(t, `(?m)^  php74 +/usr/bin/php7\.4 -d x +php83 +/usr/bin/php8\.3 -d x +composer83 +/usr/bin/php8\.3 c\s*$`, card)
	require.NotContains(t, card, "phpXY")
	require.NotContains(t, card, "phpfoo")
	require.NotContains(t, card, "php8x")
	require.NotContains(t, card, "for each installed")
}
