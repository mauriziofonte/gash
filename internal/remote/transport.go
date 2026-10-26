// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

// The agent-toolkit transport (plan S5): one ssh invocation per call over
// the multiplexed connection of remote_exec, to a registered HOST: alias
// only. Everything the agent functions run remotely (llm_* --host, the
// binary they ship, llm_remote_run, llm_same, agent_put, the llm_db_*
// +ssh engines) goes through Transport.Exec.
//
// Every script ends with a trailer line on stderr that carries the status
// of what it ran, behind a random nonce: "\n<nonce> <tag> <status>\n". So
// ssh's own exit 255 (connection refused, host key, authentication) is
// never confused with a remote command that exited 255, and a call needs
// no second "is the connection alive" probe.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
)

// Host is a registered HOST: alias and its user@host target.
type Host struct {
	Alias  string
	Target string
	// NoBinary: GASH_REMOTE_NO_BINARY names the host, so the gash binary
	// is never copied there or run from its cache; the POSIX fallbacks
	// answer.
	NoBinary bool
	// Rules are the PROTECT: lines that apply on the host (those without
	// a host and its own), sent with every script (pathguard.ProtectVar).
	Rules []pathguard.Rule
}

// Resolve returns the HOST: entry named alias. Only registered aliases are
// accepted, interactive or not (the headless rule of remote_*): a raw
// user@host, an unknown alias and an alias whose target fails
// config.ValidHostTarget are unknown_host (STOP), before anything
// connects.
func Resolve(get func(string) string, alias string) (Host, *contract.Envelope) {
	f := config.LoadCaller(get)
	if t, ok := f.HostTarget(alias); ok && config.ValidHostTarget(t) {
		return Host{Alias: alias, Target: t, NoBinary: f.NoBinary(alias), Rules: pathguard.FileRules(f, alias)}, nil
	}
	hint := "Only HOST: aliases registered in ~/.gash_env are accepted (HOST:alias=user@host); ask the user to add one"
	if t, ok := f.RejectedHost(alias); ok {
		hint = "The HOST: target of '" + alias + "' must be user@host (" + t + " is not); ask the user to fix ~/.gash_env"
	}
	e := contract.New("unknown_host", alias, hint)
	return Host{}, &e
}

// Quote is s as one single-quoted POSIX shell word.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// QuoteAll is the words of args, each quoted, joined by spaces.
func QuoteAll(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = Quote(a)
	}
	return strings.Join(q, " ")
}

// NewNonce returns a fresh random marker for the trailer and frame lines
// of one script: no remote output can contain it by chance, and remote
// content cannot forge it.
func NewNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "gash-" + hex.EncodeToString(b)
}

// Trailer is the sh line that ends a script: the nonce, tag and the status
// in $1 on stderr, after a newline so the line stands on its own whatever
// the stderr before it ended with.
func Trailer(nonce, tag, status string) string {
	return `printf '\n%s %s %s\n' ` + Quote(nonce) + " " + Quote(tag) + " " + status + " >&2"
}

// TrailerDetail is Trailer with a detail word after the status (an sh
// expression, quoted by the caller).
func TrailerDetail(nonce, tag, status, detail string) string {
	return `printf '\n%s %s %s %s\n' ` + Quote(nonce) + " " + Quote(tag) + " " + status + " " + detail + " >&2"
}

// Transport runs scripts on hosts through ssh, with the caller's
// environment, the module's process seam and the multiplexing options of
// remote_exec.
type Transport struct {
	Run runner.Runner
	Env invoke.Env
}

// Result is one ssh invocation: the remote stdout, the remote stderr with
// the trailer line removed, the trailer's tag and status, and ssh's own
// exit status.
type Result struct {
	Stdout, Stderr []byte
	// Tag and Status are the trailer's; Tag is "" when the script printed
	// no trailer (the connection failed, or the remote shell did not run
	// the script to its end).
	Tag    string
	Status int
	// Detail is the trailer's text after the status ("" for most tags).
	Detail string
	// SSH is the exit status of ssh itself.
	SSH int
	// TimedOut reports that the timeout of the call ended ssh.
	TimedOut bool
	// Ms is how long the invocation took.
	Ms float64
	// noSSH is the prerequisite envelope when ssh itself could not start
	// (not on PATH, or not executable).
	noSSH *contract.Envelope
}

// Exec runs script on h in one ssh invocation, with stdin (nil: none) as
// its standard input, and parses the trailer of nonce off its stderr.
// timeout > 0 ends ssh after that long. The caller's ~/.ssh, the directory
// of the multiplexing socket, is created 0700 when missing, as remote_exec
// creates it (LEDGER REM-07).
func (t Transport) Exec(ctx context.Context, h Host, nonce, script string, stdin io.Reader, timeout time.Duration) Result {
	if ctx == nil {
		ctx = context.Background()
	}
	home := t.Env.Get("HOME")
	if home != "" {
		if _, err := os.Lstat(home + "/.ssh"); errors.Is(err, fs.ErrNotExist) {
			_ = os.Mkdir(home+"/.ssh", 0o700)
		}
	}
	args := append(sshOpts(home), "--", h.Target, historyOff+protectExport(h)+script)
	start := time.Now()
	res, err := t.Run.Run(ctx, runner.Cmd{Path: "ssh", Args: args, Options: runner.Options{
		Env: t.Env.Child(), Stdin: stdin, Timeout: timeout, KillAfter: time.Second,
	}})
	r := Result{Stdout: res.Stdout, SSH: res.ExitCode, TimedOut: res.TimedOut, Ms: msSince(start)}
	var se *runner.StartError
	if errors.As(err, &se) {
		// The entry points check ssh before connecting (RequireSSH); this
		// is the backstop, never host_unreachable.
		e := prereq.Current(&invoke.Call{Env: t.Env}).Report("ssh", "").Envelope()
		r.noSSH = &e
	}
	r.Stderr, r.Tag, r.Status, r.Detail = cutTrailer(res.Stderr, nonce)
	return r
}

// Unreachable reports that ssh itself failed: no trailer and ssh's 255.
// A 127 is the remote login shell's "not found", or ssh missing here:
// neither is a dead connection.
func (r Result) Unreachable() bool {
	return r.Tag == "" && r.SSH == 255
}

// Failure is the envelope of an invocation that did not run to its
// trailer: the prerequisite report when ssh is missing here,
// host_unreachable (RETRY) when ssh failed or timed out,
// remote_protocol_error (STOP) when the remote side ended early. nil when
// the trailer is there.
func (r Result) Failure(h Host) *contract.Envelope {
	if r.noSSH != nil {
		return r.noSSH
	}
	if r.Tag != "" {
		return nil
	}
	if r.TimedOut {
		e := contract.New("host_unreachable", h.Alias+": timed out", "The host did not answer in time; check it is up, then retry once")
		return &e
	}
	if r.Unreachable() {
		detail := h.Alias + " (" + h.Target + ")"
		if msg := lastLine(r.Stderr); msg != "" {
			detail += ": " + msg
		}
		e := contract.New("host_unreachable", detail, "Check that the host is up and that ssh to it works without a prompt (BatchMode), then retry once")
		return &e
	}
	detail := h.Alias + ": the remote side ended before its last line (ssh status " + strconv.Itoa(r.SSH) + ")"
	if msg := lastLine(r.Stderr); msg != "" {
		detail += ": " + msg
	}
	e := contract.New("remote_protocol_error", detail, "The remote login shell must run POSIX sh commands; check the account's shell and its startup files")
	return &e
}

// cutTrailer removes the last "\n<nonce> <tag> <status>[ <detail>]\n" line
// from stderr and returns the stderr before it with the trailer's fields.
func cutTrailer(stderr []byte, nonce string) ([]byte, string, int, string) {
	marker := []byte("\n" + nonce + " ")
	i := bytes.LastIndex(stderr, marker)
	if i < 0 {
		return stderr, "", 0, ""
	}
	line := stderr[i+len(marker):]
	rest := []byte(nil)
	if j := bytes.IndexByte(line, '\n'); j >= 0 {
		rest = line[j+1:]
		line = line[:j]
	}
	fields := strings.SplitN(string(line), " ", 3)
	tag := fields[0]
	status := 0
	if len(fields) > 1 {
		status, _ = strconv.Atoi(fields[1])
	}
	detail := ""
	if len(fields) > 2 {
		detail = fields[2]
	}
	out := append(append([]byte{}, stderr[:i]...), rest...)
	return out, tag, status, detail
}

func lastLine(b []byte) string {
	s := strings.TrimRight(string(b), "\n\r ")
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(s)
}

func msSince(t time.Time) float64 {
	d := time.Since(t)
	return float64(d.Round(100*time.Microsecond).Microseconds()/100) / 10
}
