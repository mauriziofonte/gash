// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package app is the process entry of the binary: environment snapshot,
// terminal facts, signals, exit status, and the panic envelope.
package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"sync/atomic"
	"syscall"

	"github.com/mauriziofonte/gash/internal/cli"
	"github.com/mauriziofonte/gash/internal/compatgen"
	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/handlers"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// Main runs the binary with args (without the program name) and returns
// the exit status.
//
//   - A panic in a handler becomes one internal_error envelope on stderr
//     and status 1; the stack trace follows only with GASH_DEBUG=1.
//   - SIGINT and SIGTERM cancel the handler's context; the status is then
//     128+N, as for a 2.x function killed by the signal. A second signal
//     gets the default action.
func Main(args, environ []string, stdin, stdout, stderr *os.File) (status int) {
	env := invoke.EnvFrom(environ)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var got atomic.Int32
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		s, ok := <-sig
		if !ok {
			return
		}
		signal.Stop(sig)
		if n, isSys := s.(syscall.Signal); isSys {
			got.Store(int32(n))
		}
		cancel()
	}()
	defer func() {
		signal.Stop(sig)
		close(sig)
		if r := recover(); r != nil {
			status = panicked(stderr, env, r)
		}
		if n := got.Load(); n != 0 {
			status = 128 + int(n)
		}
	}()

	return cli.Run(cli.Process{
		Ctx: ctx, Args: args, Env: env,
		Stdin: stdin, Stdout: stdout, Stderr: stderr,
		Term:  term.New(env, stdin, stdout, stderr),
		Table: newTable(),
	})
}

// newTable builds the handler table (a variable for the panic test).
var newTable = func() *invoke.Table { return handlers.Table(compatgen.Register) }

func panicked(stderr io.Writer, env invoke.Env, r any) int {
	_ = contract.New("internal_error", fmt.Sprint(r), "").Write(stderr)
	if env.Get("GASH_DEBUG") == "1" {
		_, _ = stderr.Write(debug.Stack())
	}
	return 1
}
