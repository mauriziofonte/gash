// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package files

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"io"
	"os"
	"path"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
)

// member is one archive entry as the guard sees it.
type member struct {
	name       string
	isSymlink  bool
	isHardlink bool
	target     string
}

// memberGuard is the FILE-22 (option b) pre-check. It lists the members of a
// multi-member archive and reports the first one that is unsafe to extract:
// an absolute name, a ".." component, a symlink or hard link whose target
// escapes OUTPUT_DIR, or a member that lies below a symlink member. Formats
// with no member list (single-file compressors) and archives the guard
// cannot read (a missing list tool, a corrupt header) return not-unsafe, so
// 2.x's own handling takes over.
func memberGuard(c *invoke.Call, k kind, archive, outputDir string) (string, bool, error) {
	members, err := listMembers(c, k, archive)
	if err != nil || members == nil {
		return "", false, err
	}
	name, unsafe := unsafeMember(members)
	return name, unsafe, nil
}

// unsafeMember returns the first unsafe member's name. Resolution is lexical
// (OUTPUT_DIR may not exist yet): a name is relative to the destination
// root, an absolute link target or one that climbs out of the root escapes.
func unsafeMember(members []member) (string, bool) {
	var symDirs []string // cleaned member paths of symlink members
	for _, m := range members {
		raw := strings.TrimSuffix(toSlash(m.name), "/")
		if raw == "" || raw == "." {
			continue
		}
		if isAbsName(m.name) || hasDotDot(m.name) {
			return m.name, true
		}
		clean := path.Clean(raw)
		for _, sd := range symDirs {
			if clean == sd || strings.HasPrefix(clean, sd+"/") {
				return m.name, true
			}
		}
		if m.isSymlink && symlinkEscapes(clean, m.target) {
			return m.name, true
		}
		if m.isHardlink && (isAbsName(m.target) || hasDotDot(m.target)) {
			return m.name, true
		}
		if m.isSymlink {
			symDirs = append(symDirs, clean)
		}
	}
	return "", false
}

// symlinkEscapes reports whether a symlink member's target leaves the
// destination root. An absolute target does; a relative one that resolves to
// "" above the member's own directory does.
func symlinkEscapes(cleanName, target string) bool {
	t := toSlash(target)
	if t == "" {
		return false
	}
	if strings.HasPrefix(t, "/") {
		return true // absolute target: outside the (relative) destination
	}
	resolved := path.Clean(path.Dir(cleanName) + "/" + t)
	return resolved == ".." || strings.HasPrefix(resolved, "../")
}

func toSlash(s string) string { return strings.ReplaceAll(s, "\\", "/") }

func isAbsName(name string) bool {
	n := toSlash(name)
	return strings.HasPrefix(n, "/") || (len(n) >= 2 && n[1] == ':')
}

func hasDotDot(name string) bool {
	for _, p := range strings.Split(toSlash(name), "/") {
		if p == ".." {
			return true
		}
	}
	return false
}

// listMembers returns the archive's member list, or nil when the format has
// no members or the guard cannot read it.
func listMembers(c *invoke.Call, k kind, archive string) ([]member, error) {
	switch k {
	case kTarPlain:
		f, err := os.Open(archive)
		if err != nil {
			return nil, nil
		}
		defer f.Close()
		return readTar(f)
	case kTarGz, kTgz:
		f, err := os.Open(archive)
		if err != nil {
			return nil, nil
		}
		defer f.Close()
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, nil
		}
		defer gz.Close()
		return readTar(gz)
	case kTarBz2, kTbz2:
		f, err := os.Open(archive)
		if err != nil {
			return nil, nil
		}
		defer f.Close()
		return readTar(bzip2.NewReader(f))
	case kTarXz:
		return readTarPiped(c, "xz", []string{"-dc", "--", archive}, nil)
	case kTarZst:
		return readTarPiped(c, "zstd", []string{"-dc", "--", archive}, nil)
	case kTarZ:
		args, in, err := uncompressArgs(archive)
		if err != nil {
			return nil, nil
		}
		if in == nil {
			return readTarPiped(c, "uncompress", args, nil)
		}
		defer in.Close()
		return readTarPiped(c, "uncompress", args, in)
	case kZip:
		return readZip(archive)
	case kSevenZip:
		return readSevenZip(c, archive)
	case kRar:
		return readRar(c, archive)
	default:
		return nil, nil // single-file or unsupported: no member list
	}
}

// readTar reads every tar header from r (content skipped).
func readTar(r io.Reader) ([]member, error) {
	tr := tar.NewReader(r)
	var out []member
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, nil // a header the guard cannot parse: leave it to tar
		}
		out = append(out, member{
			name:       h.Name,
			isSymlink:  h.Typeflag == tar.TypeSymlink,
			isHardlink: h.Typeflag == tar.TypeLink,
			target:     h.Linkname,
		})
	}
}

// readTarPiped decompresses with `TOOL ARGS...` (the archive is the last
// argument) through the runner and reads the tar stream it produces. A
// missing tool or a read error returns nil (no guard), so the extraction
// path reports it.
func readTarPiped(c *invoke.Call, tool string, args []string, stdin io.Reader) ([]member, error) {
	// Stand down when the decompressor or tar is missing: the extraction
	// path then reports it, and a missing tar never starts zstd (FILE-19).
	for _, need := range []string{tool, "tar"} {
		if _, err := runner.LookPath(need, c.Env.Child(), ""); err != nil {
			return nil, nil
		}
	}
	rnr := runnerFor(c)
	pr, pw := io.Pipe()
	go func() {
		res, err := rnr.Run(callCtx(c), runner.Cmd{
			Path: tool, Args: args,
			Options: runner.Options{Env: c.Env.Child(), Stdin: stdin, Stdout: pw},
		})
		if err == nil && res.ExitCode != 0 {
			err = io.ErrUnexpectedEOF
		}
		_ = pw.CloseWithError(err)
	}()
	members, _ := readTar(pr)
	_, _ = io.Copy(io.Discard, pr) // drain so the decompressor can finish
	_ = pr.Close()
	return members, nil
}

// readZip lists a zip's entries, reading a symlink member's stored target.
func readZip(archive string) ([]member, error) {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return nil, nil
	}
	defer zr.Close()
	var out []member
	for _, f := range zr.File {
		m := member{name: f.Name}
		if f.Mode()&os.ModeSymlink != 0 {
			m.isSymlink = true
			if rc, e := f.Open(); e == nil {
				b, _ := io.ReadAll(io.LimitReader(rc, 4096))
				rc.Close()
				m.target = string(b)
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// readSevenZip lists a 7z archive with `7z l -slt -ba -- F`. A symlink
// member's target is its stored content (7-Zip's -snl), which the technical
// listing does not show, so it is read with `7z e -so -spd -- F NAME`
// (-spd: NAME is literal, never a wildcard).
func readSevenZip(c *invoke.Call, archive string) ([]member, error) {
	out, ok := runList(c, "7z", []string{"l", "-slt", "-ba", "--", archive})
	if !ok {
		return nil, nil
	}
	var members []member
	var cur member
	have := false
	flush := func() {
		if have {
			members = append(members, cur)
		}
		cur, have = member{}, false
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		key, val, found := strings.Cut(line, " = ")
		if !found {
			continue
		}
		switch key {
		case "Path":
			cur.name, have = val, true
		case "Attributes":
			// "A lrwxrwxrwx": the unix-mode field begins with 'l' for a link.
			if fields := strings.Fields(val); len(fields) >= 2 && strings.HasPrefix(fields[1], "l") {
				cur.isSymlink = true
			}
		}
	}
	flush()
	for i := range members {
		if members[i].isSymlink {
			if t, ok := runList(c, "7z", []string{"e", "-so", "-spd", "--", archive, members[i].name}); ok && len(t) <= 4096 {
				members[i].target = t
			}
		}
	}
	return members, nil
}

// readRar lists a rar archive with `unrar lt -- F`, reading the Name and a
// symlink's target when the listing carries one.
func readRar(c *invoke.Call, archive string) ([]member, error) {
	out, ok := runList(c, "unrar", []string{"lt", "--", archive})
	if !ok {
		return nil, nil
	}
	var members []member
	var cur member
	have := false
	flush := func() {
		if have {
			members = append(members, cur)
		}
		cur, have = member{}, false
	}
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		key, val, found := strings.Cut(line, ": ")
		if !found {
			continue
		}
		switch key {
		case "Name":
			flush()
			cur.name, have = val, true
		case "Type":
			// "Unix symbolic link", "Windows symbolic link", "Hard link",
			// "Junction"...: every link type is checked.
			switch t := strings.ToLower(val); {
			case strings.Contains(t, "hard link"):
				cur.isHardlink = true
			case strings.Contains(t, "link"), strings.Contains(t, "junction"):
				cur.isSymlink = true
			}
		case "Target", "Link target":
			cur.target = val
		}
	}
	flush()
	return members, nil
}

// runList runs a listing tool and returns its stdout. ok is false when the
// tool is missing or exits non-zero (the guard then stands down).
func runList(c *invoke.Call, tool string, args []string) (string, bool) {
	rnr := runnerFor(c)
	var out bytes.Buffer
	res, err := rnr.Run(callCtx(c), runner.Cmd{
		Path: tool, Args: args,
		Options: runner.Options{Env: c.Env.Child(), Stdout: &out, Stderr: io.Discard},
	})
	if err != nil || res.ExitCode != 0 {
		return "", false
	}
	return out.String(), true
}
