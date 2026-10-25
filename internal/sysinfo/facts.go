// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/platform"
)

// Facts is the compact machine summary of llm_sysinfo (plan R4): what an
// agent reads from uptime; free; df; nproc; uname; nvidia-smi, in about
// 200 bytes of JSON. Linux values come from /proc, the kernel and statfs
// with no program started; nvidia-smi runs only when it is installed;
// macOS asks sysctl once.
type Facts struct {
	Host    string    `json:"host"`
	OS      string    `json:"os,omitempty"`
	Kernel  string    `json:"kernel,omitempty"`
	UptimeH float64   `json:"uptime_h"`
	Load    []float64 `json:"load"`
	CPUs    int       `json:"cpus"`
	CPU     string    `json:"cpu,omitempty"`
	MemMB   MemMB     `json:"mem_mb"`
	Disks   []Disk    `json:"disks"`
	GPU     []GPU     `json:"gpu,omitempty"`
}

// MemMB is memory in MiB: total and available (MemAvailable).
type MemMB struct {
	Total int64 `json:"total"`
	Avail int64 `json:"avail"`
}

// Disk is one filesystem: its mount point, size in GiB (one decimal) and
// the used percentage as df shows it.
type Disk struct {
	Mount   string  `json:"mount"`
	SizeGB  float64 `json:"size_gb"`
	UsedPct int     `json:"used_pct"`
}

// GPU is one NVIDIA GPU: memory in MiB and utilization in percent.
type GPU struct {
	Name    string `json:"name"`
	MemMB   [2]int `json:"mem_mb"`
	UtilPct int    `json:"util_pct"`
}

// FS is the size of a filesystem from statfs.
type FS struct {
	Dev                string // a device identity, to leave out bind mounts
	Total, Free, Avail uint64 // bytes: all, free, free for users
}

// FactsEnv are the sources of Facts.
type FactsEnv struct {
	Sys      platform.Sysroot
	Hostname func() (string, error)
	// Uname is the kernel name, release and machine.
	Uname  func() (sysname, release, machine string, ok bool)
	Statfs func(path string) (FS, bool)
	NumCPU func() int
	Now    func() time.Time
	// Tool runs a program from the caller's PATH: its stdout, and false
	// when it is not installed or fails.
	Tool func(name string, args ...string) ([]byte, bool)
	// Mounts are the paths whose filesystems to report; nil picks the
	// local disks (see autoMounts).
	Mounts []string
}

// CollectFacts gathers Facts.
func CollectFacts(e FactsEnv) Facts {
	f := Facts{Load: []float64{}, Disks: []Disk{}}
	if e.Hostname != nil {
		if h, err := e.Hostname(); err == nil {
			f.Host = h
		}
	}
	sysname, release, ok := "", "", false
	if e.Uname != nil {
		sysname, release, _, ok = e.Uname()
	}
	if ok {
		f.Kernel = release
	}
	if b, err := e.Sys.ReadFile("/etc/os-release"); err == nil {
		kv := platform.ParseOSRelease(b)
		f.OS = kv["PRETTY_NAME"]
		if f.OS == "" {
			f.OS = strings.TrimSpace(kv["NAME"] + " " + kv["VERSION_ID"])
		}
	}
	if e.NumCPU != nil {
		f.CPUs = e.NumCPU()
	}
	linux := e.Sys.Exists("/proc/loadavg")
	if linux {
		linuxFacts(e, &f)
	} else if sysname == "Darwin" {
		darwinFacts(e, &f)
	}
	f.CPU = shortCPU(f.CPU)
	mounts := e.Mounts
	if mounts == nil {
		mounts = autoMounts(e, linux)
	}
	seen := map[string]bool{}
	for _, mp := range mounts {
		if e.Statfs == nil {
			break
		}
		fs, ok := e.Statfs(mp)
		if !ok || fs.Total == 0 || (fs.Dev != "" && seen[fs.Dev]) {
			continue
		}
		seen[fs.Dev] = fs.Dev != ""
		used := fs.Total - fs.Free
		pct := 0
		if used+fs.Avail > 0 {
			pct = int(math.Ceil(float64(used) * 100 / float64(used+fs.Avail)))
		}
		f.Disks = append(f.Disks, Disk{Mount: mp, SizeGB: round1(float64(fs.Total) / (1 << 30)), UsedPct: pct})
	}
	if e.Tool != nil {
		if out, ok := e.Tool("nvidia-smi", "--query-gpu=name,memory.used,memory.total,utilization.gpu", "--format=csv,noheader,nounits"); ok {
			f.GPU = parseGPUs(out)
		}
	}
	return f
}

// linuxFacts reads /proc: uptime, loadavg, the CPU model and meminfo.
func linuxFacts(e FactsEnv, f *Facts) {
	if b, err := e.Sys.ReadFile("/proc/uptime"); err == nil {
		if w := strings.Fields(string(b)); len(w) > 0 {
			if s, err := strconv.ParseFloat(w[0], 64); err == nil {
				f.UptimeH = round1(s / 3600)
			}
		}
	}
	if b, err := e.Sys.ReadFile("/proc/loadavg"); err == nil {
		w := strings.Fields(string(b))
		for i := 0; i < 3 && i < len(w); i++ {
			if v, err := strconv.ParseFloat(w[i], 64); err == nil {
				f.Load = append(f.Load, v)
			}
		}
	}
	if b, err := e.Sys.ReadFile("/proc/cpuinfo"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			k, v, ok := strings.Cut(l, ":")
			k = strings.TrimSpace(k)
			if ok && (k == "model name" || k == "Model" || k == "Hardware") {
				f.CPU = strings.TrimSpace(v)
				if k == "model name" {
					break
				}
			}
		}
	}
	if b, err := e.Sys.ReadFile("/proc/meminfo"); err == nil {
		var free, buffers, cached int64
		haveAvail := false
		for _, l := range strings.Split(string(b), "\n") {
			w := strings.Fields(l)
			if len(w) < 2 {
				continue
			}
			kb, _ := strconv.ParseInt(w[1], 10, 64)
			switch w[0] {
			case "MemTotal:":
				f.MemMB.Total = kb / 1024
			case "MemAvailable:":
				f.MemMB.Avail, haveAvail = kb/1024, true
			case "MemFree:":
				free = kb
			case "Buffers:":
				buffers = kb
			case "Cached:":
				cached = kb
			}
		}
		if !haveAvail {
			f.MemMB.Avail = (free + buffers + cached) / 1024
		}
	}
	if f.Host == "" {
		if b, err := e.Sys.ReadFile("/proc/sys/kernel/hostname"); err == nil {
			f.Host = strings.TrimSpace(string(b))
		}
	}
}

// darwinFacts asks sysctl once (macOS has no /proc).
func darwinFacts(e FactsEnv, f *Facts) {
	if e.Tool == nil {
		return
	}
	out, ok := e.Tool("sysctl", "-n", "vm.loadavg", "kern.boottime", "hw.memsize", "machdep.cpu.brand_string", "kern.osproductversion")
	if !ok {
		return
	}
	ls := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	get := func(i int) string {
		if i < len(ls) {
			return strings.TrimSpace(ls[i])
		}
		return ""
	}
	for _, w := range strings.Fields(strings.Trim(get(0), "{} ")) {
		if v, err := strconv.ParseFloat(w, 64); err == nil && len(f.Load) < 3 {
			f.Load = append(f.Load, v)
		}
	}
	// kern.boottime: "{ sec = 1729418400, usec = 0 } Sun Oct 20 ...".
	if _, rest, ok := strings.Cut(get(1), "sec = "); ok && e.Now != nil {
		sec, _, _ := strings.Cut(rest, ",")
		if boot, err := strconv.ParseInt(strings.TrimSpace(sec), 10, 64); err == nil {
			f.UptimeH = round1(float64(e.Now().Unix()-boot) / 3600)
		}
	}
	if b, err := strconv.ParseInt(get(2), 10, 64); err == nil {
		f.MemMB.Total = b >> 20
	}
	f.CPU = get(3)
	if v := get(4); v != "" && f.OS == "" {
		f.OS = "macOS " + v
	}
}

// realFS are the filesystem types of local disks.
var realFS = map[string]bool{
	"ext2": true, "ext3": true, "ext4": true, "xfs": true, "btrfs": true, "zfs": true, "f2fs": true,
	"jfs": true, "reiserfs": true, "ntfs": true, "ntfs3": true, "vfat": true, "exfat": true, "bcachefs": true,
	"apfs": true, "hfs": true,
}

// autoMounts are the filesystems reported without --mount: / and every
// other writable local disk (a /dev device with a disk filesystem), not
// /boot, /snap or the container-runtime trees, one mount per device, by
// mount point.
func autoMounts(e FactsEnv, linux bool) []string {
	if !linux {
		return []string{"/"}
	}
	b, err := e.Sys.ReadFile("/proc/self/mounts")
	if err != nil {
		if b, err = e.Sys.ReadFile("/proc/mounts"); err != nil {
			return []string{"/"}
		}
	}
	var out []string
	dev := map[string]bool{}
	for _, l := range strings.Split(string(b), "\n") {
		w := strings.Fields(l)
		if len(w) < 4 {
			continue
		}
		device, mp, typ, opts := w[0], unescapeMount(w[1]), w[2], ","+w[3]+","
		if mp == "/" {
			continue
		}
		if !strings.HasPrefix(device, "/dev/") || !realFS[typ] || strings.Contains(opts, ",ro,") || dev[device] {
			continue
		}
		skip := false
		for _, p := range []string{"/boot", "/snap", "/proc", "/sys", "/dev", "/run", "/var/lib/docker", "/var/lib/containers", "/var/snap"} {
			if mp == p || strings.HasPrefix(mp, p+"/") {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		dev[device] = true
		out = append(out, mp)
	}
	sort.Strings(out)
	return append([]string{"/"}, out...)
}

// unescapeMount undoes the octal escapes of /proc/mounts (\040 a blank).
func unescapeMount(s string) string {
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

// shortCPU drops the marks and filler words of a CPU model name:
// "Intel(R) Xeon(R) CPU E5-2690 v4 @ 2.60GHz" is "Intel Xeon E5-2690 v4
// @ 2.60GHz", "AMD Ryzen 9 7950X 16-Core Processor" is "AMD Ryzen 9 7950X
// 16-Core".
func shortCPU(s string) string {
	r := strings.NewReplacer("(R)", "", "(r)", "", "(TM)", "", "(tm)", "")
	var keep []string
	for _, w := range strings.Fields(r.Replace(s)) {
		if w == "CPU" || w == "Processor" {
			continue
		}
		keep = append(keep, w)
	}
	return strings.Join(keep, " ")
}

// parseGPUs reads nvidia-smi --query-gpu=name,memory.used,memory.total,
// utilization.gpu --format=csv,noheader,nounits.
func parseGPUs(out []byte) []GPU {
	var gs []GPU
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		p := strings.Split(l, ",")
		if len(p) < 4 {
			continue
		}
		num := func(s string) int {
			n, _ := strconv.Atoi(strings.TrimSpace(s))
			return n
		}
		gs = append(gs, GPU{Name: strings.TrimSpace(p[0]), MemMB: [2]int{num(p[1]), num(p[2])}, UtilPct: num(p[3])})
	}
	return gs
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
