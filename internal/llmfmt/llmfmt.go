// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package llmfmt is the shared output layer of the agent functions (llm_*
// and agent_*), plan S1, S2, S3, S4 and S6:
//
//   - --format json|jsonl|tsv|table for row data (Table, Write);
//   - the output budget --width N, --max-lines N, --head N, --tail N, with
//     a marker whenever something was cut (Lines, Cap);
//   - -j/--jobs N and --timeout S for multi-target functions (Map);
//   - --relative / --absolute paths (Rel) and count objects (Count).
//
// A handler builds Options with the flags it accepts and hands each word
// of its own option loop to Take first:
//
//	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags)
//	for len(args) > 0 {
//		n, ok := o.Take(c, args)
//		if !ok {
//			return 1
//		}
//		if n > 0 {
//			args = args[n:]
//			continue
//		}
//		... the function's own options ...
//	}
//
// Rules every function follows (AGENTS.md section 4): stdout carries data
// only; the metadata of a row output (rows, truncated, timings) is one JSON
// line on stderr (plan D8), written by Meta; a JSON object output carries
// "truncated": K itself; a text output carries the "[... K lines omitted]"
// marker inline. Nothing is ever cut silently.
package llmfmt

import (
	"time"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
)

// Format is an output format of --format.
type Format string

// The formats. Default is the function's own default (its JSON shape, or
// text lines for a text function).
const (
	Default Format = ""
	JSON    Format = "json"
	JSONL   Format = "jsonl"
	TSV     Format = "tsv"
	TableF  Format = "table"
)

// Flags selects the shared options a function accepts.
type Flags uint

// The shared option groups.
const (
	// FormatFlag is --format json|jsonl|tsv|table.
	FormatFlag Flags = 1 << iota
	// BudgetFlags are --width N, --max-lines N, --head N, --tail N.
	BudgetFlags
	// JobsFlag is -j N / --jobs N.
	JobsFlag
	// TimeoutFlag is --timeout S (per target).
	TimeoutFlag
	// PathFlags are --relative and --absolute.
	PathFlags
)

// MaxTimeout caps --timeout.
const MaxTimeout = 3600 * time.Second

// Options are the parsed shared options of one call.
type Options struct {
	allow Flags
	// Format is the --format value, Default when not given.
	Format Format
	// Width cuts each output line (or JSON string value) at Width display
	// columns; 0 = no cut.
	Width int
	// Head and Tail keep the first Head and the last Tail lines or rows;
	// both 0 = no cap. --max-lines N is --head N.
	Head, Tail int
	// Jobs is -j; 0 = the function's default (see JobsOr).
	Jobs int
	// Timeout bounds each target; 0 = the function's default.
	Timeout time.Duration
	// Relative is --relative; Absolute is --absolute (the two are exclusive,
	// the last one given wins).
	Relative, Absolute bool
}

// New returns Options accepting the flags of allow.
func New(allow Flags) *Options { return &Options{allow: allow} }

// Accepts reports whether the function takes the flag group f.
func (o *Options) Accepts(f Flags) bool { return o.allow&f != 0 }

// Capped reports whether a line or row budget is set.
func (o *Options) Capped() bool { return o.Head > 0 || o.Tail > 0 }

// Take parses the shared option at args[0]. It returns the number of words
// it consumed (0 when args[0] is not a shared option this function
// accepts) and false when it wrote an error envelope (the caller returns
// 1).
func (o *Options) Take(c *invoke.Call, args []string) (int, bool) {
	if len(args) == 0 {
		return 0, true
	}
	a := args[0]
	value := func() (string, bool) {
		if len(args) < 2 {
			fail(c, "missing_value", a, a+" needs a value")
			return "", false
		}
		return args[1], true
	}
	switch {
	case a == "--format" && o.Accepts(FormatFlag):
		v, ok := value()
		if !ok {
			return 0, false
		}
		switch Format(v) {
		case JSON, JSONL, TSV, TableF:
			o.Format = Format(v)
			return 2, true
		}
		fail(c, "invalid_format", v, "Use --format json, jsonl, tsv or table")
		return 0, false
	case (a == "--width" || a == "--max-lines" || a == "--head" || a == "--tail") && o.Accepts(BudgetFlags):
		v, ok := value()
		if !ok {
			return 0, false
		}
		name := "invalid_" + map[string]string{"--width": "width", "--max-lines": "max_lines", "--head": "head", "--tail": "tail"}[a]
		n, ok := positive(c, name, v)
		if !ok {
			return 0, false
		}
		switch a {
		case "--width":
			o.Width = n
		case "--max-lines", "--head":
			o.Head = n
		default:
			o.Tail = n
		}
		return 2, true
	case (a == "-j" || a == "--jobs") && o.Accepts(JobsFlag):
		v, ok := value()
		if !ok {
			return 0, false
		}
		n, ok := positive(c, "invalid_jobs", v)
		if !ok {
			return 0, false
		}
		o.Jobs = min(n, 256)
		return 2, true
	case a == "--timeout" && o.Accepts(TimeoutFlag):
		v, ok := value()
		if !ok {
			return 0, false
		}
		d, err := ParseSeconds(v)
		if err != nil {
			fail(c, "invalid_timeout", v, "Give seconds (2, 0.5) or a duration (500ms, 2s, 1m), at most 1h")
			return 0, false
		}
		o.Timeout = d
		return 2, true
	case a == "--relative" && o.Accepts(PathFlags):
		o.Relative, o.Absolute = true, false
		return 1, true
	case a == "--absolute" && o.Accepts(PathFlags):
		o.Relative, o.Absolute = false, true
		return 1, true
	}
	return 0, true
}

// JobsOr returns -j, or def when it was not given.
func (o *Options) JobsOr(def int) int {
	if o.Jobs > 0 {
		return o.Jobs
	}
	return max(def, 1)
}

// TimeoutOr returns --timeout, or def when it was not given.
func (o *Options) TimeoutOr(def time.Duration) time.Duration {
	if o.Timeout > 0 {
		return o.Timeout
	}
	return def
}

// FormatOr returns --format, or def when it was not given.
func (o *Options) FormatOr(def Format) Format {
	if o.Format != Default {
		return o.Format
	}
	return def
}

// positive parses a plain integer 1..999999999 (N <= 0 is a RETRY error,
// plan S2).
func positive(c *invoke.Call, code, v string) (int, bool) {
	n := 0
	if len(v) == 0 || len(v) > 9 {
		n = -1
	}
	for i := 0; i < len(v) && n >= 0; i++ {
		if v[i] < '0' || v[i] > '9' {
			n = -1
			break
		}
		n = n*10 + int(v[i]-'0')
	}
	if n <= 0 {
		fail(c, code, v, "Give a positive integer (1 to 999999999)")
		return 0, false
	}
	return n, true
}

// ParseSeconds parses --timeout and the other per-target durations: plain
// seconds with an optional fraction ("2", "0.5") or a Go duration with a
// unit ("500ms", "2s", "1m"); it must be above 0 and at most MaxTimeout.
func ParseSeconds(v string) (time.Duration, error) {
	d, err := time.ParseDuration(v)
	if err != nil {
		d, err = time.ParseDuration(v + "s")
	}
	if err != nil || d <= 0 || d > MaxTimeout {
		return 0, errBadDuration
	}
	return d, nil
}

type durationError struct{}

func (durationError) Error() string { return "invalid duration" }

var errBadDuration error = durationError{}

func fail(c *invoke.Call, code, details, hint string) {
	c.Fail(contract.New(code, details, hint))
}
