// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package logs

import (
	"encoding/json"
	"io"
	"sort"
	"strconv"

	"github.com/mauriziofonte/gash/internal/ui"
)

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// cleanOutput is the JSON of a clean (the report's totals plus the result).
type cleanOutput struct {
	DryRun    bool  `json:"dry_run"`
	Deleted   int   `json:"deleted"`
	Truncated int   `json:"truncated"`
	Freed     int64 `json:"freed"`
	Failed    int   `json:"failed"`
	NeedRoot  int   `json:"need_root"`
	NeedRootB int64 `json:"need_root_bytes"`
	Scanned   int   `json:"scanned_files"`
}

func cleanJSON(rep *Report, res Result, dry bool) cleanOutput {
	return cleanOutput{
		DryRun: dry, Deleted: res.Deleted, Truncated: res.Truncated, Freed: res.Freed,
		Failed: res.Failed, NeedRoot: res.NeedRoot, NeedRootB: res.NeedRootB, Scanned: rep.TotalFiles,
	}
}

// renderReport is the eye-candy scan report: a heading, a per-category
// table with a share bar, and a box with the totals.
func (r *run) renderReport(rep *Report) {
	s := r.style()
	w := r.c.Stdout
	if rep.TotalFiles == 0 {
		r.say().Info("No log files found under: " + joinShort(rep.Roots))
		return
	}
	s.Section(w, "Log files")

	maxReclaim := int64(1)
	for _, g := range rep.Groups {
		if g.Reclaim > maxReclaim {
			maxReclaim = g.Reclaim
		}
	}
	rows := []string{"category\tfiles\tsize\treclaimable\t"}
	for _, g := range rep.Groups {
		full, empty := s.Bar(int(g.Reclaim/1024), int(maxReclaim/1024), 16)
		bar := full + empty
		note := ""
		if g.Category == "systemd-journal" {
			note = "vacuum"
		}
		rows = append(rows, g.Category+"\t"+strconv.Itoa(g.Files)+"\t"+
			ui.HumanBytes(g.Size)+"\t"+ui.HumanBytes(g.Reclaim)+"\t"+bar+" "+note)
	}
	s.Table(w, rows, "--gap", "2")

	lines := []string{
		"Scanned:     " + strconv.Itoa(rep.TotalFiles) + " files in " + joinShort(rep.Roots),
		"Total size:  " + ui.HumanBytes(rep.TotalSize),
		"Reclaimable: " + ui.HumanBytes(rep.TotalReclaim),
	}
	if rep.Journal > 0 {
		lines = append(lines, "Journal:     "+ui.HumanBytes(rep.Journal)+" (use journalctl --vacuum-size=…)")
	}
	if rep.Protected > 0 {
		lines = append(lines, "Protected:   "+plural(rep.Protected, "file", "files")+" in PROTECT:content paths, never cleaned")
	}
	s.Box(w, "", lines...)
	r.say().Hint("gash logs clean deletes rotated logs; add --truncate for live ones, --sudo for /var/log.")
}

// renderPlan lists the biggest files a clean would touch (before the
// confirmation).
func (r *run) renderPlan(rep *Report, ops []op) {
	s := r.style()
	w := r.c.Stderr
	sort.Slice(ops, func(i, j int) bool { return ops[i].Size > ops[j].Size })
	top := ops
	if len(top) > 12 {
		top = top[:12]
	}
	rows := []string{"action\tsize\tfile\t"}
	for _, o := range top {
		act := "delete"
		if o.Truncate {
			act = "truncate"
		}
		if o.needRoot {
			act += " (sudo)"
		}
		rows = append(rows, act+"\t"+ui.HumanBytes(o.Size)+"\t"+o.Path)
	}
	s.Table(w, rows, "--gap", "2")
	if len(ops) > len(top) {
		r.say().Info("… and " + strconv.Itoa(len(ops)-len(top)) + " more")
	}
}

// renderClean is the outcome of a clean.
func (r *run) renderClean(res Result) {
	p := r.say()
	verb := "Reclaimed"
	if r.dryRun {
		verb = "Would reclaim"
	}
	p.Success(verb + " " + ui.HumanBytes(res.Freed) + " (" +
		strconv.Itoa(res.Deleted) + " deleted, " + strconv.Itoa(res.Truncated) + " truncated)")
	if res.NeedRoot > 0 {
		p.Warning(strconv.Itoa(res.NeedRoot) + " root-owned files (" + ui.HumanBytes(res.NeedRootB) +
			") were left: re-run with --sudo to reclaim them.")
	}
	if res.Failed > 0 {
		p.Error(strconv.Itoa(res.Failed) + " files could not be removed.")
		r.status = 1
	}
}

func joinShort(roots []string) string {
	if len(roots) == 0 {
		return "."
	}
	if len(roots) == 1 {
		return roots[0]
	}
	return roots[0] + " and " + strconv.Itoa(len(roots)-1) + " more"
}
