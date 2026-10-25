// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package wincmd

import (
	"regexp"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/winbridge"
)

// ---- registry (reg.exe) ----

// regRootRE matches a registry key that starts with a known root. reg.exe
// takes the key as one argument (never a shell string), so this is a
// usability guard, not a safety boundary.
var regRootRE = regexp.MustCompile(`(?i)^(HKLM|HKCU|HKCR|HKU|HKCC|HKEY_LOCAL_MACHINE|HKEY_CURRENT_USER|HKEY_CLASSES_ROOT|HKEY_USERS|HKEY_CURRENT_CONFIG)(\\|$)`)

// regLineRE reads one "    Name    TYPE    Data" value line.
var regLineRE = regexp.MustCompile(`^\s{4}(.*?)\s{4}(REG_[A-Z_]+)\s{4}(.*)$`)

type regValue struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Data string `json:"data"`
}

type regResult struct {
	Key     string     `json:"key"`
	Values  []regValue `json:"values"`
	Subkeys []string   `json:"subkeys"`
}

func (r *run) doReg(key string) error {
	key = strings.TrimSpace(key)
	if !regRootRE.MatchString(key) {
		return r.failErr(contract.New("invalid_windows_argument", "not a registry key: "+key,
			`start with a root: HKLM\…, HKCU\…, HKCR\…, HKU\… or HKCC\…`))
	}
	if strings.ContainsAny(key, "\"\n\r") {
		return r.failErr(contract.New("invalid_windows_argument", "the key contains an illegal character", "remove quotes and newlines"))
	}
	br := r.bridge()
	if e := br.Available(); e != nil {
		return r.winErr(e)
	}
	args := []string{"query", key}
	if r.value != "" {
		args = append(args, "/v", r.value)
	}
	out, e := br.Native(r.c.Ctx, "reg.exe", args...)
	if e != nil {
		return r.winErr(e)
	}
	res := parseReg(winbridge.Decode(out), key)
	if r.asJSON {
		return r.emitJSON(res)
	}
	s := r.style()
	s.KV(r.c.Stdout, "Key", res.Key)
	if len(res.Values) > 0 {
		rows := []string{"name\ttype\tdata\t"}
		for _, v := range res.Values {
			rows = append(rows, orDash(v.Name)+"\t"+v.Type+"\t"+clip(v.Data, 60))
		}
		s.Table(r.c.Stdout, rows, "--gap", "2")
	}
	if len(res.Subkeys) > 0 {
		r.line("Subkeys:")
		s.List(r.c.Stdout, res.Subkeys...)
	}
	return nil
}

func (r *run) line(format string) { _, _ = r.c.Stdout.Write([]byte(format + "\n")) }

// parseReg reads reg.exe query output: the first HK line is the queried
// key, indented lines are its values, later HK lines are subkeys.
func parseReg(text, asked string) regResult {
	res := regResult{Key: asked}
	seenKey := false
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if m := regLineRE.FindStringSubmatch(line); m != nil {
			res.Values = append(res.Values, regValue{Name: strings.TrimSpace(m[1]), Type: m[2], Data: m[3]})
			continue
		}
		up := strings.ToUpper(strings.TrimSpace(line))
		if strings.HasPrefix(up, "HKEY_") || regRootRE.MatchString(strings.TrimSpace(line)) {
			if !seenKey {
				res.Key = strings.TrimSpace(line)
				seenKey = true
			} else {
				res.Subkeys = append(res.Subkeys, strings.TrimSpace(line))
			}
		}
	}
	return res
}

// ---- hosts file ----

type hostsEntry struct {
	IP    string   `json:"ip"`
	Hosts []string `json:"hosts"`
}

func (r *run) doHosts() error {
	br := r.bridge()
	sys32, ok := br.System32()
	if !ok {
		return r.winErr(&winbridge.Error{Code: winbridge.CodeUnavailable,
			Details: "no Windows system drive is mounted", Hint: "mount the drive that holds Windows"})
	}
	path := strings.TrimSuffix(sys32, "/System32") + "/System32/drivers/etc/hosts"
	data, err := br.ReadWindowsFile(path)
	if err != nil {
		return r.failErr(contract.New("windows_not_found", "could not read the Windows hosts file: "+err.Error(),
			"the drive may not be mounted"))
	}
	entries := parseHosts(winbridge.Decode(data))
	if r.asJSON {
		return r.emitJSON(map[string]any{"path": path, "entries": entries})
	}
	s := r.style()
	if len(entries) == 0 {
		r.say().Info("the Windows hosts file has no active entries (" + path + ")")
		return nil
	}
	rows := []string{"ip\thostnames\t"}
	for _, e := range entries {
		rows = append(rows, e.IP+"\t"+strings.Join(e.Hosts, " "))
	}
	s.Table(r.c.Stdout, rows, "--gap", "2")
	return nil
}

func parseHosts(text string) []hostsEntry {
	var out []hostsEntry
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || line[0] == '#' {
			continue
		}
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		out = append(out, hostsEntry{IP: f[0], Hosts: f[1:]})
	}
	return out
}
