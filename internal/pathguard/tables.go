// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package pathguard

// The data tables ported from lib/modules/llm/core.sh. They are the shared
// source of truth for the guards and for the docs, so the two cannot drift.
// 3.0 closes the gaps the S1 audit recorded (LEDGER LLM-10 fixed): the
// shadow backups and /etc/security/opasswd are forbidden, and the SSH host
// keys, .envrc, *.tfvars.json and credentials_*/credentials-* are secret.
// The recipe's credentials* would also take CredentialsController.php, a
// name the 2.x specs pin as not secret, so the two separators are listed.

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
	"/etc/shadow-",
	"/etc/gshadow-",
	"/etc/security/opasswd",
}

// SecretPatterns is __LLM_SECRET_PATTERNS: secret file base-name globs,
// matched case-insensitively (the table is already lowercase, the base name
// is lowercased before matching).
var SecretPatterns = []string{
	".env", ".env.*", "*.env",
	"*.pem", "*.key", "*.p12", "*.pfx", "*.jks", "*.keystore", "*.kdbx", "*.ppk",
	"id_rsa*", "id_dsa*", "id_ecdsa*", "id_ed25519*", "*_rsa", "*_dsa", "*_ecdsa", "*_ed25519",
	"credentials", "credentials.*", "credentials_*", "credentials-*", "secrets", "secrets.*", ".gash_env",
	".pgpass", ".netrc", "_netrc", ".git-credentials", ".npmrc", ".pypirc", ".htpasswd",
	".my.cnf", ".boto", ".s3cfg", ".dockercfg", ".vault-token",
	"kubeconfig", "*.kubeconfig", "*.tfstate", "*.tfstate.*", "*.tfvars", "*.tfvars.json",
	"ssh_host_*_key", ".envrc",
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
