// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"strings"
	"testing"
)

// TestRedact covers the ai_client_spec vectors: secrets removed, the
// marker line kept, a URL password masked but the URL kept.
func TestRedact(t *testing.T) {
	in := `DB_PASSWORD=hunter2
export API_KEY="abc123def"
url: postgres://app:s3cret@db:5432/app
aws AKIAABCDEFGHIJKLMNOP
gh ghp_abcdefghijklmnopqrstuvwxyz0123456789
-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAA
-----END OPENSSH PRIVATE KEY-----
keep this line`
	out := redact(in)
	for _, secret := range []string{"hunter2", "abc123def", "s3cret", "AKIAABCDEFGHIJKLMNOP", "ghp_abcdefghij", "b3BlbnNzaC1rZXktdjEAAAAA"} {
		if strings.Contains(out, secret) {
			t.Fatalf("leaked %q in:\n%s", secret, out)
		}
	}
	if !strings.Contains(out, "keep this line") {
		t.Fatalf("dropped the kept line:\n%s", out)
	}
	if !strings.Contains(out, "postgres://app:") {
		t.Fatalf("mangled the URL:\n%s", out)
	}
	if !strings.Contains(out, "[REDACTED PRIVATE KEY]") {
		t.Fatalf("no private-key marker:\n%s", out)
	}
}

// TestRedactKeepsOrdinaryWords: a password/secret/token word followed by a
// short word is kept; a long token after it is redacted.
func TestRedactKeepsOrdinaryWords(t *testing.T) {
	out := redact("token expired at login\nmy secret is safe\ntoken Zk29dLq8Pz7Wm4Rt1xYv")
	for _, keep := range []string{"token expired at login", "my secret is safe"} {
		if !strings.Contains(out, keep) {
			t.Fatalf("dropped %q:\n%s", keep, out)
		}
	}
	if strings.Contains(out, "Zk29dLq8Pz7Wm4Rt1xYv") {
		t.Fatalf("kept the long token:\n%s", out)
	}
}

// TestLedger_AI06_2x: redact itself still drops everything after an
// unclosed PRIVATE KEY block, the question included when it is given the
// composed text. 3.0: AI-06 fixed in the callers, which redact the piped
// part and the question separately (TestLedger_AI06_fix).
func TestLedger_AI06_2x(t *testing.T) {
	in := "[PIPE INPUT - TROUBLESHOOT MODE]\n---\n-----BEGIN RSA PRIVATE KEY-----\nabc\n---\nUser question: q1\n"
	out := redact(in)
	if strings.Contains(out, "abc") || strings.Contains(out, "q1") {
		t.Fatalf("unclosed block did not swallow the rest:\n%s", out)
	}
	if !strings.HasSuffix(out, "[REDACTED PRIVATE KEY]") {
		t.Fatalf("want trailing marker, got:\n%s", out)
	}
}

// TestLedger_AI07_fix: bearer headers, SSH: passphrase lines and crypt
// hashes are redacted (3.0: AI-07 fixed; 2.x kept them); free text is not
// detected, and a KEY=value secret still is.
func TestLedger_AI07_fix(t *testing.T) {
	in := "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.e30.abcdef\n" +
		"proxy-authorization: basic dXNlcjpwYXNz\n" +
		"SSH:~/.ssh/id_rsa=correct horse battery\n" +
		"root:$6$fakesalt$fakehash:19000:0:99999:7:::\n" +
		"admin:$2y$10$abcdefghijklmnopqrstuv:x\n" +
		"the password is hunter2\nDB_PASSWORD=s3cret"
	want := "Authorization: Bearer [REDACTED]\n" +
		"proxy-authorization: basic [REDACTED]\n" +
		"SSH:~/.ssh/id_rsa=[REDACTED]\n" +
		"root:[REDACTED HASH]:19000:0:99999:7:::\n" +
		"admin:[REDACTED HASH]:x\n" +
		"the password is hunter2\nDB_PASSWORD=[REDACTED]"
	if out := redact(in); out != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

// TestRedactWholeText: a closed block in the middle of one text keeps what
// follows it.
func TestRedactWholeText(t *testing.T) {
	// A closed block in the middle keeps the following question.
	in := "pre\n-----BEGIN RSA PRIVATE KEY-----\nxyz\n-----END RSA PRIVATE KEY-----\nUser question: hello"
	out := redact(in)
	if !strings.Contains(out, "User question: hello") {
		t.Fatalf("closed block dropped the question:\n%s", out)
	}
	if strings.Contains(out, "xyz") {
		t.Fatalf("key body leaked:\n%s", out)
	}
}
