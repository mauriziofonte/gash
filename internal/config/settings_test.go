// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package config

import (
	"reflect"
	"testing"
)

func TestKnownIsTheSettingsList(t *testing.T) {
	// __GASH_SETTINGS_KNOWN, in its order.
	want := []string{"GASH_PROMPT", "GASH_BANNER", "GASH_HISTORY", "GASH_HISTORY_SYNC", "GASH_REPLACE_TOOLS",
		"GASH_SAFETY_ALIASES", "GASH_UMASK", "GASH_GIT_EXCLUDE", "GASH_SHOW_INXI", "GASH_AI_MODEL_CLAUDE",
		"GASH_AI_MODEL_GEMINI", "GASH_AI_CONFIRM", "GASH_DB_TIMEOUT", "GASH_LLM_EXEC", "GASH_LLM_EXEC_ALLOW",
		"GASH_LLM_SANDBOX", "GASH_NVM", "GASH_PATH_ADD"}
	var got, headless, fileOnly []string
	for _, s := range Known {
		got = append(got, s.Name)
		if s.Headless {
			headless = append(headless, s.Name)
		}
		if s.FileOnly {
			fileOnly = append(fileOnly, s.Name)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Known = %q", got)
	}
	// gash.sh: __gash_apply_settings GASH_DB_TIMEOUT GASH_AI_CONFIRM GASH_AI_MODEL_CLAUDE GASH_AI_MODEL_GEMINI
	if !reflect.DeepEqual(headless, []string{"GASH_AI_MODEL_CLAUDE", "GASH_AI_MODEL_GEMINI", "GASH_AI_CONFIRM", "GASH_DB_TIMEOUT"}) {
		t.Errorf("headless subset = %q", headless)
	}
	if !reflect.DeepEqual(fileOnly, []string{"GASH_LLM_EXEC", "GASH_LLM_EXEC_ALLOW", "GASH_LLM_SANDBOX"}) {
		t.Errorf("file-only = %q", fileOnly)
	}
	if IsKnown("GASH_NOPE") || !IsKnown("GASH_NVM") {
		t.Error("IsKnown")
	}
}

func settingsFile(lines string) *File { return Parse([]byte(lines), "/e", "/h") }

// settings_spec.sh:29: a variable set in the caller wins over the file.
func TestSettingsCallerWins(t *testing.T) {
	f := settingsFile("GASH_BANNER=0\nGASH_HISTORY=0\n")
	s := NewSettings(f, CallerFromEnviron([]string{"GASH_BANNER=1"}), false)
	if v, src := s.Lookup("GASH_BANNER"); v != "1" || src != SourceCaller {
		t.Errorf("GASH_BANNER = %q %v", v, src)
	}
	if v, src := s.Lookup("GASH_HISTORY"); v != "0" || src != SourceFile {
		t.Errorf("GASH_HISTORY = %q %v", v, src)
	}
}

// The first line of a setting wins (__gash_apply_settings skips a name once
// it is set; __gash_get_setting returns the first match).
func TestSettingsFirstLineWins(t *testing.T) {
	s := NewSettings(settingsFile("GASH_DB_TIMEOUT=5\nGASH_DB_TIMEOUT=9\nGASH_LLM_EXEC_ALLOW=a\nGASH_LLM_EXEC_ALLOW=b\n"), Caller{}, false)
	if got := s.Get("GASH_DB_TIMEOUT"); got != "5" {
		t.Errorf("GASH_DB_TIMEOUT = %q", got)
	}
	if got := s.Get("GASH_LLM_EXEC_ALLOW"); got != "a" {
		t.Errorf("GASH_LLM_EXEC_ALLOW = %q", got)
	}
}

// settings_spec.sh:125: headless mode applies only the function settings of
// the file, and the caller still wins.
func TestSettingsHeadlessSubset(t *testing.T) {
	f := settingsFile("GASH_DB_TIMEOUT=5   # seconds\nGASH_AI_CONFIRM=0\nGASH_AI_MODEL_CLAUDE=claude-x\nGASH_AI_MODEL_GEMINI=gemini-x\nGASH_PROMPT=0\nGASH_UMASK=077\nGASH_HISTORY=0\nGASH_NOPE=1\n")
	s := NewSettings(f, Caller{}, true)
	got := []string{}
	for _, n := range []string{"GASH_DB_TIMEOUT", "GASH_AI_CONFIRM", "GASH_AI_MODEL_CLAUDE", "GASH_AI_MODEL_GEMINI", "GASH_PROMPT", "GASH_UMASK", "GASH_HISTORY"} {
		v, src := s.Lookup(n)
		if src == SourceNone {
			v = "unset"
		}
		got = append(got, v)
	}
	if want := []string{"5", "0", "claude-x", "gemini-x", "unset", "unset", "unset"}; !reflect.DeepEqual(got, want) {
		t.Errorf("headless = %q, want %q", got, want)
	}

	s = NewSettings(f, CallerFromEnviron([]string{"GASH_DB_TIMEOUT=9", "GASH_AI_CONFIRM=1"}), true)
	if s.Get("GASH_DB_TIMEOUT") != "9" || s.Get("GASH_AI_CONFIRM") != "1" || s.Get("GASH_AI_MODEL_CLAUDE") != "claude-x" {
		t.Errorf("caller first: %q %q", s.Get("GASH_DB_TIMEOUT"), s.Get("GASH_AI_CONFIRM"))
	}

	// Interactive: every setting of the file applies.
	s = NewSettings(f, Caller{}, false)
	if s.Get("GASH_PROMPT") != "0" || s.Get("GASH_UMASK") != "077" || s.Get("GASH_HISTORY") != "0" {
		t.Error("interactive mode must apply the whole file")
	}
}

// File-only settings are read from the file and never from the caller:
// setting GASH_LLM_EXEC in the environment has no effect
// (llm_exec_security_spec.sh:117, MUST NOT 5).
func TestFileOnlySettingsIgnoreEnvironment(t *testing.T) {
	env := []string{"GASH_LLM_EXEC=unrestricted", "GASH_LLM_EXEC_ALLOW=seq", "GASH_LLM_SANDBOX=1"}
	for _, c := range []Caller{
		CallerFromEnviron(env), // direct invocation
		CallerFromEnviron(append(env, "GASH__SET=GASH_LLM_EXEC GASH_LLM_EXEC_ALLOW GASH_LLM_SANDBOX")), // a stub that would pass them
	} {
		for _, headless := range []bool{false, true} {
			s := NewSettings(settingsFile(""), c, headless)
			for _, n := range []string{"GASH_LLM_EXEC", "GASH_LLM_EXEC_ALLOW", "GASH_LLM_SANDBOX"} {
				if v, src := s.Lookup(n); src != SourceNone || v != "" {
					t.Errorf("%s from the environment: %q %v", n, v, src)
				}
				if _, set := c.Lookup(n); set {
					t.Errorf("%s recorded as a caller setting", n)
				}
			}
		}
	}

	// From the file they apply, in both modes, and the environment does not
	// override them.
	f := settingsFile("GASH_LLM_EXEC_ALLOW=\"seq\"\n")
	for _, headless := range []bool{false, true} {
		s := NewSettings(f, CallerFromEnviron([]string{"GASH_LLM_EXEC_ALLOW=rm"}), headless)
		if v, src := s.Lookup("GASH_LLM_EXEC_ALLOW"); v != "seq" || src != SourceFile {
			t.Errorf("headless=%v: %q %v", headless, v, src)
		}
		if v, _ := s.Lookup("GASH_LLM_EXEC"); v != "" {
			t.Errorf("GASH_LLM_EXEC = %q", v)
		}
	}
}

// GASH__SET tells "set to empty" from "unset" (compat.md 3).
func TestCallerFromEnvironGashSet(t *testing.T) {
	f := settingsFile("GASH_AI_CONFIRM=0\nGASH_DB_TIMEOUT=5\nGASH_SHOW_INXI=1\n")

	// Set to empty in the caller: the file is not consulted, the :- default applies.
	c := CallerFromEnviron([]string{"GASH__SET=GASH_AI_CONFIRM GASH_SHOW_INXI", "GASH_AI_CONFIRM=", "GASH_SHOW_INXI="})
	s := NewSettings(f, c, false)
	if v, src := s.Lookup("GASH_AI_CONFIRM"); v != "" || src != SourceCaller {
		t.Errorf("empty caller value: %q %v", v, src)
	}
	if got := s.Get("GASH_AI_CONFIRM"); got != "auto" {
		t.Errorf("${GASH_AI_CONFIRM:-auto} = %q", got)
	}
	if got := s.Get("GASH_SHOW_INXI"); got != "" {
		t.Errorf("${GASH_SHOW_INXI-0} with an empty value = %q", got)
	}
	// Not in GASH__SET: unset in the caller even if the variable is present.
	c = CallerFromEnviron([]string{"GASH__SET=", "GASH_DB_TIMEOUT=9"})
	s = NewSettings(f, c, false)
	if v, src := s.Lookup("GASH_DB_TIMEOUT"); v != "5" || src != SourceFile {
		t.Errorf("not listed: %q %v", v, src)
	}
	// Listed but the variable is missing: set to empty.
	c = CallerFromEnviron([]string{"GASH__SET=GASH_DB_TIMEOUT"})
	if v, ok := c.Lookup("GASH_DB_TIMEOUT"); !ok || v != "" {
		t.Errorf("listed without value: %q %v", v, ok)
	}
	// Unknown names in GASH__SET are ignored; whitespace of any kind separates.
	c = CallerFromEnviron([]string{"GASH__SET=GASH_NOPE\tGASH_NVM\nGASH_BANNER", "GASH_NOPE=1", "GASH_NVM=lazy", "GASH_BANNER=0"})
	if _, ok := c.Lookup("GASH_NOPE"); ok {
		t.Error("unknown name recorded")
	}
	if v, _ := c.Lookup("GASH_NVM"); v != "lazy" {
		t.Errorf("GASH_NVM = %q", v)
	}
	if v, _ := c.Lookup("GASH_BANNER"); v != "0" {
		t.Errorf("GASH_BANNER = %q", v)
	}
	// The first duplicate of a variable wins, as in os.Getenv (a Bash
	// caller never passes duplicates).
	c = CallerFromEnviron([]string{"GASH_NVM=on", "GASH_NVM=off"})
	if v, _ := c.Lookup("GASH_NVM"); v != "on" {
		t.Errorf("duplicate: %q", v)
	}
}

func TestSettingsDefaults(t *testing.T) {
	s := NewSettings(nil, Caller{}, false)
	want := map[string]string{
		"GASH_PROMPT": "1", "GASH_BANNER": "1", "GASH_HISTORY": "1", "GASH_HISTORY_SYNC": "0",
		"GASH_REPLACE_TOOLS": "0", "GASH_SAFETY_ALIASES": "1", "GASH_UMASK": "", "GASH_GIT_EXCLUDE": "",
		"GASH_SHOW_INXI": "0", "GASH_AI_MODEL_CLAUDE": "", "GASH_AI_MODEL_GEMINI": "", "GASH_AI_CONFIRM": "auto",
		"GASH_DB_TIMEOUT": "30", "GASH_LLM_EXEC": "", "GASH_LLM_EXEC_ALLOW": "", "GASH_LLM_SANDBOX": "",
		"GASH_NVM": "off", "GASH_PATH_ADD": "",
	}
	for name, w := range want {
		if got := s.Get(name); got != w {
			t.Errorf("%s default = %q, want %q", name, got, w)
		}
	}
	// ${GASH_PROMPT:-1}: an empty file value gives the default too.
	s = NewSettings(settingsFile("GASH_PROMPT=\nGASH_SHOW_INXI=\n"), Caller{}, false)
	if s.Get("GASH_PROMPT") != "1" || s.Get("GASH_SHOW_INXI") != "" {
		t.Errorf("empty values: %q %q", s.Get("GASH_PROMPT"), s.Get("GASH_SHOW_INXI"))
	}
	if v, src := s.Lookup("GASH_NOPE"); v != "" || src != SourceNone {
		t.Error("unknown setting")
	}
}
