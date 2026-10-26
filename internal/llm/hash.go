// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"context"
	"crypto/md5" //nolint:gosec // md5 is asked for by name, to compare with md5sum, never for security
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
)

// hashRow is the digest of one PATH of llm_hash, or its error.
type hashRow struct {
	name string
	sum  string
	size int64
	err  string
}

// hashFile is the digest of the regular file at path with algo ("sha256"
// or "md5"), in lowercase hex, and the bytes it read. ctx ends a long
// read.
func hashFile(ctx context.Context, path, algo string) (string, int64, error) {
	f, size, err := openRegular(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()
	var h hash.Hash
	if algo == "md5" {
		h = md5.New() //nolint:gosec // see the import
	} else {
		h = sha256.New()
	}
	buf := make([]byte, readBuffer(size, 256<<10))
	var n int64
	for {
		if err := ctx.Err(); err != nil {
			return "", 0, err
		}
		got, err := f.Read(buf)
		h.Write(buf[:got])
		n += int64(got)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", 0, err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// hash is llm_hash PATH... [--algo sha256|md5] [--short N] (plan F10):
// the digests of several files in parallel, in input order. Secret files
// are hashed: a digest discloses no content.
func (m *Module) hash(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags | llmfmt.JobsFlag | llmfmt.TimeoutFlag | llmfmt.PathFlags)
	var paths []string
	algo, short := "sha256", 0
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
		switch a := args[0]; {
		case a == "--algo" || a == "--short":
			v, ok := takeOption(c, args)
			if !ok {
				return 1
			}
			if a == "--algo" {
				if v != "sha256" && v != "md5" {
					return fail(c, "invalid_algo", v, "Use --algo sha256 (default) or md5")
				}
				algo = v
			} else if short, ok = takePositive(c, "invalid_short", v); !ok {
				return 1
			}
			args = args[2:]
			continue
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, "Use --algo sha256|md5, --short N, --format, --absolute, -j or --timeout")
		default:
			paths = append(paths, a)
		}
		args = args[1:]
	}
	if len(paths) == 0 {
		return fail(c, "missing_file", "", "Give one or more files: llm_hash PATH...")
	}
	targets, ok := m.textTargets(c, paths, false)
	if !ok {
		return 1
	}
	rows := llmfmt.Map(c.Ctx, o.JobsOr(llmfmt.LocalJobs()), o.Timeout, targets, func(ctx context.Context, t textTarget) hashRow {
		sum, size, err := hashFile(ctx, t.real, algo)
		r := hashRow{name: t.name(o), sum: sum, size: size}
		if err != nil {
			r.err = targetError(ctx, err)
		}
		if short > 0 && len(r.sum) > short {
			r.sum = r.sum[:short]
		}
		return r
	})
	o.Write(c, o.FormatOr(llmfmt.JSON), hashTable(rows, algo), nil)
	return 0
}

// hashTable is the rows of llm_hash: p, the digest under the algorithm's
// name, size; error when a row has one (the digest and size are null).
func hashTable(rows []hashRow, algo string) llmfmt.Table {
	withErr := false
	for _, r := range rows {
		withErr = withErr || r.err != ""
	}
	t := llmfmt.Table{Cols: []string{"p", algo, "size"}, Rows: make([][]any, len(rows))}
	if withErr {
		t.Cols = append(t.Cols, "error")
	}
	for i, r := range rows {
		row := []any{r.name, nil, nil}
		if r.err == "" {
			row[1], row[2] = r.sum, r.size
		}
		if withErr {
			if r.err != "" {
				row = append(row, r.err)
			} else {
				row = append(row, nil)
			}
		}
		t.Rows[i] = row
	}
	return t
}
