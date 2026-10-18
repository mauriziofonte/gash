// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// humanKiB is __sysinfo_human_kib: KiB to one decimal under 10 of the
// unit, none from 10 (16479424 -> 16 GiB, 10239 -> 10.0 MiB).
func humanKiB(k int64) string {
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	v, i := float64(k), 0
	for v >= 1024 && i < 4 {
		v /= 1024
		i++
	}
	if v < 10 {
		return fmt.Sprintf("%.1f %s", v, units[i])
	}
	return fmt.Sprintf("%.0f %s", v, units[i])
}

// usageRow is __sysinfo_usage_row LABEL USED TOTAL: the tab-separated
// row with the bar value, the sizes, the percentage and its status style.
func usageRow(label string, used, total int64) string {
	pct := int64(0)
	if total > 0 {
		pct = used * 100 / total
	}
	style := "success"
	if pct >= 75 {
		style = "warning"
	}
	if pct >= 90 {
		style = "error"
	}
	return label + "\t" + strconv.FormatInt(used, 10) + "/" + strconv.FormatInt(total, 10) + "\t" +
		humanKiB(used) + " / " + humanKiB(total) + "\t" + strconv.FormatInt(pct, 10) + "%\t" + style
}

// num is Bash arithmetic on a word read from a file or a tool.
func num(s string) int64 { return int64(ui.Int(s)) }

var cpuFS = regexp.MustCompile(`: *`)

// overview is __sysinfo_collect_overview: one card (verbose) or the
// OVERVIEW block (compact).
func (s *Session) overview(o *out) {
	host := s.either([]string{"hostname"}, []string{"uname", "-n"}, true)
	os := ""
	if b, err := s.m.sys.ReadFile("/etc/os-release"); err == nil {
		// Parsed, never sourced (SYS-40).
		kv := platform.ParseOSRelease(b)
		os = kv["PRETTY_NAME"]
		if os == "" {
			os = kv["NAME"]
		}
	} else if s.has("sw_vers") {
		name, _ := s.tool("sw_vers", "-productName")
		ver, _ := s.tool("sw_vers", "-productVersion")
		os = runner.CommandSubst(name) + " " + runner.CommandSubst(ver)
	}
	kernelOut, _ := s.tool("uname", "-sr")
	archOut, _ := s.tool("uname", "-m")
	kernel, arch := runner.CommandSubst(kernelOut), runner.CommandSubst(archOut)

	up := ""
	if b, err := s.m.sys.ReadFile("/proc/uptime"); err == nil {
		secs := ""
		if w := awkFields(firstLine(b)); len(w) > 0 {
			secs = w[0]
		}
		secs, _, _ = strings.Cut(secs, ".")
		n := num(secs)
		if n >= 86400 {
			up = strconv.FormatInt(n/86400, 10) + "d " + strconv.FormatInt(n%86400/3600, 10) + "h"
		} else {
			up = strconv.FormatInt(n/3600, 10) + "h " + strconv.FormatInt(n%3600/60, 10) + "m"
		}
	}
	load := ""
	if b, err := s.m.sys.ReadFile("/proc/loadavg"); err == nil {
		w := append(awkFields(firstLine(b)), "", "", "")
		load = w[0] + " " + w[1] + " " + w[2]
	} else if s.has("sysctl") {
		out, _ := s.tool("sysctl", "-n", "vm.loadavg")
		var ls []string
		for _, l := range lines([]byte(strings.NewReplacer("{", "", "}", "").Replace(string(out)))) {
			ls = append(ls, awkField(l, 1)+" "+awkField(l, 2)+" "+awkField(l, 3))
		}
		load = subst(ls)
	}

	cpu := ""
	if b, err := s.m.sys.ReadFile("/proc/cpuinfo"); err == nil {
		for _, l := range lines(b) {
			if strings.HasPrefix(l, "model name") {
				if parts := cpuFS.Split(l, -1); len(parts) > 1 {
					cpu = parts[1]
				}
				break
			}
		}
	} else if s.has("sysctl") {
		out, _ := s.tool("sysctl", "-n", "machdep.cpu.brand_string")
		cpu = runner.CommandSubst(out)
	}
	cpu = strings.TrimRight(squeezeSpaces(cpu), "\n")
	cores := s.either([]string{"getconf", "_NPROCESSORS_ONLN"}, []string{"nproc"}, false)

	var memTotal, memAvail, swapTotal, swapFree, diskTotal, diskUsed int64
	if b, err := s.m.sys.ReadFile("/proc/meminfo"); err == nil {
		for _, l := range readLines(b) {
			w := append(awkFields(l), "", "")
			switch w[0] {
			case "MemTotal:":
				memTotal = num(w[1])
			case "MemAvailable:":
				memAvail = num(w[1])
			case "SwapTotal:":
				swapTotal = num(w[1])
			case "SwapFree:":
				swapFree = num(w[1])
			}
		}
	}
	dfOut, _ := s.tool("df", "-Pk", "/")
	{
		if ls := lines(dfOut); len(ls) >= 2 {
			// awk 'NR == 2 { print $2, $3 }', then read -r total used.
			w := append(awkFields(awkField(ls[1], 2)+" "+awkField(ls[1], 3)), "", "")
			diskTotal, diskUsed = num(w[0]), num(w[1])
		}
	}
	ipOut, _ := s.tool("hostname", "-I")
	var ips []string
	for _, l := range lines(ipOut) {
		ips = append(ips, awkField(l, 1))
	}
	ip := subst(ips)

	if o.llm {
		o.write("=== OVERVIEW ===\n")
		o.kv("Host", host)
		o.kv("Uptime", up)
		o.kv("Load", load)
		cpuv := cpu
		if cores != "" {
			cpuv += " x" + cores
		}
		o.kv("CPU", cpuv)
		if memTotal > 0 {
			o.kv("Memory KiB", "used="+strconv.FormatInt(memTotal-memAvail, 10)+" total="+strconv.FormatInt(memTotal, 10))
		}
		if swapTotal > 0 {
			o.kv("Swap KiB", "used="+strconv.FormatInt(swapTotal-swapFree, 10)+" total="+strconv.FormatInt(swapTotal, 10))
		}
		if diskTotal > 0 {
			o.kv("Disk / KiB", "used="+strconv.FormatInt(diskUsed, 10)+" total="+strconv.FormatInt(diskTotal, 10))
		}
		return
	}

	st := o.s
	mu, bo, off, dot := "", "", "", st.G.Dot
	if st.Color {
		mu, bo, off = st.P.Muted, st.P.Bold, st.P.Off
	}
	first := bo + host + off
	if os != "" {
		first += " " + dot + " " + os
	}
	second := mu + kernel + " " + arch
	if up != "" {
		second += " " + dot + " up " + up
	}
	if load != "" {
		second += " " + dot + " load " + load
	}
	second += off
	box := []string{first, second, ""}
	if cpu != "" || cores != "" {
		v := cpu
		if v == "" {
			v = "?"
		}
		if cores != "" {
			v += " " + dot + " " + cores + " cores"
		}
		box = append(box, st.TableLines([]string{"CPU\t" + v}, "--no-header", "--min", "1:7", "--color", "1:muted")...)
	}
	var rows []string
	if memTotal > 0 {
		rows = append(rows, usageRow("Memory", memTotal-memAvail, memTotal))
	}
	if swapTotal > 0 {
		rows = append(rows, usageRow("Swap", swapTotal-swapFree, swapTotal))
	}
	if diskTotal > 0 {
		rows = append(rows, usageRow("Disk /", diskUsed, diskTotal))
	}
	box = append(box, st.TableLines(rows, "--no-header", "--min", "1:7", "--color", "1:muted", "--bar", "2:10",
		"--cell-color", "2:5", "--cell-color", "4:5")...)
	if ip != "" {
		box = append(box, st.TableLines([]string{"Address\t" + ip}, "--no-header", "--min", "1:7", "--color", "1:muted")...)
	}
	o.write("\n")
	st.Box(o.w, st.P.Primary, box...)
	o.write("\n")
}

// firstLine is what `read -r A B ... <FILE` reads: the first line.
func firstLine(b []byte) string {
	s := strings.ReplaceAll(string(b), "\x00", "")
	l, _, _ := strings.Cut(s, "\n")
	return l
}
