// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTCPOpenAndClosed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	dead, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closed := dead.Addr().String()
	require.NoError(t, dead.Close())
	open := ln.Addr().String()
	_, port, _ := net.SplitHostPort(open)
	r := netRun(t, nil, "llm_tcp", open, closed, "localhost:"+port)
	require.Regexp(t, `^\[\{"target":"`+open+`","open":true,"ms":[0-9.]+,"error":null\},`+
		`\{"target":"`+closed+`","open":false,"ms":[0-9.]+,"error":"connection_refused"\},`+
		`\{"target":"localhost:`+port+`","open":true,"ms":[0-9.]+,"error":null\}\]`+"\n$", r.Stdout)
	r = netRun(t, nil, "llm_tcp", open, closed, "--format", "tsv")
	lines := strings.Split(strings.TrimSuffix(r.Stdout, "\n"), "\n")
	require.Equal(t, "target\topen\tms\terror", lines[0])
	require.Regexp(t, `\tfalse\t[0-9.]+\tconnection_refused$`, lines[2])
}

func TestTCPParallelKeepsOrder(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	var targets []string
	for i := 0; i < 20; i++ {
		targets = append(targets, ln.Addr().String(), "127.0.0.1:1")
	}
	norm := func(r result) string { return msRE.ReplaceAllString(r.Stdout, `"ms":0`) }
	serial := netRun(t, nil, "llm_tcp", append(targets, "-j", "1")...)
	parallel := netRun(t, nil, "llm_tcp", append(targets, "--jobs", "16")...)
	require.Equal(t, norm(serial), norm(parallel))
}

func TestTCPArgumentErrors(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "missing_target/RETRY"},
		{[]string{"--timeout", "1"}, "missing_target/RETRY"},
		{[]string{"example.com"}, "invalid_target/RETRY"},
		{[]string{"example.com:"}, "invalid_target/RETRY"},
		{[]string{"example.com:0"}, "invalid_target/RETRY"},
		{[]string{"example.com:65536"}, "invalid_target/RETRY"},
		{[]string{"example.com:08"}, "invalid_target/RETRY"},
		{[]string{"::1:22"}, "invalid_target/RETRY"},
		{[]string{":22"}, "invalid_target/RETRY"},
		{[]string{"h:1", "--bogus"}, "unknown_option/RETRY"},
		{[]string{"h:1", "--timeout", "x"}, "invalid_timeout/RETRY"},
	} {
		require.Equal(t, c.want, errCode(t, netRun(t, nil, "llm_tcp", c.args...)), c.args)
	}
	for in, want := range map[string]bool{"h:1": true, "[::1]:22": true, "10.0.0.1:65535": true, "[fe80::1%eth0]:22": true, "h": false, "[::1]": false} {
		_, _, ok := splitHostPort(in, "")
		require.Equal(t, want, ok, in)
	}
}

func TestNetErrorCodes(t *testing.T) {
	ctx := t.Context()
	require.Equal(t, "", netError(ctx, nil))
	require.Equal(t, "dns_not_found", netError(ctx, &net.DNSError{IsNotFound: true}))
	require.Equal(t, "timeout", netError(ctx, &net.DNSError{IsTimeout: true}))
	require.Equal(t, "dns_error", netError(ctx, &net.DNSError{}))
	require.Equal(t, "transport_error", netError(ctx, net.UnknownNetworkError("x")))
}
