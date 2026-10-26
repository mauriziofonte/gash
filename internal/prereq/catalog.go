// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package prereq

// pkgs builds a Packages map: the same name for every manager, then the
// overrides (an empty override leaves the manager out).
func pkgs(name string, over ...string) map[Manager]string {
	m := map[Manager]string{}
	for _, mg := range Managers {
		m[mg] = name
	}
	for i := 0; i+1 < len(over); i += 2 {
		if over[i+1] == "" {
			delete(m, Manager(over[i]))
		} else {
			m[Manager(over[i])] = over[i+1]
		}
	}
	return m
}

// The URLs of the tools Gash never installs.
const (
	dockerURL   = "https://docs.docker.com/engine/install/"
	nvmURL      = "https://github.com/nvm-sh/nvm#installing-and-updating"
	composerURL = "https://getcomposer.org/download/"
	cosignURL   = "https://docs.sigstore.dev/cosign/system_config/installation/"
	brewURL     = "https://brew.sh"
)

// Areas, in report order, with the heading gash_doctor prints.
var Areas = []struct{ Key, Title string }{
	{"core", "Install and update"},
	{"git", "Git"},
	{"remote", "Remote hosts and SSH"},
	{"processes", "Processes and ports"},
	{"docker", "Docker"},
	{"archives", "Archives"},
	{"runtimes", "Runtimes and linters"},
	{"exec", "Agent command runner (llm_exec)"},
	{"system", "System"},
	{"hte", "Local web stack (gash hte)"},
	{"windows", "Windows (WSL)"},
	{"release", "Release signatures"},
}

// catalog is every external program Gash runs that a system may lack
// (the census of 2026-10-26, dev/port/archive and docs/requirements.md).
// Programs every POSIX system has (cat, mkdir, mv, rm, ...) are left out.
var catalog = []Tool{
	// Install and update.
	{Name: "bash", Purpose: "runs Gash; version 4.4 or newer", Level: Required, Area: "core",
		Packages: pkgs("bash"), MinVersion: "4.4", Note: "On macOS, make the Homebrew bash your login shell: chsh -s \"$(brew --prefix)/bin/bash\""},
	{Name: "curl", Commands: []string{"curl", "wget"}, Purpose: "downloads the Gash binary (wget works too)",
		Level: Required, Area: "core", Packages: pkgs("curl"), Builtin: []string{"darwin"}},
	{Name: "tar", Purpose: "unpacks the Gash release archive", Level: Required, Area: "core",
		Packages: pkgs("tar", "brew", ""), Builtin: []string{"darwin"}},
	{Name: "gzip", Purpose: "decompresses the Gash release archive and .gz files", Level: Required, Area: "core",
		Packages: pkgs("gzip", "brew", ""), Builtin: []string{"darwin"}},
	{Name: "sha256sum", Commands: []string{"sha256sum", "shasum"}, Purpose: "verifies the downloaded binary against its pinned hash",
		Level: Required, Area: "core", Packages: pkgs("coreutils", "brew", ""), Builtin: []string{"darwin"}},
	{Name: "git", Purpose: "installs and updates Gash, and runs the git functions",
		Level: Required, Area: "core", Packages: pkgs("git"), Code: "git_not_found",
		Note: "On macOS git comes with the Command Line Tools: xcode-select --install"},
	{Name: "grep", Purpose: "is used by the installer", Level: Required, Area: "core",
		Packages: pkgs("grep", "brew", ""), Builtin: []string{"darwin"}},
	{Name: "sed", Purpose: "is used by the installer and gash_uninstall", Level: Required, Area: "core",
		Packages: pkgs("sed", "brew", ""), Builtin: []string{"darwin"}},

	// Remote hosts and SSH.
	{Name: "ssh", Purpose: "connects to remote hosts",
		Level: Feature, Area: "remote", Builtin: []string{"darwin"},
		Packages: pkgs("openssh-client", "dnf", "openssh-clients", "pacman", "openssh", "zypper", "openssh-clients", "brew", "")},
	{Name: "scp", Purpose: "copies files to and from remote hosts",
		Level: Feature, Area: "remote", Builtin: []string{"darwin"},
		Packages: pkgs("openssh-client", "dnf", "openssh-clients", "pacman", "openssh", "zypper", "openssh-clients", "brew", "")},
	{Name: "rsync", Purpose: "syncs directories with remote hosts",
		Level: Feature, Area: "remote", Packages: pkgs("rsync", "brew", ""), Builtin: []string{"darwin"}},
	{Name: "ssh-add", Commands: []string{"ssh-add"}, Purpose: "loads SSH keys into the agent",
		Level: Feature, Area: "remote", Builtin: []string{"darwin"},
		Packages: pkgs("openssh-client", "dnf", "openssh-clients", "pacman", "openssh", "zypper", "openssh-clients", "brew", "")},
	{Name: "ssh-agent", Purpose: "holds the unlocked SSH keys",
		Level: Feature, Area: "remote", Builtin: []string{"darwin"},
		Packages: pkgs("openssh-client", "dnf", "openssh-clients", "pacman", "openssh", "zypper", "openssh-clients", "brew", "")},

	// Processes and ports.
	{Name: "ps", Purpose: "lists the running processes",
		Level: Feature, Area: "processes", Code: "ps_not_found", Builtin: []string{"darwin"},
		Packages: pkgs("procps", "dnf", "procps-ng", "pacman", "procps-ng", "brew", "")},
	{Name: "pgrep", Purpose: "finds processes by name",
		Level: Feature, Area: "processes", Builtin: []string{"darwin"},
		Packages: pkgs("procps", "dnf", "procps-ng", "pacman", "procps-ng", "brew", "")},
	{Name: "lsof", Purpose: "maps ports to the processes that hold them",
		Level: Feature, Area: "processes", Packages: pkgs("lsof", "brew", ""), Builtin: []string{"darwin"}},
	{Name: "ss", Commands: []string{"ss", "netstat"}, Purpose: "lists the listening ports; netstat works too",
		Level: Feature, Area: "processes", Code: "ports_tool_not_found", Builtin: []string{"darwin"},
		Packages: pkgs("iproute2", "dnf", "iproute", "brew", "")},

	// Docker.
	{Name: "docker", Purpose: "runs and inspects containers",
		Level: Feature, Area: "docker", Code: "docker_not_found", NoInstall: true, Manual: dockerURL},
	{Name: "docker-compose", Commands: []string{"docker-compose"}, Purpose: "runs compose projects; the docker compose plugin works too",
		Level: Optional, Area: "docker", Code: "compose_not_installed", NoInstall: true, Manual: dockerURL},

	// Archives (archive_extract, one format each).
	{Name: "unzip", Purpose: "extracts .zip archives",
		Level: Feature, Area: "archives", Packages: pkgs("unzip", "brew", ""), Builtin: []string{"darwin"}},
	{Name: "bzip2", Commands: []string{"bzip2", "bunzip2"}, Purpose: "extracts .bz2 archives",
		Level: Feature, Area: "archives", Packages: pkgs("bzip2", "brew", ""), Builtin: []string{"darwin"}},
	{Name: "xz", Purpose: "extracts .xz archives, and lets archive_extract check a .tar.xz before extracting it",
		Level: Feature, Area: "archives", Packages: pkgs("xz", "apt", "xz-utils")},
	{Name: "zstd", Purpose: "extracts .zst archives", Level: Feature, Area: "archives", Packages: pkgs("zstd")},
	{Name: "7z", Commands: []string{"7z", "7zz"}, Purpose: "extracts .7z archives", Level: Feature, Area: "archives",
		Packages: pkgs("7zip", "apt", "p7zip-full", "dnf", "p7zip-plugins", "brew", "p7zip"), EPEL: true},
	{Name: "unrar", Purpose: "extracts .rar archives", Level: Feature, Area: "archives", NonFree: true,
		Packages: pkgs("unrar", "dnf", "", "brew", ""), Manual: "https://www.rarlab.com/download.htm"},
	{Name: "uncompress", Purpose: "extracts .Z archives", Level: Feature, Area: "archives",
		Packages: pkgs("ncompress", "brew", ""), Builtin: []string{"darwin"}},

	// Runtimes and linters.
	{Name: "php", Purpose: "runs PHP code and lints .php files",
		Level: Feature, Area: "runtimes", Code: "runtime_not_installed",
		Packages: pkgs("php-cli", "pacman", "php", "zypper", "php8-cli", "apk", "php83", "brew", "php")},
	{Name: "composer", Purpose: "runs composer with a chosen PHP version",
		Level: Optional, Area: "runtimes", Code: "runtime_not_installed", NoInstall: true, Manual: composerURL},
	{Name: "node", Purpose: "runs Node code and lints .js files",
		Level: Feature, Area: "runtimes", Code: "runtime_not_installed", NoInstall: true, Manual: nvmURL,
		Note: "Gash works with the Node versions nvm installs"},
	{Name: "python3", Purpose: "lints .py files", Level: Feature, Area: "runtimes", Code: "runtime_not_installed",
		Packages: pkgs("python3", "pacman", "python", "brew", "python")},

	// The agent command runner: GNU tools with a sandbox mode.
	{Name: "gawk", Purpose: "lets llm_exec run awk, through gawk --sandbox",
		Level: Optional, Area: "exec", Packages: pkgs("gawk"), Probe: []string{"--sandbox", "BEGIN{}"}},
	{Name: "gnu-sed", Commands: []string{"sed", "gsed"}, Purpose: "lets llm_exec run sed, through GNU sed --sandbox",
		Level: Optional, Area: "exec", Packages: pkgs("sed", "brew", "gnu-sed"), Probe: []string{"--sandbox", "-n", "p"},
		Note: "Homebrew installs it as gsed; Gash finds it there"},
	{Name: "diffutils", Commands: []string{"diff"}, Purpose: "lets llm_exec run diff -r, through GNU diff --no-dereference",
		Level: Optional, Area: "exec", Packages: pkgs("diffutils"), Probe: []string{"--no-dereference", "/dev/null", "/dev/null"}},
	{Name: "bwrap", Purpose: "sandboxes llm_exec when GASH_LLM_SANDBOX=1", Level: Optional, Area: "exec", OS: []string{"linux"},
		Packages: pkgs("bubblewrap", "brew", ""), Code: "sandbox_unavailable"},

	// System.
	{Name: "sudo", Purpose: "runs the privileged steps of some functions",
		Level: Feature, Area: "system", Packages: pkgs("sudo", "brew", ""), Builtin: []string{"darwin"}},
	{Name: "systemctl", Purpose: "reads and controls systemd services",
		Level: Feature, Area: "system", OS: []string{"linux"}, Code: "systemd_unavailable", NoInstall: true,
		Note: "systemd is part of the operating system: Gash cannot install it. Under WSL, enable it in /etc/wsl.conf"},
	{Name: "journalctl", Purpose: "adds the service journal to llm_service --log", Level: Optional, Area: "system",
		OS: []string{"linux"}, NoInstall: true, Note: "part of systemd"},
	{Name: "fzf", Purpose: "picks an entry from a list", Level: Optional, Area: "system",
		Packages: pkgs("fzf"), EPEL: true},
	{Name: "bc", Purpose: "is the calculator behind the calc alias", Level: Optional, Area: "system",
		Packages: pkgs("bc", "brew", ""), Builtin: []string{"darwin"}},

	// Local web stack.
	{Name: "apache", Commands: []string{"apache2", "httpd"}, Purpose: "serves the gash hte sites",
		Level: Feature, Area: "hte", Code: "apache_not_found",
		Packages: pkgs("apache2", "dnf", "httpd", "pacman", "apache", "brew", "httpd")},
	{Name: "php-fpm", Purpose: "runs PHP for the gash hte sites, one pool per PHP version", Level: Feature, Area: "hte",
		Packages: pkgs("php-fpm", "zypper", "php8-fpm", "apk", "php83-fpm", "brew", "php")},
	{Name: "openssl", Purpose: "makes the gash hte local certificate authority and site certificates",
		Level: Feature, Area: "hte", Code: "openssl_not_found", Packages: pkgs("openssl", "brew", "openssl@3")},

	// Windows, under WSL: these come with Windows.
	{Name: "powershell.exe", Purpose: "answers the gash win queries and edits the Windows hosts file",
		Level: Feature, Area: "windows", OS: []string{"wsl"}, Code: "windows_not_found", NoInstall: true,
		Note: "It comes with Windows: check that WSL interop is enabled ([interop] enabled=true in /etc/wsl.conf)"},
	{Name: "cmd.exe", Purpose: "restarts and shuts down WSL from inside it",
		Level: Feature, Area: "windows", OS: []string{"wsl"}, NoInstall: true,
		Note: "It comes with Windows: check that WSL interop is enabled ([interop] enabled=true in /etc/wsl.conf)"},

	// Release signatures: only with GASH_REQUIRE_SIGNED=1.
	{Name: "cosign", Purpose: "checks the Sigstore signature of a release",
		Level: Optional, Area: "release", Manual: cosignURL,
		Packages: map[Manager]string{Brew: "cosign", Pacman: "cosign", Apk: "cosign"}},
	{Name: "gpg", Purpose: "checks signed release tags", Level: Optional, Area: "release",
		Packages: pkgs("gnupg", "dnf", "gnupg2", "zypper", "gpg2")},
}
