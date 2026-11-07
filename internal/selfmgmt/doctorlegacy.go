// Gash 4.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/ui"
)

// StartupFiles are the files gash doctor --legacy reads without FILE, and
// the files the interactive reminder checks: the ones Bash and Gash source
// at startup, under HOME.
var StartupFiles = []string{".bashrc", ".bash_profile", ".profile", ".bash_aliases", ".bash_local", ".gashrc"}

// legacyFile is what one file of gash doctor --legacy holds.
type legacyFile struct {
	path, shown string
	hits        []registry.LegacyHit
}

// doctorLegacy is gash doctor --legacy [--json] [FILE...]: the former
// flat names of Gash 3.x (files_largest) that each file calls, with the
// command that replaces them. Without FILE it reads the startup files of
// HOME that exist. Status 1 when a name is found or a named file cannot be
// read, 0 otherwise.
func (m *Module) doctorLegacy(c *invoke.Call, s ui.Style, f doctorFlags) int {
	home := c.Env.Get("HOME")
	paths, named := f.files, len(f.files) > 0
	if !named {
		for _, n := range StartupFiles {
			paths = append(paths, home+"/"+n)
		}
	}
	p := s.Printer(c.Stderr)
	var files []legacyFile
	status := 0
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			if !named && errors.Is(err, fs.ErrNotExist) {
				continue
			}
			p.Error("Cannot read " + config.ShownPath(path, home) + ": " + errText(err))
			status = 1
			continue
		}
		files = append(files, legacyFile{path, config.ShownPath(path, home), registry.ScanLegacy(string(data))})
	}
	found := 0
	for _, lf := range files {
		found += len(lf.hits)
	}
	if found > 0 {
		status = 1
	}
	if f.json {
		q := func(v string) string { return contract.Quote(v, contract.EscapeJQ) }
		var b strings.Builder
		b.WriteString(`{"files":[`)
		for i, lf := range files {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(`{"path":` + q(lf.path) + `,"names":[`)
			for j, h := range lf.hits {
				if j > 0 {
					b.WriteByte(',')
				}
				b.WriteString(`{"line":` + strconv.Itoa(h.Line) + `,"name":` + q(h.Name) + `,"command":` + q(h.Command))
				if h.Shortcut != "" {
					b.WriteString(`,"shortcut":` + q(h.Shortcut))
				}
				b.WriteByte('}')
			}
			b.WriteString(`]}`)
		}
		b.WriteString("],\"found\":" + strconv.Itoa(found) + "}\n")
		_, _ = c.Stdout.Write([]byte(b.String()))
		return status
	}
	var rows []string
	for _, lf := range files {
		for _, h := range lf.hits {
			now := h.Command
			if h.Shortcut != "" {
				now += " (shortcut " + h.Shortcut + ")"
			}
			rows = append(rows, lf.shown+":"+strconv.Itoa(h.Line)+"\t"+h.Name+"\t"+now)
		}
	}
	if len(rows) > 0 {
		s.Table(c.Stdout, append([]string{"WHERE\tFORMER NAME\tUSE INSTEAD"}, rows...), "--color", "2:accent", "--color", "3:accent")
	}
	what := strconv.Itoa(len(files)) + " file"
	if len(files) != 1 {
		what += "s"
	}
	switch {
	case len(files) == 0 && !named:
		p.Info("No startup file to check under " + config.ShownPath(home, home) + ".")
	case len(files) == 0:
		// Every named file failed: its error said so.
	case found == 0:
		p.Success("No former name of Gash 3.x in " + what + ".")
	default:
		p.Warning(strconv.Itoa(found) + " former name" + plural(found) + " of Gash 3.x in " + what + ": Gash 4 does not define them.")
		p.Hint("Replace each with its command, or set GASH_LEGACY_NAMES=1 in ~/.gash_env to keep them for now.")
	}
	return status
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// errText is the reason of a file error without the path os puts first.
func errText(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}
