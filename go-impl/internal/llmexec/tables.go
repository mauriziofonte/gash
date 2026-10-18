// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package llmexec is the Gash 2.x llm_exec pipeline in Go, up to execution
// (architecture.md 8.3, operator decision D-1 option A). It ports the
// blocklist, the 2.x character-level parser, the read-only command allowlist
// with its per-command and walk rules, the argument path and secret checks,
// glob expansion (by Go, in byte order), the restricted child environment,
// the git hardening flags, sed --sandbox / gawk --sandbox and the bwrap argv,
// and renders the validated parse as a canonical script: every word single
// quoted, the operators and allowed redirections where the parser found them,
// so bash re-reads it without parsing anything of its own (MUST NOT 7: one
// parser decides).
//
// Build validates a line and returns the argv and environment to run; it
// never runs anything. The library prints nothing; a refusal is a
// *pathguard.Refusal carrying the exact 2.x code, details, action and hint.
package llmexec

import "regexp"

// allowedCommands is __LLM_EXEC_ALLOWED (lib/modules/llm/exec.sh:35): the
// read-only commands allowed with any arguments unless a per-command rule
// constrains them. The order is kept for the prelude's `unset -f` line.
var allowedCommands = []string{
	"ls", "cat", "head", "tail", "wc", "sort", "uniq", "cut", "tr", "paste",
	"join", "comm", "column", "nl", "tac", "rev", "fold", "expand", "unexpand",
	"grep", "egrep", "fgrep", "zcat", "zgrep", "bzcat", "xzcat", "rg", "fd",
	"find", "tree", "stat", "file", "du", "df", "free", "uptime", "uname",
	"whoami", "id", "groups", "w", "who", "last", "hostname", "date", "pwd",
	"cd", "echo", "printf", "true", "false", "test", "which", "type",
	"basename", "dirname", "realpath", "readlink", "diff", "cmp", "md5sum",
	"sha1sum", "sha256sum", "sha512sum", "base64", "strings", "od", "hexdump",
	"jq", "yq", "ps", "pgrep", "pidof", "lsof", "ss", "netstat", "ip", "nproc",
	"lscpu", "lsblk", "findmnt", "getent", "locale", "crontab", "git", "sed",
	"awk", "gawk", "systemctl", "journalctl", "docker", "php", "node",
	"python", "python3", "ruby", "java", "go", "npm", "pip", "pip3", "composer",
}

var allowedSet = func() map[string]bool {
	m := make(map[string]bool, len(allowedCommands))
	for _, c := range allowedCommands {
		m[c] = true
	}
	return m
}()

// blocklistPatterns is __LLM_DANGEROUS_PATTERNS (core.sh:13-68), the regex
// tripwire applied to the whitespace-normalized line (quotes included,
// EXEC-03). The strings are the 2.x bash regexes verbatim; they are matched,
// unanchored, with Go's RE2, which reads the same escaped literals.
var blocklistPatterns = []string{
	`rm -rf /`,
	`rm -rf ~`,
	`rm -rf \$HOME`,
	`rm -rf \*`,
	`rm -fr /`,
	`rm -fr ~`,
	`rm  -rf`,
	"rm -rf `",
	`rm -rf \$\(`,
	`rm -r /`,
	`rm -f /`,
	`cd / && rm`,
	`cd /; rm`,
	`dd if=`,
	`mkfs\.`,
	`> /dev/sd`,
	`> /dev/nvme`,
	`> /dev/vd`,
	`chmod -R 777 /`,
	`chmod 777 /`,
	`chown -R .* /`,
	`sudo rm -rf`,
	`sudo dd `,
	`sudo mkfs`,
	`:\(\)\{:\|:&\};:`,
	`shutdown`,
	`reboot`,
	`init 0`,
	`init 6`,
	`halt`,
	`poweroff`,
	`curl.*\|.*sh`,
	`curl.*\|.*bash`,
	`wget.*\|.*sh`,
	`wget.*\|.*bash`,
	`history -c`,
	`history -w`,
	`HISTFILE=`,
	`cat.*/etc/shadow`,
	`cat.*/etc/passwd`,
	`\.ssh/id_`,
	`AWS_SECRET`,
	`PRIVATE_KEY`,
	`> /dev/sda`,
	`> /dev/mem`,
	`> /dev/kmem`,
}

var blocklistRegexps = func() []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(blocklistPatterns))
	for i, p := range blocklistPatterns {
		out[i] = regexp.MustCompile(p)
	}
	return out
}()
