// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package invoke

import (
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// TelemetryVar names the opt-in local usage log (plan S10): when it holds
// a path, every agent function call appends one JSON line to that file:
//
//	{"ts":UNIX,"fn":NAME,"ms":MS,"out_bytes":N,"rc":RC,"truncated":K}
//
// Off by default, a local file only, never the network, never an argument
// or a value. A write failure is ignored: telemetry never fails a call.
const TelemetryVar = "GASH_TELEMETRY_FILE"

// Telemetered reports whether calls of name are logged: the agent family
// (llm_*, agent_*) and the agent commands that pass their name here.
func Telemetered(name string) bool {
	return strings.HasPrefix(name, "llm_") || strings.HasPrefix(name, "agent_")
}

// WithTelemetry runs fn and, when the caller set GASH_TELEMETRY_FILE,
// counts the bytes fn writes to c.Stdout and appends the call's line.
func WithTelemetry(c *Call, name string, fn func() int) int {
	path := c.Env.Get(TelemetryVar)
	if path == "" {
		return fn()
	}
	cw := &countWriter{w: c.Stdout}
	c.Stdout = cw
	t0 := time.Now()
	rc := fn()
	ms := time.Since(t0).Microseconds()
	c.Stdout = cw.w
	line := make([]byte, 0, 128)
	line = append(line, `{"ts":`...)
	line = strconv.AppendInt(line, t0.Unix(), 10)
	line = append(line, `,"fn":`...)
	line = strconv.AppendQuote(line, name)
	line = append(line, `,"ms":`...)
	line = strconv.AppendFloat(line, float64(ms)/1000, 'f', 3, 64)
	line = append(line, `,"out_bytes":`...)
	line = strconv.AppendInt(line, cw.n, 10)
	line = append(line, `,"rc":`...)
	line = strconv.AppendInt(line, int64(rc), 10)
	line = append(line, `,"truncated":`...)
	line = strconv.AppendInt(line, int64(c.truncated), 10)
	line = append(line, "}\n"...)
	// One write with O_APPEND: concurrent calls never interleave a line.
	if f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600); err == nil {
		_, _ = f.Write(line)
		_ = f.Close()
	}
	return rc
}

type countWriter struct {
	w io.Writer
	n int64
}

func (cw *countWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	cw.n += int64(n)
	return n, err
}
