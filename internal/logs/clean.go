// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package logs

import (
	"context"
	"os"

	"golang.org/x/sys/unix"

	"github.com/mauriziofonte/gash/internal/runner"
)

// op is one planned change to one file.
type op struct {
	Path     string
	Truncate bool  // true: truncate in place; false: delete
	Size     int64 // bytes it will free
	needRoot bool  // the process cannot do it; sudo can
}

// plan turns a report into the list of changes clean would make. truncate
// says whether active logs are truncated (false leaves them alone and only
// rotated copies are deleted). It never plans anything for a journal.
func plan(r *Report, truncate bool) []op {
	var ops []op
	for _, g := range r.Groups {
		for _, e := range g.Entries {
			switch e.kindCode {
			case rotated:
				ops = append(ops, op{Path: e.Path, Truncate: false, Size: e.Size, needRoot: !canDelete(e.Path)})
			case active:
				if truncate && e.Size > 0 {
					ops = append(ops, op{Path: e.Path, Truncate: true, Size: e.Size, needRoot: !canWrite(e.Path)})
				}
			}
		}
	}
	return ops
}

// Result is the outcome of a clean.
type Result struct {
	Deleted     int   `json:"deleted"`
	Truncated   int   `json:"truncated"`
	Freed       int64 `json:"freed"`
	Failed      int   `json:"failed"`
	NeedRoot    int   `json:"need_root"` // files skipped because sudo was not used
	NeedRootB   int64 `json:"need_root_bytes"`
	failedPaths []string
}

// cleaner carries the seam for the privileged path.
type cleaner struct {
	run     runner.Runner
	dryRun  bool
	useSudo bool
	// truncate is the program of the sudo truncate batch (truncate when
	// empty).
	truncate string
}

// apply runs the ops. The files the process can change it changes directly
// (os.Remove, truncate in place so a daemon's open file keeps its inode).
// Root-owned files go to one `sudo rm` and one `sudo truncate` batch when
// useSudo is set, else they are counted as needing sudo and left.
func (c cleaner) apply(ctx context.Context, ops []op) Result {
	var res Result
	var rootDel, rootTrunc []string
	var rootDelB, rootTruncB int64
	for _, o := range ops {
		if o.needRoot {
			if !c.useSudo {
				res.NeedRoot++
				res.NeedRootB += o.Size
				continue
			}
			if o.Truncate {
				rootTrunc = append(rootTrunc, o.Path)
				rootTruncB += o.Size
			} else {
				rootDel = append(rootDel, o.Path)
				rootDelB += o.Size
			}
			continue
		}
		if c.dryRun {
			c.count(&res, o)
			continue
		}
		if err := doOp(o); err != nil {
			res.Failed++
			res.failedPaths = append(res.failedPaths, o.Path)
			continue
		}
		c.count(&res, o)
	}
	if c.useSudo && (len(rootDel) > 0 || len(rootTrunc) > 0) {
		c.applyRoot(ctx, &res, rootDel, rootDelB, rootTrunc, rootTruncB)
	}
	return res
}

func (c cleaner) count(res *Result, o op) {
	if o.Truncate {
		res.Truncated++
	} else {
		res.Deleted++
	}
	res.Freed += o.Size
}

// applyRoot runs the sudo batches. The file lists are explicit arguments
// (never a shell string), validated before they got here.
func (c cleaner) applyRoot(ctx context.Context, res *Result, del []string, delB int64, trunc []string, truncB int64) {
	if len(del) > 0 {
		if c.dryRun || c.sudo(ctx, append([]string{"rm", "-f", "--"}, del...)) {
			res.Deleted += len(del)
			res.Freed += delB
		} else {
			res.Failed += len(del)
		}
	}
	if len(trunc) > 0 {
		prog := c.truncate
		if prog == "" {
			prog = "truncate"
		}
		if c.dryRun || c.sudo(ctx, append([]string{prog, "-s", "0", "--"}, trunc...)) {
			res.Truncated += len(trunc)
			res.Freed += truncB
		} else {
			res.Failed += len(trunc)
		}
	}
}

// sudo runs `sudo ARGS...` and reports success.
func (c cleaner) sudo(ctx context.Context, args []string) bool {
	res, err := c.run.Run(ctx, runner.Cmd{Path: "sudo", Args: args,
		Options: runner.Options{Env: os.Environ(), Stdout: os.Stderr, Stderr: os.Stderr}})
	return err == nil && res.ExitCode == 0
}

// doOp performs one unprivileged change. A truncate keeps the inode so a
// program that holds the file open keeps writing to it.
func doOp(o op) error {
	if o.Truncate {
		return os.Truncate(o.Path, 0)
	}
	return os.Remove(o.Path)
}

// canWrite reports write access to a file (for a truncate).
func canWrite(p string) bool { return unix.Access(p, unix.W_OK) == nil }

// canDelete reports write access to the parent directory (for a delete).
func canDelete(p string) bool {
	dir := p[:max(0, lastSlash(p))]
	if dir == "" {
		dir = "/"
	}
	return unix.Access(dir, unix.W_OK) == nil
}

func lastSlash(p string) int {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return i
		}
	}
	return 0
}
