// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package htecmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/hte"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// call runs gash hte ARGS with an engine builder.
func call(build func(*invoke.Call, bool) (*hte.Engine, error), args ...string) (stdout, stderr string, status int) {
	var out, errb bytes.Buffer
	env := invoke.EnvMap(nil)
	status = Handler(Options{Engine: build})(&invoke.Call{
		Ctx: context.Background(), Name: "hte", Args: args, Env: env,
		Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb, Term: term.Fixed(env, false, false, false),
	})
	return out.String(), errb.String(), status
}

// An engine error is the envelope of its code with the code's action and
// hint; any other error is internal_error (STOP).
func TestEnvelope(t *testing.T) {
	e := Envelope(hteerr.New(hteerr.PortInUse, "127.0.0.1:80 is held by uid 0", "free the port"))
	if e.Code != "port_in_use" || e.Action != contract.Stop || e.Details != "127.0.0.1:80 is held by uid 0" || e.Hint != "free the port" {
		t.Errorf("%+v", e)
	}
	if e := Envelope(hteerr.New(hteerr.RootNotAllowed, "x", "y")); e.Action != contract.Fatal {
		t.Errorf("root_not_allowed: %+v", e)
	}
	if e := Envelope(errors.New("boom")); e.Code != "internal_error" || e.Action != contract.Stop || e.Details != "boom" {
		t.Errorf("plain error: %+v", e)
	}
}

// An engine that cannot be built fails the command with its envelope;
// usage errors and help never build one.
func TestEngineBuild(t *testing.T) {
	built := 0
	fail := func(*invoke.Call, bool) (*hte.Engine, error) {
		built++
		return nil, errors.New("uid 4242 has no user name")
	}
	stdout, stderr, status := call(fail, "site", "list")
	if status != 1 || stdout != "" ||
		stderr != `{"error":"internal_error","details":"uid 4242 has no user name","action":"STOP","recoverable":true,"hint":"run gash hte doctor"}`+"\n" {
		t.Errorf("status %d stdout %q stderr %q", status, stdout, stderr)
	}
	for _, args := range [][]string{{"--help"}, {"site", "create"}, {"install", "--nope"}, {"uninstall"}} {
		call(fail, args...)
	}
	if built != 2 { // site list, and uninstall (its --yes is the engine's check)
		t.Errorf("the engine was built %d times", built)
	}
}

func TestMissingArg(t *testing.T) {
	for msg, want := range map[string]bool{
		"accepts 2 arg(s), received 1":           true,
		"accepts 1 arg(s), received 0":           true,
		"accepts 1 arg(s), received 2":           false,
		"unknown command \"x\" for \"gash hte\"": false,
	} {
		if missingArg(msg) != want {
			t.Errorf("%q: %v", msg, !want)
		}
	}
}
