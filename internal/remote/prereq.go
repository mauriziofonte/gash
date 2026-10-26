// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

// The missing prerequisites of the remote functions, on both sides of the
// connection:
//
//   - this machine: ssh must be on PATH before anything connects. Every
//     entry point (--host, llm_remote_run, llm_same, agent_put, the +ssh
//     databases) asks RequireSSH or SSHMissing once per call, so a missing
//     client is the prerequisite report (prerequisite_missing, STOP, with
//     the install command of this system), never host_unreachable;
//   - the host: the POSIX sh scripts test the programs they run (ShTools,
//     command -v) and answer "missing TOOL" instead of an empty result.
//     MissingOnHost is the envelope of that answer, the one code of a
//     program the host lacks: prerequisite_missing with the members
//     "missing" and "host" (and "container" inside one). There is no
//     install command: the host's package manager is not this system's.

import (
	"bytes"
	"encoding/json"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/prereq"
)

// RequireSSH reports whether ssh is on the caller's PATH; when it is not,
// it writes the prerequisite envelope for need ("llm_find --host",
// "agent_put") and the caller returns 1.
func RequireSSH(c *invoke.Call, need string) bool {
	_, ok := prereq.Require(c, "ssh", need, true)
	return ok
}

// SSHMissing is the prerequisite envelope of a missing ssh for need, for
// the callers that write their own output (llm_db_*); nil when ssh is
// there.
func SSHMissing(c *invoke.Call, need string) *contract.Envelope {
	if _, ok := prereq.Locate(c, "ssh"); ok {
		return nil
	}
	e := prereq.Current(c).Report("ssh", need).Envelope()
	return &e
}

// MissingFrame is the TOOL of the "missing TOOL" frame of gash_tools.
func MissingFrame(stderr []byte, nonce string) (string, bool) {
	return Frame(stderr, nonce, "missing")
}

// hostPackages are the packages that usually provide the programs the
// host scripts run, named in the hint (the host's distribution is not
// known here).
var hostPackages = map[string]string{
	"find": "findutils", "grep": "grep", "bash": "bash", "docker": "docker",
	"sort": "coreutils", "head": "coreutils", "tr": "coreutils", "wc": "coreutils", "ls": "coreutils",
	"cat": "coreutils", "uname": "coreutils", "mkdir": "coreutils", "chmod": "coreutils", "mv": "coreutils",
	"cp": "coreutils", "rm": "coreutils", "date": "coreutils", "readlink": "coreutils",
	"psql": "postgresql-client", "sqlite3": "sqlite3", "mysql": "mysql-client or mariadb-client",
}

// where is "on ALIAS", or "in container CTR on ALIAS".
func where(h Host, container string) string {
	if container != "" {
		return "in container " + container + " on " + h.Alias
	}
	return "on " + h.Alias
}

// MissingOnHost is the envelope of a program the host (or the container
// on it) does not have: "llm_find on web needs find, which is not
// installed there".
func MissingOnHost(need, tool string, h Host, container string) contract.Envelope {
	details := need + " on " + h.Alias + " needs " + tool + ", which is not installed there"
	if container != "" {
		details = need + " on " + h.Alias + " needs " + tool + ", which is not installed in container " + container
	}
	hint := "Install " + tool + " " + where(h, container)
	if pkg := hostPackages[tool]; pkg != "" {
		hint += " (usually the " + pkg + " package)"
	}
	return hostMembers(contract.New(prereq.CodeMissing, details, hint+", then retry"), tool, h, container)
}

// OldOnHost is the envelope of a program the host has in a version too old
// for the call: "psql 12 or newer is needed on web (found 11.22)". found
// is the version the host reported ("" when it printed none).
func OldOnHost(tool, minVersion, found string, h Host, container string) contract.Envelope {
	if found == "" {
		found = "an older version"
	}
	details := tool + " " + minVersion + " or newer is needed " + where(h, container) + " (found " + found + ")"
	hint := "Upgrade " + tool + " " + where(h, container)
	if pkg := hostPackages[tool]; pkg != "" {
		hint += " (usually the " + pkg + " package)"
	}
	return hostMembers(contract.New(prereq.CodeMissing, details, hint+", then retry"), tool, h, container)
}

func hostMembers(e contract.Envelope, tool string, h Host, container string) contract.Envelope {
	e = e.With("missing", tool).With("host", h.Alias)
	if container != "" {
		e = e.With("container", container)
	}
	return e
}

// missingCode reports a code of a missing program: the catalog's own
// codes and prerequisite_missing, plus the ones the local functions give
// a tool outside the catalog.
func missingCode(code string) bool {
	switch code {
	case prereq.CodeMissing, "jq_not_found", "hash_tool_not_found":
		return true
	}
	for _, t := range prereq.Catalog() {
		if t.Code == code {
			return true
		}
	}
	return false
}

// AttributeHost is the stderr of a binary-dispatched call with its
// missing-program envelope (a catalog code, or the "missing" member) made
// to say where the program is missing: " on ALIAS" after the details, "On
// ALIAS: " before the hint (its install command is the host's: the gash
// there detected its own system), and the member "host". Any other stderr
// is returned as it is.
func AttributeHost(stderr []byte, alias string) []byte {
	trimmed := bytes.TrimRight(stderr, "\n")
	start := bytes.LastIndexByte(trimmed, '\n') + 1
	line := trimmed[start:]
	e, ok := parseEnvelope(line)
	if !ok || !(missingCode(e.Code) || hasExtra(e, "missing")) || hasExtra(e, "host") {
		return stderr
	}
	if e.Details != "" {
		e.Details += " on " + alias
	}
	if e.Hint != "" {
		e.Hint = "On " + alias + ": " + e.Hint
	}
	e = e.With("host", alias)
	return append(append([]byte{}, stderr[:start]...), e.Bytes()...)
}

func hasExtra(e contract.Envelope, key string) bool {
	for _, f := range e.Extra {
		if f.Key == key {
			return true
		}
	}
	return false
}

// parseEnvelope reads one envelope line back: the contract members and
// the extra string members, in order. ok is false for anything else (an
// extra member that is not a string included).
func parseEnvelope(line []byte) (contract.Envelope, bool) {
	var e contract.Envelope
	if len(line) == 0 || line[0] != '{' || !json.Valid(line) {
		return e, false
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	if _, err := dec.Token(); err != nil {
		return e, false
	}
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			return e, false
		}
		key, _ := k.(string)
		v, err := dec.Token()
		if err != nil {
			return e, false
		}
		if key == "recoverable" {
			if _, ok := v.(bool); !ok {
				return e, false
			}
			continue
		}
		s, ok := v.(string)
		if !ok {
			return e, false
		}
		switch key {
		case "error":
			e.Code = s
		case "details":
			e.Details = s
		case "action":
			e.Action = contract.NormalizeAction(s)
		case "hint":
			e.Hint = s
		default:
			e.Extra = append(e.Extra, contract.Field{Key: key, Value: s})
		}
	}
	return e, e.Code != "" && e.Action != ""
}
