// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ssh

import (
	"context"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/runner"
)

// The patterns of the 2.x expect script, in its order: the prompt is
// answered (exp_continue), the others end the dialog. Tcl's . matches a
// newline.
var (
	passPrompt  = regexp.MustCompile(`(?s)Enter passphrase.*:`)
	dialogEnds  = []*regexp.Regexp{regexp.MustCompile(`(?s)Identity added:.*`), regexp.MustCompile(`(?s)Bad passphrase.*`), regexp.MustCompile(`(?s)` + noAgent + `.*`)}
	readBufSize = 4096
)

// child is one ssh-add on its terminal, with a reader that reads only
// when asked, so that a wait can time out.
type child struct {
	pty    runner.PTY
	cancel context.CancelFunc
	req    chan struct{}
	res    chan chunk
	done   chan struct{}
	asked  bool // a read was asked for and not received
	ended  bool // the terminal returned an error (eof)
}

type chunk struct {
	data string
	err  error
}

// addKeys is the expect script: for each key `spawn ssh-add -- KEY` on a
// terminal, the passphrase typed at each prompt, until a closing pattern,
// eof or the timeout. It returns the transcript expect printed: the spawn
// lines and everything read. The terminals are closed at the end, as
// expect's exit closed them.
func (m *Module) addKeys(s *session, keys []config.Entry) string {
	var t strings.Builder
	var kids []*child
	for _, k := range keys {
		t.WriteString("spawn ssh-add -- " + k.Name + "\r\n")
		ctx, cancel := context.WithCancel(s.ctx())
		p, err := m.run.StartPTY(ctx, runner.Cmd{Path: "ssh-add", Args: []string{"--", k.Name},
			Options: runner.Options{Env: s.env}})
		if err != nil {
			cancel()
			continue
		}
		ch := &child{pty: p, cancel: cancel, req: make(chan struct{}), res: make(chan chunk, 1), done: make(chan struct{})}
		go ch.reader()
		kids = append(kids, ch)
		t.WriteString(ch.converse(k.Value, m.wait))
	}
	for _, ch := range kids {
		close(ch.done)
		_ = ch.pty.Close()
	}
	for _, ch := range kids {
		ch.reap(m.settle)
	}
	return t.String()
}

// reader reads the terminal once per request.
func (ch *child) reader() {
	b := make([]byte, readBufSize)
	for {
		select {
		case <-ch.req:
		case <-ch.done:
			return
		}
		n, err := ch.pty.Read(b)
		ch.res <- chunk{string(b[:n]), err}
		if err != nil {
			return
		}
	}
}

// read waits for the next output until timeout; false on eof or timeout.
func (ch *child) read(timeout <-chan time.Time) (string, bool) {
	if ch.ended {
		return "", false
	}
	if !ch.asked {
		ch.req <- struct{}{}
		ch.asked = true
	}
	select {
	case r := <-ch.res:
		ch.asked = false
		if r.err != nil {
			ch.ended = true
		}
		return r.data, r.data != "" || r.err == nil
	case <-timeout:
		return "", false
	}
}

// converse is the expect block for one key; it returns what was read.
func (ch *child) converse(pass string, wait time.Duration) string {
	var seen strings.Builder
	buf := ""
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		if loc := passPrompt.FindStringIndex(buf); loc != nil {
			buf = buf[loc[1]:]
			// send -- "$pass\r": typed, never in argv, env or a file.
			if _, err := io.WriteString(ch.pty, pass+"\r"); err != nil {
				return seen.String()
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(wait)
			continue
		}
		for _, re := range dialogEnds {
			if re.MatchString(buf) {
				return seen.String()
			}
		}
		data, ok := ch.read(timer.C)
		if !ok {
			return seen.String()
		}
		seen.WriteString(data)
		buf += data
	}
}

// reap waits for the child after its terminal was closed; one that does
// not exit within settle is signaled.
func (ch *child) reap(settle time.Duration) {
	exited := make(chan struct{})
	go func() {
		_, _ = ch.pty.Wait()
		close(exited)
	}()
	select {
	case <-exited:
	case <-time.After(settle):
		ch.cancel()
		<-exited
	}
	ch.cancel()
}
