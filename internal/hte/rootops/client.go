// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package rootops

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/hte/child"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/runner"
)

// RootArgs are the arguments of the gash binary that serve a root session.
var RootArgs = []string{"hte", "__root"}

// SessionTimeout bounds one root session (migrate --apply verifies sites
// while it is open).
const SessionTimeout = 30 * time.Minute

// Client is the user side of a root session.
type Client struct {
	sess runner.Session
	enc  *json.Encoder
	dec  *json.Decoder
	plan map[int]Op
	done bool
	wait error
	// Receipt is the root copy after the last answer.
	Receipt string
	// OnReceipt, when set, gets the root copy after every answer that
	// carries one (the user side stores it as its own copy).
	OnReceipt func(text string) error
}

// SudoCmd is the command that starts a root session: sudo, -n when no
// prompt is allowed (headless, no terminal), then the gash binary. It
// stays in the caller's session, so that sudo can prompt on the terminal;
// interactive, sudo's prompt and messages go to the terminal's standard
// error, else they are captured for the error message.
func SudoCmd(sudo, self string, interactive bool) runner.Cmd {
	args := []string{}
	if !interactive {
		args = append(args, "-n")
	}
	args = append(args, "--", self)
	args = append(args, RootArgs...)
	c := child.Cmd(sudo, nil, args...)
	c.Timeout, c.Setsid, c.FileCapture = SessionTimeout, false, false
	if interactive {
		c.Stderr = os.Stderr
	}
	return c
}

// Open starts the session and sends the plan.
func Open(ctx context.Context, st runner.SessionStarter, cmd runner.Cmd, plan []Op) (*Client, error) {
	sess, err := st.StartSession(ctx, cmd)
	if err != nil {
		return nil, hteerr.Newf(hteerr.PrivilegedStepRequired, "", "starting %s: %v", child.Line(append([]string{cmd.Path}, cmd.Args...)), err)
	}
	c := &Client{sess: sess, enc: json.NewEncoder(sess.Stdin()), dec: json.NewDecoder(bufio.NewReader(sess.Stdout())), plan: map[int]Op{}}
	for _, o := range plan {
		c.plan[o.ID] = o
	}
	if err := c.enc.Encode(Request{Hello: &Hello{Plan: plan}}); err != nil {
		return nil, c.failed(err)
	}
	resp, err := c.read()
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		c.Close()
		return nil, resp.Error.Err()
	}
	return c, nil
}

// failed closes the session and explains why it ended: sudo refusing (no
// password without a terminal, not in sudoers) ends it before any answer.
func (c *Client) failed(cause error) error {
	c.sess.Stdin().Close()
	res, _ := c.sess.Wait()
	msg := strings.TrimSpace(string(res.Stderr))
	if msg == "" && cause != nil {
		msg = cause.Error()
	}
	hint := "run the command on a terminal: sudo asks for your password once"
	if strings.Contains(msg, "not in the sudoers") || strings.Contains(msg, "not allowed") {
		hint = "an administrator must allow you to run gash through sudo"
	}
	return hteerr.Newf(hteerr.PrivilegedStepRequired, hint, "the root step did not run: %s", msg)
}

func (c *Client) read() (Response, error) {
	var resp Response
	if err := c.dec.Decode(&resp); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return resp, c.failed(nil)
		}
		return resp, c.failed(err)
	}
	if resp.Receipt != "" || resp.OK {
		c.Receipt = resp.Receipt
		if c.OnReceipt != nil {
			if err := c.OnReceipt(resp.Receipt); err != nil {
				return resp, err
			}
		}
	}
	return resp, nil
}

// Run asks root for operation id and waits for its answer.
func (c *Client) Run(id int) (Response, error) {
	if _, ok := c.plan[id]; !ok {
		return Response{}, fmt.Errorf("rootops: operation %d is not in the plan", id)
	}
	if err := c.enc.Encode(Request{Run: id}); err != nil {
		return Response{}, c.failed(err)
	}
	resp, err := c.read()
	if err != nil {
		return resp, err
	}
	if resp.Error != nil {
		return resp, resp.Error.Err()
	}
	return resp, nil
}

// Close ends the session; a second call returns the first one's result.
func (c *Client) Close() error {
	if !c.done {
		c.done = true
		c.sess.Stdin().Close()
		_, c.wait = c.sess.Wait()
	}
	return c.wait
}
