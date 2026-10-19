// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

//go:build !linux

package main

import "net"

// peerEnv needs /proc: elsewhere the knobs are not seen.
func peerEnv(net.Conn) map[string]string { return map[string]string{} }

func loopbackUp() error { return nil }
