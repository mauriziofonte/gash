// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

// --host ALIAS on the agent_* writes (plan S5, A2): the same call on a
// registered host, run by the gash binary shipped there (internal/remote,
// host.go); no POSIX fallback for a write, so a host of another platform
// is remote_binary_unavailable. Every positional is a remote target and
// passes the remote write rails (CheckWrite: the secret-file rule, DENY,
// ASK without --force) before anything connects; the binary on the host
// applies the agent_* rails again on the resolved path.

import (
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/remote"
)

// hostSpecs are the agent_* functions that take --host.
func hostSpecs() map[string]remote.Spec {
	return map[string]remote.Spec{
		"agent_replace": {Values: []string{"--old", "--new", "--count", "--min"}, PathFrom: 0, Write: true},
	}
}

// hostable is h with --host ALIAS; name must have a hostSpecs entry.
func (m *Module) hostable(name string, h invoke.Handler) invoke.Handler {
	spec, ok := hostSpecs()[name]
	if !ok {
		panic("agent: no --host spec for " + name)
	}
	return remote.Hoster{Run: m.run, Exe: func() (string, error) { return m.exe() }}.Wrap(name, spec, h)
}
