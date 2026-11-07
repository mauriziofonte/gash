// Gash 3.5. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package complete

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/runner"
)

func vals(r Reply) []string {
	var out []string
	for _, c := range r.Cands {
		out = append(out, c.Value)
	}
	return out
}

func words(line string) ([]string, string) {
	f := strings.Split(line, " ")
	return f[:len(f)-1], f[len(f)-1]
}

func ask(t *testing.T, env Env, line string) Reply {
	t.Helper()
	w, p := words(line)
	return Words(w, p, env)
}

func TestTreeWords(t *testing.T) {
	env := Env{}
	r := ask(t, env, "")
	require.Equal(t, "files", r.Cands[0].Value)
	require.Equal(t, registry.Namespaces()[0].Summary, r.Cands[0].Desc)
	require.Contains(t, vals(r), "doctor")
	require.Equal(t, DirNoFileComp, r.Directive)

	require.Equal(t, []string{"git"}, vals(ask(t, env, "gi")))
	require.Equal(t, []string{"add", "delete", "list"}, vals(ask(t, env, "git tag ")))
	r = ask(t, env, "git ")
	require.Contains(t, vals(r), "tag")
	for _, c := range r.Cands {
		if c.Value == "tag" {
			require.Equal(t, "3 commands", c.Desc)
		}
	}
	// A command that is a group too: its subcommands.
	require.Equal(t, []string{"all"}, vals(ask(t, env, "git log a")))
	// An unknown word: nothing, no files.
	r = ask(t, env, "nosuch ")
	require.Empty(t, r.Cands)
	require.Equal(t, DirNoFileComp, r.Directive)
}

func TestOptionsAndValues(t *testing.T) {
	env := Env{}
	r := ask(t, env, "files largest --li")
	require.Equal(t, []Candidate{{"--limit", "Cap results (default 100)"}}, r.Cands)
	// "-" offers each option once, by its long name.
	r = ask(t, env, "files largest -")
	require.Contains(t, vals(r), "--limit")
	require.NotContains(t, vals(r), "-n")
	require.Equal(t, "-n", vals(ask(t, env, "files largest -n"))[0])
	// An option given is not offered again.
	require.NotContains(t, vals(ask(t, env, "files largest --limit 5 --")), "--limit")
	// A value that is a number: nothing, no files.
	r = ask(t, env, "files largest --limit ")
	require.Empty(t, r.Cands)
	require.Equal(t, DirNoFileComp, r.Directive)
	// The positional of a directory kind.
	require.Equal(t, DirFilterDirs, ask(t, env, "files largest --limit 5 ").Directive)
	// --format and --format=.
	require.Equal(t, []string{"json", "jsonl"}, vals(ask(t, env, "llm grep --format j")))
	require.Equal(t, []string{"--format=json", "--format=jsonl"}, vals(ask(t, env, "llm grep --format=j")))
	// Choices of an argument and of an option's value (name|mtime|size).
	require.Equal(t, []string{"webstack"}, vals(ask(t, env, "system info we")))
	r = ask(t, env, "llm find --sort ")
	require.Equal(t, []string{"name", "mtime", "size"}, vals(r))
	// The options a USAGE synopsis alone names.
	require.Contains(t, vals(ask(t, env, "docker prune-all -")), "--dry-run")
	require.Contains(t, vals(ask(t, env, "remote exec --s")), "--script")
}

func TestCommandLines(t *testing.T) {
	env := Env{}
	require.Equal(t, 2, ask(t, env, "run -- ").Offset)
	require.Equal(t, 4, ask(t, env, "agent test --fail-only -- ").Offset)
	// llm exec's CMD is its positional.
	require.Equal(t, 2, ask(t, env, "llm exec ").Offset)
	// Without "--" gash run's options go to its own completer.
	r := ask(t, env, "run --wi")
	require.Equal(t, []string{"run", "--wi"}, r.Relay)
}

func TestRelays(t *testing.T) {
	env := Env{}
	require.Equal(t, []string{"hte", "site", ""}, ask(t, env, "hte site ").Relay)
	require.Equal(t, []string{"update", "--r"}, ask(t, env, "update --r").Relay)
	require.Equal(t, []string{"runtime", "php", ""}, ask(t, env, "runtime which php ").Relay)
	// A delegate namespace merges its 3.x words with the tree's.
	env.Delegate = func(name string, args []string) []Candidate {
		require.Equal(t, "wsl", name)
		return []Candidate{{"info", "A quick summary"}, {"restart", "dup"}}
	}
	r := ask(t, env, "wsl ")
	require.Equal(t, []string{"restart", "shutdown", "compact", "info"}, vals(r))
	// gash runtime which stands for the words runtime lists bare.
	env.Delegate = func(string, []string) []Candidate { return []Candidate{{"php", ""}, {"node", ""}} }
	require.NotContains(t, vals(ask(t, env, "runtime ")), "php")
}

func TestHelpWords(t *testing.T) {
	env := Env{}
	require.Equal(t, []string{"git"}, vals(ask(t, env, "help gi")))
	require.Equal(t, []string{"sync"}, vals(ask(t, env, "help git sy")))
	require.Equal(t, []string{"shortcuts", "shell-defaults"}, vals(ask(t, env, "help sh"))[1:])
	require.Equal(t, []string{"--search", "--short"}, vals(ask(t, env, "help --s")))
	require.Empty(t, ask(t, env, "help --search ").Cands)
}

func TestProviders(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gash_env"), []byte(
		"HOST:web=deploy@web.example.com\nHOST:db1=root@10.0.0.5\nDB:main=mysql://app:s3cret@localhost:3306/app\n"), 0o600))
	env := Env{Get: func(n string) string {
		if n == "HOME" {
			return home
		}
		return ""
	}}
	r := ask(t, env, "remote exec ")
	require.Equal(t, []Candidate{{"web", "deploy@web.example.com"}, {"db1", "root@10.0.0.5"}}, r.Cands)
	require.Equal(t, []string{"web"}, vals(ask(t, env, "llm read --host w")))
	r = ask(t, env, "db test ")
	require.Len(t, r.Cands, 1)
	require.Equal(t, "main", r.Cands[0].Value)
	require.NotContains(t, r.Cands[0].Desc, "s3cret", "a password never shows")

	env.Run = runner.NewFake(t, runner.Expect{Argv: []string{"git", "tag", "-l", "--sort=-creatordate"}, Stdout: "v2.0\nv1.0\n"})
	require.Equal(t, []string{"v2.0", "v1.0"}, vals(ask(t, env, "git tag delete ")))
	env.Run = runner.NewFake(t, runner.Expect{Argv: []string{"docker", "ps", "-a", "--format", "{{.Names}}\t{{.Status}}"}, Stdout: "web\tUp 2 hours\napi\tExited (0)\n"})
	require.Equal(t, []Candidate{{"web", "Up 2 hours"}}, ask(t, env, "llm docker-logs w").Cands)
	// A program that fails lists nothing.
	env.Run = runner.NewFake(t, runner.Expect{Argv: []string{"git", "tag", "-l", "--sort=-creatordate"}, Status: 128})
	require.Empty(t, ask(t, env, "git tag delete ").Cands)
}

func TestWrite(t *testing.T) {
	var b bytes.Buffer
	Write(&b, Reply{Cands: []Candidate{{"a", "the\tfirst\nline"}, {"b", ""}}, Directive: 4, Offset: -1}, true)
	require.Equal(t, "a\tthe first\nb\n:4\n", b.String())
	b.Reset()
	Write(&b, Reply{Cands: []Candidate{{"a", "x"}}, Offset: 2}, false)
	require.Equal(t, "a\n!offset 2\n:0\n", b.String())
}

// LEDGER SHELL-03 (moved here in 3.5, where the binary answers): TAB after
// gash offers the namespaces and Gash's own commands, the words gash
// accepts, and never migrate (it only points at the 2.x route).
func TestLedger_SHELL03(t *testing.T) {
	got := vals(Words(nil, "", Env{}))
	require.Equal(t, registry.Children(), got)
	require.NotContains(t, got, "migrate")
	for _, w := range got {
		_, isNS := registry.NamespaceOf(w)
		_, isCmd := registry.CommandAt(w)
		require.True(t, isNS || isCmd || registry.IsGroup(w), w)
	}
}
