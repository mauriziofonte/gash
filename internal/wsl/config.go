// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package wsl

import (
	"bytes"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// INI is a parsed wsl.conf or .wslconfig: sections of key = value lines.
// Section and key names compare case-insensitively; the first spelling
// seen is kept for display. Comments start with # or ; at the start of a
// line. Malformed lines are kept in Bad (WSL ignores them; doctor names
// them).
type INI struct {
	Sections []Section
	Bad      []BadLine
}

// Section is one [name] block.
type Section struct {
	Name string
	Keys []KV
}

// KV is one key = value line.
type KV struct {
	Key   string
	Value string
	Line  int
}

// BadLine is a line that is neither a comment, a section nor a key.
type BadLine struct {
	Line int
	Text string
}

// ParseINI parses b; it never fails.
func ParseINI(b []byte) INI {
	var ini INI
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	cur := -1
	for n, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' {
			name, ok := strings.CutSuffix(line[1:], "]")
			if !ok {
				ini.Bad = append(ini.Bad, BadLine{n + 1, line})
				continue
			}
			name = strings.TrimSpace(name)
			cur = ini.section(name)
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" || cur < 0 {
			ini.Bad = append(ini.Bad, BadLine{n + 1, line})
			continue
		}
		v = strings.TrimSpace(stripComment(v))
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		ini.Sections[cur].Keys = append(ini.Sections[cur].Keys, KV{Key: k, Value: v, Line: n + 1})
	}
	return ini
}

// stripComment drops a # or ; comment that follows a value after a space.
func stripComment(v string) string {
	for i := 1; i < len(v); i++ {
		if (v[i] == '#' || v[i] == ';') && (v[i-1] == ' ' || v[i-1] == '\t') {
			return v[:i]
		}
	}
	return v
}

func (ini *INI) section(name string) int {
	for i, s := range ini.Sections {
		if strings.EqualFold(s.Name, name) {
			return i
		}
	}
	ini.Sections = append(ini.Sections, Section{Name: name})
	return len(ini.Sections) - 1
}

// Get returns the last value of section.key.
func (ini INI) Get(section, key string) (string, bool) {
	v, ok := "", false
	for _, s := range ini.Sections {
		if !strings.EqualFold(s.Name, section) {
			continue
		}
		for _, kv := range s.Keys {
			if strings.EqualFold(kv.Key, key) {
				v, ok = kv.Value, true
			}
		}
	}
	return v, ok
}

// Setting is one documented key of wsl.conf or .wslconfig with its
// default (learn.microsoft.com/windows/wsl/wsl-config, 2026-10).
type Setting struct {
	File    string `json:"file"`    // "wsl.conf" or ".wslconfig"
	Section string `json:"section"` // "boot"
	Key     string `json:"key"`     // "systemd"
	Default string `json:"default"`
	Note    string `json:"note"`
}

// Settings lists the keys gash knows, in the order of Microsoft's page.
var Settings = []Setting{
	{"wsl.conf", "boot", "systemd", "false", "systemd as PID 1 (WSL 2)"},
	{"wsl.conf", "boot", "command", "", "command run as root at start"},
	{"wsl.conf", "boot", "protectBinfmt", "true", "keeps systemd from replacing the WSLInterop binfmt entry"},
	{"wsl.conf", "automount", "enabled", "true", "mount the Windows drives"},
	{"wsl.conf", "automount", "mountFsTab", "true", "process /etc/fstab at start"},
	{"wsl.conf", "automount", "root", "/mnt/", "where the drives are mounted"},
	{"wsl.conf", "automount", "options", "", "drvfs options (metadata, uid, gid, umask, fmask, dmask, case)"},
	{"wsl.conf", "network", "generateHosts", "true", "WSL writes /etc/hosts"},
	{"wsl.conf", "network", "generateResolvConf", "true", "WSL writes /etc/resolv.conf"},
	{"wsl.conf", "network", "hostname", "", "the Windows host name"},
	{"wsl.conf", "interop", "enabled", "true", "run Windows programs"},
	{"wsl.conf", "interop", "appendWindowsPath", "true", "Windows PATH appended to PATH"},
	{"wsl.conf", "user", "default", "", "the user of a new session"},
	{"wsl.conf", "gpu", "enabled", "true", "GPU paravirtualization"},
	{"wsl.conf", "time", "useWindowsTimezone", "true", "follow the Windows time zone"},
	{".wslconfig", "wsl2", "memory", "50% of RAM", "VM memory"},
	{".wslconfig", "wsl2", "processors", "all", "VM processors"},
	{".wslconfig", "wsl2", "swap", "25% of memory", "swap size, 0 turns it off"},
	{".wslconfig", "wsl2", "networkingMode", "nat", "nat, mirrored, none, consomme"},
	{".wslconfig", "wsl2", "dnsTunneling", "true", "DNS answered by Windows (VPN friendly)"},
	{".wslconfig", "wsl2", "firewall", "true", "Windows and Hyper-V firewall rules apply to WSL"},
	{".wslconfig", "wsl2", "autoProxy", "true", "use the Windows HTTP proxy"},
	{".wslconfig", "wsl2", "localhostForwarding", "true", "localhost reaches WSL ports (NAT mode)"},
	{".wslconfig", "wsl2", "guiApplications", "true", "WSLg"},
	{".wslconfig", "wsl2", "vmIdleTimeout", "60000", "ms before an idle VM stops"},
	{".wslconfig", "wsl2", "nestedVirtualization", "true", "nested VMs"},
	{".wslconfig", "wsl2", "kernel", "", "custom kernel"},
	{".wslconfig", "wsl2", "defaultVhdSize", "1TB", "maximum size of a new distribution disk"},
	{".wslconfig", "wsl2", "virtiofs", "false", "experimental: Windows drives over virtiofs"},
	{".wslconfig", "general", "instanceIdleTimeout", "15000", "ms before an idle distribution stops"},
	{".wslconfig", "experimental", "autoMemoryReclaim", "dropCache", "disabled, gradual, dropCache"},
	{".wslconfig", "experimental", "sparseVhd", "false", "new disks are sparse"},
	{".wslconfig", "experimental", "hostAddressLoopback", "false", "mirrored: reach the host on its own addresses"},
	{".wslconfig", "experimental", "ignoredPorts", "", "mirrored: ports Linux may bind although Windows uses them"},
	{".wslconfig", "experimental", "bestEffortDnsParsing", "false", "DNS tunneling ignores unknown records"},
}

// Effective is one documented setting with the value in force.
type Effective struct {
	Setting
	Value  string `json:"value"`
	Source string `json:"source"` // "file" or "default"
	Line   int    `json:"line,omitempty"`
}

// Resolve returns the documented settings of file with their values: from
// ini when it sets them, else the default. Keys ini sets that are not
// documented are returned as unknown.
func Resolve(file string, ini INI) (known []Effective, unknown []KV) {
	seen := map[string]bool{}
	for _, s := range Settings {
		if s.File != file {
			continue
		}
		e := Effective{Setting: s, Value: s.Default, Source: "default"}
		for _, sec := range ini.Sections {
			if !strings.EqualFold(sec.Name, s.Section) {
				continue
			}
			for _, kv := range sec.Keys {
				if strings.EqualFold(kv.Key, s.Key) {
					e.Value, e.Source, e.Line = kv.Value, "file", kv.Line
				}
			}
		}
		seen[strings.ToLower(s.Section+"."+s.Key)] = true
		known = append(known, e)
	}
	for _, sec := range ini.Sections {
		for _, kv := range sec.Keys {
			if !seen[strings.ToLower(sec.Name+"."+kv.Key)] {
				unknown = append(unknown, KV{Key: sec.Name + "." + kv.Key, Value: kv.Value, Line: kv.Line})
			}
		}
	}
	return known, unknown
}

// DecodeText decodes a configuration file written on Windows: UTF-8 (with
// or without a BOM), else UTF-16 with a BOM (Notepad's "Unicode").
func DecodeText(b []byte) []byte {
	if len(b) >= 2 && (b[0] == 0xff && b[1] == 0xfe || b[0] == 0xfe && b[1] == 0xff) {
		le := b[0] == 0xff
		units := make([]uint16, 0, len(b)/2)
		for i := 2; i+1 < len(b); i += 2 {
			if le {
				units = append(units, uint16(b[i])|uint16(b[i+1])<<8)
			} else {
				units = append(units, uint16(b[i])<<8|uint16(b[i+1]))
			}
		}
		var out []byte
		for _, r := range utf16.Decode(units) {
			out = utf8.AppendRune(out, r)
		}
		return out
	}
	return bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
}
