// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package compatgen

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Implementation paths of 2.x: left out of a staged tree, which gets the
// generated files instead. lib/shell and lib/aliases go too: in 3.x that
// layer comes from `gash init bash`.
var implementation = map[string]bool{
	"gash.sh":         true,
	"bash_completion": true,
	"lib":             true,
	"bin":             true,
}

// Not copied into a staged tree at all.
var skipped = map[string]bool{
	".git":    true,
	"go-impl": true,
}

// StageBin is where Stage places the binary, relative to the staged tree.
const StageBin = ".gash-stage/gash"

// FindRoot returns the Gash checkout that contains dir: the nearest
// directory above it with a VERSION file and go.mod.
func FindRoot(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if isFile(filepath.Join(dir, "VERSION")) && isFile(filepath.Join(dir, "go.mod")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no Gash checkout found")
		}
		dir = parent
	}
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// Stage builds in out a copy of the checkout at root whose Bash
// implementation is replaced by the generated files (with a shim for every
// module), and places bin at out/StageBin. It returns the absolute path of
// the placed binary, the GASH_BIN of the staged tree.
func Stage(root, out, bin string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	out, err = filepath.Abs(out)
	if err != nil {
		return "", err
	}
	if rel, err := filepath.Rel(root, out); err == nil && (rel == "." || !strings.HasPrefix(rel, "..")) {
		return "", fmt.Errorf("stage: %s is inside the checkout %s", out, root)
	}
	if entries, err := os.ReadDir(out); err == nil && len(entries) > 0 {
		return "", fmt.Errorf("stage: %s is not empty", out)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", err
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == "." {
			return err
		}
		top := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
		if skipped[top] || implementation[top] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return copyEntry(p, filepath.Join(out, rel), d)
	})
	if err != nil {
		return "", fmt.Errorf("stage: %w", err)
	}
	files, err := Generate(true)
	if err != nil {
		return "", err
	}
	if err := Write(files, out); err != nil {
		return "", fmt.Errorf("stage: %w", err)
	}
	dst := filepath.Join(out, filepath.FromSlash(StageBin))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	if err := copyFile(bin, dst, 0o755); err != nil {
		return "", fmt.Errorf("stage: %w", err)
	}
	return dst, nil
}

func copyEntry(src, dst string, d fs.DirEntry) error {
	switch {
	case d.IsDir():
		return os.MkdirAll(dst, 0o755)
	case d.Type()&fs.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	case d.Type().IsRegular():
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(src, dst, info.Mode().Perm())
	}
	return nil // sockets, devices: not part of a checkout
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}
