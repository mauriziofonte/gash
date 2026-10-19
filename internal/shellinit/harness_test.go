// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package shellinit

import (
	"bytes"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

// harness drives Emit without the real system: a set of programs on PATH, a
// virtual filesystem for the reads and stats, and a probe responder.
type harness struct {
	env      map[string]string
	programs map[string]string // name -> resolved path
	files    map[string]string // path -> content
	dirs     map[string]bool   // directory paths
	// probe answers process starts; the default is a failing probe.
	probe func(runner.Call) (runner.Expect, bool)
	// writes records what migrateReminder stamps.
	writes map[string]string
	now    int64
	ttyOut bool
	// goos is the target OS the generated layer is built for; "linux" by
	// default so the suite does not depend on the host it runs on.
	goos string
}

func newHarness() *harness {
	return &harness{
		env:      map[string]string{"HOME": "/home/u", "PATH": "/usr/bin:/bin", "USER": "u", "LANG": "C.UTF-8"},
		programs: map[string]string{},
		files:    map[string]string{},
		dirs:     map[string]bool{"/usr/bin": true, "/bin": true, "/home/u": true},
		writes:   map[string]string{},
		now:      1_700_000_000,
		goos:     "linux",
	}
}

func (h *harness) withProgram(names ...string) *harness {
	for _, n := range names {
		h.programs[n] = "/usr/bin/" + n
	}
	return h
}

func (h *harness) withFile(path, content string) *harness {
	h.files[path] = content
	return h
}

func (h *harness) withDir(paths ...string) *harness {
	for _, p := range paths {
		h.dirs[p] = true
	}
	return h
}

// fakeInfo is the minimal os.FileInfo the stat seam returns.
type fakeInfo struct {
	name string
	size int64
	dir  bool
}

func (f fakeInfo) Name() string { return f.name }
func (f fakeInfo) Size() int64  { return f.size }
func (f fakeInfo) Mode() fs.FileMode {
	if f.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (f fakeInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (f fakeInfo) IsDir() bool        { return f.dir }
func (f fakeInfo) Sys() any           { return nil }

func base(path string) string {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[i+1:]
	}
	return path
}

// emit builds the Emitter from the harness and returns the layer. The
// probes default to failure (no GNU tools, no configured graphColors) unless
// the harness sets a responder.
func (h *harness) emit(t *testing.T) string {
	t.Helper()
	fake := runner.NewFake(t)
	fake.Respond(func(c runner.Call) (runner.Expect, bool) {
		if h.probe != nil {
			if e, ok := h.probe(c); ok {
				return e, true
			}
		}
		return runner.Expect{Status: 1}, true
	})
	e := &Emitter{
		run: fake,
		lookPath: func(name string, _ []string, _ string) (string, error) {
			if p, ok := h.programs[name]; ok {
				return p, nil
			}
			return "", os.ErrNotExist
		},
		readFile: func(path string) ([]byte, error) {
			if c, ok := h.files[path]; ok {
				return []byte(c), nil
			}
			return nil, os.ErrNotExist
		},
		stat: func(path string) (os.FileInfo, error) {
			if _, ok := h.files[path]; ok {
				return fakeInfo{name: base(path), size: int64(len(h.files[path]))}, nil
			}
			if h.dirs[path] {
				return fakeInfo{name: base(path), dir: true}, nil
			}
			return nil, os.ErrNotExist
		},
		getwd:     func() (string, error) { return "/work", nil },
		mkdirAll:  func(string, os.FileMode) error { return nil },
		writeFile: func(path string, data []byte, _ os.FileMode) error { h.writes[path] = string(data); return nil },
		now:       func() int64 { return h.now },
		goos:      h.goos,
	}
	env := invoke.EnvMap(h.env)
	c := &invoke.Call{
		Name: "init", Env: env,
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		Term: term.Fixed(env, false, h.ttyOut, h.ttyOut),
	}
	out, err := e.Emit(c)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	return out
}
