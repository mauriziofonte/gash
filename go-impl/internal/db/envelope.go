// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
)

// slowQueryUS is the query time (µs) from which llm_db_query adds the
// plan as slow_query_explain.
const slowQueryUS = 100000

// envelope is __llm_db_envelope: impl runs under the clock (argument
// checks, settings, connection and query, as 2.x measured; LEDGER DB-06).
// Success prints {data, rows, query_time} (and slow_query_explain when
// slow is given and the call took 100 ms), an error gets query_time added.
func (m *Module) envelope(c *invoke.Call, slow func() output, impl func() output) int {
	t0 := m.now()
	o := impl()
	us := m.now().Sub(t0).Microseconds()
	if us < 0 {
		us = 0
	}
	qt := formatQueryTime(us)
	if o.fail != nil {
		return c.Fail(o.fail.With("query_time", qt))
	}
	var xpl *jv
	if slow != nil && us >= slowQueryUS {
		x := jvArray(nil)
		if p := slow(); p.fail == nil && !p.isText && len(p.vals) == 1 && p.vals[0].truthy() {
			x = p.vals[0]
		}
		xpl = &x
	}
	if o.isText {
		// 2.x piped the sqlite3 shell's text rendering of an EXPLAIN into
		// jq, which refused it.
		_, _ = fmt.Fprintf(c.Stderr, "jq: parse error: Invalid numeric literal at line 1, column %d\n", firstWordLen(o.text)+1)
		return 5
	}
	for _, v := range o.vals {
		var obj object
		obj.set("data", v)
		obj.set("rows", jvNumber(strconv.Itoa(v.length())))
		obj.set("query_time", jvString(qt))
		if xpl != nil {
			obj.set("slow_query_explain", *xpl)
		}
		_, _ = c.Stdout.Write(append(obj.value().appendJSON(nil), '\n'))
	}
	return 0
}

func firstWordLen(s string) int {
	if i := strings.IndexAny(s, " \t\n"); i >= 0 {
		return i
	}
	return len(s)
}

// formatQueryTime is __llm_format_query_time: 842µs, 12.3ms, 1.23s
// (digits truncated, not rounded).
func formatQueryTime(us int64) string {
	switch {
	case us < 1000:
		return fmt.Sprintf("%dµs", us)
	case us < 1000000:
		return fmt.Sprintf("%d.%dms", us/1000, (us%1000)/100)
	}
	return fmt.Sprintf("%d.%02ds", us/1000000, (us%1000000)/10000)
}
