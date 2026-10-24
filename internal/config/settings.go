// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package config

import "strings"

// Expansion is how the 2.x reader of a setting falls back to its default.
type Expansion int

const (
	// ColonDash is ${NAME:-default}: empty or unset gives the default.
	ColonDash Expansion = iota
	// Dash is ${NAME-default}: only unset gives the default.
	Dash
)

// SettingInfo describes one GASH_* setting of __GASH_SETTINGS_KNOWN.
type SettingInfo struct {
	Name    string
	Default string // "" where the reader has no default, or keeps it in its own package (AI models)
	Expand  Expansion
	// Headless: gash.sh applies this setting from the file in headless mode
	// (__gash_apply_settings GASH_DB_TIMEOUT GASH_AI_CONFIRM
	// GASH_AI_MODEL_CLAUDE GASH_AI_MODEL_GEMINI).
	Headless bool
	// FileOnly: read with __gash_get_setting, from the file and never from
	// the environment (llm_exec, MUST NOT 5).
	FileOnly bool
}

// Known is __GASH_SETTINGS_KNOWN in its 2.x order, with the default and the
// expansion each 2.x reader uses.
var Known = []SettingInfo{
	{Name: "GASH_PROMPT", Default: "1"},
	{Name: "GASH_BANNER", Default: "1"},
	{Name: "GASH_HISTORY", Default: "1"},
	{Name: "GASH_HISTORY_SYNC", Default: "0"},
	{Name: "GASH_REPLACE_TOOLS", Default: "0"},
	{Name: "GASH_SAFETY_ALIASES", Default: "1"},
	{Name: "GASH_UMASK", Expand: Dash},
	{Name: "GASH_GIT_EXCLUDE", Expand: Dash},
	{Name: "GASH_SHOW_INXI", Default: "0", Expand: Dash},
	{Name: "GASH_AI_MODEL_CLAUDE", Headless: true},
	{Name: "GASH_AI_MODEL_GEMINI", Headless: true},
	{Name: "GASH_AI_CONFIRM", Default: "auto", Headless: true},
	{Name: "GASH_DB_TIMEOUT", Default: "30", Headless: true},
	{Name: "GASH_LLM_EXEC", FileOnly: true},
	{Name: "GASH_LLM_EXEC_ALLOW", FileOnly: true},
	{Name: "GASH_LLM_SANDBOX", FileOnly: true},
	{Name: "GASH_NVM", Default: "off"},
	{Name: "GASH_PATH_ADD", Expand: Dash},
}

// SplitPathAdd splits a GASH_PATH_ADD value into words on space, tab and
// newline, regardless of the caller's IFS and with no glob expansion
// (forced divergence SHELL-07): 2.x word-split the unquoted value on the
// caller's IFS and pathname-expanded it, which depended on shell state and
// the filesystem at load and cannot be reproduced from the binary.
func SplitPathAdd(v string) []string {
	return strings.FieldsFunc(v, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n'
	})
}

// Info returns the description of a known setting.
func Info(name string) (SettingInfo, bool) {
	for _, s := range Known {
		if s.Name == name {
			return s, true
		}
	}
	return SettingInfo{}, false
}

// IsKnown reports whether name is in __GASH_SETTINGS_KNOWN.
func IsKnown(name string) bool {
	_, ok := Info(name)
	return ok
}

// SetVar is the variable through which the compat stub names the settings
// that are set in the caller's shell: a whitespace-separated list of names,
// whose values travel in variables of the same names (compat.md 3). "Set to
// empty" is in the list, "unset" is not.
const SetVar = "GASH__SET"

// Caller holds the settings that are set in the calling shell.
type Caller struct {
	values map[string]string
}

// CallerFromEnviron reads the caller's settings from the binary's
// environment ("NAME=value" entries, as os.Environ returns them).
//
// With GASH__SET present (the compat stub), a setting is set exactly when
// GASH__SET names it. Without it (direct invocation), a setting is set when
// the environment holds it. File-only settings are never taken from the
// environment either way.
func CallerFromEnviron(environ []string) Caller {
	env := make(map[string]string, len(environ))
	for _, kv := range environ {
		if i := strings.IndexByte(kv, '='); i > 0 {
			if _, dup := env[kv[:i]]; !dup {
				env[kv[:i]] = kv[i+1:] // the first one wins, as in os.Getenv
			}
		}
	}
	c := Caller{values: map[string]string{}}
	if list, ok := env[SetVar]; ok {
		for _, name := range strings.Fields(list) {
			if info, known := Info(name); known && !info.FileOnly {
				c.values[name] = env[name]
			}
		}
		return c
	}
	for _, info := range Known {
		if v, ok := env[info.Name]; ok && !info.FileOnly {
			c.values[info.Name] = v
		}
	}
	return c
}

// Lookup returns the caller's value of a setting and whether it is set.
func (c Caller) Lookup(name string) (string, bool) {
	v, ok := c.values[name]
	return v, ok
}

// Source says where a setting's value came from.
type Source int

const (
	SourceNone Source = iota
	SourceCaller
	SourceFile
)

// Settings resolves GASH_* settings with the 2.x precedence.
type Settings struct {
	file     *File
	caller   Caller
	headless bool
}

// NewSettings combines the caller's settings and the file. headless is
// GASH_HEADLESS=1 in the caller: 2.x then applied only the headless subset
// from the file.
func NewSettings(f *File, c Caller, headless bool) *Settings {
	if f == nil {
		f = &File{}
	}
	return &Settings{file: f, caller: c, headless: headless}
}

// Lookup returns a setting's value and where it came from (SourceNone and
// "" when unset). File-only settings come from the file alone. Others: the
// caller's value when set, else the first file line when 2.x would have
// applied the file in this mode.
func (s *Settings) Lookup(name string) (string, Source) {
	info, ok := Info(name)
	if !ok {
		return "", SourceNone
	}
	if !info.FileOnly {
		if v, set := s.caller.Lookup(name); set {
			return v, SourceCaller
		}
		if s.headless && !info.Headless {
			return "", SourceNone
		}
	}
	if v, set := s.file.Setting(name); set {
		return v, SourceFile
	}
	return "", SourceNone
}

// Get returns the value the 2.x reader of the setting saw, with its default
// applied by its expansion (${NAME:-d} or ${NAME-d}).
func (s *Settings) Get(name string) string {
	v, src := s.Lookup(name)
	info, _ := Info(name)
	switch info.Expand {
	case Dash:
		if src == SourceNone {
			return info.Default
		}
	default:
		if v == "" {
			return info.Default
		}
	}
	return v
}
