// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package wslcmd

import (
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/wsl"
)

// ---- info ----

func (r *run) doInfo() error {
	f := r.detect()
	// info is a report, not a check: off WSL it answers and exits 0.
	if !f.WSL {
		if r.asJSON {
			writeJSON(r.c.Stdout, map[string]any{"wsl": false})
		} else {
			r.say().Info("Not running under WSL (the Windows Subsystem for Linux).")
		}
		return nil
	}
	if r.asJSON {
		writeJSON(r.c.Stdout, f)
		return nil
	}
	s := r.style()
	kv := []string{
		"WSL", "version " + strconv.Itoa(f.Version),
		"Distro", orDash(f.Distro),
		"Kernel", orDash(f.Kernel),
		"Interop", interopWord(f),
		"Windows PATH", onOff(f.WindowsPath),
		"Automount", f.AutomountRoot,
		"Drives", drivesLine(f),
		"systemd", onOff(f.Systemd),
		"Networking", orDash(f.Networking),
		"DNS tunneling", onOff(f.DNSTunneling),
		"WSLg", onOff(f.WSLg),
	}
	s.KV(r.c.Stdout, kv...)
	return nil
}

// ---- doctor ----

// check is one line of the doctor report.
type check struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // ok, warn, fail, info
	Details string `json:"details"`
	Remedy  string `json:"remedy,omitempty"`
}

func (r *run) doDoctor() error {
	if r.notWSL() {
		r.status = 1
		return nil
	}
	f := r.detect()
	checks := r.doctorChecks(f)
	failed := 0
	for _, c := range checks {
		if c.Status == "fail" {
			failed++
		}
	}
	if r.asJSON {
		writeJSON(r.c.Stdout, map[string]any{"wsl": true, "checks": checks, "facts": f, "failed": failed})
		if failed > 0 {
			r.status = 1
		}
		return nil
	}
	s := r.style()
	s.Section(r.c.Stdout, "WSL doctor")
	for _, c := range checks {
		s.Mark(r.c.Stdout, markOf(c.Status), c.Name+": "+c.Details)
		if c.Remedy != "" {
			_, _ = r.c.Stdout.Write([]byte("    remedy: " + c.Remedy + "\n"))
		}
	}
	if failed > 0 {
		r.say().Error(strconv.Itoa(failed) + " check(s) need attention")
		r.status = 1
		return nil
	}
	r.say().Success("WSL looks healthy")
	return nil
}

func (r *run) doctorChecks(f wsl.Facts) []check {
	var cs []check
	cs = append(cs, check{"version", "ok", "WSL " + strconv.Itoa(f.Version) + " (kernel " + orDash(f.Kernel) + ")", ""})

	switch {
	case !f.Interop.Registered:
		cs = append(cs, check{"interop", "warn", "the WSLInterop handler is not registered (no Windows programs)",
			"enable [interop] in /etc/wsl.conf and restart WSL"})
	case !f.Interop.Enabled:
		cs = append(cs, check{"interop", "warn", "the WSLInterop handler is disabled", "set [interop] enabled=true in /etc/wsl.conf"})
	case f.Interop.Stale:
		cs = append(cs, check{"interop", "fail", "WSL_INTEROP points at a socket that is gone (a reattached tmux/screen)",
			"run: eval \"$(gash wsl interop --export)\" — or open a new terminal"})
	case f.Interop.Socket == "":
		cs = append(cs, check{"interop", "fail", "no live interop server was found in " + wsl.InteropDir, "open a new terminal"})
	default:
		cs = append(cs, check{"interop", "ok", "live via " + f.Interop.Socket, ""})
	}

	if f.WindowsPath {
		cs = append(cs, check{"windows-path", "ok", "the Windows PATH is on PATH (.exe programs resolve by name)", ""})
	} else {
		cs = append(cs, check{"windows-path", "info", "appendWindowsPath is off; Gash finds Windows programs by absolute path anyway", ""})
	}

	if len(f.Drives) == 0 {
		cs = append(cs, check{"drives", "warn", "no Windows drive is mounted", "check [automount] in /etc/wsl.conf"})
	} else {
		meta := 0
		for _, d := range f.Drives {
			if d.Metadata {
				meta++
			}
		}
		det := drivesLine(f) + " on " + f.AutomountRoot
		if meta < len(f.Drives) {
			cs = append(cs, check{"drives", "info", det + "; metadata is off on some (chmod/owner do not persist there)",
				"add metadata to [automount] options if you need Unix permissions on /mnt"})
		} else {
			cs = append(cs, check{"drives", "ok", det, ""})
		}
	}

	if f.Networking != "" {
		cs = append(cs, check{"networking", "ok", f.Networking + networkNote(f), ""})
	}
	if f.DNSTunneling {
		cs = append(cs, check{"dns", "ok", "DNS tunneling is on (VPN-friendly)", ""})
	}
	cs = append(cs, check{"systemd", statusBool(f.Systemd), "systemd as PID 1: " + onOff(f.Systemd), ""})

	cs = append(cs, check{"filesystem", "info",
		"build and run projects on the Linux disk (ext4); /mnt is for reading Windows files, not for compiling", ""})
	return cs
}

func networkNote(f wsl.Facts) string {
	if f.Networking == "mirrored" {
		return " (Windows and WSL share the network stack)"
	}
	return ""
}

// ---- path ----

func (r *run) doPath(args []string) error {
	if len(args) == 0 {
		return r.failErr(contract.New("missing_value", "no path given", "Run: gash wsl path PATH"))
	}
	f := r.detect()
	in := args[0]
	var out string
	var err error
	switch {
	case r.toLinux:
		out, err = f.ToLinux(in)
	case r.toMixed:
		out, err = f.ToWindows(linuxify(f, in), wsl.FormMixed)
	case r.toWin:
		out, err = f.ToWindows(linuxify(f, in), wsl.FormWindows)
	case wsl.IsWindowsPath(in):
		out, err = f.ToLinux(in)
	default:
		out, err = f.ToWindows(in, wsl.FormWindows)
	}
	if err != nil {
		return r.failErr(contract.New("invalid_path", err.Error(), "give an absolute Linux path, or a Windows path on a mounted drive"))
	}
	if r.asJSON {
		writeJSON(r.c.Stdout, map[string]string{"input": in, "output": out})
		return nil
	}
	_, _ = r.c.Stdout.Write([]byte(out + "\n"))
	return nil
}

// linuxify turns a Windows-looking input into a Linux path first, so that
// --windows/--mixed can be asked for any input (a round trip normalises it).
func linuxify(f wsl.Facts, in string) string {
	if wsl.IsWindowsPath(in) {
		if lp, err := f.ToLinux(in); err == nil {
			return lp
		}
	}
	return in
}

// ---- config ----

func (r *run) doConfig(args []string) error {
	f := r.detect()
	type section struct {
		File     string          `json:"file"`
		Path     string          `json:"path"`
		Found    bool            `json:"found"`
		Settings []wsl.Effective `json:"settings"`
		Unknown  []wsl.KV        `json:"unknown,omitempty"`
		Bad      []wsl.BadLine   `json:"bad,omitempty"`
	}
	var want []struct{ file, path string }
	if len(args) == 1 {
		// A named file: guess which schema by its base name.
		file := "wsl.conf"
		if strings.Contains(strings.ToLower(args[0]), "wslconfig") {
			file = ".wslconfig"
		}
		want = append(want, struct{ file, path string }{file, args[0]})
	} else {
		want = append(want, struct{ file, path string }{"wsl.conf", "/etc/wsl.conf"})
		if p := r.wslconfigPath(f); p != "" {
			want = append(want, struct{ file, path string }{".wslconfig", p})
		}
	}

	var out []section
	for _, w := range want {
		sec := section{File: w.file, Path: w.path}
		if b, err := r.opt.Sysroot.ReadFile(w.path); err == nil {
			sec.Found = true
			ini := wsl.ParseINI(wsl.DecodeText(b))
			sec.Settings, sec.Unknown = wsl.Resolve(w.file, ini)
			sec.Bad = ini.Bad
		} else {
			sec.Settings, _ = wsl.Resolve(w.file, wsl.INI{})
		}
		out = append(out, sec)
	}

	if r.asJSON {
		writeJSON(r.c.Stdout, map[string]any{"sections": out})
		return nil
	}
	s := r.style()
	for _, sec := range out {
		title := sec.File + "  (" + sec.Path + ")"
		if !sec.Found {
			title += "  — not present, defaults shown"
		}
		s.Section(r.c.Stdout, title)
		rows := []string{"section.key\tvalue\tsource\t"}
		for _, e := range sec.Settings {
			rows = append(rows, e.Section+"."+e.Key+"\t"+orDash(e.Value)+"\t"+e.Source)
		}
		s.Table(r.c.Stdout, rows, "--gap", "2")
		for _, u := range sec.Unknown {
			r.say().Warning("unknown key: " + u.Key + " = " + u.Value)
		}
		for _, b := range sec.Bad {
			r.say().Warning("line " + strconv.Itoa(b.Line) + ": not a setting: " + b.Text)
		}
	}
	return nil
}

// wslconfigPath finds the Windows .wslconfig from USERPROFILE, translated
// to its Linux path; "" when it cannot be located from here.
func (r *run) wslconfigPath(f wsl.Facts) string {
	up := r.opt.Getenv("USERPROFILE")
	if up == "" {
		return ""
	}
	lp, err := f.ToLinux(up)
	if err != nil {
		return ""
	}
	return lp + "/.wslconfig"
}

// ---- interop ----

func (r *run) doInterop() error {
	if r.notWSL() {
		r.status = 1
		return nil
	}
	f := r.detect()
	live := f.Interop.Socket
	if r.doExport {
		if live != "" {
			_, _ = r.c.Stdout.Write([]byte("export WSL_INTEROP=" + live + "\n"))
		}
		return nil
	}
	if r.asJSON {
		writeJSON(r.c.Stdout, f.Interop)
		return nil
	}
	s := r.style()
	s.KV(r.c.Stdout,
		"Registered", onOff(f.Interop.Registered),
		"Enabled", onOff(f.Interop.Enabled),
		"WSL_INTEROP", orDash(f.Interop.Env),
		"Live socket", orDash(live),
		"Usable", yesNo(f.Interop.Usable()))
	switch {
	case f.Interop.Stale && live != "":
		r.say().Warning("WSL_INTEROP is stale; run:  eval \"$(gash wsl interop --export)\"")
	case f.Interop.Stale:
		r.say().Error("interop is broken and no live socket was found; open a new terminal")
	}
	return nil
}

func (r *run) failErr(e contract.Envelope) error {
	r.fail(e)
	return nil
}

// ---- small helpers ----

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func statusBool(b bool) string {
	if b {
		return "ok"
	}
	return "info"
}

func markOf(status string) string {
	switch status {
	case "ok":
		return "ok"
	case "warn":
		return "warning"
	case "fail":
		return "error"
	}
	return "info"
}

func interopWord(f wsl.Facts) string {
	if f.Interop.Usable() {
		return "usable"
	}
	if f.Interop.Stale {
		return "stale"
	}
	return "unavailable"
}

func drivesLine(f wsl.Facts) string {
	if len(f.Drives) == 0 {
		return "-"
	}
	var parts []string
	for _, d := range f.Drives {
		s := d.Letter + ": " + d.Mount
		var notes []string
		if d.Metadata {
			notes = append(notes, "metadata")
		}
		// The other mount points of the drive (3.4.2), with the directory
		// of a mount that shows one.
		var also []string
		for _, m := range d.Mounts {
			if m.Path == d.Mount {
				continue
			}
			if m.Dir != `\` {
				also = append(also, m.Path+" = "+d.Letter+":"+m.Dir)
			} else {
				also = append(also, m.Path)
			}
		}
		if len(also) > 0 {
			notes = append(notes, "also "+strings.Join(also, ", "))
		}
		if len(notes) > 0 {
			s += " (" + strings.Join(notes, "; ") + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}
