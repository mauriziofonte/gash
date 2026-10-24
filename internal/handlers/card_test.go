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
				require.Equal(t, fields[2*i], o+e, "%q", args)
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
