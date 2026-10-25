// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package wincmd

import (
	"net"
	"regexp"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
)

// `gash win fix` is the one write path toward Windows. Decision #3 of the
// WSL plan: elevated corrections live inside Gash, but they are human-only
// and always go through a UAC prompt. So the command refuses in headless
// mode (an agent cannot stand at the Windows UAC dialog), shows the human
// the exact PowerShell it will run elevated, asks once, and only then hands
// it to winbridge.RunElevated (Start-Process -Verb RunAs).
//
// The actions are a fixed, reviewable allowlist. No action is built from
// raw user text: every argument is validated to a safe shape and embedded
// as single-quoted PowerShell data, never as code. An agent can still
// prepare an action with --dry-run (the exact command is printed, nothing
// runs) and hand it to a human to carry out.
//
//	gash win fix flush-dns
//	gash win fix restart-service NAME
//	gash win fix hosts-add IP HOSTNAME
//	gash win fix hosts-remove HOSTNAME
//	                       [--dry-run] [--yes] [--json]

// fixAction is one entry of the allowlist. build validates args and returns
// a human sentence and the PowerShell command (one line); it reports a bad
// argument through the envelope and ok=false.
type fixAction struct {
	name  string
	usage string
	short string
	min   int
	max   int
	build func(args []string) (desc, command string, bad contract.Envelope, ok bool)
}

var fixActions = []fixAction{
	{
		name: "flush-dns", usage: "flush-dns", short: "Clear the Windows DNS client cache", min: 0, max: 0,
		build: func([]string) (string, string, contract.Envelope, bool) {
			return "Flush the Windows DNS resolver cache", "Clear-DnsClientCache", contract.Envelope{}, true
		},
	},
	{
		name: "restart-service", usage: "restart-service NAME", short: "Restart a Windows service by its service name", min: 1, max: 1,
		build: func(a []string) (string, string, contract.Envelope, bool) {
			name := strings.TrimSpace(a[0])
			if !serviceNameRE.MatchString(name) {
				return "", "", contract.New("invalid_windows_argument", "not a service name: "+a[0],
					"use the short service name (letters, digits, dot, dash, underscore): gash win services"), false
			}
			return "Restart the Windows service " + name,
				"Restart-Service -Name " + psLit(name) + " -Force", contract.Envelope{}, true
		},
	},
	{
		name: "hosts-add", usage: "hosts-add IP HOSTNAME", short: "Add a mapping to the Windows hosts file", min: 2, max: 2,
		build: func(a []string) (string, string, contract.Envelope, bool) {
			ip, host := strings.TrimSpace(a[0]), strings.TrimSpace(a[1])
			if net.ParseIP(ip) == nil {
				return "", "", contract.New("invalid_windows_argument", "not an IP address: "+a[0], "give an IPv4 or IPv6 address"), false
			}
			if !hostnameRE.MatchString(host) {
				return "", "", contract.New("invalid_windows_argument", "not a hostname: "+a[1], "give a DNS name such as app.local"), false
			}
			return "Add " + ip + " " + host + " to the Windows hosts file",
				"Add-Content -LiteralPath (Join-Path $env:WINDIR 'System32\\drivers\\etc\\hosts') -Value " +
					psLit(ip+" "+host+"  # added by gash"), contract.Envelope{}, true
		},
	},
	{
		name: "hosts-remove", usage: "hosts-remove HOSTNAME", short: "Remove a hostname from the Windows hosts file", min: 1, max: 1,
		build: func(a []string) (string, string, contract.Envelope, bool) {
			host := strings.TrimSpace(a[0])
			if !hostnameRE.MatchString(host) {
				return "", "", contract.New("invalid_windows_argument", "not a hostname: "+a[0], "give a DNS name such as app.local"), false
			}
			// Keep every line whose active part (before a '#') does not list
			// the hostname as a whitespace-delimited field. Comments and
			// blank lines are kept. $n is validated data, single-quoted.
			cmd := "$h=Join-Path $env:WINDIR 'System32\\drivers\\etc\\hosts'; $n=" + psLit(host) +
				"; (Get-Content -LiteralPath $h) | Where-Object { $c=($_ -split '#',2)[0].Trim(); ($c -eq '') -or (-not (($c -split '\\s+') -contains $n)) } | Set-Content -LiteralPath $h"
			return "Remove " + host + " from the Windows hosts file", cmd, contract.Envelope{}, true
		},
	},
}

var (
	serviceNameRE = regexp.MustCompile(`^[A-Za-z0-9._ -]{1,128}$`)
	hostnameRE    = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,62})(\.[A-Za-z0-9]([A-Za-z0-9-]{0,62}))*$`)
)

// previewFix shows the human the action and the exact elevated command.
// The command is printed verbatim on its own line (never clipped to the
// terminal width, as a box would): the human is about to trust it, so it
// must be whole.
func (r *run) previewFix(title, desc, command string) {
	s := r.style()
	s.Section(r.c.Stdout, title)
	s.KV(r.c.Stdout, "Action", desc)
	s.Mark(r.c.Stdout, "warning", "runs elevated as administrator (Windows will ask for UAC)")
	on, off := "", ""
	if s.Color {
		on, off = s.P.Accent, s.P.Off
	}
	_, _ = r.c.Stdout.Write([]byte("\n    " + on + "powershell " + command + off + "\n\n"))
}

// psLit wraps s as a single-quoted PowerShell string literal (doubling an
// inner single quote), so a validated value is data to PowerShell, never
// code.
func psLit(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func lookupFix(name string) *fixAction {
	for i := range fixActions {
		if fixActions[i].name == name {
			return &fixActions[i]
		}
	}
	return nil
}

// doFix runs `gash win fix ACTION ARGS...`.
func (r *run) doFix(name string, args []string) error {
	act := lookupFix(name)
	if act == nil {
		return r.failErr(contract.New("invalid_windows_argument", "unknown fix: "+name, "run: gash win fix --help"))
	}
	if len(args) < act.min || len(args) > act.max {
		return r.failErr(contract.New("invalid_windows_argument",
			"wrong number of arguments for "+name, "usage: gash win fix "+act.usage))
	}
	desc, command, bad, ok := act.build(args)
	if !ok {
		return r.failErr(bad)
	}

	// An agent (headless, or no controlling terminal) cannot authorize a UAC
	// prompt. --dry-run still works for it: it gets the exact command to hand
	// to a human, with nothing executed.
	if r.dryRun {
		if r.asJSON {
			return r.emitJSON(map[string]any{"action": name, "description": desc, "command": command, "elevated": true, "executed": false})
		}
		r.previewFix("Would run elevated", desc, command)
		r.say().Note("this is a preview; run it without --dry-run on the Windows desktop to apply it")
		return nil
	}

	st := r.style()
	if st.Headless {
		return r.failErr(contract.New("elevation_required",
			"win fix changes Windows and needs a human to approve the UAC prompt; it will not run headless",
			"run it yourself in an interactive shell on the Windows desktop, or use --dry-run to see the exact command"))
	}

	// Show the human exactly what will run elevated, then ask once (the UAC
	// dialog is the second, authoritative gate).
	r.previewFix("Elevated Windows change", desc, command)
	if !r.yes {
		if !st.Confirm(r.c.Term, r.c.Stderr, "Run this elevated? Windows will ask you to confirm (UAC)") {
			return r.failErr(contract.New("confirmation_required", "not confirmed", "re-run and answer y, or pass --yes"))
		}
	}
	r.say().Info("waiting for the Windows UAC prompt…")
	if e := r.bridge().RunElevated(r.c.Ctx, command); e != nil {
		return r.winErr(e)
	}
	r.say().Success(desc + ": done")
	return nil
}
