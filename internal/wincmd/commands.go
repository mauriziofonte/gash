// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package wincmd

import (
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/ui"
	"github.com/mauriziofonte/gash/internal/winbridge"
)

// ---- shared helpers ----

// itemsOf decodes a list script (its $result is {items:[…]}) into a slice.
func itemsOf[T any](r *run, s winbridge.Script, args map[string]string) ([]T, *winbridge.Error) {
	var w struct {
		Items []T `json:"items"`
	}
	e := r.bridge().DecodeJSON(r.c.Ctx, s, args, &w)
	return w.Items, e
}

func (r *run) emitJSON(v any) error { writeJSON(r.c.Stdout, v); return nil }

// ---- info ----

type winInfo struct {
	Caption      string `json:"caption"`
	Version      string `json:"version"`
	Build        string `json:"build"`
	UBR          int    `json:"ubr"`
	Hostname     string `json:"hostname"`
	Domain       string `json:"domain"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	User         string `json:"user"`
	Elevated     bool   `json:"elevated"`
	Boot         string `json:"boot"`
	Arch         string `json:"arch"`
}

func (r *run) doInfo() error {
	var info winInfo
	if e := r.bridge().DecodeJSON(r.c.Ctx, winbridge.ScriptInfo, nil, &info); e != nil {
		return r.winErr(e)
	}
	if r.asJSON {
		return r.emitJSON(info)
	}
	s := r.style()
	build := info.Build
	if info.UBR > 0 {
		build += "." + strconv.Itoa(info.UBR)
	}
	s.KV(r.c.Stdout,
		"Edition", orDash(info.Caption),
		"Version", orDash(info.Version)+"  (build "+orDash(build)+")",
		"Host", orDash(info.Hostname),
		"Model", strings.TrimSpace(info.Manufacturer+" "+info.Model),
		"User", orDash(info.User),
		"Elevated", yesNo(info.Elevated),
		"Arch", orDash(info.Arch),
		"Booted", boot(info.Boot))
	if !info.Elevated {
		r.say().Note("this is a normal token; data that needs an administrator will read as access denied")
	}
	return nil
}

// ---- disk ----

type winVolume struct {
	Drive string `json:"drive"`
	Label string `json:"label"`
	Size  int64  `json:"size"`
	Free  int64  `json:"free"`
	FS    string `json:"fs"`
}

func (r *run) doDisk() error {
	vols, e := itemsOf[winVolume](r, winbridge.ScriptDisk, nil)
	if e != nil {
		return r.winErr(e)
	}
	if r.asJSON {
		return r.emitJSON(map[string]any{"volumes": vols})
	}
	s := r.style()
	rows := []string{"drive\tlabel\tsize\tfree\tused\t"}
	for _, v := range vols {
		used := v.Size - v.Free
		full, empty := s.Bar(int(used/(1<<20)), int(max64(v.Size, 1)/(1<<20)), 16)
		rows = append(rows, v.Drive+"\t"+orDash(v.Label)+"\t"+ui.HumanBytes(v.Size)+"\t"+
			ui.HumanBytes(v.Free)+"\t"+full+empty)
	}
	s.Table(r.c.Stdout, rows, "--gap", "2")
	return nil
}

// ---- net ----

type winAdapter struct {
	Alias   string   `json:"alias"`
	Status  string   `json:"status"`
	IPv4    string   `json:"ipv4"`
	Gateway string   `json:"gateway"`
	DNS     []string `json:"dns"`
}

type winNet struct {
	Adapters []winAdapter `json:"adapters"`
	Proxy    struct {
		Enabled bool   `json:"enabled"`
		Server  string `json:"server"`
		Bypass  string `json:"bypass"`
	} `json:"proxy"`
}

func (r *run) doNet() error {
	var net winNet
	if e := r.bridge().DecodeJSON(r.c.Ctx, winbridge.ScriptNet, nil, &net); e != nil {
		return r.winErr(e)
	}
	if r.asJSON {
		return r.emitJSON(net)
	}
	s := r.style()
	rows := []string{"adapter\tstatus\tIPv4\tgateway\tDNS\t"}
	for _, a := range net.Adapters {
		rows = append(rows, a.Alias+"\t"+orDash(a.Status)+"\t"+orDash(a.IPv4)+"\t"+orDash(a.Gateway)+"\t"+orDash(strings.Join(a.DNS, ", ")))
	}
	s.Table(r.c.Stdout, rows, "--gap", "2")
	proxy := "off"
	if net.Proxy.Enabled {
		proxy = net.Proxy.Server
	}
	s.KV(r.c.Stdout, "Proxy", orDash(proxy))
	return nil
}

// ---- updates ----

type winHotfix struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Installed string `json:"installed"`
}

type winUpdates struct {
	Hotfixes      []winHotfix `json:"hotfixes"`
	RebootPending bool        `json:"reboot_pending"`
}

func (r *run) doUpdates() error {
	var up winUpdates
	if e := r.bridge().DecodeJSON(r.c.Ctx, winbridge.ScriptUpdates, nil, &up); e != nil {
		return r.winErr(e)
	}
	if r.asJSON {
		return r.emitJSON(up)
	}
	s := r.style()
	if up.RebootPending {
		s.Mark(r.c.Stdout, "warning", "a reboot is pending")
	} else {
		s.Mark(r.c.Stdout, "ok", "no reboot pending")
	}
	rows := []string{"hotfix\tkind\tinstalled\t"}
	for _, h := range up.Hotfixes {
		rows = append(rows, h.ID+"\t"+orDash(h.Kind)+"\t"+orDash(h.Installed))
	}
	s.Table(r.c.Stdout, rows, "--gap", "2")
	return nil
}

// ---- defender ----

type winDefender struct {
	Available        bool   `json:"available"`
	AntivirusEnabled bool   `json:"antivirus_enabled"`
	Realtime         bool   `json:"realtime"`
	Tamper           bool   `json:"tamper_protection"`
	Engine           string `json:"engine"`
	SignatureVersion string `json:"signature_version"`
	SignatureAge     int    `json:"signature_age_days"`
}

func (r *run) doDefender() error {
	var d winDefender
	if e := r.bridge().DecodeJSON(r.c.Ctx, winbridge.ScriptDefender, nil, &d); e != nil {
		return r.winErr(e)
	}
	if r.asJSON {
		return r.emitJSON(d)
	}
	s := r.style()
	if !d.Available {
		r.say().Info("Microsoft Defender is not available (another antivirus may be managing the system).")
		return nil
	}
	s.Mark(r.c.Stdout, markBool(d.Realtime), "real-time protection: "+onOff(d.Realtime))
	s.Mark(r.c.Stdout, markBool(d.AntivirusEnabled), "antivirus: "+onOff(d.AntivirusEnabled))
	s.Mark(r.c.Stdout, markBool(d.Tamper), "tamper protection: "+onOff(d.Tamper))
	s.KV(r.c.Stdout, "Engine", orDash(d.Engine), "Signatures", orDash(d.SignatureVersion)+" ("+strconv.Itoa(d.SignatureAge)+"d old)")
	return nil
}

// ---- services / procs / apps ----

type winService struct {
	Name        string `json:"Name"`
	DisplayName string `json:"DisplayName"`
	State       string `json:"State"`
	StartMode   string `json:"StartMode"`
	PID         int    `json:"pid"`
	Account     string `json:"account"`
}

func (r *run) doServices(filter string) error {
	svcs, e := itemsOf[winService](r, winbridge.ScriptServices, argFilter(filter))
	if e != nil {
		return r.winErr(e)
	}
	if r.asJSON {
		return r.emitJSON(map[string]any{"services": svcs})
	}
	s := r.style()
	rows := []string{"service\tstate\tstart\tpid\t"}
	for _, v := range svcs {
		pid := "-"
		if v.PID > 0 {
			pid = strconv.Itoa(v.PID)
		}
		rows = append(rows, v.Name+"\t"+v.State+"\t"+v.StartMode+"\t"+pid)
	}
	s.Table(r.c.Stdout, rows, "--gap", "2")
	return nil
}

type winProc struct {
	PID  int     `json:"pid"`
	Name string  `json:"name"`
	WS   int64   `json:"ws"`
	CPU  float64 `json:"cpu"`
}

func (r *run) doProcs(filter string) error {
	procs, e := itemsOf[winProc](r, winbridge.ScriptProcs, argFilter(filter))
	if e != nil {
		return r.winErr(e)
	}
	if r.asJSON {
		return r.emitJSON(map[string]any{"processes": procs})
	}
	s := r.style()
	rows := []string{"pid\tprocess\tmemory\tcpu(s)\t"}
	for _, p := range procs {
		rows = append(rows, strconv.Itoa(p.PID)+"\t"+p.Name+"\t"+ui.HumanBytes(p.WS)+"\t"+strconv.FormatFloat(p.CPU, 'f', -1, 64))
	}
	s.Table(r.c.Stdout, rows, "--gap", "2")
	return nil
}

type winApp struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Publisher string `json:"publisher"`
}

func (r *run) doApps(filter string) error {
	apps, e := itemsOf[winApp](r, winbridge.ScriptApps, argFilter(filter))
	if e != nil {
		return r.winErr(e)
	}
	if r.asJSON {
		return r.emitJSON(map[string]any{"apps": apps})
	}
	s := r.style()
	rows := []string{"program\tversion\tpublisher\t"}
	for _, a := range apps {
		rows = append(rows, a.Name+"\t"+orDash(a.Version)+"\t"+orDash(a.Publisher))
	}
	s.Table(r.c.Stdout, rows, "--gap", "2")
	return nil
}

// ---- events ----

type winEvent struct {
	Time     string `json:"time"`
	Level    string `json:"level"`
	ID       int    `json:"id"`
	Provider string `json:"provider"`
	Message  string `json:"message"`
}

func (r *run) doEvents() error {
	args := map[string]string{"log": r.log, "level": strconv.Itoa(levelCode(r.level)), "max": strconv.Itoa(r.max)}
	if r.since != "" {
		d, ok := parseDur(r.since)
		if !ok {
			return r.failErr(contract.New("invalid_duration", "not a duration: "+r.since, "use a suffix: 1h, 24h, 7d"))
		}
		args["since"] = time.Now().Add(-d).Format(time.RFC3339)
	}
	evs, e := itemsOf[winEvent](r, winbridge.ScriptEvents, args)
	if e != nil {
		return r.winErr(e)
	}
	if r.asJSON {
		return r.emitJSON(map[string]any{"events": evs})
	}
	s := r.style()
	rows := []string{"time\tlevel\tid\tprovider\tmessage\t"}
	for _, ev := range evs {
		rows = append(rows, shortTime(ev.Time)+"\t"+ev.Level+"\t"+strconv.Itoa(ev.ID)+"\t"+ev.Provider+"\t"+clip(ev.Message, 60))
	}
	s.Table(r.c.Stdout, rows, "--gap", "2")
	return nil
}

// ---- ports ----

type winPort struct {
	Local   string `json:"local"`
	Remote  string `json:"remote"`
	State   string `json:"state"`
	PID     int    `json:"pid"`
	Process string `json:"process"`
}

func (r *run) doPorts() error {
	args := map[string]string{}
	if r.port > 0 {
		args["port"] = strconv.Itoa(r.port)
	}
	ports, e := itemsOf[winPort](r, winbridge.ScriptPorts, args)
	if e != nil {
		return r.winErr(e)
	}
	if r.asJSON {
		return r.emitJSON(map[string]any{"ports": ports})
	}
	s := r.style()
	rows := []string{"local\tremote\tstate\tpid\tprocess\t"}
	for _, p := range ports {
		rows = append(rows, p.Local+"\t"+orDash(p.Remote)+"\t"+p.State+"\t"+strconv.Itoa(p.PID)+"\t"+orDash(p.Process))
	}
	s.Table(r.c.Stdout, rows, "--gap", "2")
	return nil
}

// ---- file ----

type winFile struct {
	Path       string `json:"path"`
	Length     int64  `json:"length"`
	Attributes string `json:"attributes"`
	Created    string `json:"created"`
	Modified   string `json:"modified"`
	Owner      string `json:"owner"`
	Access     []struct {
		Identity string `json:"identity"`
		Rights   string `json:"rights"`
		Type     string `json:"type"`
	} `json:"access"`
	Signature struct {
		Status string `json:"status"`
		Signer string `json:"signer"`
	} `json:"signature"`
	Version string `json:"version"`
}

func (r *run) doFile(path string) error {
	win, err := r.bridge().Facts().ToWindows(path, "windows")
	if err != nil {
		// Maybe the user already gave a Windows path.
		win = path
	}
	var f winFile
	if e := r.bridge().DecodeJSON(r.c.Ctx, winbridge.ScriptFile, map[string]string{"path": win}, &f); e != nil {
		return r.winErr(e)
	}
	if f.Path == "" {
		return r.failErr(contract.New("windows_not_found", "no such file on Windows: "+win, "give a path on a mounted drive"))
	}
	if r.asJSON {
		return r.emitJSON(f)
	}
	s := r.style()
	s.KV(r.c.Stdout,
		"Path", f.Path,
		"Size", ui.HumanBytes(f.Length),
		"Attributes", orDash(f.Attributes),
		"Owner", orDash(f.Owner),
		"Modified", boot(f.Modified),
		"Signature", orDash(f.Signature.Status)+sig(f.Signature.Signer),
		"Version", orDash(f.Version))
	if len(f.Access) > 0 {
		rows := []string{"identity\trights\ttype\t"}
		for _, a := range f.Access {
			rows = append(rows, a.Identity+"\t"+a.Rights+"\t"+a.Type)
		}
		s.Table(r.c.Stdout, rows, "--gap", "2")
	}
	return nil
}

func (r *run) failErr(e contract.Envelope) error {
	r.fail(e)
	return nil
}

// ---- small helpers ----

func argFilter(f string) map[string]string {
	if f == "" {
		return nil
	}
	return map[string]string{"filter": f}
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func markBool(b bool) string {
	if b {
		return "ok"
	}
	return "warning"
}

func sig(signer string) string {
	if signer == "" {
		return ""
	}
	return " (" + signer + ")"
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func levelCode(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "critical":
		return 1
	case "error":
		return 2
	case "warning", "warn":
		return 3
	case "information", "info":
		return 4
	case "verbose":
		return 5
	}
	return 0
}

func parseDur(s string) (time.Duration, bool) {
	s = strings.TrimSpace(s)
	if n, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.ParseFloat(n, 64)
		if err != nil || days < 0 {
			return 0, false
		}
		return time.Duration(days * 24 * float64(time.Hour)), true
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, false
	}
	return d, true
}

// boot formats an ISO-8601 timestamp for display, falling back to the raw
// string when it does not parse.
func boot(iso string) string {
	if iso == "" {
		return "-"
	}
	if t, err := time.Parse(time.RFC3339, iso); err == nil {
		return t.Local().Format("2006-01-02 15:04")
	}
	return iso
}

func shortTime(iso string) string {
	if t, err := time.Parse(time.RFC3339, iso); err == nil {
		return t.Local().Format("01-02 15:04:05")
	}
	return iso
}

func clip(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if ui.Len(s) <= n {
		return s
	}
	return ui.Sub(s, 0, n-1) + "…"
}
