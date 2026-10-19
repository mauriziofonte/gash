// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package runner

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// ConstScript names one of the shell scripts compiled into this package
// that Gash still runs through `sh -c`, because 2.x did and a stub or a
// sudo log sees that exact argv (LEDGER SYS-46). The whole command line is
// a constant here: the caller only selects it, so no caller-supplied text
// ever reaches a shell. Run keeps refusing every shell command string.
type ConstScript int

const (
	// ScriptWSLCompact is wsl_compact's memory compaction:
	//	sudo sh -c 'sync && echo 3 > /proc/sys/vm/drop_caches && echo 1 > /proc/sys/vm/compact_memory'
	ScriptWSLCompact ConstScript = iota + 1
)

// constScripts are the registered command lines.
var constScripts = map[ConstScript][]string{
	ScriptWSLCompact: {"sudo", "sh", "-c", "sync && echo 3 > /proc/sys/vm/drop_caches && echo 1 > /proc/sys/vm/compact_memory"},
}

func (s ConstScript) String() string {
	switch s {
	case ScriptWSLCompact:
		return "wsl_compact"
	}
	return fmt.Sprintf("ConstScript(%d)", int(s))
}

// ErrScript is returned by RunScript for a script that is not registered.
var ErrScript = errors.New("runner: not a registered constant script")

// Script is a run of a registered constant script.
type Script struct {
	Name ConstScript
	Options
}

// argv returns the registered command line of s.
func (s Script) argv() ([]string, error) {
	argv, ok := constScripts[s.Name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrScript, s.Name)
	}
	return slices.Clone(argv), nil
}

// RunScript implements Runner.
func (Exec) RunScript(ctx context.Context, s Script) (Result, error) {
	argv, err := s.argv()
	if err != nil {
		return Result{ExitCode: 126}, err
	}
	return start(ctx, argv, s.Options)
}
