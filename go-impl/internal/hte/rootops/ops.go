// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package rootops is the privileged side of gash hte (design/hte.md 7): a
// closed, typed set of operations that install, migrate --apply and
// uninstall run as root through one interactive sudo per command,
//
//	sudo -- <gash> hte __root
//
// The user side sends the plan the user saw (Hello), then asks for its
// operations one by one by id; root validates every operation against its
// own reading of the system before acting, records each change in its own
// copy of the receipt before making it, and never executes anything
// generated, from /tmp or elsewhere. Nothing is read from the user side but
// the plan: the user is the one sudo names (SUDO_UID), not the one the plan
// claims.
package rootops

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/receipt"
)

// Operation kinds.
const (
	// OpInstallHelper copies the hosts helper (Src, whose SHA256 the plan
	// names) to /usr/local/libexec/gash/gash-hosts-helper, root 0755.
	OpInstallHelper = "install_helper"
	// OpWriteSudoers writes /etc/sudoers.d/gash-hte for the sudo user:
	// a temporary file, visudo -c -f, rename.
	OpWriteSudoers = "write_sudoers"
	// OpSysctlPorts sets net.ipv4.ip_unprivileged_port_start to Value
	// unless it is already that or lower.
	OpSysctlPorts = "sysctl_ports"
	// OpLinger enables systemd linger for the user.
	OpLinger = "linger"
	// OpUnitStop stops and disables a system unit (Name).
	OpUnitStop = "unit_stop"
	// OpUndoUnitStop starts and enables again the system unit Name that
	// this session stopped (its change must be the last of the receipt):
	// the rollback of migrate --apply when a site does not answer after
	// the switch. The user side asks for it only on that failure.
	OpUndoUnitStop = "undo_unit_stop"
	// OpUnitReload reloads a running system unit (Name), after its pools
	// were archived; no receipt change.
	OpUnitReload = "unit_reload"
	// OpHostsSnapshot records the hosts block as it is before install.
	OpHostsSnapshot = "hosts_snapshot"
	// OpArchiveVhost disables a legacy vhost (Path) and moves it into the
	// archive.
	OpArchiveVhost = "archive_vhost"
	// OpArchivePool moves a legacy pool (Path) into the archive.
	OpArchivePool = "archive_pool"
	// OpCopyKey hands the legacy key Src, opened by root, to openssl pkey
	// running as the user, which writes Dst; root never reads the key.
	OpCopyKey = "copy_key"
	// OpRecordAutostart records the autostart the user side set up (Mode,
	// Path), so that both receipt copies stay equal.
	OpRecordAutostart = "record_autostart"
	// OpRestore undoes the change Change of the root receipt; it must be
	// the last one (changes are undone in reverse order).
	OpRestore = "restore"
	// OpFinish removes the root receipt once every change is undone, and
	// the archive and backup directories when empty.
	OpFinish = "finish_uninstall"
)

var opKinds = []string{OpInstallHelper, OpWriteSudoers, OpSysctlPorts, OpLinger, OpUnitStop, OpUndoUnitStop, OpUnitReload, OpHostsSnapshot,
	OpArchiveVhost, OpArchivePool, OpCopyKey, OpRecordAutostart, OpRestore, OpFinish}

// Op is one operation of a plan.
type Op struct {
	ID     int    `json:"id"`
	Op     string `json:"op"`
	Src    string `json:"src,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Dst    string `json:"dst,omitempty"`
	Path   string `json:"path,omitempty"`
	Name   string `json:"name,omitempty"`
	Value  int    `json:"value,omitempty"`
	Mode   string `json:"mode,omitempty"`
	Change int    `json:"change,omitempty"`
}

// String is the plan line shown to the user before sudo asks.
func (o Op) String() string {
	switch o.Op {
	case OpInstallHelper:
		return "install " + HelperPath + " (root 0755) from " + o.Src
	case OpWriteSudoers:
		return "write " + SudoersPath + " (NOPASSWD for the hosts helper only), validated with visudo -c -f"
	case OpSysctlPorts:
		return fmt.Sprintf("set %s to %d in %s (every local user may then bind ports %d to 1023)", PortSysctl, o.Value, SysctlFile, o.Value)
	case OpLinger:
		return "enable systemd linger (loginctl enable-linger), so the stack starts at boot"
	case OpUnitStop:
		return "stop and disable the system unit " + o.Name
	case OpUndoUnitStop:
		return "only if a site does not answer after the switch: start and enable " + o.Name + " again"
	case OpUnitReload:
		return "reload the system unit " + o.Name
	case OpHostsSnapshot:
		return "record the gash block of /etc/hosts as it is now, for uninstall"
	case OpArchiveVhost:
		return "disable " + o.Path + " and move it into " + receipt.ArchiveDir
	case OpArchivePool:
		return "move " + o.Path + " into " + receipt.ArchiveDir
	case OpCopyKey:
		return "copy the key " + o.Src + " to " + o.Dst + " (through openssl, as you)"
	case OpRecordAutostart:
		return "record the autostart mode " + o.Mode
	case OpRestore:
		return fmt.Sprintf("undo change %d of the receipt", o.Change)
	case OpFinish:
		return "remove the root receipt"
	}
	return o.Op
}

// Fixed system paths.
const (
	HelperPath  = "/usr/local/libexec/gash/gash-hosts-helper"
	SudoersDir  = "/etc/sudoers.d"
	SudoersPath = SudoersDir + "/gash-hte"
	// sudoersTmp has a dot: sudo ignores such names in sudoers.d.
	sudoersTmp = SudoersDir + "/.gash-hte.tmp"
	PortSysctl = "net.ipv4.ip_unprivileged_port_start"
	SysctlFile = "/etc/sysctl.d/50-gash-hte.conf"
	sysctlProc = "/proc/sys/net/ipv4/ip_unprivileged_port_start"
	lingerDir  = "/var/lib/systemd/linger"
)

var (
	sha256RE = regexp.MustCompile(`^[0-9a-f]{64}$`)
	// unitRE is the closed set of system units the root step touches.
	unitRE = regexp.MustCompile(`^(apache2|httpd|php-fpm|php[5-8]\.[0-9]-fpm|php[5-8][0-9]-php-fpm)$`)
	modes  = []string{"systemd", "launchd", "shell", "none"}
)

func absClean(p string) bool {
	return strings.HasPrefix(p, "/") && path.Clean(p) == p && !strings.ContainsAny(p, "\x00\n")
}

// Check validates an operation's arguments by themselves; Serve then
// checks them against the system before running it.
func (o Op) Check() error {
	bad := func(format string, args ...any) error {
		return hteerr.Newf(hteerr.InvalidState, "", "root operation %d (%s): "+format, append([]any{o.ID, o.Op}, args...)...)
	}
	if !slices.Contains(opKinds, o.Op) {
		return bad("unknown operation")
	}
	switch o.Op {
	case OpInstallHelper:
		if !absClean(o.Src) || !sha256RE.MatchString(o.SHA256) {
			return bad("needs an absolute source and its sha256")
		}
	case OpSysctlPorts:
		if o.Value < 0 || o.Value > 1024 {
			return bad("port %d", o.Value)
		}
	case OpUnitStop, OpUndoUnitStop, OpUnitReload:
		if !unitRE.MatchString(o.Name) {
			return bad("%q is not a unit gash may touch", o.Name)
		}
	case OpArchiveVhost, OpArchivePool:
		if !absClean(o.Path) || !strings.HasSuffix(o.Path, ".conf") {
			return bad("path %q", o.Path)
		}
	case OpCopyKey:
		if !absClean(o.Src) || !absClean(o.Dst) || path.Base(o.Dst) != "key.pem" {
			return bad("needs an absolute source and a key.pem destination")
		}
	case OpRecordAutostart:
		if !slices.Contains(modes, o.Mode) || (o.Path != "" && !absClean(o.Path)) {
			return bad("mode %q path %q", o.Mode, o.Path)
		}
	case OpRestore:
		if o.Change <= 0 {
			return bad("change %d", o.Change)
		}
	}
	return nil
}

// Hello opens a session: the plan.
type Hello struct {
	Plan []Op `json:"plan"`
}

// Request is one line from the user side: the hello, then one run per
// operation.
type Request struct {
	Hello *Hello `json:"hello,omitempty"`
	Run   int    `json:"run,omitempty"`
}

// WireError is an error on the wire.
type WireError struct {
	Code    hteerr.Code `json:"code"`
	Details string      `json:"details"`
	Hint    string      `json:"hint,omitempty"`
}

// Response is one line from root.
type Response struct {
	ID       int              `json:"id"`
	OK       bool             `json:"ok"`
	Changes  []receipt.Change `json:"changes,omitempty"`
	Warnings []string         `json:"warnings,omitempty"`
	// Receipt is the root copy after the operation, byte for byte ("" when
	// there is none): the user side stores it as its own copy.
	Receipt string     `json:"receipt"`
	Error   *WireError `json:"error,omitempty"`
}

func wire(err error) *WireError {
	if e, ok := hteerr.As(err); ok {
		return &WireError{Code: e.Code, Details: e.Details, Hint: e.Hint}
	}
	return &WireError{Code: hteerr.PrivilegedStepRequired, Details: err.Error(), Hint: "run gash hte doctor"}
}

// Err turns a wire error back into an error.
func (w *WireError) Err() error {
	if w == nil {
		return nil
	}
	return hteerr.New(w.Code, w.Details, w.Hint)
}
