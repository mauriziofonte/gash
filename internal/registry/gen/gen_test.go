// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"
)

const repoRoot = "../../.."

func needs2x(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repoRoot, "lib/core/help.sh")); err != nil {
		t.Skip("the 2.x sources are not in this tree")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
}

// run2x sources the 2.x gash.sh of the checkout in a scrubbed headless
// (or interactive) shell with a temporary HOME and runs script.
func run2x(t *testing.T, interactive bool, script string) []byte {
	t.Helper()
	root, err := filepath.Abs(repoRoot)
	require.NoError(t, err)
	args := []string{"--noprofile", "--norc"}
	env := []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "LANG=C.UTF-8", "TERM=dumb"}
	if interactive {
		args = append(args, "-i")
	} else {
		env = append(env, "GASH_HEADLESS=1")
	}
	args = append(args, "-c", `source "$1/gash.sh" >/dev/null 2>&1; `+script, "_", root)
	cmd := exec.Command("bash", args...)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	require.NoError(t, cmd.Run())
	return out.Bytes()
}

// The extracted help entries are byte for byte what 2.x registers.
func TestHelpMatchesThe2xRegistry(t *testing.T) {
	needs2x(t)
	c, err := extract(repoRoot)
	require.NoError(t, err)
	out := run2x(t, false, `for f in "${!__GASH_HELP_REGISTRY[@]}"; do
		printf '%s\0%s\0%s\0%s\0%s\0%s\0' "$f" "${__GASH_HELP_REGISTRY[$f]}" "${__GASH_HELP_SHORT[$f]-}" \
			"${__GASH_HELP_ALIASES[$f]-}" "${__GASH_HELP_MODULE[$f]-}" "${__GASH_HELP_SEE_ALSO[$f]-}"
	done`)
	fields := strings.Split(string(out), "\x00")
	fields = fields[:len(fields)-1]
	require.Equal(t, 0, len(fields)%6)
	seen := map[string]bool{}
	for i := 0; i < len(fields); i += 6 {
		name := fields[i]
		seen[name] = true
		h, ok := c.helps[name]
		require.True(t, ok, "2.x registers help for %s, gen does not", name)
		require.Equal(t, fields[i+1], h.body, name)
		require.Equal(t, fields[i+2], h.short, name)
		require.Equal(t, fields[i+3], h.aliases, name)
		require.Equal(t, fields[i+4], h.module, name)
		require.Equal(t, fields[i+5], h.seeAlso, name)
	}
	for name := range c.helps {
		require.True(t, seen[name], "gen registers help for %s, 2.x does not", name)
	}
}

// The public functions are those that `declare -F` lists after a 2.x load.
func TestFunctionsMatchDeclareF(t *testing.T) {
	needs2x(t)
	c, err := extract(repoRoot)
	require.NoError(t, err)
	fns, err := c.build()
	require.NoError(t, err)
	public := func(b []byte) []string {
		var out []string
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			name := strings.TrimPrefix(l, "declare -f ")
			if !strings.HasPrefix(name, "_") {
				out = append(out, name)
			}
		}
		sort.Strings(out)
		return out
	}
	var headless, all []string
	for _, f := range fns {
		all = append(all, f.Name)
		if f.Headless {
			headless = append(headless, f.Name)
		}
	}
	require.Equal(t, headless, public(run2x(t, false, "declare -F")))
	// Interactive: every function but the nvm stub, which 2.x defines only
	// with GASH_NVM=lazy and an nvm install.
	got := public(run2x(t, true, "declare -F"))
	var want []string
	for _, n := range all {
		if n != "nvm" {
			want = append(want, n)
		}
	}
	require.Equal(t, want, got)
}

// Module short aliases are the aliases 2.x defines in headless mode.
func TestModuleAliasesMatch2x(t *testing.T) {
	needs2x(t)
	c, err := extract(repoRoot)
	require.NoError(t, err)
	var want []string
	for _, a := range c.aliases {
		if a.pack == "" && a.condition == "" {
			want = append(want, "alias "+a.name+"='"+a.function+"'")
		}
	}
	sort.Strings(want)
	var got []string
	for _, l := range strings.Split(strings.TrimSpace(string(run2x(t, false, "alias"))), "\n") {
		if !strings.HasPrefix(l, "alias hg=") { // conditional on Mercurial
			got = append(got, l)
		}
	}
	sort.Strings(got)
	require.Equal(t, want, got)
}

// Every expansion form of a shell variable is a read, and the variables
// naming the calling shell, which Bash sets without exporting them, reach
// the stubs of the functions that read them (MATRIX row "BASH
// BASH_VERSION ZSH_VERSION"; llm_exec runs its child with ${BASH:-bash}).
func TestCallerShellVars(t *testing.T) {
	needs2x(t)
	c, err := extract(repoRoot)
	require.NoError(t, err)
	fns, err := c.build()
	require.NoError(t, err)
	shell := map[string]bool{"BASH": true, "BASH_VERSION": true, "ZSH_VERSION": true}
	got := map[string][]string{}
	for _, f := range fns {
		for _, v := range f.ShellVars {
			if shell[v] {
				got[f.Name] = append(got[f.Name], v)
			}
		}
	}
	require.Equal(t, map[string][]string{
		"ai_ask":      {"BASH_VERSION", "ZSH_VERSION"},
		"ai_query":    {"BASH_VERSION", "ZSH_VERSION"},
		"gash_doctor": {"BASH", "BASH_VERSION"},
		"llm_exec":    {"BASH"},
	}, got)

	r := newRefs()
	f, err := syntax.NewParser().Parse(strings.NewReader(`f() { a=${A:-x} b=${B-} c=${C:+y} d=${#D} e=${E%/*}; }`), "")
	require.NoError(t, err)
	r.collect(f)
	for _, v := range []string{"A", "B", "C", "D", "E"} {
		require.True(t, r.vars[v], v)
	}
}

// A MATRIX Name cell is a list of variable names, or prose that names
// none (its words, such as git or in, are not variables).
func TestMatrixNames(t *testing.T) {
	require.Equal(t, []string{"LANG", "LC_ALL", "LC_CTYPE"}, names("LANG LC_ALL LC_CTYPE"))
	require.Equal(t, []string{"HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy"}, names("HTTPS_PROXY/https_proxy, NO_PROXY/no_proxy"))
	require.Equal(t, []string{"GASH_HEADLESS"}, names("GASH_HEADLESS (node_upgrade, ai_ask)"))
	require.Equal(t, []string{"HTTPS_PROXY", "ALL_PROXY"}, names("HTTPS_PROXY, ALL_PROXY (and lowercase)"))
	require.Nil(t, names("GIT_* environment and git config"))
	require.Nil(t, names("${VAR} in compose image references"))
	require.Nil(t, names("__GASH_GLYPH_* and __GASH_SYMBOL_*"))
}

// catalog_gen.go is what gen writes from the 2.x sources and MATRIX.md
// today: run `go generate ./internal/registry` after changing either.
func TestCatalogIsCurrent(t *testing.T) {
	needs2x(t)
	c, err := extract(repoRoot)
	require.NoError(t, err)
	want, err := c.render()
	require.NoError(t, err)
	got, err := os.ReadFile("../catalog_gen.go")
	require.NoError(t, err)
	if !bytes.Equal(want, got) {
		t.Fatal("internal/registry/catalog_gen.go is stale: run `go generate ./internal/registry`")
	}
}

// help_spec.sh "rejects empty func name": a registration without a
// name stores nothing, as 2.x returned 1 and stored nothing.
func TestHelpCallSkipsEmptyName(t *testing.T) {
	dir := t.TempDir()
	src := "__gash_register_help \"\" --short test <<'HELP'\nbody\nHELP\n" +
		"__gash_register_help x --short s --aliases a,b --module m --see-also \"y z\" <<'HELP'\nUSAGE\n  x\n\nHELP\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "h.sh"), []byte(src), 0o644))
	s, err := parseSource(dir, "h.sh")
	require.NoError(t, err)
	c := &catalog{defs: map[string][]*fnDef{}, helps: map[string]*helpEntry{}}
	c.scan(s, s.file.Stmts, scope{})
	require.Len(t, c.helps, 1)
	h := c.helps["x"]
	require.Equal(t, helpEntry{name: "x", file: "h.sh", line: 4, body: "USAGE\n  x", short: "s", aliases: "a,b", module: "m", seeAlso: "y z"}, *h)
}

// The card entries of every topic alias file are what the awk program of
// __gash_card_aliases prints: with no alias defined, the 2.x function
// prints them all but the identity entries ("ls -> ls").
func TestCardAliasesMatch2x(t *testing.T) {
	needs2x(t)
	c, err := extract(repoRoot)
	require.NoError(t, err)
	require.NotEmpty(t, c.cardAliases)
	for f, list := range c.cardAliases {
		require.Equal(t, cardLines(list), string(run2x(t, false, "unalias -a; __gash_card_aliases '"+f+"'")), f)
	}
}

// cardLines is what __gash_card_aliases prints for list in a shell that
// defines none of its aliases.
func cardLines(list []cardAlias) string {
	var b strings.Builder
	for _, a := range list {
		if a.value != a.name {
			b.WriteString(a.name + "\t" + a.value + "\n")
		}
	}
	return b.String()
}

// The awk rules one by one, against the 2.x function over the same file:
// comments name the function below them, a trailing comment and one pair
// of quotes leave the value, load-time variables show what they hold,
// names with $ and empty values are skipped, a docker alias call gives
// its name and command, a name counts once.
func TestCardAliasesOf(t *testing.T) {
	src := `# Compact log
function gl {
    :
}
function help {
x=1
function gx {
alias a='ls -l'  # list
alias b="echo $__gash_composer1 $__gash_composer x"
alias c=$__gash_hte' create'
alias d="$__gash_php_latest -v $__gash_bin $__gash_other"
alias $e=x
alias f=''
alias a='again'
__gash_add_docker_alias "docker" "dps" "docker ps"
\t# indented comment
alias g=plain
alias h=h
__gash_add_docker_alias "docker" "dps" "other"
function gy { # comment
`
	got := cardAliasesOf(src)
	require.Equal(t, []cardAlias{
		{"gl", "Compact log"}, {"gx", ""}, {"a", "ls -l"}, {"b", "echo composer1 composer x"},
		{"c", "sudo phpX.Y hte-cli' create'"}, {"d", "phpX.Y -v phpX.Y"}, {"dps", "docker ps"}, {"g", "plain"},
		{"h", "h"}, {"gy", ""},
	}, got)
	needs2x(t)
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "lib/aliases"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "lib/aliases/x.sh"), []byte(src), 0o644))
	require.Equal(t, cardLines(got), string(run2x(t, false, "unalias -a; GASH_DIR='"+dir+"' __gash_card_aliases x")))
}
