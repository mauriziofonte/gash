// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package registry is the declarative catalog of Gash: every 2.x public
// function with its module, aliases, class, help text and the caller
// state it reads, the alias packs, the settings and the card topics.
//
// The data in catalog_gen.go is produced at development time from the 2.x
// sources by ./gen (go generate) and committed; the binary never reads
// Bash. The cobra tree, the generated compat Bash and the help output are
// built from it.
package registry

//go:generate go run ./gen -root ../../.. -out catalog_gen.go

// Class is the MATRIX class of a function.
type Class string

// Classes of go-impl/PORT/MATRIX.md.
const (
	PureOutput    Class = "pure output"
	ShellMutating Class = "shell-mutating"
	Interactive   Class = "interactive"
)

// Impl says where a function's code lives in 3.x.
type Impl string

const (
	// ImplBinary: a one-line stub calls the binary (gash NAME).
	ImplBinary Impl = "binary"
	// ImplShim: a Bash function in a generated core shim file
	// (needs_help, needs_confirm_prompt, path_add).
	ImplShim Impl = "shim"
	// ImplInit: Bash emitted by gash init bash, because it acts on the
	// calling shell (help, nvm, gash_unload).
	ImplInit Impl = "init"
)

// Help is a function's entry of the 2.x help registry, exactly as
// __gash_register_help stored it (Body without trailing newlines).
type Help struct {
	Body    string
	Short   string
	Aliases string // comma-separated, as registered
	Module  string
	SeeAlso string // space-separated, as registered
}

// Usage is the needs_help fallback a function passes (program, usage,
// description): what 2.x printed for --help when the help registry was not
// loaded.
type Usage struct {
	Program     string
	Usage       string
	Description string
}

// Function is one 2.x public function.
type Function struct {
	Name string
	// Module is the help registry module (or, without help, the 2.x
	// module file name).
	Module string
	// Topic is the reference card topic that lists the module.
	Topic string
	// File is the 2.x source file that defines the function.
	File string
	// StubFile is the 2.x file a caller sources to get the function (a
	// part file under lib/modules/llm/ or lib/modules/ai/ maps to its
	// loader); the generated shim of that path defines the stub.
	StubFile string
	Class    Class
	// Decision is the MATRIX decision: port, redesign or drop.
	Decision string
	Impl     Impl
	// Headless is true when 2.x defines the function in headless mode too;
	// false for the interactive-only ones (alias pack functions, the nvm
	// stub).
	Headless bool
	// Aliases are the module short aliases that point to it.
	Aliases []string
	// HasHelp reports whether the function registers help.
	HasHelp bool
	Help    Help
	// Fallback is the needs_help triple of its --help path, if any.
	Fallback *Usage
	// Settings are the settings of __GASH_SETTINGS_KNOWN that its 2.x code
	// reads from the shell (passed by the stub, with GASH__SET).
	Settings []string
	// ShellVars are the shell variables, beyond CommonShellVars, that its
	// 2.x code reads and the stub passes.
	ShellVars []string
	// Env are the MATRIX environment variables its 2.x code reads
	// (informational: what the Go port must honor).
	Env []string
	// LoadsEnv reports that its 2.x body reaches __gash_load_env (directly
	// or through the config.sh helpers): its stub runs the shim's cached
	// __gash_load_env first, so the settings warnings come once per shell
	// from the shell, as in 2.x (LEDGER CORE-18).
	LoadsEnv bool
	// Completion is the body of its 2.x __gash_complete case branch
	// (Bash, dedented), empty when 2.x completed it with the default.
	Completion string
	// CompletionComment is the comment line above that branch.
	CompletionComment string
}

// Alias is a module short alias or an alias pack entry.
type Alias struct {
	Name string
	// Definition is the alias argument as written in 2.x (name=value,
	// quoting kept); for pack aliases it may reference load-time
	// variables.
	Definition string
	// Function is the target of a module short alias.
	Function string
	// Pack is the alias pack (lib/aliases/<pack>.sh); empty for module
	// short aliases.
	Pack string
	// Condition is the Bash condition under which 2.x defined it (the
	// enclosing if and && / || tests, joined with &&); empty when always.
	Condition string
	// Source is file:line of the 2.x definition.
	Source string
}

// Pack is an alias pack, in 2.x load order.
type Pack struct {
	Name string
	File string
}

// Setting is a ~/.gash_env GASH_* setting.
type Setting struct {
	Name string
	// Headless is true for the subset 2.x applied in headless mode.
	Headless bool
	// FileOnly is true for the settings read from the file only, never
	// from the environment or the stub (GASH_LLM_EXEC and friends).
	FileOnly bool
	// Passed is true when the stub passes the caller's value.
	Passed bool
}

// Topic is a reference card topic (__GASH_CARD_TOPICS).
type Topic struct {
	Name        string
	Modules     []string
	AliasFiles  []string
	Description string
}

// CardAlias is one entry of the reference card for an alias file: an
// alias with its value as the file writes it (unquoted, load-time
// variables shown by what they hold), or a function with the comment
// above it, as __gash_card_aliases parsed lib/aliases/FILE.sh.
type CardAlias struct {
	Name  string
	Value string
}

// CommonShellVars are passed by every stub (compat.md 3): mode and
// output, locations, and the locale that selects the glyph set.
var CommonShellVars = []string{
	"GASH_HEADLESS", "NO_COLOR", "GASH_NO_COLOR", "GASH_DEBUG", "GASH_TUI", "COLUMNS",
	"GASH_DIR", "GASH_ENV_FILE", "LANG", "LC_ALL", "LC_CTYPE",
}
