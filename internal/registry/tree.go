// Gash 3.5. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

import (
	"slices"
	"sort"
	"strings"
	"sync"
)

// The command tree of 3.5: every function of the catalog has one canonical
// name, `gash NAMESPACE COMMAND` (a third word for a real group: `gash git
// tag add`). The handlers keep their catalog names (the invoke.Table keys),
// so the canonical name, the shortcut and the former flat name of a command
// run the same code and print the same bytes.

// Group places a namespace on the overview page (`gash`).
type Group string

// The groups of the overview, in page order.
const (
	GroupEveryday Group = "EVERYDAY"
	GroupPlatform Group = "PLATFORM"
	GroupAgents   Group = "FOR AI AGENTS"
	GroupGash     Group = "GASH"
)

// Groups are the overview groups in page order.
var Groups = []Group{GroupEveryday, GroupPlatform, GroupAgents, GroupGash}

// Namespace is the first word of a canonical command.
type Namespace struct {
	Name    string
	Summary string
	Group   Group
	// Area is the docs/ folder that documents the namespace.
	Area string
	// Packs are the alias packs whose shortcuts its page shows.
	Packs []string
	// Delegate is the 3.x command whose own tree (cobra, in its handler)
	// serves the words the tree does not list: hte, wsl, win, logs,
	// runtime. Empty for a namespace made of catalog functions only.
	Delegate string
	// Topics are the former card topic words that open its page.
	Topics []string
}

// ArgKind is what a positional argument or a flag value holds: it selects
// the completion of the word.
type ArgKind string

// The argument kinds.
const (
	ArgNone      ArgKind = ""
	ArgAny       ArgKind = "any"       // free text: no completion
	ArgFile      ArgKind = "file"      // a path
	ArgDir       ArgKind = "dir"       // a directory
	ArgHost      ArgKind = "host"      // a remote host alias of ~/.gash_env
	ArgDB        ArgKind = "db"        // a database connection of ~/.gash_env
	ArgAI        ArgKind = "ai"        // an AI provider
	ArgGitTag    ArgKind = "gittag"    // a tag of the repository
	ArgGitRef    ArgKind = "gitref"    // a branch or tag of the repository
	ArgContainer ArgKind = "container" // a Docker container
	ArgUnit      ArgKind = "unit"      // a systemd unit
	ArgProc      ArgKind = "proc"      // a process name
	ArgPort      ArgKind = "port"      // a TCP port in use
	ArgRuntime   ArgKind = "runtime"   // php, node, composer, npm, npx
	ArgFormat    ArgKind = "format"    // json, jsonl, tsv, table
	ArgCommand   ArgKind = "command"   // a program and its arguments
	ArgChoice    ArgKind = "choice"    // one of the command's Choices
)

// Command is one canonical command.
type Command struct {
	// Path are the words after gash: {"git", "tag", "add"}.
	Path []string
	// Handler is the catalog function that runs it (Function.Name).
	Handler string
	// Command3x is the 3.x command that runs it instead of a catalog
	// function (gash llm list runs agent-functions), with Prefix words
	// put before the arguments.
	Command3x string
	// Prefix are words the handler receives before the caller's
	// arguments (none so far for catalog functions).
	Prefix []string
	// Summary is the one line of the pages; empty takes the function's
	// help short description.
	Summary string
	// Usage is the usage line of the pages (the words after the command)
	// for a command with no catalog function; empty takes the help's.
	Usage string
	// Shortcut is the interactive alias that expands to the command.
	Shortcut string
	// Args are the kinds of the positional arguments; the last one
	// repeats.
	Args []ArgKind
	// Values are the kinds of the values of its options, by option name.
	Values map[string]ArgKind
	// Choices are the words an ArgChoice argument takes (the sections of
	// gash system info).
	Choices []string
	// Shell is true when the calling shell keeps a part of the command
	// (cd, history, ssh-agent, nvm, unload): the generated gash function
	// runs it through the shell function of its handler.
	Shell bool
	// Hidden keeps it out of the pages and the completion (a synonym).
	Hidden bool
}

// Name is the canonical name: "gash git tag add".
func (c *Command) Name() string { return "gash " + strings.Join(c.Path, " ") }

// Words is the canonical name without "gash ": "git tag add".
func (c *Command) Words() string { return strings.Join(c.Path, " ") }

// Namespace returns the namespace of the command; ok is false for a
// top-level command of Gash itself (gash doctor).
func (c *Command) Namespace() (Namespace, bool) {
	if len(c.Path) < 2 {
		return Namespace{}, false
	}
	return NamespaceOf(c.Path[0])
}

// Function returns the catalog function of the command.
func (c *Command) Function() (*Function, bool) {
	if c.Handler == "" {
		return nil, false
	}
	return Lookup(c.Handler)
}

// Short is the one-line description of the command.
func (c *Command) Short() string {
	if c.Summary != "" {
		return c.Summary
	}
	if f, ok := c.Function(); ok {
		return f.Help.Short
	}
	return ""
}

// Permanent reports a command whose flat function name is a stable name
// of its own (the llm_* and agent_* tools, decision D2 of 3.5): never a
// former name, never deprecated.
func (c *Command) Permanent() bool {
	return strings.HasPrefix(c.Handler, "llm_") || strings.HasPrefix(c.Handler, "agent_")
}

// formerOf3x are the catalog functions that a 3.x command of the tree
// replaces: gash_help is gash help, gash_upgrade is gash update.
var formerOf3x = map[string]string{"help": "gash_help", "update": "gash_upgrade"}

// Exempt are the catalog functions that stay functions with no canonical
// command: the script helpers (library API for the user's scripts and
// ~/.bash_local), the help wrapper of Bash's builtin, the nvm loader, the
// card itself (gash) and the 1.x route (gash_migrate, a legacy name since
// 4.0: gash migrate stays).
var Exempt = []string{"gash", "help", "nvm", "needs_help", "needs_confirm_prompt", "path_add", "gash_migrate"}

// FormerNames are the flat names of the command in 3.x (the catalog
// function name), empty for a permanent one and for a function that is the
// command's shortcut (gl). Since 4.0 a shell defines them only with
// GASH_LEGACY_NAMES=1 (Legacy), gash_username (Kept) always.
func (c *Command) FormerNames() []string {
	if f, ok := formerOf3x[c.Command3x]; ok && len(c.Path) == 1 {
		return []string{f}
	}
	if c.Handler == "" || c.Permanent() || c.Handler == c.Shortcut {
		// The gl* log functions are their own shortcuts: they stay.
		return nil
	}
	return []string{c.Handler}
}

// Kept are the former flat names that 4.0 still defines in every shell:
// gash_username, which prompts call and the update of a 3.x install runs
// to check the tree it moves to.
var Kept = []string{"gash_username"}

// LegacyVar is the setting that brings the former flat names back in 4.x.
const LegacyVar = "GASH_LEGACY_NAMES"

// Legacy reports a flat function name that 4.0 defines only with
// GASH_LEGACY_NAMES=1: a former name of a command (files_largest), and
// gash_migrate. The shell keeps its function as __gash_fn_NAME, which the
// gash function routes to.
func Legacy(name string) bool {
	if name == "gash_migrate" {
		return true
	}
	if slices.Contains(Kept, name) {
		return false
	}
	c, ok := CommandOf(name)
	return ok && slices.Contains(c.FormerNames(), name)
}

// LegacyNames are the names Legacy reports, sorted.
func LegacyNames() []string {
	out := []string{"gash_migrate"}
	for i := range tree {
		for _, n := range tree[i].FormerNames() {
			if !slices.Contains(Kept, n) && !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

var (
	treeOnce   sync.Once
	byPath     map[string]*Command
	byHandler  map[string]*Command
	byShortcut map[string]*Command
	children   map[string][]string // "git" -> {"sync", "tag", ...}; "" -> top words
)

func buildTree() {
	treeOnce.Do(func() {
		byPath = map[string]*Command{}
		byHandler = map[string]*Command{}
		byShortcut = map[string]*Command{}
		children = map[string][]string{}
		add := func(parent, word string) {
			if !slices.Contains(children[parent], word) {
				children[parent] = append(children[parent], word)
			}
		}
		for _, n := range namespaces {
			add("", n.Name)
		}
		for i := range tree {
			c := &tree[i]
			byPath[c.Words()] = c
			if f, ok := formerOf3x[c.Command3x]; ok && len(c.Path) == 1 {
				byHandler[f] = c
			}
			if c.Handler != "" && !c.Hidden {
				if _, dup := byHandler[c.Handler]; !dup {
					byHandler[c.Handler] = c
				}
			}
			if c.Shortcut != "" {
				byShortcut[c.Shortcut] = c
			}
			if c.Hidden {
				continue
			}
			for j := range c.Path {
				add(strings.Join(c.Path[:j], " "), c.Path[j])
			}
		}
	})
}

// Tree returns every canonical command in tree order. The slice is
// shared: do not modify it.
func Tree() []Command { return tree }

// Namespaces returns the namespaces in overview order.
func Namespaces() []Namespace { return namespaces }

// NamespaceOf returns the namespace named name.
func NamespaceOf(name string) (Namespace, bool) {
	for _, n := range namespaces {
		if n.Name == name {
			return n, true
		}
	}
	return Namespace{}, false
}

// NamespaceOfTopic returns the namespace whose page a word opens: its
// name or a former card topic (nav, dev, php, node).
func NamespaceOfTopic(word string) (Namespace, bool) {
	for _, n := range namespaces {
		if n.Name == word || slices.Contains(n.Topics, word) {
			return n, true
		}
	}
	return Namespace{}, false
}

// Forwards reports a namespace with a tree command that hands the words
// of its 3.x command over as they are (gash runtime which php 8.4 is gash
// runtime php 8.4): the pages and the completion list that command, not
// the 3.x words.
func Forwards(ns Namespace) bool {
	if ns.Delegate == "" {
		return false
	}
	for _, c := range CommandsUnder(ns.Name) {
		if c.Command3x == ns.Delegate && len(c.Prefix) == 0 {
			return true
		}
	}
	return false
}

// CommandAt returns the command whose words are exactly words.
func CommandAt(words ...string) (*Command, bool) {
	buildTree()
	c, ok := byPath[strings.Join(words, " ")]
	return c, ok
}

// CommandOf returns the canonical command a catalog function runs.
func CommandOf(handler string) (*Command, bool) {
	buildTree()
	c, ok := byHandler[handler]
	return c, ok
}

// CommandOfShortcut returns the command a shortcut expands to.
func CommandOfShortcut(name string) (*Command, bool) {
	buildTree()
	c, ok := byShortcut[name]
	return c, ok
}

// Resolve finds the longest command at the start of args (the words
// after gash) and returns it with the words left for it.
func Resolve(args []string) (*Command, []string, bool) {
	buildTree()
	for n := min(len(args), 3); n > 0; n-- {
		if c, ok := byPath[strings.Join(args[:n], " ")]; ok {
			return c, args[n:], true
		}
	}
	return nil, nil, false
}

// Children returns the next words after the given ones, in tree order:
// Children() are the namespaces and the top-level commands.
func Children(words ...string) []string {
	buildTree()
	return slices.Clone(children[strings.Join(words, " ")])
}

// IsGroup reports words that name a group of commands (gash git, gash git
// tag) rather than a command.
func IsGroup(words ...string) bool {
	return len(Children(words...)) > 0
}

// CommandsUnder returns the visible commands whose path starts with
// words, in tree order.
func CommandsUnder(words ...string) []*Command {
	var out []*Command
	for i := range tree {
		c := &tree[i]
		if c.Hidden || len(c.Path) <= len(words) || !slices.Equal(c.Path[:len(words)], words) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// TopCommands are the commands of Gash itself at the first level.
func TopCommands() []*Command {
	var out []*Command
	for i := range tree {
		if c := &tree[i]; len(c.Path) == 1 && !c.Hidden {
			out = append(out, c)
		}
	}
	return out
}

// Shortcuts returns the shortcut of every command, sorted.
func Shortcuts() []string {
	buildTree()
	out := make([]string, 0, len(byShortcut))
	for s := range byShortcut {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// CanonicalName returns the canonical name for any name a user may type:
// a canonical name ("files largest", "gash files largest"), a shortcut
// (flf) or a flat function name (files_largest, llm_read).
func CanonicalName(name string) (*Command, bool) {
	name = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), "gash "))
	if c, ok := CommandAt(strings.Fields(name)...); ok {
		return c, true
	}
	if c, ok := CommandOfShortcut(name); ok {
		return c, true
	}
	if c, ok := CommandOf(name); ok {
		return c, true
	}
	if f, ok := ByAlias(name); ok {
		return CommandOf(f.Name)
	}
	return nil, false
}
