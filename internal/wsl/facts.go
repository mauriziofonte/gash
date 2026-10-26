// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package wsl reads what the Windows Subsystem for Linux tells a
// distribution about itself, from files only (no process starts): whether
// this is WSL and which version, the interop registration and its socket,
// the Windows drives and where they are mounted, systemd, networking and
// the two configuration files. It also translates paths between the Linux
// and the Windows forms, as wslpath does, without starting wslpath.
//
// Everything is read through a platform.Sysroot, so the tests run against
// a directory that holds a fake /proc and /etc.
package wsl

import (
	"bytes"
	"io/fs"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/platform"
)

// InteropDir holds the Unix sockets of the interop servers: one per
// session leader and one for init, named PID_interop.
const InteropDir = "/run/WSL"

// Facts is what Detect found.
type Facts struct {
	// WSL is true under the Windows Subsystem for Linux.
	WSL bool `json:"wsl"`
	// Version is 2 for the lightweight VM, 1 for the translation layer,
	// 0 off WSL.
	Version int `json:"version"`
	// Kernel is /proc/sys/kernel/osrelease.
	Kernel string `json:"kernel"`
	// Distro is the distribution name Windows knows (WSL_DISTRO_NAME).
	Distro string `json:"distro"`
	// Interop describes the Windows interop of this shell.
	Interop Interop `json:"interop"`
	// AutomountRoot is where Windows drives are mounted ("/mnt/"), from
	// the drvfs mounts (symlinkroot) or /etc/wsl.conf; "/mnt/" by default.
	AutomountRoot string `json:"automount_root"`
	// Drives are the mounted Windows drives, by letter.
	Drives []Drive `json:"drives"`
	// Systemd is true when PID 1 is systemd ([boot] systemd=true).
	Systemd bool `json:"systemd"`
	// WindowsPath is true when PATH holds directories of a Windows drive
	// ([interop] appendWindowsPath, on by default).
	WindowsPath bool `json:"windows_path"`
	// Networking is "mirrored", "nat" or "" when unknown.
	Networking string `json:"networking"`
	// DNSTunneling is true when /etc/resolv.conf names the DNS tunneling
	// address (10.255.255.254, dnsTunneling=true).
	DNSTunneling bool `json:"dns_tunneling"`
	// WSLg is true when the WSLg mount is there (Linux GUI apps).
	WSLg bool `json:"wslg"`
}

// Interop is the state of the Windows interop.
type Interop struct {
	// Registered is true when the WSLInterop binfmt entry exists: the
	// kernel hands Windows executables to /init.
	Registered bool `json:"registered"`
	// Enabled is the entry's state ("enabled" on its first line).
	Enabled bool `json:"enabled"`
	// Socket is the interop server this shell reaches: WSL_INTEROP when it
	// names a socket, else the newest live one in /run/WSL; "" when none.
	Socket string `json:"socket"`
	// Stale is true when WSL_INTEROP names a socket that is gone (a tmux
	// or screen session reattached from a new terminal).
	Stale bool `json:"stale"`
	// Env is WSL_INTEROP as the shell has it.
	Env string `json:"env"`
}

// Usable reports interop that can start a Windows program.
func (i Interop) Usable() bool { return i.Registered && i.Enabled && i.Socket != "" }

// Drive is one Windows drive mounted in the distribution. A drive can be
// mounted more than once (the automount and an fstab or a manual drvfs
// mount, or one of its directories elsewhere): Mount is the preferred
// mount point of the whole drive (the automount one, as /mnt/c, first),
// Mounts lists every one, Mount's first. FSType, Options and Metadata are
// Mount's.
type Drive struct {
	Letter  string `json:"letter"` // "C"
	Mount   string `json:"mount"`  // "/mnt/c"
	FSType  string `json:"fstype"` // "9p", "drvfs" (WSL 1), "virtiofs"
	Options string `json:"options"`
	// Metadata is the drvfs metadata option: Linux permissions and owners
	// kept in NTFS extended attributes.
	Metadata bool         `json:"metadata"`
	Mounts   []DriveMount `json:"mounts"`
}

// DriveMount is one mount point of a drive and the directory of the drive
// it shows: `\` for the whole drive, `\Users\me` for a mount of a
// directory (a drvfs mount of C:\Users\me, or a bind mount).
type DriveMount struct {
	Path     string `json:"path"`
	Dir      string `json:"dir"`
	FSType   string `json:"fstype"`
	Options  string `json:"options"`
	Metadata bool   `json:"metadata"`
}

// whole reports a mount of the drive's root.
func (m DriveMount) whole() bool { return m.Dir == `\` }

// mounts is d.Mounts, or the one Mount of a Drive built without them.
func (d Drive) mounts() []DriveMount {
	if len(d.Mounts) > 0 || d.Mount == "" {
		return d.Mounts
	}
	return []DriveMount{{Path: d.Mount, Dir: `\`, FSType: d.FSType, Options: d.Options, Metadata: d.Metadata}}
}

// Detect reads the facts through sr. getenv is the caller's environment
// (WSL_DISTRO_NAME, WSL_INTEROP, PATH).
func Detect(sr platform.Sysroot, getenv func(string) string) Facts {
	var f Facts
	rel, _ := sr.ReadFile("/proc/sys/kernel/osrelease")
	f.Kernel = strings.TrimSpace(string(rel))
	ver, _ := sr.ReadFile("/proc/version")
	entry, entryErr := sr.ReadFile("/proc/sys/fs/binfmt_misc/WSLInterop")
	lowerKernel := strings.ToLower(f.Kernel + " " + string(ver))
	f.WSL = entryErr == nil || strings.Contains(lowerKernel, "microsoft")
	if !f.WSL {
		return f
	}
	f.Version = 1
	if strings.Contains(lowerKernel, "wsl2") || strings.Contains(lowerKernel, "microsoft-standard") {
		f.Version = 2
	}
	f.Distro = getenv("WSL_DISTRO_NAME")
	f.Interop = detectInterop(sr, entry, entryErr == nil, getenv("WSL_INTEROP"))

	mi, _ := sr.ReadFile("/proc/self/mountinfo")
	mounts, root := parseMounts(mi)
	f.AutomountRoot = root
	if f.AutomountRoot == "" {
		if conf, err := sr.ReadFile("/etc/wsl.conf"); err == nil {
			if v, ok := ParseINI(conf).Get("automount", "root"); ok && v != "" {
				f.AutomountRoot = strings.TrimSuffix(v, "/") + "/"
			}
		}
	}
	if f.AutomountRoot == "" {
		f.AutomountRoot = "/mnt/"
	}
	f.Drives = groupDrives(mounts, f.AutomountRoot)
	comm, _ := sr.ReadFile("/proc/1/comm")
	f.Systemd = strings.TrimSpace(string(comm)) == "systemd"
	for _, dir := range strings.Split(getenv("PATH"), ":") {
		if f.OnDrive(dir) {
			f.WindowsPath = true
			break
		}
	}
	if _, err := sr.Lstat("/sys/class/net/loopback0"); err == nil {
		f.Networking = "mirrored"
	} else if _, err := sr.Lstat("/sys/class/net/eth0"); err == nil {
		f.Networking = "nat"
	}
	if rc, err := sr.ReadFile("/etc/resolv.conf"); err == nil {
		for _, line := range strings.Split(string(rc), "\n") {
			if fs := strings.Fields(line); len(fs) == 2 && fs[0] == "nameserver" && fs[1] == DNSTunnelingAddress {
				f.DNSTunneling = true
			}
		}
	}
	if fi, err := sr.Lstat("/mnt/wslg"); err == nil && fi.IsDir() {
		f.WSLg = true
	}
	return f
}

// DNSTunnelingAddress is the nameserver WSL writes into resolv.conf when
// DNS tunneling is on (the default of dnsTunnelingIpAddress).
const DNSTunnelingAddress = "10.255.255.254"

// detectInterop reads the binfmt entry and picks the socket.
func detectInterop(sr platform.Sysroot, entry []byte, registered bool, env string) Interop {
	i := Interop{Registered: registered, Env: env}
	if registered {
		first, _, _ := bytes.Cut(entry, []byte("\n"))
		i.Enabled = strings.TrimSpace(string(first)) == "enabled"
	}
	if env != "" {
		if isSocket(sr, env) {
			i.Socket = env
			return i
		}
		i.Stale = true
	}
	i.Socket = LiveSocket(sr)
	return i
}

// LiveSocket returns the interop socket of the newest live server in
// /run/WSL (the highest PID whose process exists), "" when there is none.
// Any live server reaches Windows; the newest is the one a new terminal
// would get.
func LiveSocket(sr platform.Sysroot) string {
	names, err := readDirNames(sr, InteropDir)
	if err != nil {
		return ""
	}
	best, bestPID := "", -1
	for _, n := range names {
		pidText, ok := strings.CutSuffix(n, "_interop")
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(pidText)
		if err != nil || pid <= bestPID {
			continue
		}
		p := InteropDir + "/" + n
		if !isSocket(sr, p) {
			continue
		}
		if _, err := sr.Lstat("/proc/" + pidText); err != nil {
			continue
		}
		best, bestPID = p, pid
	}
	return best
}

func isSocket(sr platform.Sysroot, p string) bool {
	fi, err := sr.Stat(p)
	return err == nil && fi.Mode().Type() == fs.ModeSocket
}

// letterMount is a mount point of the drive of letter.
type letterMount struct {
	letter string
	DriveMount
}

// parseMounts returns the mounts of Windows drives of a mountinfo text, in
// its order, and the automount root their options name (symlinkroot=, ""
// when none does). A later mount on the same mount point hides the first:
// only the later one is kept.
func parseMounts(mi []byte) ([]letterMount, string) {
	var mounts []letterMount
	root := ""
	for _, line := range strings.Split(string(mi), "\n") {
		pre, post, ok := strings.Cut(line, " - ")
		if !ok {
			continue
		}
		pf, qf := strings.Fields(pre), strings.Fields(post)
		if len(pf) < 5 || len(qf) < 2 {
			continue
		}
		fsRoot, mount, fstype, source := unescape(pf[3]), unescape(pf[4]), qf[0], unescape(qf[1])
		opts := ""
		if len(qf) > 2 {
			opts = qf[2]
		}
		if fstype != "9p" && fstype != "drvfs" && fstype != "virtiofs" {
			continue
		}
		letter, dir := driveLetter(source, opts, mount, fstype)
		if letter == "" {
			continue
		}
		m := letterMount{letter, DriveMount{Path: mount, Dir: joinDir(dir, fsRoot), FSType: fstype, Options: opts}}
		for _, o := range strings.FieldsFunc(opts, func(r rune) bool { return r == ',' || r == ';' }) {
			if o == "metadata" {
				m.Metadata = true
			}
			if v, ok := strings.CutPrefix(o, "symlinkroot="); ok && root == "" {
				root = v
			}
		}
		mounts = slices.DeleteFunc(mounts, func(x letterMount) bool { return x.Path == mount })
		mounts = append(mounts, m)
	}
	return mounts, root
}

// groupDrives makes one Drive per letter of mounts, sorted by letter. Of
// the mounts of a letter, the whole drive at the automount root (/mnt/c)
// comes first, then the other mounts of the whole drive, then those of a
// directory, each group in mountinfo order.
func groupDrives(mounts []letterMount, automountRoot string) []Drive {
	var drives []Drive
	for _, m := range mounts {
		i := slices.IndexFunc(drives, func(d Drive) bool { return d.Letter == m.letter })
		if i < 0 {
			drives = append(drives, Drive{Letter: m.letter})
			i = len(drives) - 1
		}
		drives[i].Mounts = append(drives[i].Mounts, m.DriveMount)
	}
	auto := strings.TrimSuffix(automountRoot, "/") + "/"
	rank := func(letter string, m DriveMount) int {
		switch {
		case m.whole() && m.Path == auto+strings.ToLower(letter):
			return 0
		case m.whole():
			return 1
		}
		return 2
	}
	for i := range drives {
		d := &drives[i]
		slices.SortStableFunc(d.Mounts, func(a, b DriveMount) int { return rank(d.Letter, a) - rank(d.Letter, b) })
		first := d.Mounts[0]
		d.Mount, d.FSType, d.Options, d.Metadata = first.Path, first.FSType, first.Options, first.Metadata
	}
	slices.SortFunc(drives, func(a, b Drive) int { return strings.Compare(a.Letter, b.Letter) })
	return drives
}

// joinDir is the drive directory of a mount: dir (`\` or `\Users`, from
// the source or path=) and below it the mountinfo root of a bind mount
// (/me).
func joinDir(dir, fsRoot string) string {
	if fsRoot == "" || fsRoot == "/" {
		return dir
	}
	sub := strings.ReplaceAll(strings.Trim(fsRoot, "/"), "/", `\`)
	if dir == `\` {
		return `\` + sub
	}
	return dir + `\` + sub
}

// driveLetter finds the drive letter of a drvfs mount and the directory
// of the drive it mounts (`\` for the root): path=C:\ in the 9p options,
// the source C:\ (or C:), or the last component of a mount point that is
// one letter, for a virtiofs share.
func driveLetter(source, opts, mount, fstype string) (string, string) {
	isDrvfs := fstype == "drvfs" || strings.Contains(opts, "aname=drvfs")
	for _, o := range strings.FieldsFunc(opts, func(r rune) bool { return r == ',' || r == ';' }) {
		if v, ok := strings.CutPrefix(o, "path="); ok && isLetterColon(v) {
			return strings.ToUpper(v[:1]), driveDir(v[2:])
		}
	}
	if isLetterColon(source) && (isDrvfs || fstype == "virtiofs") {
		return strings.ToUpper(source[:1]), driveDir(source[2:])
	}
	if fstype == "virtiofs" || isDrvfs {
		if b := path.Base(mount); len(b) == 1 && isLetter(b[0]) {
			return strings.ToUpper(b), `\`
		}
	}
	return "", ""
}

// driveDir is the directory part of a Windows path after its X:, as `\`
// or `\Users\me`.
func driveDir(rest string) string {
	rest = strings.Trim(strings.ReplaceAll(rest, "/", `\`), `\`)
	if rest == "" {
		return `\`
	}
	return `\` + rest
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func isLetterColon(s string) bool { return len(s) >= 2 && isLetter(s[0]) && s[1] == ':' }

// unescape decodes the octal escapes of a mountinfo field (\040 for a
// space, \134 for a backslash).
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Drive returns the drive mounted for letter.
func (f Facts) Drive(letter string) (Drive, bool) {
	letter = strings.ToUpper(letter)
	for _, d := range f.Drives {
		if d.Letter == letter {
			return d, true
		}
	}
	return Drive{}, false
}

// DriveOf returns the drive one of whose mounts holds the absolute path p.
func (f Facts) DriveOf(p string) (Drive, bool) {
	d, _, ok := f.mountOf(p)
	return d, ok
}

// mountOf returns the drive mount that holds the absolute path p: of
// nested mounts, the deepest.
func (f Facts) mountOf(p string) (Drive, DriveMount, bool) {
	p = path.Clean(p)
	var bd Drive
	var bm DriveMount
	found := false
	for _, d := range f.Drives {
		for _, m := range d.mounts() {
			if (p == m.Path || strings.HasPrefix(p, strings.TrimSuffix(m.Path, "/")+"/")) && (!found || len(m.Path) > len(bm.Path)) {
				bd, bm, found = d, m, true
			}
		}
	}
	return bd, bm, found
}

// OnDrive reports an absolute path on a Windows drive.
func (f Facts) OnDrive(p string) bool {
	if !strings.HasPrefix(p, "/") {
		return false
	}
	_, ok := f.DriveOf(p)
	return ok
}

// SystemDrive is the drive that holds Windows (C, else the first drive
// with a Windows directory), with Mount set to the mount of the whole
// drive where Windows was found: every mount of the drive is tried, the
// preferred first.
func (f Facts) SystemDrive(exists func(string) bool) (Drive, bool) {
	try := func(d Drive) (Drive, bool) {
		for _, m := range d.mounts() {
			if m.whole() && exists(m.Path+"/Windows/System32") {
				d.Mount, d.FSType, d.Options, d.Metadata = m.Path, m.FSType, m.Options, m.Metadata
				return d, true
			}
		}
		return Drive{}, false
	}
	if d, ok := f.Drive("C"); ok {
		if d, ok := try(d); ok {
			return d, true
		}
	}
	for _, d := range f.Drives {
		if d, ok := try(d); ok {
			return d, true
		}
	}
	return Drive{}, false
}

// readDirNames lists a directory of the sysroot.
func readDirNames(sr platform.Sysroot, dir string) ([]string, error) {
	entries, err := os.ReadDir(sr.Path(dir))
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names, nil
}
