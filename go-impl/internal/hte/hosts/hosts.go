// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package hosts keeps the managed block of the hosts file (design/hte.md
// 7.2): the desired block from the site state and hosts.toml, the drift
// check against /etc/hosts, the apply step through the root-owned helper
// (the only NOPASSWD entry), and the lines WSL users add to the Windows
// hosts file.
package hosts

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/google/renameio/v2"

	"github.com/mauriziofonte/gash/internal/hostshelper"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/proc"
	"github.com/mauriziofonte/gash/internal/hte/state"
	"github.com/mauriziofonte/gash/internal/platform"
)

// HelperPath is where install puts the helper (design/hte.md 7.1).
const HelperPath = "/usr/local/libexec/gash/gash-hosts-helper"

// Address is the only address of the block's lines: 127.0.0.1, also where
// IPv6 loopback exists (lead decision, 2026-10-18; LEDGER HTE-37). Apache
// still answers on [::1] for clients that resolve ::1 another way.
const Address = state.LoopbackIPv4

// State is hosts.toml: the names kept in the block beyond the sites' own.
type State struct {
	Names []string `toml:"names"`
}

// LoadState reads hosts.toml; a missing file is an empty state. Every
// name must be a valid hostname: the block is the helper's input grammar.
func LoadState(path string) (State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return State{Names: []string{}}, nil
	}
	if err != nil {
		return State{}, err
	}
	var st State
	md, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&st)
	if err != nil {
		return State{}, hteerr.Newf(hteerr.InvalidState, "", "%s: %v", path, err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return State{}, hteerr.Newf(hteerr.InvalidState, "", "%s: unknown key %s", path, und[0])
	}
	if !md.IsDefined("names") {
		return State{}, hteerr.Newf(hteerr.InvalidState, "", "%s: key names is missing", path)
	}
	for _, n := range st.Names {
		if !hostshelper.ValidHostname(n) {
			return State{}, hteerr.Newf(hteerr.InvalidState, "", "%s: %q is not a valid hostname", path, n)
		}
	}
	return st, nil
}

// SaveState writes hosts.toml atomically (0600).
func SaveState(path string, st State) error {
	for _, n := range st.Names {
		if !hostshelper.ValidHostname(n) {
			return hteerr.Newf(hteerr.InvalidDomain, "", "%q is not a valid hostname", n)
		}
	}
	if st.Names == nil {
		st.Names = []string{}
	}
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(st); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return renameio.WriteFile(path, b.Bytes(), 0o600)
}

// Desired returns the block: one line per enabled site, in domain order,
// with every name the site serves, then one line with the extra names no
// site serves. A disabled site has no line.
func Desired(sites []state.Site, extra []string) []hostshelper.Entry {
	sorted := slices.Clone(sites)
	slices.SortFunc(sorted, func(a, b state.Site) int { return strings.Compare(a.Domain, b.Domain) })
	out := []hostshelper.Entry{}
	served := map[string]bool{}
	for _, s := range sorted {
		if !s.Enabled {
			continue
		}
		out = append(out, hostshelper.Entry{Address: Address, Names: s.Names()})
		for _, n := range s.Names() {
			served[n] = true
		}
	}
	var rest []string
	for _, n := range extra {
		if !served[n] && !slices.Contains(rest, n) {
			rest = append(rest, n)
		}
	}
	if len(rest) > 0 {
		out = append(out, hostshelper.Entry{Address: Address, Names: rest})
	}
	return out
}

// Drift compares the hosts file's block with the desired one.
type Drift struct {
	InSync     bool     `json:"in_sync"`
	Found      bool     `json:"found"`             // the markers are in the hosts file
	Missing    []string `json:"missing"`           // desired names the block lacks
	Unexpected []string `json:"unexpected"`        // names in the block that are not desired
	Problem    string   `json:"problem,omitempty"` // the block could not be read
}

func names(entries []hostshelper.Entry) map[string]bool {
	out := map[string]bool{}
	for _, e := range entries {
		for _, n := range e.Names {
			out[n] = true
		}
	}
	return out
}

// Check reads the hosts file through sr and compares its block with
// desired, byte for byte (the helper writes exactly FormatBlock).
func Check(sr platform.Sysroot, desired []hostshelper.Entry) Drift {
	d := Drift{Missing: []string{}, Unexpected: []string{}}
	data, err := sr.ReadFile(hostshelper.HostsFile)
	if err != nil {
		d.Problem = err.Error()
		return d
	}
	block, found, err := hostshelper.Extract(data)
	if err != nil {
		d.Problem = err.Error()
		return d
	}
	d.Found = found
	want := hostshelper.FormatBlock(desired)
	if found && bytes.Equal(block, want) {
		d.InSync = true
		return d
	}
	if !found && len(desired) == 0 {
		d.InSync = true
		return d
	}
	current, err := hostshelper.ParseBlock(block)
	if err != nil {
		d.Problem = "the managed block does not parse: " + err.Error()
	}
	have, wantNames := names(current), names(desired)
	for _, e := range desired {
		for _, n := range e.Names {
			if !have[n] {
				d.Missing = append(d.Missing, n)
			}
		}
	}
	for _, e := range current {
		for _, n := range e.Names {
			if !wantNames[n] {
				d.Unexpected = append(d.Unexpected, n)
			}
		}
	}
	return d
}

// Applier writes the block through the helper.
type Applier struct {
	Runner proc.Runner
	Sudo   string // absolute path of sudo
	Helper string // HelperPath, or a test path
}

// Apply runs `sudo -n <helper> apply` with the block on stdin. sudo never
// prompts here: the sudoers drop-in of install allows exactly this command
// without a password; without it the step is privileged_step_required.
func (a Applier) Apply(ctx context.Context, desired []hostshelper.Entry) error {
	block := hostshelper.FormatBlock(desired)
	res, err := a.Runner.Run(ctx, proc.Cmd{Path: a.Sudo, Args: []string{"-n", a.Helper, "apply"}, Stdin: block, Setsid: true})
	if err != nil {
		return hteerr.Newf(hteerr.PrivilegedStepRequired, "run gash hte doctor", "running the hosts helper: %v", err)
	}
	switch {
	case res.OK():
		return nil
	case res.ExitCode == hostshelper.ExitRefused:
		return hteerr.Newf(hteerr.InvalidState, "", "the hosts helper refused the block: %s", res.Output())
	case strings.Contains(string(res.Stderr), "gash-hosts-helper:"):
		return hteerr.Newf(hteerr.PrivilegedStepRequired, "run gash hte doctor", "the hosts helper failed: %s", res.Output())
	}
	return hteerr.Newf(hteerr.PrivilegedStepRequired, "run gash hte install: it installs the helper and its sudoers line",
		"sudo -n %s apply: %s", a.Helper, res.Output())
}

// WindowsLines are the lines to add to C:\Windows\System32\drivers\etc\hosts
// for the names Windows cannot resolve (design/hte.md 7.2).
func WindowsLines(missing []string) []string {
	out := make([]string, 0, len(missing))
	for _, n := range missing {
		out = append(out, Address+" "+n)
	}
	return out
}

// windowsScript resolves the names it reads on stdin, one per line, and
// prints "ok NAME" or "fail NAME". The names are data, never code.
const windowsScript = `$input | ForEach-Object { $n = $_.Trim(); if ($n) { try { [void][System.Net.Dns]::GetHostAddresses($n); "ok $n" } catch { "fail $n" } } }`

// WindowsUnresolved asks Windows, through WSL interop, which names it
// cannot resolve.
func WindowsUnresolved(ctx context.Context, r proc.Runner, powershell string, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	res, err := r.Run(ctx, proc.Cmd{Path: powershell, Args: []string{"-NoProfile", "-NonInteractive", "-Command", windowsScript},
		Stdin: []byte(strings.Join(names, "\n") + "\n"), Setsid: true})
	if err != nil {
		return nil, err
	}
	if !res.OK() {
		return nil, errors.New("powershell.exe: " + res.Output())
	}
	resolved := map[string]bool{}
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		if n, ok := strings.CutPrefix(strings.TrimSpace(line), "ok "); ok {
			resolved[n] = true
		}
	}
	var out []string
	for _, n := range names {
		if !resolved[n] {
			out = append(out, n)
		}
	}
	return out, nil
}

// PowerShell is the WSL interop path of Windows PowerShell.
const PowerShell = "/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe"
