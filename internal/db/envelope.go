// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
)

// slowQueryUS is the query time (µs) from which llm_db_query adds the
// plan as slow_query_explain.
const slowQueryUS = 100000

// queryClock adds up the time the queries of one call ran: from sending a
// query to having read its last row, after the connection and the session
// settings (LEDGER DB-06).
type queryClock struct {
	took time.Duration
	runs int
}

type queryClockKey struct{}

// startQuery starts timing one query of the call ctx belongs to; the
// function it returns stops it. Outside an envelope it does nothing.
func (m *Module) startQuery(ctx context.Context) func() {
	qc, _ := ctx.Value(queryClockKey{}).(*queryClock)
	if qc == nil {
		return func() {}
	}
	t0 := m.now()
	return func() {
		qc.took += m.now().Sub(t0)
		qc.runs++
	}
}

// envelope is __llm_db_envelope. query_time, and the 100 ms rule that adds
// slow_query_explain when slow is given, count the time the queries ran,
// not the argument checks, the connection or the session settings: 2.x
// started the clock before the client was spawned, so a slow connection
// with a trivial query looked slow (3.0: LEDGER DB-06 fixed). A call that
// ran no query (refused, or no connection) counts its whole time.
// Success prints {data, rows, query_time}, an error gets query_time added.
func (m *Module) envelope(c *invoke.Call, slow func() output, impl func() output) int {
	qc := &queryClock{}
	saved := c.Ctx
	c.Ctx = context.WithValue(ctxOf(c), queryClockKey{}, qc)
	t0 := m.now()
	o := impl()
	elapsed := m.now().Sub(t0)
	c.Ctx = saved
	if qc.runs > 0 {
		elapsed = qc.took
	}
	us := elapsed.Microseconds()
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
		// A text plan (SQLite's EXPLAIN QUERY PLAN through llm_db_query)
		// is [{plan: TEXT}], as llm_db_explain gives it. 2.x piped the
		// sqlite3 shell's text into jq, which refused it: a raw jq error,
		// exit 5 (3.0: LEDGER DB-14 fixed).
		o = planArray(o)
	}
	if o.mode.active() {
		return m.writeRows(c, o, qt, xpl)
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
