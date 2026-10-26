// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package wincmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
	"github.com/mauriziofonte/gash/internal/winbridge"
)

// callEnv is call() with environment overrides, so a test can drive the
// headless gate (GASH_HEADLESS=1).
func callEnv(t *testing.T, envm map[string]string, fx map[string]string, args ...string) (string, string, int) {
	t.Helper()
	sr := fixtureSys(t)
	var out, errb bytes.Buffer
	env := invoke.EnvMap(envm)
	opt := Options{Bridge: func(c *invoke.Call) *winbridge.Bridge {
		return winbridge.New(facts(), sr, scriptRunner{fixtures: fx}, c.Env.Get)
	}}
	status := Handler(opt)(&invoke.Call{
		Ctx: context.Background(), Name: "win", Args: args, Env: env,
		Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb, Term: term.Fixed(env, false, false, false),
	})
	return out.String(), errb.String(), status
}

// TestFixList: with no action, fix lists the allowlist (and as JSON).
func TestFixList(t *testing.T) {
	out, _, status := call(t, nil, "fix", "--json")
	if status != 0 {
		t.Fatalf("status %d", status)
	}
	for _, want := range []string{`"action":"flush-dns"`, `"action":"restart-service"`, `"action":"hosts-add"`, `"action":"hosts-remove"`, "gash win fix"} {
		if !strings.Contains(out, want) {
			t.Errorf("fix list json missing %s: %s", want, out)
		}
	}
}

// TestFixDryRun: --dry-run prints the exact command and runs nothing, even
// headless (so an agent can prepare it for a human).
func TestFixDryRun(t *testing.T) {
	out, _, status := callEnv(t, map[string]string{"GASH_HEADLESS": "1"}, nil, "fix", "flush-dns", "--dry-run", "--json")
	if status != 0 {
		t.Fatalf("dry-run status %d out %s", status, out)
	}
	for _, want := range []string{`"command":"Clear-DnsClientCache"`, `"executed":false`, `"elevated":true`} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run json missing %s: %s", want, out)
		}
	}
}

// TestFixHeadlessRefused: without --dry-run, a headless caller is refused
// with elevation_required (decision #3: human-only, UAC).
func TestFixHeadlessRefused(t *testing.T) {
	_, errb, status := callEnv(t, map[string]string{"GASH_HEADLESS": "1"}, nil, "fix", "flush-dns")
	if status != 1 || !strings.Contains(errb, "elevation_required") {
		t.Errorf("headless fix: status %d err %s", status, errb)
	}
}

// TestFixValidation: bad arguments never reach Windows.
func TestFixValidation(t *testing.T) {
	cases := [][]string{
		{"fix", "restart-service", "bad;name"},
		{"fix", "restart-service", "name'; Remove-Item"},
		{"fix", "hosts-add", "999.1.1.1", "app.test"},
		{"fix", "hosts-add", "10.0.0.1", "bad host!"},
		{"fix", "hosts-remove", "../etc"},
		{"fix", "no-such-action"},
		{"fix", "flush-dns", "extra-arg"},
	}
	for _, args := range cases {
		_, errb, status := call(t, nil, args...)
		if status != 1 || !strings.Contains(errb, "invalid_windows_argument") {
			t.Errorf("args %v: status %d err %s", args, status, errb)
		}
	}
}

// TestFixDryRunHostsRemove: the hosts-remove command embeds the hostname as
// single-quoted data and filters by whitespace fields, never by a built
// regex.
func TestFixDryRunHostsRemove(t *testing.T) {
	out, _, status := call(t, nil, "fix", "hosts-remove", "app.test", "--dry-run")
	if status != 0 {
		t.Fatalf("status %d", status)
	}
	if !strings.Contains(out, "$n='app.test'") || !strings.Contains(out, "-contains $n") {
		t.Errorf("hosts-remove command = %s", out)
	}
}

// TestFixApplies: a human path (not headless) with --yes skips the in-shell
// question and hands the command to RunElevated, which the fake runner
// accepts (exit 0).
func TestFixApplies(t *testing.T) {
	out, _, status := call(t, nil, "fix", "flush-dns", "--yes")
	if status != 0 {
		t.Fatalf("apply status %d out %s", status, out)
	}
	if !strings.Contains(out, "powershell Clear-DnsClientCache") || !strings.Contains(out, "runs elevated as administrator") {
		t.Errorf("apply did not preview the command: %s", out)
	}
}

// TestPSLit doubles an inner single quote so a value stays data.
func TestPSLit(t *testing.T) {
	if got := psLit("a'b"); got != "'a''b'" {
		t.Errorf("psLit = %q", got)
	}
}
