// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/mauriziofonte/gash/internal/dev"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/llmfmt"
)

// defaultTools are the programs llm_tools reports without a NAME.
var defaultTools = []string{"git", "php", "composer", "node", "npm", "python3", "go", "rg", "jq", "docker"}

// versionArgs are the version probes of the programs that do not take
// --version.
var versionArgs = map[string][]string{
	"go": {"version"}, "ssh": {"-V"}, "java": {"-version"}, "javac": {"-version"},
}

var toolNameRE = lazyre.MustCompile(`^[A-Za-z0-9._+-]+$`)

// tools is llm_tools [NAME...] [-j N] [--timeout S] (plan V1): the path
// and version of each program, one --version probe each, run in
// parallel; php and node list every installed version (/usr/bin/phpX.Y,
// the nvm tree, the one on PATH: the gash runtime resolver); a program
// that is not installed is null. Keys keep the input order.
func (m *Module) tools(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.JobsFlag | llmfmt.TimeoutFlag)
	var names []string
	seen := map[string]bool{}
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
		args = args[1:]
		if strings.HasPrefix(a, "-") {
			return fail(c, "unknown_option", a, "Use -j N or --timeout S")
		}
		if !toolNameRE.MatchString(a) {
			return fail(c, "invalid_filter", a, "Name programs by their plain name (git, php), not by path")
		}
		if !seen[a] {
			seen[a] = true
			names = append(names, a)
		}
	}
	if len(names) == 0 {
		names = defaultTools
	}
	r := dev.NewResolver(c.Ctx, c.Env, m.run, false)
	timeout := o.TimeoutOr(dev.ProbeTimeout)
	vals := llmfmt.Map(c.Ctx, o.JobsOr(llmfmt.NetworkJobs), timeout, names, func(ctx context.Context, name string) json.RawMessage {
		if name == "php" || name == "node" {
			list := r.Installed(name)
			if len(list) == 0 {
				return json.RawMessage("null")
			}
			b, _ := json.Marshal(list)
			return b
		}
		path, ok := m.lookPath(c, name)
		if !ok {
			return json.RawMessage("null")
		}
		v := npmVersion(name, path)
		if v == "" {
			args, special := versionArgs[name]
			if !special {
				args = []string{"--version"}
			}
			v = dev.ProbeVersion(ctx, m.run, c.Env, path, args...)
		}
		var b bytes.Buffer
		b.WriteString(`{"version":`)
		if v == "" {
			b.WriteString("null")
		} else {
			b.WriteString(llmfmt.Quote(v))
		}
		b.WriteString(`,"path":` + llmfmt.Quote(path) + `}`)
		return b.Bytes()
	})
	var b bytes.Buffer
	b.WriteByte('{')
	for i, name := range names {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(llmfmt.Quote(name) + ":")
		b.Write(vals[i])
	}
	b.WriteString("}\n")
	_, _ = c.Stdout.Write(b.Bytes())
	return 0
}

// npmVersion reads the version of npm and npx from npm's package.json
// (their --version starts node and loads npm: hundreds of ms); "" for
// any other program or a layout it does not know.
func npmVersion(name, path string) string {
	if name != "npm" && name != "npx" {
		return ""
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	// .../lib/node_modules/npm/bin/npm-cli.js (or npx-cli.js)
	dir := filepath.Dir(filepath.Dir(real))
	if filepath.Base(dir) != "npm" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return ""
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return ""
	}
	return pkg.Version
}
