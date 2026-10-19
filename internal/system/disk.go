// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// diskTypes are the disk-backed filesystem types disk_usage lists.
var diskTypes = regexp.MustCompile(`^(ext[2-4]|xfs|btrfs|zfs|f2fs|fat|vfat|exfat|msdos|ntfs|ntfs3|fuseblk|apfs|hfs|reiserfs|jfs)$`)

// wslDrive is a Windows drive mounted by WSL (drvfs or 9p on /mnt/X).
var wslDrive = regexp.MustCompile(`^/mnt/[a-z]$`)

// diskUsage is disk_usage: `df -PTk`, else `df -Pk` (BSD df), each one's
// output dropped when df fails (SYS-14), filtered and drawn as 2.x did.
func (m *Module) diskUsage(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	p := ui.NewPrinter(c.Stderr, c.Term, false)
	noColor := false
	for _, a := range c.Args {
		if a != "--no-color" {
			p.Error("disk_usage: unknown option '" + a + "'")
			return 1
		}
		noColor = true
	}
	typed := true
	raw := m.df(c, "-PTk")
	if raw == "" {
		typed = false
		raw = m.df(c, "-Pk")
	}
	if raw == "" {
		p.Error("disk_usage: df failed")
		return 1
	}
	rows := diskRows(raw, typed)
	if len(rows) == 0 {
		p.Info("No disk-backed filesystems found (df lists only virtual ones).")
		return 0
	}
	s := ui.StyleOf(c.Term, noColor)
	table := []string{"MOUNT\tDEVICE\tTYPE\tSIZE\tUSED\tFREE\tUSE"}
	for _, r := range rows {
		style := "success"
		if r.pct >= 75 {
			style = "warning"
		}
		if r.pct >= 90 {
			style = "error"
		}
		pct := strconv.FormatInt(r.pct, 10)
		table = append(table, strings.Join([]string{r.mnt, r.dev, r.typ, r.size, r.used, r.avail,
			pct + "/100", pct + "%", style}, "\t"))
	}
	var args []string
	if !c.Term.UseColor() {
		args = append(args, "--no-header")
	}
	args = append(args, "--indent", "2", "--right", "4,5,6,8", "--max-left", "2:28",
		"--color", "1:accent", "--color", "2:muted", "--bar", "7:10", "--cell-color", "7:9", "--cell-color", "8:9")
	write(c.Stdout, "\n")
	s.Table(c.Stdout, table, args...)
	write(c.Stdout, "\n")
	return 0
}

// df is `raw="$(df FLAGS 2>/dev/null)" || raw=""`.
func (m *Module) df(c *invoke.Call, flags string) string {
	res, err := m.run.Run(ctx(c), runner.Cmd{Path: "df", Args: []string{flags},
		Options: runner.Options{Env: c.Env.Child(), Stderr: io.Discard}})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return runner.CommandSubst(res.Stdout)
}

type diskRow struct {
	mnt, dev, typ, size, used, avail string
	pct                              int64
}

// diskRows is the 2.x awk program over df's POSIX output: the header
// skipped, typed rows kept by type (or a WSL drive), untyped ones by a
// /dev/ device, one row per mount point.
func diskRows(raw string, typed bool) []diskRow {
	var rows []diskRow
	seen := map[string]bool{}
	for i, line := range strings.Split(raw, "\n") {
		if i == 0 {
			continue
		}
		f := awkFields(line)
		typ, k := "-", 1
		if typed {
			typ, k = field(f, 1), 2
		}
		mnt := field(f, k+4)
		for j := k + 5; j < len(f); j++ {
			mnt += " " + f[j]
		}
		if typed {
			if !diskTypes.MatchString(typ) && !((typ == "drvfs" || typ == "9p") && wslDrive.MatchString(mnt)) {
				continue
			}
		} else if !strings.HasPrefix(field(f, 0), "/dev/") {
			continue
		}
		if seen[mnt] {
			continue
		}
		seen[mnt] = true
		pct := strings.TrimSuffix(field(f, k+3), "%")
		rows = append(rows, diskRow{mnt: mnt, dev: field(f, 0), typ: typ, size: kib(field(f, k)),
			used: kib(field(f, k+1)), avail: kib(field(f, k+2)), pct: int64(awkNumber(pct))})
	}
	return rows
}

// kib is the awk h(): KiB scaled to the largest unit under 1024, one
// decimal below 10.
func kib(field string) string {
	k := awkNumber(field)
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	i := 0
	for k >= 1024 && i < 4 {
		k /= 1024
		i++
	}
	if k < 10 {
		return strconv.FormatFloat(k, 'f', 1, 64) + " " + units[i]
	}
	return strconv.FormatFloat(k, 'f', 0, 64) + " " + units[i]
}

// awkNumber is awk's numeric value of a string: its leading number
// (blanks skipped), 0 without one.
func awkNumber(s string) float64 {
	s = strings.TrimLeft(s, " \t\n")
	j := 0
	if j < len(s) && (s[j] == '-' || s[j] == '+') {
		j++
	}
	digits := 0
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
		digits++
	}
	if j < len(s) && s[j] == '.' {
		j++
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
			digits++
		}
	}
	if digits == 0 {
		return 0
	}
	if j < len(s) && (s[j] == 'e' || s[j] == 'E') {
		e := j + 1
		if e < len(s) && (s[e] == '-' || s[e] == '+') {
			e++
		}
		if e < len(s) && s[e] >= '0' && s[e] <= '9' {
			for e < len(s) && s[e] >= '0' && s[e] <= '9' {
				e++
			}
			j = e
		}
	}
	v, _ := strconv.ParseFloat(s[:j], 64)
	return v
}
