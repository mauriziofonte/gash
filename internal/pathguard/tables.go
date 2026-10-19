// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package pathguard

// The data tables ported verbatim from lib/modules/llm/core.sh. They are the
// shared source of truth for the guards and for docs generation, so the two
// cannot drift. The gaps the S1 audit recorded (LEDGER LLM-10) are kept here
// unchanged: rc.A is a pure port, so /etc/shadow-, /etc/gshadow- and
// /etc/security/opasswd are *not* forbidden, and ssh_host_*_key, .envrc,
// *.tfvars.json, auth.json and credentials_old are *not* secret. The proving
// tests name LLM-10 where they assert those gaps.

// ForbiddenBasePaths is __LLM_FORBIDDEN_PATHS before root's home is appended.
// A path is refused when it equals one of these or lives below it (whole
// components: /root, not /rootfs).
var ForbiddenBasePaths = []string{
	"/proc",
	"/sys",
	"/dev",
	"/boot",
	"/root",
	"/etc/shadow",
	"/etc/gshadow",
	"/etc/passwd",
	"/etc/sudoers",
	"/etc/sudoers.d",
}

// SecretPatterns is __LLM_SECRET_PATTERNS: secret file base-name globs,
// matched case-insensitively (the table is already lowercase, the base name
// is lowercased before matching).
var SecretPatterns = []string{
	".env", ".env.*", "*.env",
	"*.pem", "*.key", "*.p12", "*.pfx", "*.jks", "*.keystore", "*.kdbx", "*.ppk",
	"id_rsa*", "id_dsa*", "id_ecdsa*", "id_ed25519*", "*_rsa", "*_dsa", "*_ecdsa", "*_ed25519",
	"credentials", "credentials.*", "secrets", "secrets.*", ".gash_env",
	".pgpass", ".netrc", "_netrc", ".git-credentials", ".npmrc", ".pypirc", ".htpasswd",
	".my.cnf", ".boto", ".s3cfg", ".dockercfg", ".vault-token",
	"kubeconfig", "*.kubeconfig", "*.tfstate", "*.tfstate.*", "*.tfvars",
}

// SecretExceptions is __LLM_SECRET_EXCEPTIONS: templates that share a secret
// name but hold no secrets. An exception wins over a pattern.
var SecretExceptions = []string{
	"*.pub", ".env.example", ".env.sample", ".env.dist", ".env.template", "*.env.example",
}

// SecretDirs is __LLM_SECRET_DIRS: directories whose whole content is secret,
// matched (case-insensitively) as a component of the full path.
var SecretDirs = []string{
	".ssh", ".gnupg", ".aws", ".azure", ".kube", ".docker", ".config/gcloud",
}

// GitSafeConfig is __LLM_GIT_SAFE_CONFIG: the git -c flags that keep a
// repository's own configuration from running programs or paging. Shared by
// llm_git_* and by the git wrapper inside llm_exec.
var GitSafeConfig = []string{
	"-c", "core.fsmonitor=false",
	"-c", "core.hooksPath=/dev/null",
	"-c", "diff.external=",
	"-c", "core.pager=cat",
}
