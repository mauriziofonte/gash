// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

// catalogRuntime are the 3.4 agent-toolkit functions of one area, runtimes,
// lint and tests: llm_tools, llm_runtime, llm_lint, agent_test (V1, V2, L1,
// L2; a test suite runs the project's code, so agent_test is a write
// tool, D1). Entries follow the catalog_gen.go format; mergeCatalog sorts them in.
// The commands of the area (gash runtime, gash run, gash agent-functions)
// are not registry functions: internal/runcmd.
var catalogRuntime = []Function{
	{
		Name:     "llm_lint",
		Module:   "llm",
		Topic:    "llm",
		File:     "lib/modules/llm.sh",
		StubFile: "lib/modules/llm.sh",
		Class:    Class("pure output"),
		Decision: "new",
		Impl:     Impl("binary"),
		Headless: true,
		HasHelp:  true,
		Help: Help{
			Short:   "Syntax-check files by extension, in parallel (php, sh, js, json, yaml, toml, py)",
			Module:  "llm",
			SeeAlso: "agent_test llm_runtime",
			Body: "" +
				"USAGE\n" +
				"  llm_lint [PATH...] [--ext php,sh,js,json,yaml,toml,py] [--php VER] [--node MAJOR] [-j N] [--timeout S] [--max-lines N] [--width N] [--absolute]\n" +
				"           [--host ALIAS]\n" +
				"\n" +
				"OPTIONS\n" +
				"  --ext LIST             Languages to check in directories (default: all):\n" +
				"                         php, sh (also .bash), js (.mjs, .cjs), json, yaml\n" +
				"                         (.yml), toml, py\n" +
				"  --php VER              The php that checks .php files: 8.4, 8 (gash\n" +
				"                         runtime's resolution; default: php on PATH)\n" +
				"  --node MAJOR           The node that checks .js files: 24 (default: node on\n" +
				"                         PATH)\n" +
				"  -j, --jobs N           Checks at a time (default: the CPU count)\n" +
				"  --timeout S            Time limit of one checker process (default 60s)\n" +
				"  --max-lines N          Failures listed (default 100); --head N and --tail N\n" +
				"                         keep the first and last N, \"truncated\" counts the rest\n" +
				"  --width N              Cut file names and messages at N columns\n" +
				"  --relative, --absolute File paths relative to their PATH (default) or\n" +
				"                         absolute\n" +
				"  --host ALIAS           Run on a registered HOST: alias over ssh, one round\n" +
				"                         trip (repeatable; see AGENTS.md 3.12)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  # Every PHP file of src with php 8.4\n" +
				"  llm_lint src --ext php --php 8.4\n" +
				"\n" +
				"  # A whole project, every language\n" +
				"  llm_lint .\n" +
				"\n" +
				"  # Two files\n" +
				"  llm_lint deploy.sh config/app.yaml\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces for f in *.php; do php -l \"$f\"; done, bash -n and node --check\n" +
				"  loops. Output: {\"checked\":N,\"failed\":K,\"failures\":[{\"file\",\"line\",\"error\"}]}.\n" +
				"  php 8.3+ runs once per chunk of files (php -l a b c); an older php -l\n" +
				"  checks only its first file, so there each file gets its own run. bash -n\n" +
				"  once per file, node once per chunk (each file compiled as node loads it; a\n" +
				"  failure is re-checked with node --check for its message), python compiles\n" +
				"  in one process (no .pyc written). A file passes only on its own verdict: one\n" +
				"  a batch said nothing about is checked again alone. JSON, YAML and TOML are\n" +
				"  parsed by gash (tsconfig.json and .vscode\n" +
				"  files may hold comments). Directories skip node_modules, vendor, .git and\n" +
				"  __pycache__ and never follow symlinks; secret files are skipped there and\n" +
				"  refused as arguments. A FILE argument with another extension is\n" +
				"  unsupported_file. A missing checker is runtime_not_installed (STOP).",
		},
		ShellVars: []string{"HOME", "NVM_DIR"},
		Env:       []string{"HOME", "NVM_DIR", "PATH", "PWD", "XDG_CACHE_HOME"},
		Replaces:  "for f; do php -l $f; done; bash -n x.sh; node --check f.js",
	},
	{
		Name:     "llm_runtime",
		Module:   "llm",
		Topic:    "llm",
		File:     "lib/modules/llm.sh",
		StubFile: "lib/modules/llm.sh",
		Class:    Class("pure output"),
		Decision: "new",
		Impl:     Impl("binary"),
		Headless: true,
		HasHelp:  true,
		Help: Help{
			Short:   "Resolve a node major or php version to its binary, without nvm.sh",
			Module:  "llm",
			SeeAlso: "llm_tools node_detect_version php_detect_version",
			Body: "" +
				"USAGE\n" +
				"  llm_runtime php|node|composer|npm|npx VERSION\n" +
				"\n" +
				"EXAMPLES\n" +
				"  # The node 24 binary\n" +
				"  llm_runtime node 24\n" +
				"\n" +
				"  # composer under php 8.4\n" +
				"  llm_runtime composer 8.4\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces source ~/.nvm/nvm.sh; nvm use 24; which node (and php84 wrappers).\n" +
				"  Output: {\"runtime\":\"node\",\"request\":\"24\",\"path\":\".../bin/node\",\"version\":\"24.1.0\"};\n" +
				"  composer adds \"php\" (the binary it runs under), npm and npx report their\n" +
				"  node's version. VERSION is a prefix: 24, 24.1, 8.4, 8: the newest installed\n" +
				"  version that starts with it. node comes from the nvm tree\n" +
				"  (${NVM_DIR:-~/.nvm}/versions/node), then the node on PATH; php from\n" +
				"  /usr/bin/phpX.Y and /usr/local/bin/phpX.Y, then the php on PATH. Not\n" +
				"  installed: runtime_not_installed (STOP) with the installed versions. The\n" +
				"  JSON form of gash runtime; it reads gash's runtime cache, never writes it.",
		},
		ShellVars: []string{"HOME", "NVM_DIR"},
		Env:       []string{"HOME", "NVM_DIR", "PATH", "XDG_CACHE_HOME"},
		Replaces:  "source ~/.nvm/nvm.sh; nvm use 24; node ...; php84 ...",
	},
	{
		Name:     "agent_test",
		Module:   "agent",
		Topic:    "agent",
		File:     "lib/modules/llm.sh",
		StubFile: "lib/modules/llm.sh",
		Class:    Class("shell-mutating"),
		Decision: "new",
		Impl:     Impl("binary"),
		Headless: true,
		HasHelp:  true,
		Help: Help{
			Short:   "Run a test suite and summarize it: counts and each failure's file:line",
			Module:  "agent",
			SeeAlso: "llm_lint",
			Body: "" +
				"USAGE\n" +
				"  agent_test [--runner auto|NAME] [--fail-only] [--dry-run] [--max-lines N] [--width N] [--timeout S] [--absolute] -- CMD...\n" +
				"\n" +
				"OPTIONS\n" +
				"  --runner auto|NAME     The output format: pest, phpunit, phpstan, psalm,\n" +
				"                         vitest, jest, mocha, pytest, go, cargo, playwright\n" +
				"                         (default auto: from CMD and the output)\n" +
				"  --fail-only            Print nothing when CMD exits 0 with no failure\n" +
				"  --dry-run              Print the command, its format and the directory;\n" +
				"                         run nothing\n" +
				"  --max-lines N          Failures listed (default 50; --head N, --tail N);\n" +
				"                         unparsed output: the last N lines (default 20)\n" +
				"  --width N              Cut messages at N columns (default 200)\n" +
				"  --timeout S            End CMD after S (default: no limit)\n" +
				"  --relative, --absolute Paths below the working directory relative (default)\n" +
				"                         or as the runner printed them\n" +
				"\n" +
				"EXAMPLES\n" +
				"  # Pest with a filter\n" +
				"  agent_test -- vendor/bin/pest --filter UserTest\n" +
				"\n" +
				"  # A package script; silent when green\n" +
				"  agent_test --fail-only -- npm test\n" +
				"\n" +
				"  # Go with the JSON reporter\n" +
				"  agent_test -- go test -json ./...\n" +
				"\n" +
				"NOTES\n" +
				"  A write tool: the suite runs the project's own code, which can change\n" +
				"  files, caches or a test database, so it is an agent_* function and not\n" +
				"  an llm_* read. Replaces pest | grep -E 'Tests:|FAIL' and npm test |\n" +
				"  tail -20. CMD is a test suite: a runner (pest, phpunit, paratest,\n" +
				"  phpstan, psalm, vitest, jest, mocha, playwright, pytest; a path such as\n" +
				"  vendor/bin/pest, also through php, node, npx, pnpm exec or python -m),\n" +
				"  php artisan test, go test, cargo test, the test script of npm, pnpm,\n" +
				"  yarn, bun or composer, make test; any other command is\n" +
				"  not_a_test_command (gash run runs it). CMD runs as given, stdout and\n" +
				"  stderr merged, stdin /dev/null; the exit status is CMD's. Output:\n" +
				"  {\"runner\",\"passed\",\"failed\",\"skipped\",\"duration_s\",\"failures\":[{\"name\",\n" +
				"  \"file\",\"line\",\"message\"}],\"parsed\":true,\"exit_code\"}; analyzer findings\n" +
				"  (phpstan, psalm) are failures. Output no parser reads:\n" +
				"  {\"runner\":null,\"duration_s\",\"parsed\":false,\"exit_code\",\"tail\":[...]}.\n" +
				"  --dry-run: {\"argv\":[...],\"runner\",\"cwd\",\"dry_run\":true,\"written\":false}\n" +
				"  (runner null when only the output would tell). Options come before --;\n" +
				"  a program not on PATH is command_not_found. The suite runs without\n" +
				"  llm_exec's rails.",
		},
		ShellVars: []string{"HOME"},
		Env:       []string{"HOME", "PATH", "PWD"},
		Replaces:  "vendor/bin/pest | grep -E 'Tests:|FAIL'; npm run test | tail -20",
	},
	{
		Name:     "llm_tools",
		Module:   "llm",
		Topic:    "llm",
		File:     "lib/modules/llm.sh",
		StubFile: "lib/modules/llm.sh",
		Class:    Class("pure output"),
		Decision: "new",
		Impl:     Impl("binary"),
		Headless: true,
		HasHelp:  true,
		Help: Help{
			Short:   "Path and version of each tool, probed in parallel; php and node per version",
			Module:  "llm",
			SeeAlso: "llm_runtime llm_env",
			Body: "" +
				"USAGE\n" +
				"  llm_tools [NAME...] [-j N] [--timeout S] [--host ALIAS]\n" +
				"\n" +
				"OPTIONS\n" +
				"  -j, --jobs N           Probes at a time (default 8)\n" +
				"  --timeout S            Time limit of one probe (default 10s)\n" +
				"  --host ALIAS           Run on a registered HOST: alias over ssh, one round\n" +
				"                         trip (repeatable; see AGENTS.md 3.12)\n" +
				"\n" +
				"EXAMPLES\n" +
				"  # The usual toolchain\n" +
				"  llm_tools\n" +
				"\n" +
				"  # A few tools\n" +
				"  llm_tools git python3 rg php\n" +
				"\n" +
				"NOTES\n" +
				"  Replaces git --version; php -v | head -1; command -v rg. Output:\n" +
				"  {\"git\":{\"version\":\"2.43.0\",\"path\":\"/usr/bin/git\"},\"rg\":null,\n" +
				"  \"php\":[{\"version\":\"8.3.6\",\"path\":\"/usr/bin/php8.3\"}]}, keys in the order\n" +
				"  asked. php and node list every installed version (/usr/bin/phpX.Y, the nvm\n" +
				"  tree, the one on PATH), oldest first; a tool not on PATH is null, one whose\n" +
				"  probe prints no version has \"version\":null. Each NAME is a plain program\n" +
				"  name, probed with --version (go version, ssh -V, java -version; npm and npx\n" +
				"  read npm's package.json). Without NAME: git php composer node npm python3 go\n" +
				"  rg jq docker.",
		},
		ShellVars: []string{"HOME", "NVM_DIR"},
		Env:       []string{"HOME", "NVM_DIR", "PATH", "XDG_CACHE_HOME"},
		Replaces:  "php84 -v | head -1; node24 --version; git --version; command -v rg",
	},
}
