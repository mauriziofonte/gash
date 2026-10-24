// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/selfmgmt"
)

var cardCalls = [][]string{
	{}, {"all"}, {"files"}, {"system"}, {"nav"}, {"git"}, {"docker"}, {"remote"}, {"dev"}, {"llm"}, {"ai"},
	{"gash"}, {"navigation"}, {"management"}, {"php"}, {"node"}, {"bogus"}, {"--help"}, {"-h"},
	{"git", "--no-color"}, {"--no-color"}, {"all", "extra", "nav"},
}

// cardScript runs every card call in the 2.x shell, after printing the
// alias values the gash stub passes (its loop over the card names).
func cardScript(prefix string, shadow bool) string {
	var b strings.Builder
	b.WriteString(prefix)
	b.WriteString(`__c=""; for __n in`)
	for _, n := range registry.CardAliasNames() {
		b.WriteString(" " + shq(n))
	}
	b.WriteString(`; do [[ -n "${BASH_ALIASES[$__n]+x}" ]] || continue; __v=${BASH_ALIASES[$__n]//"\\"/"\\\\"}; ` +
		`__c+=$__n$'\t'${__v//$'\n'/'\n'}$'\n'; done; printf '%s\0' "$__c"` + "\n")
	for _, args := range cardCalls {
		if shadow {
			args = append([]string{"--no-color"}, args...)
		}
		b.WriteString("gash")
		for _, a := range args {
			b.WriteString(" " + shq(a))
		}
		b.WriteString(" 2>&1; printf '\\0%s\\0' $?\n")
	}
	return b.String()
}

// devOverview is the dev row of the card overview, up to its alias count.
var devOverview = regexp.MustCompile(`(?m)^(.*dev(?:\x1b\[[0-9;]*m| )+[0-9]+ +)[0-9]+`)

// topicHeading is a card topic heading line ("  LLM  ...").
var topicHeading = regexp.MustCompile(`^  (\x1b\[[0-9;]*m)*[A-Z][A-Z-]+(\x1b\[[0-9;]*m)* `)

// stripDevAliases drops the alias block of the dev topic (from its
// "Aliases" heading to the next topic heading or the end) and the empty
// lines in it.
func stripDevAliases(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	inDev, skip := false, false
	for _, l := range lines {
		switch {
		case strings.Contains(l, "PHP and Node.js versions"):
			inDev, skip = true, false
		case skip && topicHeading.MatchString(l):
			inDev, skip = false, false
		case inDev && strings.Contains(l, "Aliases"):
			skip = true
		}
		if skip {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// cardRule is a card section rule (a line of ─ only).
var cardRule = regexp.MustCompile(`(?m)^(\s*)─+$`)

// sgr is an SGR escape sequence of the palette.
var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

// requireCardLayout fails unless got is want line for line, except that a
// function's row may differ after its name, and the column header and a
// section rule in spacing: 3.0's fixes rewrote many usages and summaries
// (each under its own ledger item), while the card's topics, header names,
// grouping, row order and line count still match 2.x.
func requireCardLayout(t *testing.T, want, got string, args []string) {
	t.Helper()
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	if len(w) != len(g) {
		require.Equal(t, want, got, "%q", args)
	}
	for i := range w {
		if w[i] == g[i] {
			continue
		}
		wf, gf := strings.Fields(sgr.ReplaceAllString(w[i], "")), strings.Fields(sgr.ReplaceAllString(g[i], ""))
		_, isFunc := registry.Lookup(firstField(gf))
		// The column header (its last name cut to fit) and a section rule
		// follow the widest usage.
		spacing := slices.EqualFunc(wf, gf, sameCell) || (isRule(wf) && isRule(gf))
		if !spacing && (!isFunc || firstField(wf) != firstField(gf)) {
			require.Equal(t, want, got, "%q: line %d", args, i+1)
		}
	}
}

// sameCell reports whether two header cells are one name, either side
// possibly cut with an ellipsis where the narrower layout ran out of room.
func sameCell(a, b string) bool {
	cut := func(x, y string) bool {
		p, ok := strings.CutSuffix(x, "…")
		return ok && strings.HasPrefix(y, p)
	}
	return a == b || cut(a, b) || cut(b, a)
}

func isRule(f []string) bool {
	return len(f) == 1 && strings.Trim(f[0], "─") == ""
}

func firstField(f []string) string {
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// gash prints the card 2.x prints: every topic and form, colors off, on
// and shadowed by --no-color, at three widths, with the aliases of an
// interactive shell and with aliases the user redefined (their values
// shown, identity ones left out, a newline in a value split as 2.x split
// it).
func TestCardMatches2x(t *testing.T) {
	redefine := `alias gst='git status --short' gl='log graph' ls=ls ll=$'ls\n-l' gd='git\diff' ` +
		`cp='cp -i' unalias_me=x; unalias la 2>/dev/null; `
	for _, tc := range []struct{ prefix, cols, mode string }{
		{"", "200", "off"}, {"", "100", "on"}, {"", "100", "shadow"}, {"", "60", "on"}, {"", "60", "off"},
		{redefine, "100", "on"}, {redefine, "60", "shadow"},
	} {
		t.Run(tc.cols+"/"+tc.mode+"/"+strconv.FormatBool(tc.prefix != ""), func(t *testing.T) {
			t.Parallel()
			gate := "__gash_use_color() { return 1; }; "
			if tc.mode != "off" {
				gate = "__gash_use_color() { return 0; }; "
			}
			shadow := tc.mode == "shadow"
			out, _, _ := run2xGash(t, []string{"LANG=C.UTF-8", "COLUMNS=" + tc.cols}, gate+cardScript(tc.prefix, shadow))
			fields := strings.Split(out, "\x00")
			live := fields[0]
			fields = fields[1:]
			for i, args := range cardCalls {
				if shadow {
					args = append([]string{"--no-color"}, args...)
				}
				env := map[string]string{"LANG": "C.UTF-8", "COLUMNS": tc.cols, "GASH_VERSION": "test", selfmgmt.CardAliasesVar: live}
				o, e, rc := run(t, "gash", env, tc.mode != "off", args...)
				if slices.Contains(args, "--help") || slices.Contains(args, "-h") {
					// 3.0: SELF-12 fixed: the help says which aliases the
					// card lists; the help package tests its rendering.
					require.Contains(t, o, "counts and lists only the aliases the shell", "%q", args)
					require.Equal(t, fields[2*i+1], strconv.Itoa(rc), "%q", args)
					continue
				}
				// 3.0: SHELL-18 fixed: gash_migrate's usage and description
				// are the D-6 pointer (migrateListing, help_test.go), and the
				// gash section's rule follows its widest line.
				// 3.0: DEV-21 fixed: the dev topic's php aliases are the
				// 3.0 pack with conditions, so its alias block and count
				// are left out of the comparison (TestLedger_DEV21).
				norm := func(s string) string {
					s = stripDevAliases(devOverview.ReplaceAllString(s, "${1}N"))
					if !strings.Contains(s, "gash_migrate") {
						return s
					}
					return cardRule.ReplaceAllString(migrateListing.ReplaceAllString(s, "$1"), "$1─")
				}
				requireCardLayout(t, norm(fields[2*i]), norm(o+e), args)
				require.Equal(t, fields[2*i+1], strconv.Itoa(rc), "%q", args)
			}
		})
	}
}

// gash_card_spec.sh "gash all lists every registered function" and "the
// overview counts add up to the registry", over the registry the binary
// holds (the spec reads __GASH_HELP_REGISTRY, which only 2.x has).
func TestCardListsTheRegistry(t *testing.T) {
	env := map[string]string{"LC_ALL": "C", "COLUMNS": "200"}
	all, _, rc := run(t, "gash", env, false, "all", "--no-color")
	require.Equal(t, 0, rc)
	fns := help.Registered()
	require.NotEmpty(t, fns)
	for _, f := range fns {
		require.Regexp(t, `(?m)^  `+regexp.QuoteMeta(f.Name)+`( |$)`, all)
	}
	overview, _, rc := run(t, "gash", env, false, "--no-color")
	require.Equal(t, 0, rc)
	sum := 0
	for _, m := range regexp.MustCompile(`(?m)^  [a-z]+ +([0-9]+) +[0-9]+ `).FindAllStringSubmatch(overview, -1) {
		n, err := strconv.Atoi(m[1])
		require.NoError(t, err)
		sum += n
	}
	require.Equal(t, len(fns), sum)
	require.Contains(t, overview, strconv.Itoa(len(fns))+" functions")
}

// LEDGER SELF-12: from a shell that is not headless the card counts and
// lists only the aliases the shell defines (3.0: SELF-12 fixed; rc.A
// counted 44 git aliases and listed gp and gc although the shadow rule had
// dropped them). Without the live flag (headless, a script) the card still
// shows every registered alias.
func TestLedger_SELF12(t *testing.T) {
	gitCount := regexp.MustCompile(`(?m)^  git +7 +([0-9]+) `)
	// Every git alias of the card but gp and gc, as a shell where programs
	// named gp and gc exist defines them.
	var live strings.Builder
	n := 0
	for _, a := range registry.CardAliases("git") {
		if a.Name != "gp" && a.Name != "gc" && a.Name != a.Value {
			live.WriteString(a.Name + "\t" + a.Value + "\n")
			n++
		}
	}
	require.Equal(t, 42, n)
	env := map[string]string{"LC_ALL": "C", "COLUMNS": "200", selfmgmt.CardAliasesVar: live.String(), selfmgmt.CardLiveVar: "1"}
	overview, _, _ := run(t, "gash", env, false)
	require.Equal(t, "42", gitCount.FindStringSubmatch(overview)[1])
	card, _, _ := run(t, "gash", env, false, "git")
	require.NotRegexp(t, `(?m)  gp +git pull( |$)`, card)
	require.NotRegexp(t, `(?m)  gc +git commit( |$)`, card)
	require.Regexp(t, `(?m)  gst +git status( |$)`, card)

	// A shell with no alias defined: no alias counted or listed.
	env[selfmgmt.CardAliasesVar] = ""
	overview, _, _ = run(t, "gash", env, false)
	require.Equal(t, "0", gitCount.FindStringSubmatch(overview)[1])

	// Headless or a script (no live flag): every registered alias, the
	// values of defined ones from the shell.
	for _, live := range []string{"", "gst\tgit status --short\n"} {
		env := map[string]string{"LC_ALL": "C", "COLUMNS": "200", selfmgmt.CardAliasesVar: live}
		overview, _, _ := run(t, "gash", env, false)
		require.Equal(t, "44", gitCount.FindStringSubmatch(overview)[1])
		card, _, _ := run(t, "gash", env, false, "git")
		require.Regexp(t, `(?m)  gp +git pull( |$)`, card)
		require.Regexp(t, `(?m)  gc +git commit( |$)`, card)
		if live != "" {
			require.Regexp(t, `(?m)  gst +git status --short( |$)`, card)
		}
	}
}

// LEDGER DEV-21: the dev card shows the php pack's families with their
// conditions; a shell that defines php83 and php74 gets those listed (and
// counted) in place of phpXY, and no condition notes.
func TestLedger_DEV21(t *testing.T) {
	devCount := regexp.MustCompile(`(?m)^  dev +[0-9]+ +([0-9]+) `)
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
	overview, _, _ := run(t, "gash", env, false)
	require.Equal(t, "3", devCount.FindStringSubmatch(overview)[1])
}
