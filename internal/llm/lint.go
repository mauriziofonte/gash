// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"go.yaml.in/yaml/v3"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/dev"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/runner"
)

// The llm_lint languages: each --ext word names one, and the file
// extensions it checks.
var lintExts = map[string][]string{
	"php":  {"php"},
	"sh":   {"sh", "bash"},
	"js":   {"js", "mjs", "cjs"},
	"json": {"json"},
	"yaml": {"yaml", "yml"},
	"toml": {"toml"},
	"py":   {"py"},
}

// lintOrder is the --ext default, in help order.
var lintOrder = []string{"php", "sh", "js", "json", "yaml", "toml", "py"}

// lintLang is the language of an extension (bash is sh, yml is yaml).
func lintLang(ext string) string {
	for lang, exts := range lintExts {
		for _, e := range exts {
			if e == ext {
				return lang
			}
		}
	}
	return ""
}

// lintDefaultCap is the number of failures printed without a budget flag.
const lintDefaultCap = 100

// lintTimeout bounds one linter process without --timeout.
const lintTimeout = 60 * time.Second

// lintFile is one file to check: its path as printed, the path the linter
// opens, its language.
type lintFile struct {
	show, abs, lang string
}

// lintResult is the outcome of one file: ok, or the first error.
type lintResult struct {
	done  bool
	ok    bool
	line  int
	error string
}

// lintFailure is one row of "failures".
type lintFailure struct {
	File  string `json:"file"`
	Line  int    `json:"line"`
	Error string `json:"error"`
}

// lint is llm_lint [PATH...] [--ext LIST] [--php VER] [--node MAJOR]
// [-j N] [--timeout S] (plan L1): every file of the languages asked,
// checked by its own tool, {"checked","failed","failures"}. php 8.3+ is
// batched (php -l a b c per chunk); an older php -l checks only its first
// file, so there each file is its own job. Shell scripts pooled (bash -n), js
// batched through one node per chunk that compiles each file as node
// would load it (failures re-checked with node --check for its exact
// message), python batched (compile(), no .pyc written), JSON, YAML and
// TOML parsed in process. Directories are walked like llm_find (no
// node_modules, vendor, .git, __pycache__, no symlinks followed);
// secret files are skipped there and refused as arguments.
func (m *Module) lint(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.JobsFlag | llmfmt.TimeoutFlag | llmfmt.BudgetFlags | llmfmt.PathFlags)
	var paths []string
	langs := map[string]bool{}
	phpReq, nodeReq := "", ""
	args := c.Args
	for len(args) > 0 {
		n, ok := o.Take(c, args)
		if !ok {
			return 1
		}
		if n > 0 {
			args = args[n:]
			continue
		}
		a := args[0]
		switch a {
		case "--ext", "--php", "--node":
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a value")
			}
			v := args[1]
			args = args[2:]
			switch a {
			case "--ext":
				for _, w := range strings.Split(v, ",") {
					w = strings.TrimPrefix(strings.TrimSpace(w), ".")
					lang := lintLang(w)
					if lang == "" {
						return fail(c, "invalid_ext", w, "Use a comma list of php, sh, js, json, yaml, toml, py")
					}
					langs[lang] = true
				}
			case "--php":
				_, req, ok := RuntimeRequest(c, "php", v)
				if !ok {
					return 1
				}
				phpReq = req
			default:
				_, req, ok := RuntimeRequest(c, "node", v)
				if !ok {
					return 1
				}
				nodeReq = req
			}
			continue
		}
		if strings.HasPrefix(a, "-") {
			return fail(c, "unknown_option", a, registry.OptionsHint(c.Name))
		}
		paths = append(paths, a)
		args = args[1:]
	}
	if len(langs) == 0 {
		for _, l := range lintOrder {
			langs[l] = true
		}
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}
	files, ok := m.lintFiles(c, o, paths, langs)
	if !ok {
		return 1
	}

	// The tools, resolved before anything runs.
	need := map[string]int{}
	for _, f := range files {
		need[f.lang]++
	}
	tools := map[string]string{}
	phpBatch := false
	r := dev.NewResolver(c.Ctx, c.Env, m.run, false)
	for _, lang := range lintOrder {
		if need[lang] == 0 {
			continue
		}
		var tool string
		switch lang {
		case "php", "js":
			fam, req := "php", phpReq
			if lang == "js" {
				fam, req = "node", nodeReq
			}
			p, ver, ok := m.lintRuntime(c, r, fam, req, need[lang], lang)
			if !ok {
				return 1
			}
			tool = p
			if lang == "php" {
				phpBatch = phpLintsMany(ver)
			}
		case "sh":
			p, found := m.lookPath(c, "bash")
			if !found {
				return c.Fail(contract.New("runtime_not_installed",
					"bash not found for "+strconv.Itoa(need[lang])+" .sh files", "Install bash or leave .sh out with --ext; ask the user"))
			}
			tool = p
		case "py":
			// python3 as gash_doctor finds it: the macOS /usr/bin/python3
			// stub without the Command Line Tools counts as missing, and
			// the report names the install command (3.4; before, the stub
			// ran and every file failed with its message).
			p, found := m.locate(c, "python3")
			if !found {
				r := prereq.Current(c).Report("python3", "llm_lint on "+strconv.Itoa(need[lang])+" .py files")
				e := r.Envelope()
				e.Hint += "; or leave .py out with --ext"
				return c.Fail(e)
			}
			tool = p
		}
		tools[lang] = tool
	}

	results := m.runLint(c, o, files, tools, phpBatch)
	var failures []lintFailure
	for i, f := range files {
		if res := results[i]; !res.ok {
			failures = append(failures, lintFailure{File: f.show, Line: res.line, Error: res.error})
		}
	}
	if !o.Capped() {
		o.Head = lintDefaultCap
	}
	head, tail, omitted := llmfmt.Cap(o, failures)
	var b bytes.Buffer
	b.WriteString(`{"checked":` + strconv.Itoa(len(files)) + `,"failed":` + strconv.Itoa(len(failures)) + `,"failures":[`)
	first := true
	for _, part := range [][]lintFailure{head, tail} {
		for _, f := range part {
			if !first {
				b.WriteByte(',')
			}
			first = false
			b.WriteString(`{"file":` + llmfmt.Quote(llmfmt.Cut(f.File, o.Width)) + `,"line":` + strconv.Itoa(f.Line) +
				`,"error":` + llmfmt.Quote(llmfmt.Cut(f.Error, o.Width)) + `}`)
		}
	}
	b.WriteByte(']')
	if omitted > 0 {
		b.WriteString(`,"truncated":` + strconv.Itoa(omitted))
		c.NoteTruncated(omitted)
	}
	b.WriteString("}\n")
	_, _ = c.Stdout.Write(b.Bytes())
	return 0
}

// lintRuntime is the php or node that checks the files, and its version:
// the requested version (gash runtime's resolution), else the one on
// PATH, else the newest installed; none is runtime_not_installed (STOP).
func (m *Module) lintRuntime(c *invoke.Call, r *dev.Resolver, fam, req string, n int, lang string) (string, string, bool) {
	if req != "" {
		res, err := r.Resolve(fam, req)
		if err != nil {
			RuntimeFail(c, err)
			return "", "", false
		}
		return res.Interp, res.Version, true
	}
	if p, ok := m.lookPath(c, fam); ok {
		return p, r.Version(p), true
	}
	if list := r.Installed(fam); len(list) > 0 {
		return list[len(list)-1].Path, list[len(list)-1].Version, true
	}
	flag := "--php X.Y"
	if fam == "node" {
		flag = "--node MAJOR"
	}
	c.Fail(contract.New("runtime_not_installed", "no "+fam+" found for "+strconv.Itoa(n)+" ."+lang+" files",
		"Install "+fam+" (gash runtime "+fam+" VERSION resolves an installed one), pass "+flag+", or leave ."+lang+" out with --ext; ask the user"))
	return "", "", false
}

// phpLintsMany reports a php whose php -l gives a verdict for each of its
// files: 8.3 and later. Before, it checks the first file only and prints
// nothing about the others; an unknown version is treated as old.
func phpLintsMany(version string) bool {
	major, rest, _ := strings.Cut(version, ".")
	minor, _, _ := strings.Cut(rest, ".")
	ma, err1 := strconv.Atoi(major)
	mi, err2 := strconv.Atoi(minor)
	if err1 != nil || err2 != nil {
		return false
	}
	return ma > 8 || ma == 8 && mi >= 3
}

// lintFiles expands the PATH arguments into the files to check, in
// argument order, each directory in walk order.
func (m *Module) lintFiles(c *invoke.Call, o *llmfmt.Options, paths []string, langs map[string]bool) ([]lintFile, bool) {
	var files []lintFile
	seen := map[string]bool{}
	g := m.guard(c)
	for _, p := range paths {
		safe, ok := m.validatePath(c, p)
		if !ok {
			return nil, false
		}
		st, err := os.Stat(safe)
		if err != nil {
			fail(c, "path_not_found", safe, "Provide an existing file or directory")
			return nil, false
		}
		if !st.IsDir() {
			if g.IsSecretFile(safe) {
				fail(c, "secret_file_blocked", safe, "This file contains secrets and cannot be read")
				return nil, false
			}
			lang := lintLang(strings.TrimPrefix(filepath.Ext(safe), "."))
			if lang == "" {
				fail(c, "unsupported_file", p, "llm_lint checks .php .sh .bash .js .mjs .cjs .json .yaml .yml .toml .py files")
				return nil, false
			}
			show := filepath.Clean(p)
			if o.Absolute {
				show = safe
			}
			if !seen[safe] {
				seen[safe] = true
				files = append(files, lintFile{show: show, abs: safe, lang: lang})
			}
			continue
		}
		forbidden, ok := m.walkRoot(c, safe, "content")
		if !ok {
			return nil, false
		}
		w := findWalker{maxDepth: -1, withStart: false, forbidden: forbidden}
		w.visit = func(e findEntry) bool {
			name := e.path[strings.LastIndexByte(e.path, '/')+1:]
			if e.mode.IsDir() {
				return !findPruned[name]
			}
			if e.typeLetter() != 'f' || strings.Contains(e.rel, "\n") {
				return true
			}
			lang := lintLang(strings.TrimPrefix(filepath.Ext(name), "."))
			if lang == "" || !langs[lang] || g.IsSecretFile(e.path) || seen[e.path] {
				return true
			}
			seen[e.path] = true
			show := llmfmt.Rel(safe, e.path)
			if o.Absolute {
				show = e.path
			}
			files = append(files, lintFile{show: show, abs: e.path, lang: lang})
			return true
		}
		w.walk(safe)
	}
	return files, true
}

// lintJob is one unit of work: a linter run over some files.
type lintJob struct {
	lang  string
	files []int
}

// runLint runs the checks: the batched linters in chunks (php only when
// phpBatch), the others one job per file, at most -j jobs at a time; then
// the js files the batch flagged, through node --check.
func (m *Module) runLint(c *invoke.Call, o *llmfmt.Options, files []lintFile, tools map[string]string, phpBatch bool) []lintResult {
	jobs := o.JobsOr(llmfmt.LocalJobs())
	byLang := map[string][]int{}
	for i, f := range files {
		byLang[f.lang] = append(byLang[f.lang], i)
	}
	var work []lintJob
	// Batched first: they start the slow interpreters.
	for _, lang := range []string{"php", "js", "py"} {
		idx := byLang[lang]
		if len(idx) == 0 {
			continue
		}
		size := max(20, (len(idx)+jobs-1)/jobs)
		if lang != "php" {
			size = max(50, (len(idx)+jobs-1)/jobs)
		}
		size = min(size, 200)
		if lang == "php" && !phpBatch {
			size = 1
		}
		for i := 0; i < len(idx); i += size {
			work = append(work, lintJob{lang: lang, files: idx[i:min(i+size, len(idx))]})
		}
	}
	for _, lang := range []string{"sh", "json", "yaml", "toml"} {
		for _, i := range byLang[lang] {
			work = append(work, lintJob{lang: lang, files: []int{i}})
		}
	}
	results := make([]lintResult, len(files))
	timeout := o.TimeoutOr(lintTimeout)
	parts := llmfmt.Map(c.Ctx, jobs, 0, work, func(ctx context.Context, j lintJob) []lintResult {
		return m.lintJob(ctx, c, j, files, tools, timeout)
	})
	var suspects []int
	for k, j := range work {
		for n, i := range j.files {
			results[i] = parts[k][n]
			if j.lang == "js" && !results[i].ok {
				suspects = append(suspects, i)
			}
		}
	}
	// node --check on the files the batch did not pass: its own verdict
	// and message.
	checked := llmfmt.Map(c.Ctx, jobs, 0, suspects, func(ctx context.Context, i int) lintResult {
		return m.nodeCheck(ctx, c, tools["js"], files[i].abs, timeout)
	})
	for n, i := range suspects {
		results[i] = checked[n]
	}
	return results
}

// lintJob runs one job and returns one result per file of the job.
func (m *Module) lintJob(ctx context.Context, c *invoke.Call, j lintJob, files []lintFile, tools map[string]string, timeout time.Duration) []lintResult {
	out := make([]lintResult, len(j.files))
	switch j.lang {
	case "json", "yaml", "toml":
		for n, i := range j.files {
			out[n] = lintData(j.lang, files[i].abs)
		}
	case "sh":
		for n, i := range j.files {
			out[n] = m.bashCheck(ctx, c, tools["sh"], files[i].abs, timeout)
		}
	case "php":
		return m.phpCheck(ctx, c, tools["php"], j.files, files, timeout)
	case "py":
		return m.batchCheck(ctx, c, tools["py"], []string{"-I", "-c", pyLintScript}, j.files, files, timeout)
	case "js":
		return m.batchCheck(ctx, c, tools["js"], []string{"--no-warnings", "--experimental-vm-modules", "-e", jsLintScript, "--"}, j.files, files, timeout)
	}
	return out
}

// lintRun runs a linter with the caller's environment and returns its
// result; a timeout is the error of every file of the run.
func (m *Module) lintRun(ctx context.Context, c *invoke.Call, path string, args []string, timeout time.Duration) (runner.Result, string) {
	res, err := m.run.Run(ctx, runner.Cmd{Path: path, Args: args, Options: runner.Options{
		Env: c.Env.Child(), Timeout: timeout, KillAfter: time.Second,
	}})
	switch {
	case res.TimedOut:
		return res, "timeout after " + timeout.String()
	case err != nil && len(res.Stdout) == 0 && len(res.Stderr) == 0:
		return res, err.Error()
	}
	return res, ""
}

var reBashErr = lazyre.MustCompile(`^line (\d+): (.*)$`)

// bashCheck is bash -n FILE: the first "FILE: line N: message".
func (m *Module) bashCheck(ctx context.Context, c *invoke.Call, bash, file string, timeout time.Duration) lintResult {
	res, e := m.lintRun(ctx, c, bash, []string{"-n", file}, timeout)
	if e != "" {
		return lintResult{done: true, error: e}
	}
	if res.ExitCode == 0 {
		return lintResult{done: true, ok: true}
	}
	r := lintResult{done: true}
	for _, l := range strings.Split(string(res.Stderr), "\n") {
		rest, ok := strings.CutPrefix(l, file+": ")
		if mm := reBashErr.FindStringSubmatch(rest); ok && mm != nil {
			r.line, r.error = atoi(mm[1]), mm[2]
			return r
		}
	}
	r.error = firstLine(string(res.Stderr), "bash -n failed")
	return r
}

var rePHPErr = lazyre.MustCompile(`^(?:PHP )?([A-Za-z ]*error): {1,2}(.*) on line (\d+)$`)

// phpCheck is php -l FILE... on one chunk: "No syntax errors detected in
// F", or "PHP Parse error:  MESSAGE in F on line N" then "Errors parsing
// F" (php 8.3+ keeps going after an error; an older php checks the first
// file only, so it gets chunks of one). A file passes only on its own
// verdict line: one of a chunk without a verdict is checked again alone,
// and alone without one it fails, its error naming it.
func (m *Module) phpCheck(ctx context.Context, c *invoke.Call, php string, idx []int, files []lintFile, timeout time.Duration) []lintResult {
	out := make([]lintResult, len(idx))
	args := []string{"-d", "display_errors=stderr", "-d", "log_errors=0", "-d", "html_errors=0", "-d", "error_reporting=-1", "-l"}
	pos := map[string]int{}
	for n, i := range idx {
		args = append(args, files[i].abs)
		pos[files[i].abs] = n
	}
	res, e := m.lintRun(ctx, c, php, args, timeout)
	if e != "" {
		for n := range out {
			out[n] = lintResult{done: true, error: e}
		}
		return out
	}
	text := string(res.Stdout) + "\n" + string(res.Stderr)
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if f, ok := strings.CutPrefix(l, "No syntax errors detected in "); ok {
			if n, found := pos[f]; found {
				out[n] = lintResult{done: true, ok: true}
			}
			continue
		}
		mm := rePHPErr.FindStringSubmatch(l)
		if mm == nil {
			continue
		}
		// "MESSAGE in FILE": the file is one of the chunk's, the longest
		// that ends the text (a message may say " in " itself).
		body, best := mm[2], ""
		for f := range pos {
			if strings.HasSuffix(body, " in "+f) && len(f) > len(best) {
				best = f
			}
		}
		if best == "" {
			continue
		}
		n := pos[best]
		if !out[n].done {
			msg := strings.TrimSuffix(body, " in "+best)
			if t := strings.TrimSpace(mm[1]); t != "" && !strings.EqualFold(t, "parse error") {
				msg = t + ": " + msg
			}
			out[n] = lintResult{done: true, line: atoi(mm[3]), error: msg}
		}
	}
	// A verdict without a message the pattern reads: "Errors parsing F"
	// (stdout, before the stderr message), "Could not open input file: F".
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		for prefix, msg := range map[string]string{"Errors parsing ": "php -l found errors in ", "Could not open input file: ": "php could not open "} {
			if f, ok := strings.CutPrefix(l, prefix); ok {
				if n, found := pos[f]; found && !out[n].done {
					out[n] = lintResult{done: true, error: msg + files[idx[n]].show}
				}
			}
		}
	}
	for n := range out {
		switch {
		case out[n].done:
		case len(idx) > 1:
			out[n] = m.phpCheck(ctx, c, php, idx[n:n+1], files, timeout)[0]
		default:
			out[n] = lintResult{done: true, error: "php -l gave no verdict for " + files[idx[n]].show}
		}
	}
	return out
}

// pyLintScript compiles each file (compile(): the parser and the
// compiler, nothing executed, no .pyc written) and prints one JSON line
// per file: [index] when it compiles, [index, line, message] when not.
const pyLintScript = `import sys, json
for i, f in enumerate(sys.argv[1:]):
    try:
        with open(f, 'rb') as h:
            compile(h.read(), f, 'exec', dont_inherit=True)
        print(json.dumps([i]), flush=True)
    except SyntaxError as e:
        print(json.dumps([i, e.lineno or 0, e.msg]), flush=True)
    except Exception as e:
        print(json.dumps([i, 0, str(e)]), flush=True)
`

// jsLintScript compiles each file as node loads it, without running it:
// .mjs and a .js under "type": "module" as an ES module, .cjs and other
// .js files as a CommonJS function body, retried as a module where node
// detects ES syntax (22.7+, 20.19+). It prints [index] for a file that
// compiles and [index, 0, message] for one that does not; llm_lint then
// asks node --check about the latter.
const jsLintScript = `const fs = require('fs'), vm = require('vm'), path = require('path');
const v = process.versions.node.split('.').map(Number);
const detect = v[0] > 22 || (v[0] === 22 && v[1] >= 7) || (v[0] === 20 && v[1] >= 19);
const types = new Map();
function pkgType(dir) {
  const walked = [];
  let t = 'commonjs';
  for (let d = dir; ; d = path.dirname(d)) {
    if (types.has(d)) { t = types.get(d); break; }
    walked.push(d);
    let raw = null;
    try { raw = fs.readFileSync(path.join(d, 'package.json'), 'utf8'); } catch (e) {}
    if (raw !== null) { try { t = JSON.parse(raw).type === 'module' ? 'module' : 'commonjs'; } catch (e) { t = 'unknown'; } break; }
    if (path.dirname(d) === d) break;
  }
  for (const d of walked) types.set(d, t);
  return t;
}
function asModule(src, f) {
  if (!vm.SourceTextModule) throw new Error('no vm modules');
  new vm.SourceTextModule(src, { identifier: f });
}
function asScript(src, f) {
  vm.compileFunction(src, ['exports', 'require', 'module', '__filename', '__dirname'], { filename: f });
}
const files = process.argv.slice(1);
files.forEach((f, i) => {
  try {
    let src = fs.readFileSync(f, 'utf8');
    if (src.charCodeAt(0) === 0xFEFF) src = src.slice(1);
    const ext = path.extname(f);
    const type = ext === '.mjs' ? 'module' : ext === '.cjs' ? 'commonjs' : pkgType(path.dirname(path.resolve(f)));
    if (type === 'unknown') throw new Error('package.json');
    if (type === 'module') { asModule(src, f); console.log(JSON.stringify([i])); return; }
    try { asScript(src, f); } catch (e) {
      if (!detect || ext === '.cjs' || !(e instanceof SyntaxError)) throw e;
      asModule(src, f);
    }
    console.log(JSON.stringify([i]));
  } catch (e) {
    console.log(JSON.stringify([i, 0, String(e && e.message || e)]));
  }
});
`

// batchCheck runs a checker that prints one JSON line per file: [index]
// when it passes, [index, line, message] when not. A file passes only on
// its own line: one of a batch without a line is checked again alone,
// and alone without one it fails (the checker stopped early, or crashed).
func (m *Module) batchCheck(ctx context.Context, c *invoke.Call, tool string, pre []string, idx []int, files []lintFile, timeout time.Duration) []lintResult {
	out := make([]lintResult, len(idx))
	args := append([]string(nil), pre...)
	for _, i := range idx {
		args = append(args, files[i].abs)
	}
	res, e := m.lintRun(ctx, c, tool, args, timeout)
	if e != "" {
		for n := range out {
			out[n] = lintResult{done: true, error: e}
		}
		return out
	}
	for _, l := range strings.Split(string(res.Stdout), "\n") {
		var rec []json.RawMessage
		if !strings.HasPrefix(l, "[") || json.Unmarshal([]byte(l), &rec) != nil || (len(rec) != 1 && len(rec) != 3) {
			continue
		}
		var n, line int
		var msg string
		if json.Unmarshal(rec[0], &n) != nil || n < 0 || n >= len(out) || out[n].done {
			continue
		}
		if len(rec) == 1 {
			out[n] = lintResult{done: true, ok: true}
			continue
		}
		if json.Unmarshal(rec[1], &line) != nil || json.Unmarshal(rec[2], &msg) != nil {
			continue
		}
		out[n] = lintResult{done: true, line: line, error: oneLine(msg)}
	}
	for n := range out {
		switch {
		case out[n].done:
		case len(idx) > 1:
			out[n] = m.batchCheck(ctx, c, tool, pre, idx[n:n+1], files, timeout)[0]
		case res.ExitCode != 0:
			out[n] = lintResult{done: true, error: firstLine(string(res.Stderr), filepath.Base(tool)+" exited with status "+strconv.Itoa(res.ExitCode))}
		default:
			out[n] = lintResult{done: true, error: filepath.Base(tool) + " gave no verdict for " + files[idx[n]].show}
		}
	}
	return out
}

var reNodeLoc = lazyre.MustCompile(`^(.+):(\d+)$`)
var reNodeErr = lazyre.MustCompile(`^([A-Za-z]*Error|Error)(?: \[[A-Z_]+\])?: (.*)$`)

// nodeCheck is node --check FILE: "FILE:LINE" on the first line of its
// report, the error on the "SyntaxError: ..." line.
func (m *Module) nodeCheck(ctx context.Context, c *invoke.Call, node, file string, timeout time.Duration) lintResult {
	res, e := m.lintRun(ctx, c, node, []string{"--check", file}, timeout)
	if e != "" {
		return lintResult{done: true, error: e}
	}
	if res.ExitCode == 0 {
		return lintResult{done: true, ok: true}
	}
	r := lintResult{done: true}
	for _, l := range strings.Split(string(res.Stderr), "\n") {
		if mm := reNodeLoc.FindStringSubmatch(l); mm != nil && r.line == 0 && mm[1] == file {
			r.line = atoi(mm[2])
			continue
		}
		if mm := reNodeErr.FindStringSubmatch(strings.TrimSpace(l)); mm != nil {
			r.error = mm[1] + ": " + mm[2]
			return r
		}
	}
	r.error = firstLine(string(res.Stderr), "node --check failed")
	return r
}

// lintData parses a JSON, YAML or TOML file in process.
func lintData(lang, file string) lintResult {
	data, err := os.ReadFile(file)
	if err != nil {
		return lintResult{done: true, error: err.Error()}
	}
	switch lang {
	case "json":
		src := data
		if jsoncFile(file) {
			src = stripJSONC(data)
		}
		var v any
		if err := json.Unmarshal(src, &v); err != nil {
			line := 0
			var se *json.SyntaxError
			if errors.As(err, &se) {
				line = 1 + bytes.Count(src[:min(int(se.Offset), len(src))], []byte("\n"))
			}
			return lintResult{done: true, line: line, error: err.Error()}
		}
	case "yaml":
		dec := yaml.NewDecoder(bytes.NewReader(data))
		for {
			var n yaml.Node
			err := dec.Decode(&n)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				msg := strings.TrimPrefix(err.Error(), "yaml: ")
				line := 0
				if mm := reYAMLLine.FindStringSubmatch(msg); mm != nil {
					line, msg = atoi(mm[1]), mm[2]
				}
				return lintResult{done: true, line: line, error: oneLine(msg)}
			}
		}
	case "toml":
		var v map[string]any
		if _, err := toml.Decode(string(data), &v); err != nil {
			var pe toml.ParseError
			if errors.As(err, &pe) {
				return lintResult{done: true, line: pe.Position.Line, error: oneLine(pe.Message)}
			}
			return lintResult{done: true, error: oneLine(err.Error())}
		}
	}
	return lintResult{done: true, ok: true}
}

var reYAMLLine = lazyre.MustCompile(`^line (\d+): (.*)$`)

// jsoncFile reports the JSON files that by convention allow comments and
// trailing commas (tsconfig.json, .vscode settings, ...).
func jsoncFile(p string) bool {
	base := filepath.Base(p)
	switch {
	case strings.HasPrefix(base, "tsconfig") || strings.HasPrefix(base, "jsconfig"),
		base == ".eslintrc.json", base == ".babelrc.json", base == "devcontainer.json",
		strings.Contains(p, "/.vscode/"), strings.Contains(p, "/.devcontainer/"):
		return true
	}
	return false
}

// stripJSONC blanks // and /* */ comments outside strings and drops a
// comma before } or ], keeping every newline (the line numbers stay).
func stripJSONC(src []byte) []byte {
	out := make([]byte, 0, len(src))
	inStr, esc := false, false
	for i := 0; i < len(src); i++ {
		ch := src[i]
		switch {
		case inStr:
			out = append(out, ch)
			if esc {
				esc = false
			} else if ch == '\\' {
				esc = true
			} else if ch == '"' {
				inStr = false
			}
		case ch == '"':
			inStr = true
			out = append(out, ch)
		case ch == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			if i < len(src) {
				out = append(out, '\n')
			}
		case ch == '/' && i+1 < len(src) && src[i+1] == '*':
			i += 2
			for i < len(src) && (src[i] != '*' || i+1 >= len(src) || src[i+1] != '/') {
				if src[i] == '\n' {
					out = append(out, '\n')
				}
				i++
			}
			i++
		default:
			out = append(out, ch)
		}
	}
	// Trailing commas: ",<space>}" and ",<space>]".
	res := make([]byte, 0, len(out))
	for i := 0; i < len(out); i++ {
		if out[i] == ',' {
			j := i + 1
			for j < len(out) && (out[j] == ' ' || out[j] == '\t' || out[j] == '\n' || out[j] == '\r') {
				j++
			}
			if j < len(out) && (out[j] == '}' || out[j] == ']') {
				res = append(res, ' ')
				continue
			}
		}
		res = append(res, out[i])
	}
	return res
}

// firstLine is the first non-blank line of s, or def.
func firstLine(s, def string) string {
	for _, l := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			return t
		}
	}
	return def
}

// oneLine joins the lines of a message with "; ".
func oneLine(s string) string {
	var parts []string
	for _, l := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "; ")
}
