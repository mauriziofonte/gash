// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package hostshelper is the core of gash-hosts-helper, the only program
// gash runs as root without a password (design/hte.md section 7.1). It
// edits one region of the hosts file, between "# >>> gash hte >>>" and
// "# <<< gash hte <<<", and accepts only loopback addresses and validated
// hostnames. It never takes a path from its caller: the hosts file is a
// constant of the binary, and Config.HostsPath exists only for tests.
//
// The package is a leaf (standard library only), so that the helper binary
// stays small, and internal/hte/state shares ValidHostname with it: every
// site name fits in the block.
package hostshelper

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Markers of the managed region, each a full line of the hosts file.
const (
	BeginMarker = "# >>> gash hte >>>"
	EndMarker   = "# <<< gash hte <<<"
)

// Limits of the input grammar.
const (
	MaxLines      = 4096    // lines of desired block
	MaxInputBytes = 1 << 20 // bytes read from stdin
	MaxHostsBytes = 16 << 20
	MaxNameBytes  = 253
)

// HostsFile is the only file the helper binary writes.
const HostsFile = "/etc/hosts"

// Exit statuses of the helper.
const (
	ExitOK      = 0
	ExitFailure = 1 // I/O error or a hosts file whose markers are broken
	ExitRefused = 2 // usage error or input outside the grammar; nothing written
)

// Addresses allowed in the block.
var allowedAddresses = map[string]bool{"127.0.0.1": true, "::1": true}

// Entry is one line of the block: a loopback address and its names.
type Entry struct {
	Address string   `json:"address"`
	Names   []string `json:"names"`
}

// InputError is a refusal of the input grammar.
type InputError struct {
	Line   int // 1-based; 0 for the input as a whole
	Reason string
}

func (e *InputError) Error() string {
	if e.Line == 0 {
		return "refused: " + e.Reason
	}
	return fmt.Sprintf("refused: line %d: %s", e.Line, e.Reason)
}

// ErrCorruptBlock is returned when the hosts file holds the markers in any
// shape other than one begin line followed later by one end line.
var ErrCorruptBlock = errors.New("the gash hte markers in the hosts file are broken")

// ValidHostname reports whether name is a lowercase hostname gash accepts
// for a site or alias: labels [a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?, at least
// two labels, at most 253 bytes, no wildcard and no trailing dot. The last
// label may not be a number (decimal, or 0x hexadecimal): such a name is
// parsed as an IPv4 address by the resolver (inet_aton accepts "127.1" and
// "0x7f.1") and never looked up in the hosts file; this is how IP addresses
// are refused as site names (LEDGER HTE-21).
func ValidHostname(name string) bool {
	if name == "" || len(name) > MaxNameBytes {
		return false
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if !validLabel(l) {
			return false
		}
	}
	return !numericLabel(labels[len(labels)-1])
}

func validLabel(l string) bool {
	if len(l) < 1 || len(l) > 63 {
		return false
	}
	for i := 0; i < len(l); i++ {
		c := l[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-' && i > 0 && i < len(l)-1:
		default:
			return false
		}
	}
	return true
}

func numericLabel(l string) bool {
	if hex, ok := strings.CutPrefix(l, "0x"); ok {
		return strings.Trim(hex, "0123456789abcdef") == ""
	}
	return strings.Trim(l, "0123456789") == ""
}

// ParseBlock parses the desired block: at most MaxLines lines, each
// "ADDRESS NAME..." with single spaces, ADDRESS exactly 127.0.0.1 or ::1,
// every NAME a ValidHostname, every line ending in a newline. Empty input
// is an empty block. Anything else is an *InputError.
func ParseBlock(input []byte) ([]Entry, error) {
	if len(input) > MaxInputBytes {
		return nil, &InputError{Reason: fmt.Sprintf("input larger than %d bytes", MaxInputBytes)}
	}
	if len(input) == 0 {
		return nil, nil
	}
	for i, c := range input {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == ':' || c == ' ' || c == '\n') {
			line := bytes.Count(input[:i], []byte("\n")) + 1
			return nil, &InputError{Line: line, Reason: fmt.Sprintf("byte 0x%02x is not allowed", c)}
		}
	}
	if input[len(input)-1] != '\n' {
		return nil, &InputError{Line: bytes.Count(input, []byte("\n")) + 1, Reason: "the last line has no newline"}
	}
	lines := strings.Split(string(input[:len(input)-1]), "\n")
	if len(lines) > MaxLines {
		return nil, &InputError{Reason: fmt.Sprintf("more than %d lines", MaxLines)}
	}
	entries := make([]Entry, 0, len(lines))
	for i, line := range lines {
		fields := strings.Split(line, " ")
		if len(fields) < 2 {
			return nil, &InputError{Line: i + 1, Reason: "want an address and at least one name"}
		}
		if !allowedAddresses[fields[0]] {
			return nil, &InputError{Line: i + 1, Reason: fmt.Sprintf("address %q is not 127.0.0.1 or ::1", fields[0])}
		}
		for _, n := range fields[1:] {
			if n == "" {
				return nil, &InputError{Line: i + 1, Reason: "fields must be separated by exactly one space"}
			}
			if !ValidHostname(n) {
				return nil, &InputError{Line: i + 1, Reason: fmt.Sprintf("%q is not a valid hostname", n)}
			}
		}
		entries = append(entries, Entry{Address: fields[0], Names: fields[1:]})
	}
	return entries, nil
}

// FormatBlock renders entries as the block's inner lines (the input grammar).
func FormatBlock(entries []Entry) []byte {
	var b bytes.Buffer
	for _, e := range entries {
		b.WriteString(e.Address)
		for _, n := range e.Names {
			b.WriteByte(' ')
			b.WriteString(n)
		}
		b.WriteByte('\n')
	}
	return b.Bytes()
}

// region locates the managed region: the byte offsets of the begin marker
// line and of the end of the end marker line. found is false when neither
// marker is present.
func region(hosts []byte) (start, end int, found bool, err error) {
	begins, ends := 0, 0
	start, end = -1, -1
	off := 0
	for off < len(hosts) {
		nl := bytes.IndexByte(hosts[off:], '\n')
		next := len(hosts)
		line := hosts[off:]
		if nl >= 0 {
			next = off + nl + 1
			line = hosts[off : off+nl]
		}
		switch string(line) {
		case BeginMarker:
			begins++
			start = off
		case EndMarker:
			ends++
			end = next
		}
		off = next
	}
	switch {
	case begins == 0 && ends == 0:
		return 0, 0, false, nil
	case begins == 1 && ends == 1 && start < end:
		return start, end, true, nil
	}
	return 0, 0, false, ErrCorruptBlock
}

// Extract returns the inner lines of the managed region of hosts.
func Extract(hosts []byte) (block []byte, found bool, err error) {
	start, end, found, err := region(hosts)
	if err != nil || !found {
		return nil, false, err
	}
	inner := hosts[start+len(BeginMarker)+1 : end]
	inner = bytes.TrimSuffix(inner, []byte(EndMarker+"\n"))
	inner = bytes.TrimSuffix(inner, []byte(EndMarker))
	return inner, true, nil
}

// Splice returns hosts with the managed region replaced by block (inner
// lines), or with the region appended when it is absent. Every byte outside
// the region is kept.
func Splice(hosts, block []byte) ([]byte, error) {
	start, end, found, err := region(hosts)
	if err != nil {
		return nil, err
	}
	var region bytes.Buffer
	region.WriteString(BeginMarker + "\n")
	region.Write(block)
	region.WriteString(EndMarker + "\n")
	var out bytes.Buffer
	if !found {
		out.Write(hosts)
		if len(hosts) > 0 && hosts[len(hosts)-1] != '\n' {
			out.WriteByte('\n')
		}
		out.Write(region.Bytes())
		return out.Bytes(), nil
	}
	out.Write(hosts[:start])
	out.Write(region.Bytes())
	out.Write(hosts[end:])
	return out.Bytes(), nil
}

// Strip returns hosts without the managed region (both markers included),
// every other byte kept. Uninstall uses it when no block existed before
// install; hosts without a region is returned unchanged.
func Strip(hosts []byte) ([]byte, error) {
	start, end, found, err := region(hosts)
	if err != nil || !found {
		return hosts, err
	}
	out := append([]byte{}, hosts[:start]...)
	return append(out, hosts[end:]...), nil
}

// readHosts reads the hosts file, refusing a symlink or a non-regular file
// (renaming over a symlink would replace it with a file).
func readHosts(path string) ([]byte, os.FileInfo, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s is not a regular file", path)
	}
	if fi.Size() > MaxHostsBytes {
		return nil, nil, fmt.Errorf("%s is larger than %d bytes", path, MaxHostsBytes)
	}
	b, err := os.ReadFile(path)
	return b, fi, err
}

// Rewrite replaces the hosts file at path with edit's result, atomically,
// keeping its owner and mode; a symlink or non-regular file is refused.
// The root step of uninstall uses it to put back the block that was there
// before install (Splice), or to remove the block (Strip).
func Rewrite(path string, edit func(hosts []byte) ([]byte, error)) error {
	data, fi, err := readHosts(path)
	if err != nil {
		return err
	}
	out, err := edit(data)
	if err != nil {
		return err
	}
	if bytes.Equal(out, data) {
		return nil
	}
	return writeAtomic(path, out, fi)
}

// rename is os.Rename; tests replace it to make the rename fail as over a
// bind mount.
var rename = os.Rename

// writeAtomic replaces path with data: a temporary file in the same
// directory, the original owner and mode, fsync, rename, fsync of the
// directory. A container bind-mounts its hosts file, and a rename over a
// mount point fails with EBUSY (EXDEV across devices): then the temporary
// file is removed and the same content is written in place (LEDGER
// HTE-69).
func writeAtomic(path string, data []byte, orig os.FileInfo) (err error) {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".hosts.gash-hte-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Chmod(orig.Mode().Perm()); err != nil {
		return err
	}
	if st, ok := orig.Sys().(*syscall.Stat_t); ok {
		if err = f.Chown(int(st.Uid), int(st.Gid)); err != nil {
			return err
		}
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = rename(tmp, path); err != nil {
		if !errors.Is(err, syscall.EBUSY) && !errors.Is(err, syscall.EXDEV) {
			return err
		}
		if rerr := os.Remove(tmp); rerr != nil {
			return rerr
		}
		return writeInPlace(path, data, orig)
	}
	if d, derr := os.Open(dir); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// writeInPlace writes data over the file that was read: opened without
// following a symlink and without truncation, checked to be the same
// regular file, written from the start, truncated to the new length and
// synced. Owner and mode stay those of the file.
func writeInPlace(path string, data []byte, orig os.FileInfo) error {
	f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	fi, err := f.Stat()
	if err == nil && (!fi.Mode().IsRegular() || !os.SameFile(fi, orig)) {
		err = fmt.Errorf("%s changed while it was edited", path)
	}
	if err == nil {
		_, err = f.WriteAt(data, 0)
	}
	if err == nil {
		err = f.Truncate(int64(len(data)))
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// Config is the environment of Run. The helper binary passes HostsFile,
// runtime.GOOS and a real command runner; tests pass a temporary file and
// a recording fake.
type Config struct {
	HostsPath string
	GOOS      string
	// Exec runs a fixed command (absolute path first) with an empty
	// environment. It is only used for the macOS resolver flush.
	Exec func(argv []string) error
}

// FlushCommands returns the fixed commands that flush the resolver cache
// after a change: two on macOS, none elsewhere (LEDGER HTE-46, option a).
func FlushCommands(goos string) [][]string {
	if goos == "darwin" {
		return [][]string{
			{"/usr/bin/dscacheutil", "-flushcache"},
			{"/usr/bin/killall", "-HUP", "mDNSResponder"},
		}
	}
	return nil
}

const usage = "usage: gash-hosts-helper apply|check (apply reads the block from stdin)"

// Run implements the helper's argv protocol: exactly one argument, "apply"
// (the desired block on stdin) or "check" (prints the current block).
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer, cfg Config) int {
	fail := func(code int, format string, a ...any) int {
		fmt.Fprintf(stderr, "gash-hosts-helper: "+format+"\n", a...)
		return code
	}
	if len(args) != 1 {
		return fail(ExitRefused, "%s", usage)
	}
	switch args[0] {
	case "check":
		hosts, _, err := readHosts(cfg.HostsPath)
		if err != nil {
			return fail(ExitFailure, "%v", err)
		}
		block, _, err := Extract(hosts)
		if err != nil {
			return fail(ExitFailure, "%v", err)
		}
		if _, err := stdout.Write(block); err != nil {
			return ExitFailure
		}
		return ExitOK
	case "apply":
		input, err := io.ReadAll(io.LimitReader(stdin, MaxInputBytes+1))
		if err != nil {
			return fail(ExitFailure, "reading stdin: %v", err)
		}
		entries, err := ParseBlock(input)
		if err != nil {
			return fail(ExitRefused, "%v", err)
		}
		hosts, fi, err := readHosts(cfg.HostsPath)
		if err != nil {
			return fail(ExitFailure, "%v", err)
		}
		out, err := Splice(hosts, FormatBlock(entries))
		if err != nil {
			return fail(ExitFailure, "%v", err)
		}
		if bytes.Equal(out, hosts) {
			return ExitOK
		}
		if err := writeAtomic(cfg.HostsPath, out, fi); err != nil {
			return fail(ExitFailure, "writing %s: %v", cfg.HostsPath, err)
		}
		for _, argv := range FlushCommands(cfg.GOOS) {
			if cfg.Exec == nil {
				break
			}
			if err := cfg.Exec(argv); err != nil {
				fmt.Fprintf(stderr, "gash-hosts-helper: warning: %s: %v\n", argv[0], err)
			}
		}
		return ExitOK
	}
	return fail(ExitRefused, "%s", usage)
}
