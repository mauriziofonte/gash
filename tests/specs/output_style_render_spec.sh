#!/usr/bin/env bash
#
# Output style, rendering (R1-R7). Every public human-facing function runs
# under a real pseudo-terminal (script(1)), loaded as an interactive user
# has it, and the bytes it prints are checked against the style contract:
#
#   R1  every key of __GASH_HELP_REGISTRY not starting with llm_, plus
#       gash_username, has at least one fixture
#   R2  fixtures run on a PTY with tests/mocks/bin first on PATH,
#       COLUMNS=100, LANG=C.UTF-8 (no network, no sudo, no real Docker)
#   R3  every SGR sequence is a palette value, an attribute, the reset, or
#       one that output.sh lists in __gash_sgr_extra
#   R4  the only other CSI sequences are erase-line, cursor hide/show and
#       the clear-screen primitive's
#   R5  status lines on the terminal start with a glyph, never with a word
#       prefix or an ASCII marker
#   R6  with stdout piped, and with NO_COLOR=1: no ESC byte at all, and
#       status lines carry their word prefix
#   R7  headless loading prints nothing
#
# Fixture format: fx FUNCTION LABEL CODE [INPUT]
#   CODE runs in bash after Gash is loaded, in a fresh directory that is
#   also HOME. INPUT is typed on the terminal (e.g. "n" answers a
#   confirmation); end of input follows, so a further read gets EOF.

describe "Output style: rendering on a terminal"

__fx_ids=()
declare -A __fx_func=() __fx_code=() __fx_input=()
fx() {
  local id="$1:$2"
  __fx_ids+=("$id")
  __fx_func[$id]="$1"
  __fx_code[$id]="$3"
  __fx_input[$id]="${4-}"
}

# -----------------------------------------------------------------------------
# Fixtures
# -----------------------------------------------------------------------------

# gash module (lib/modules/gash.sh), core (utils, config), migrate.

fx gash overview 'GASH_DIR="$GASH_TEST_ROOT" gash'
fx gash topic 'GASH_DIR="$GASH_TEST_ROOT" gash git'
fx gash all 'GASH_DIR="$GASH_TEST_ROOT" gash all'
fx gash unknown-topic 'gash nosuch'
fx gash_doctor ok 'GASH_DIR="$GASH_TEST_ROOT" gash_doctor'
fx gash_doctor problems 'printf "GASH_DB_TIMEOUT=5\n" >.gash_env; chmod 644 .gash_env; export GASH_DB_TIMEOUT=5; __GASH_LOAD_ERRORS=("lib/modules/broken.sh: exit 2"); GASH_DIR="$HOME/nowhere" gash_doctor'
fx gash_doctor timing 'GASH_DIR="$GASH_TEST_ROOT" gash_doctor --timing'
fx gash_help overview 'gash_help'
fx gash_help list 'gash_help --list'
fx gash_help search 'gash_help --search docker'
fx gash_help search-none 'gash_help --search zzzqqq'
fx gash_help search-usage 'gash_help --search'
fx gash_help function 'gash_help files_largest'
fx gash_help alias 'gash_help flf'
fx gash_help self 'gash_help -h'
fx gash_help short 'gash_help --short git_sync; gash_help --short nosuch'
fx gash_help not-found 'gash_help nosuch'
fx gash_inspiring_quote plain 'GASH_DIR="$GASH_TEST_ROOT" gash_inspiring_quote'
fx gash_inspiring_quote source 'GASH_DIR="$GASH_TEST_ROOT" gash_inspiring_quote --source'
fx gash_inspiring_quote bad-option 'gash_inspiring_quote --nope'
fx gash_uninstall not-installed 'GASH_DIR="$HOME/nowhere" gash_uninstall'
fx gash_uninstall not-gash 'mkdir -p "$HOME/x"; GASH_DIR="$HOME/x" gash_uninstall'
fx gash_uninstall cancel 'mkdir -p "$HOME/g/lib/core"; : >"$HOME/g/gash.sh"; GASH_DIR="$HOME/g" gash_uninstall' 'n'
fx gash_uninstall confirm 'mkdir -p "$HOME/g/lib/core"; : >"$HOME/g/gash.sh"; printf "# >>> GASH START >>>\nsource ~/.gashrc\n# <<< GASH END <<<\n" >.bashrc; : >.gashrc; : >.gash_env; GASH_DIR="$HOME/g" gash_uninstall' 'y
n'
fx gash_unload no-snapshot 'gash_unload'
fx gash_upgrade not-a-checkout 'GASH_DIR="$HOME/nowhere" gash_upgrade'
fx gash_upgrade upgrade 'export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t; git init -q o; git -C o commit -q --allow-empty -m one; git -C o tag v1.0; git clone -q o g; git -C g -c advice.detachedHead=false checkout -q v1.0; git -C o commit -q --allow-empty -m two; git -C o tag v1.1; GASH_DIR="$HOME/g" gash_upgrade; GASH_DIR="$HOME/g" gash_upgrade'
fx gash_upgrade dirty 'export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t; git init -q g; git -C g commit -q --allow-empty -m one; : >g/new; GASH_DIR="$HOME/g" gash_upgrade'
fx gash_username plain 'gash_username'
fx all_colors chart 'all_colors | head -n 20'
fx path_add plain 'mkdir -p bin; path_add "$HOME/bin" /nonexistent; printf "%s\n" "${PATH%%:*}"'
fx gash_env_init create 'GASH_DIR="$GASH_TEST_ROOT" gash_env_init; GASH_DIR="$GASH_TEST_ROOT" gash_env_init'
fx gash_env_init no-template 'GASH_DIR="$HOME/nowhere" gash_env_init'
fx gash_db_list empty 'gash_db_list'
fx gash_db_list list 'printf "DB:default=mysql://u:secret@db:3306/app\nDB:pg=pgsql://p:x@h:5432/d\n" >.gash_env; chmod 600 .gash_env; gash_db_list'
fx gash_db_test missing 'gash_db_test nosuch'
fx gash_db_test unknown-driver 'printf "DB:default=oracle://u:p@h:1/d\n" >.gash_env; chmod 600 .gash_env; gash_db_test'
fx gash_ai_list empty 'gash_ai_list'
fx gash_ai_list list 'printf "AI:claude=sk-ant-api03-abcdefghijkl\n" >.gash_env; chmod 600 .gash_env; gash_ai_list'
fx gash_migrate status 'gash_migrate --status'
fx gash_migrate menu 'ln -s "$GASH_TEST_ROOT" .gash; printf "export GASH_BASH=\"\$HOME/.gash/gash.sh\"\n" >.gashrc; printf "AI:claude=sk-test\n" >.gash_env; chmod 644 .gash_env; gash_migrate' '2'
fx gash_migrate plain-yes 'ln -s "$GASH_TEST_ROOT" .gash; printf "export GASH_BASH=\"\$HOME/.gash/gash.sh\"\nexport MY_OWN=1\n" >.gashrc; printf "AI:claude=sk-test\nFOO BAR\n" >.gash_env; chmod 644 .gash_env; GASH_TUI=plain gash_migrate --yes; gash_migrate --status; gash_migrate --rollback --yes'
fx gash_migrate rollback-none 'gash_migrate --rollback --yes'

# files.sh: filesystem scanners, archive_extract, file_backup
fx files_largest list 'mkdir -p d/a/b d/c && head -c 70000 /dev/zero >d/a/b/logo.png && head -c 524288 /dev/zero >d/c/app.log && printf x >d/notes.txt && files_largest d'
fx files_largest no-human 'mkdir -p d/a && head -c 3000 /dev/zero >d/a/x.bin && printf xy >d/y.txt && files_largest --no-human d'
fx files_largest no-color 'mkdir -p d && head -c 2048 /dev/zero >d/x.bin && files_largest --no-color d'
fx files_largest long-path 'p=d/$(printf "very-long-directory-name-%s/" 1 2 3 4 5) && mkdir -p "$p" && head -c 4096 /dev/zero >"$p/file.bin" && files_largest d'
fx files_largest empty 'mkdir -p d && printf x >d/x && files_largest --min-size 1G d'
fx files_largest unreadable 'mkdir -p d && printf x >d/x && find() { command find "$@"; printf "find: d/locked: Permission denied\n" >&2; } && files_largest d'
fx files_largest forbidden 'files_largest /proc'
fx files_largest not-found 'files_largest ./missing'
fx files_largest unknown-option 'files_largest --bogus'
fx files_largest json-error 'files_largest --json --limit x .'
fx dirs_largest list 'mkdir -p d/big d/small d/empty && head -c 65536 /dev/zero >d/big/f && printf x >d/small/f && dirs_largest d'
fx dirs_largest depth 'mkdir -p d/a/b && head -c 8192 /dev/zero >d/a/b/f && dirs_largest --depth 2 --no-human d'
fx dirs_largest empty 'mkdir -p d/a && dirs_largest --min-size 1G d'
fx dirs_largest root-blocked 'dirs_largest /'
fx dirs_find_large list 'mkdir -p d/big/inner d/small && head -c 65536 /dev/zero >d/big/inner/f && printf x >d/small/f && dirs_find_large --size 16K d'
fx dirs_find_large with-mtime 'mkdir -p d/big d/none && head -c 65536 /dev/zero >d/big/f && dirs_find_large --size 1 --with-mtime d'
fx dirs_find_large empty 'mkdir -p d/a && dirs_find_large d'
fx dirs_find_large invalid-size 'dirs_find_large --size huge .'
fx dirs_list_empty list 'mkdir -p d/e1 d/e2 d/full && printf x >d/full/f && dirs_list_empty d'
fx dirs_list_empty count 'mkdir -p d/e1 d/e2 && dirs_list_empty --count d'
fx dirs_list_empty none 'mkdir -p d && printf x >d/f && dirs_list_empty d'
fx dirs_list_empty not-a-directory 'printf x >f && dirs_list_empty f'
fx tree_stats stats 'mkdir -p d/src/deep d/empty && head -c 300000 /dev/zero >d/a.bin && printf x >d/src/b.txt && printf y >d/src/deep/c.txt && printf z >d/README && tree_stats d'
fx tree_stats top-1 'mkdir -p d && printf x >d/a.js && printf xy >d/b.verylongextension && tree_stats --top 1 d'
fx tree_stats empty-dir 'mkdir -p d && tree_stats d'
fx tree_stats no-color 'mkdir -p d && printf x >d/a.txt && tree_stats --no-color d'
fx tree_stats invalid-top 'tree_stats --top x .'
fx archive_extract tar 'mkdir -p src && printf x >src/a.txt && tar czf a.tar.gz src && archive_extract a.tar.gz out'
fx archive_extract missing-arg 'archive_extract'
fx archive_extract missing-file 'archive_extract nope.zip'
fx archive_extract unsupported 'printf x >file.abc && archive_extract file.abc'
fx archive_extract no-zstd 'printf x >a.tar.zst && mkdir e && PATH="$PWD/e" archive_extract a.tar.zst'
fx file_backup copy 'printf x >notes.txt && file_backup notes.txt'
fx file_backup missing-arg 'file_backup'
fx file_backup missing-file 'file_backup nope.txt'

# chrome.sh: chrome_headless_update (curl, uname, unzip and ldd as functions)
fx chrome_headless_update dry-run-new 'curl() { local u="" h=0 o=""; while (($#)); do case $1 in -fsIL) h=1 ;; -o) o=$2; shift ;; http*) u=$1 ;; esac; shift; done; if ((h)); then printf "HTTP/1.1 200 OK\r\nContent-Length: 2048\r\n\r\n"; return 0; fi; if [[ -n $o ]]; then head -c 2048 /dev/zero >"$o"; return 0; fi; case $u in *npmjs*) echo "{\"version\":\"1.56.0\"}" ;; *browsers.json) echo "{\"browsers\":[{\"name\":\"chromium-headless-shell\",\"revision\":\"1194\",\"browserVersion\":\"141.0.7390.37\"}]}" ;; *141.0.7390.37.json) echo "{\"downloads\":{\"chrome-headless-shell\":[{\"platform\":\"linux64\",\"url\":\"https://storage.example/chs.zip\"}]}}" ;; *) return 22 ;; esac; }; uname() { case $1 in -m) echo x86_64 ;; *) echo Linux ;; esac; }; unzip() { case $1 in -Z1) echo chrome-headless-shell-linux64/chrome-headless-shell ;; -q) mkdir -p "$4/chrome-headless-shell-linux64" && printf "#!/bin/sh\necho Chrome-Headless-Shell 141.0.7390.37\n" >"$4/chrome-headless-shell-linux64/chrome-headless-shell" && chmod +x "$4/chrome-headless-shell-linux64/chrome-headless-shell" ;; esac; }; ldd() { :; }; export GASH_CHROME_ROOT="$HOME/opt" GASH_CHROME_LINK="$HOME/bin/chs"; mkdir -p "$HOME/bin"; chrome_headless_update --dry-run'
fx chrome_headless_update upgrade-declined 'curl() { local u="" h=0 o=""; while (($#)); do case $1 in -fsIL) h=1 ;; -o) o=$2; shift ;; http*) u=$1 ;; esac; shift; done; if ((h)); then printf "HTTP/1.1 200 OK\r\nContent-Length: 2048\r\n\r\n"; return 0; fi; if [[ -n $o ]]; then head -c 2048 /dev/zero >"$o"; return 0; fi; case $u in *npmjs*) echo "{\"version\":\"1.56.0\"}" ;; *browsers.json) echo "{\"browsers\":[{\"name\":\"chromium-headless-shell\",\"revision\":\"1194\",\"browserVersion\":\"141.0.7390.37\"}]}" ;; *141.0.7390.37.json) echo "{\"downloads\":{\"chrome-headless-shell\":[{\"platform\":\"linux64\",\"url\":\"https://storage.example/chs.zip\"}]}}" ;; *) return 22 ;; esac; }; uname() { case $1 in -m) echo x86_64 ;; *) echo Linux ;; esac; }; unzip() { case $1 in -Z1) echo chrome-headless-shell-linux64/chrome-headless-shell ;; -q) mkdir -p "$4/chrome-headless-shell-linux64" && printf "#!/bin/sh\necho Chrome-Headless-Shell 141.0.7390.37\n" >"$4/chrome-headless-shell-linux64/chrome-headless-shell" && chmod +x "$4/chrome-headless-shell-linux64/chrome-headless-shell" ;; esac; }; ldd() { :; }; export GASH_CHROME_ROOT="$HOME/opt" GASH_CHROME_LINK="$HOME/bin/chs"; mkdir -p "$HOME/bin"; old() { mkdir -p "$GASH_CHROME_ROOT"/{139.0.7258.5,140.0.7339.16}/chrome-headless-shell-linux64 && printf "#!/bin/sh\necho Chrome-Headless-Shell 140.0.7339.16\n" >"$GASH_CHROME_ROOT/140.0.7339.16/chrome-headless-shell-linux64/chrome-headless-shell" && chmod +x "$GASH_CHROME_ROOT/140.0.7339.16/chrome-headless-shell-linux64/chrome-headless-shell" && ln -s "$GASH_CHROME_ROOT/140.0.7339.16/chrome-headless-shell-linux64/chrome-headless-shell" "$GASH_CHROME_LINK"; }; chrome_headless_update' 'n'
fx chrome_headless_update install 'curl() { local u="" h=0 o=""; while (($#)); do case $1 in -fsIL) h=1 ;; -o) o=$2; shift ;; http*) u=$1 ;; esac; shift; done; if ((h)); then printf "HTTP/1.1 200 OK\r\nContent-Length: 2048\r\n\r\n"; return 0; fi; if [[ -n $o ]]; then head -c 2048 /dev/zero >"$o"; return 0; fi; case $u in *npmjs*) echo "{\"version\":\"1.56.0\"}" ;; *browsers.json) echo "{\"browsers\":[{\"name\":\"chromium-headless-shell\",\"revision\":\"1194\",\"browserVersion\":\"141.0.7390.37\"}]}" ;; *141.0.7390.37.json) echo "{\"downloads\":{\"chrome-headless-shell\":[{\"platform\":\"linux64\",\"url\":\"https://storage.example/chs.zip\"}]}}" ;; *) return 22 ;; esac; }; uname() { case $1 in -m) echo x86_64 ;; *) echo Linux ;; esac; }; unzip() { case $1 in -Z1) echo chrome-headless-shell-linux64/chrome-headless-shell ;; -q) mkdir -p "$4/chrome-headless-shell-linux64" && printf "#!/bin/sh\necho Chrome-Headless-Shell 141.0.7390.37\n" >"$4/chrome-headless-shell-linux64/chrome-headless-shell" && chmod +x "$4/chrome-headless-shell-linux64/chrome-headless-shell" ;; esac; }; ldd() { :; }; export GASH_CHROME_ROOT="$HOME/opt" GASH_CHROME_LINK="$HOME/bin/chs"; mkdir -p "$HOME/bin"; old() { mkdir -p "$GASH_CHROME_ROOT"/{139.0.7258.5,140.0.7339.16}/chrome-headless-shell-linux64 && printf "#!/bin/sh\necho Chrome-Headless-Shell 140.0.7339.16\n" >"$GASH_CHROME_ROOT/140.0.7339.16/chrome-headless-shell-linux64/chrome-headless-shell" && chmod +x "$GASH_CHROME_ROOT/140.0.7339.16/chrome-headless-shell-linux64/chrome-headless-shell" && ln -s "$GASH_CHROME_ROOT/140.0.7339.16/chrome-headless-shell-linux64/chrome-headless-shell" "$GASH_CHROME_LINK"; }; chrome_headless_update' 'y'
fx chrome_headless_update up-to-date 'curl() { local u="" h=0 o=""; while (($#)); do case $1 in -fsIL) h=1 ;; -o) o=$2; shift ;; http*) u=$1 ;; esac; shift; done; if ((h)); then printf "HTTP/1.1 200 OK\r\nContent-Length: 2048\r\n\r\n"; return 0; fi; if [[ -n $o ]]; then head -c 2048 /dev/zero >"$o"; return 0; fi; case $u in *npmjs*) echo "{\"version\":\"1.56.0\"}" ;; *browsers.json) echo "{\"browsers\":[{\"name\":\"chromium-headless-shell\",\"revision\":\"1194\",\"browserVersion\":\"141.0.7390.37\"}]}" ;; *141.0.7390.37.json) echo "{\"downloads\":{\"chrome-headless-shell\":[{\"platform\":\"linux64\",\"url\":\"https://storage.example/chs.zip\"}]}}" ;; *) return 22 ;; esac; }; uname() { case $1 in -m) echo x86_64 ;; *) echo Linux ;; esac; }; unzip() { case $1 in -Z1) echo chrome-headless-shell-linux64/chrome-headless-shell ;; -q) mkdir -p "$4/chrome-headless-shell-linux64" && printf "#!/bin/sh\necho Chrome-Headless-Shell 141.0.7390.37\n" >"$4/chrome-headless-shell-linux64/chrome-headless-shell" && chmod +x "$4/chrome-headless-shell-linux64/chrome-headless-shell" ;; esac; }; ldd() { :; }; export GASH_CHROME_ROOT="$HOME/opt" GASH_CHROME_LINK="$HOME/bin/chs"; mkdir -p "$HOME/bin"; mkdir -p "$GASH_CHROME_ROOT/141.0.7390.37/chrome-headless-shell-linux64" && ln -s "$GASH_CHROME_ROOT/141.0.7390.37/chrome-headless-shell-linux64/chrome-headless-shell" "$GASH_CHROME_LINK"; chrome_headless_update'
fx chrome_headless_update missing-libs 'curl() { local u="" h=0 o=""; while (($#)); do case $1 in -fsIL) h=1 ;; -o) o=$2; shift ;; http*) u=$1 ;; esac; shift; done; if ((h)); then printf "HTTP/1.1 200 OK\r\nContent-Length: 2048\r\n\r\n"; return 0; fi; if [[ -n $o ]]; then head -c 2048 /dev/zero >"$o"; return 0; fi; case $u in *npmjs*) echo "{\"version\":\"1.56.0\"}" ;; *browsers.json) echo "{\"browsers\":[{\"name\":\"chromium-headless-shell\",\"revision\":\"1194\",\"browserVersion\":\"141.0.7390.37\"}]}" ;; *141.0.7390.37.json) echo "{\"downloads\":{\"chrome-headless-shell\":[{\"platform\":\"linux64\",\"url\":\"https://storage.example/chs.zip\"}]}}" ;; *) return 22 ;; esac; }; uname() { case $1 in -m) echo x86_64 ;; *) echo Linux ;; esac; }; unzip() { case $1 in -Z1) echo chrome-headless-shell-linux64/chrome-headless-shell ;; -q) mkdir -p "$4/chrome-headless-shell-linux64" && printf "#!/bin/sh\necho Chrome-Headless-Shell 141.0.7390.37\n" >"$4/chrome-headless-shell-linux64/chrome-headless-shell" && chmod +x "$4/chrome-headless-shell-linux64/chrome-headless-shell" ;; esac; }; ldd() { :; }; export GASH_CHROME_ROOT="$HOME/opt" GASH_CHROME_LINK="$HOME/bin/chs"; mkdir -p "$HOME/bin"; ldd() { printf "\tlibnss3.so => not found\n\tlibgbm.so.1 => not found\n"; }; chrome_headless_update -y'
fx chrome_headless_update no-network 'curl() { local u="" h=0 o=""; while (($#)); do case $1 in -fsIL) h=1 ;; -o) o=$2; shift ;; http*) u=$1 ;; esac; shift; done; if ((h)); then printf "HTTP/1.1 200 OK\r\nContent-Length: 2048\r\n\r\n"; return 0; fi; if [[ -n $o ]]; then head -c 2048 /dev/zero >"$o"; return 0; fi; case $u in *npmjs*) echo "{\"version\":\"1.56.0\"}" ;; *browsers.json) echo "{\"browsers\":[{\"name\":\"chromium-headless-shell\",\"revision\":\"1194\",\"browserVersion\":\"141.0.7390.37\"}]}" ;; *141.0.7390.37.json) echo "{\"downloads\":{\"chrome-headless-shell\":[{\"platform\":\"linux64\",\"url\":\"https://storage.example/chs.zip\"}]}}" ;; *) return 22 ;; esac; }; uname() { case $1 in -m) echo x86_64 ;; *) echo Linux ;; esac; }; unzip() { case $1 in -Z1) echo chrome-headless-shell-linux64/chrome-headless-shell ;; -q) mkdir -p "$4/chrome-headless-shell-linux64" && printf "#!/bin/sh\necho Chrome-Headless-Shell 141.0.7390.37\n" >"$4/chrome-headless-shell-linux64/chrome-headless-shell" && chmod +x "$4/chrome-headless-shell-linux64/chrome-headless-shell" ;; esac; }; ldd() { :; }; export GASH_CHROME_ROOT="$HOME/opt" GASH_CHROME_LINK="$HOME/bin/chs"; mkdir -p "$HOME/bin"; curl() { return 6; }; chrome_headless_update -y'
fx chrome_headless_update unknown-option 'chrome_headless_update --bogus'

# Fixtures of worker "system": lib/modules/system.sh, lib/modules/docker.sh,
# lib/modules/docker-compose.sh. External programs that could reach the
# network or change the machine (wget, sudo, cmd.exe, fzf, kill, lsof, ps)
# are shell-function stubs or tests/mocks/bin mocks.

# disk_usage (df mock)
fx disk_usage table 'disk_usage'
fx disk_usage no-color 'disk_usage --no-color'
fx disk_usage no-types 'MOCK_DF_NO_TYPE=1 disk_usage'
fx disk_usage virtual-only 'df() { printf "%s\n" "Filesystem Type 1024-blocks Used Available Capacity Mounted on" "tmpfs tmpfs 16 0 16 0% /run"; }; disk_usage'
fx disk_usage df-failed 'df() { return 1; }; disk_usage'
fx disk_usage bad-option 'disk_usage --bogus'

# history_grep (history builtin stubbed)
fx history_grep matches 'history() { printf "%5d  %s\n" 1 "echo hello" 2 "ls -la" 3 "echo world one two"; }; history_grep echo'
fx history_grep no-match 'history() { :; }; history_grep zzz'
fx history_grep missing-arg 'history_grep'

# hgrep (a HISTFILE with timestamps; fzf stubbed for --pick)
fx hgrep flat 'printf "#1700000000\ncd /tmp && ls\n#1700000100\ncd /tmp && git status\n#1700000200\ncd /tmp && ls\n#1700000300\ngit push origin main\n" >h; HISTFILE="$PWD/h" hgrep git'
fx hgrep group 'printf "#1700000000\ncd /tmp && ls\n#1700000100\ncd /tmp && git status\n#1700000200\ncd /tmp && ls\n#1700000300\ngit push origin main\n" >h; HISTFILE="$PWD/h" hgrep cd -g'
fx hgrep group-no-color 'printf "#1700000000\ncd /tmp && ls\n#1700000100\ncd /tmp && git status\n#1700000200\ncd /tmp && ls\n" >h; HISTFILE="$PWD/h" hgrep cd -g --no-color'
fx hgrep regex-reverse-limit 'printf "#1700000000\ngit push\n#1700000100\ngit pull\n#1700000200\ngit status\n" >h; HISTFILE="$PWD/h" hgrep -E "git (push|pull)" -r -n 1'
fx hgrep no-highlight 'printf "#1700000000\ngit push\n" >h; HISTFILE="$PWD/h" hgrep git -H'
fx hgrep count 'printf "#1700000000\ngit push\n#1700000100\ngit pull\n" >h; HISTFILE="$PWD/h" hgrep git -c; HISTFILE="$PWD/h" hgrep zzz -c'
fx hgrep json 'printf "#1700000000\ngit push\n" >h; HISTFILE="$PWD/h" hgrep git -j; HISTFILE="$PWD/h" hgrep zzz -j'
fx hgrep empty 'printf "#1700000000\ngit push\n" >h; HISTFILE="$PWD/h" hgrep zzz'
fx hgrep pick 'printf "#1700000000\ngit push\n#1700000100\ngit pull\n" >h; fzf() { head -n 1; }; HISTFILE="$PWD/h" hgrep git --pick'
fx hgrep missing-pattern 'hgrep'
fx hgrep bad-option 'hgrep git --bogus'
fx hgrep bad-limit 'hgrep git -n x'
fx hgrep no-histfile 'HISTFILE="$PWD/none" hgrep git'

# ip_public (wget stubbed: no network)
fx ip_public ip 'wget() { printf "203.0.113.7"; }; ip_public'
fx ip_public no-color 'wget() { printf "203.0.113.7"; }; ip_public --no-color'
fx ip_public failed 'wget() { return 4; }; ip_public'
fx ip_public no-tool 'PATH=/nonexistent ip_public'

# mkdir_cd
fx mkdir_cd create 'mkdir_cd a/b && pwd'
fx mkdir_cd missing-arg 'mkdir_cd'

# process_find (ps mock)
fx process_find matches 'process_find proc'
fx process_find no-match 'process_find zzz'
fx process_find missing-arg 'process_find'

# process_kill (pgrep mock: my-proc is 2222; ps and kill stubbed)
fx process_kill declined 'ps() { printf "%s\n" "my-proc --flag"; }; process_kill my-proc' 'n'
fx process_kill yes 'kill() { [[ "$1" != -0 ]]; }; process_kill -y my-proc'
fx process_kill survivor 'kill() { return 0; }; sleep() { :; }; process_kill -y my-proc'
fx process_kill no-terminal 'setsid -w bash --noprofile --norc -c "source \"$GASH_TEST_ROOT/tests/gash-test.sh\"; gash_source_all \"$GASH_TEST_ROOT\"; process_kill my-proc"'
fx process_kill no-match 'process_kill nothing-here'
fx process_kill bad-option 'process_kill --bogus x'
fx process_kill missing-arg 'process_kill'

# port_kill (lsof, ps and kill stubbed)
fx port_kill declined 'lsof() { if [[ "$*" == *LISTEN* ]]; then printf "%s\n" 4242 815; fi; }; ps() { printf "%s\n" node; }; port_kill 3000' 'n'
fx port_kill yes 'lsof() { if [[ "$*" == *LISTEN* ]]; then printf "%s\n" 4242; fi; }; ps() { printf "%s\n" node; }; kill() { [[ "$1" != -0 ]]; }; port_kill -y 3000'
fx port_kill nothing 'port_kill 8080'
fx port_kill invalid 'port_kill 70000'
fx port_kill no-lsof 'PATH=/nonexistent port_kill 3000'
fx port_kill missing-arg 'port_kill'

# services_stop (systemctl and sudo mocks)
fx services_stop dry-run '__GASH_SYSTEMD_RUNDIR="$PWD" MOCK_SYSTEMCTL_ACTIVE=mysql services_stop --dry-run'
fx services_stop declined '__GASH_SYSTEMD_RUNDIR="$PWD" MOCK_SYSTEMCTL_ACTIVE=mysql services_stop' 'n'
fx services_stop yes '__GASH_SYSTEMD_RUNDIR="$PWD" MOCK_SYSTEMCTL_ACTIVE=mysql services_stop -y'
fx services_stop none '__GASH_SYSTEMD_RUNDIR="$PWD" MOCK_SYSTEMCTL_ACTIVE= services_stop -y'
fx services_stop bad-option 'services_stop --bogus'

# sudo_last (sudo mock; fc stubbed)
fx sudo_last command 'sudo_last echo ok'
fx sudo_last declined 'fc() { printf "%s\n" "   apt update"; }; sudo_last' 'n'

# WSL (detection, cmd.exe, explorer.exe, wslpath, dmesg and sudo stubbed)
fx wsl_restart declined '__gash_require_wsl() { return 0; }; wsl_restart' 'n'
fx wsl_restart yes '__gash_require_wsl() { return 0; }; cmd.exe() { printf "%s\n" "cmd $*"; }; wsl_restart -y'
fx wsl_restart not-wsl '__gash_require_wsl() { __gash_error "This command requires WSL (Windows Subsystem for Linux)."; return 1; }; wsl_restart -y'
fx wsl_shutdown yes '__gash_require_wsl() { return 0; }; cmd.exe() { printf "%s\n" "cmd $*"; }; wsl_shutdown -y'
fx wsl_shutdown bad-option 'wsl_shutdown --now'
fx wsl_explorer open '__gash_require_wsl() { return 0; }; wslpath() { printf "%s\n" "WIN:$2"; }; explorer.exe() { printf "%s\n" "explorer $*"; return 1; }; wsl_explorer .'
fx wsl_explorer missing-path '__gash_require_wsl() { return 0; }; wsl_explorer ./none'
fx wsl_explorer not-wsl '__gash_require_wsl() { __gash_error "This command requires WSL (Windows Subsystem for Linux)."; return 1; }; wsl_explorer'
fx wsl_taskmanager open '__gash_require_wsl() { return 0; }; cmd.exe() { :; }; wsl_taskmanager; wait'
fx wsl_taskmanager not-wsl '__gash_require_wsl() { __gash_error "This command requires WSL (Windows Subsystem for Linux)."; return 1; }; wsl_taskmanager'
fx wsl_compact status '__gash_require_wsl() { return 0; }; dmesg() { [[ "${1-}" == -T ]] && printf "%s\n" "[Mon Oct 12 10:00:00 2026] page allocation failure: order:7"; [[ "${1-}" != -T ]] && printf "%s\n" "[1.0] page allocation failure: order:7"; return 0; }; __gash_wsl_mem_snapshot() { printf "%s\n" "812 MiB|120 MiB|DMA=1 DMA32=5 Normal=30"; }; wsl_compact --status'
fx wsl_compact compact '__gash_require_wsl() { return 0; }; dmesg() { :; }; sudo() { return 0; }; __gash_wsl_mem_snapshot() { printf "%s\n" "812 MiB|120 MiB|DMA=1 DMA32=5 Normal=30"; }; wsl_compact'
fx wsl_compact sudo-failed '__gash_require_wsl() { return 0; }; dmesg() { :; }; sudo() { return 1; }; __gash_wsl_mem_snapshot() { printf "%s\n" "812 MiB|120 MiB|DMA=1"; }; wsl_compact'
fx wsl_compact bad-option 'wsl_compact --now'

# docker.sh (docker mock)
fx docker_stop_all running 'MOCK_DOCKER_CONTAINERS=1 docker_stop_all'
fx docker_stop_all none 'docker_stop_all'
fx docker_stop_all no-docker 'PATH=/nonexistent docker_stop_all'
fx docker_start_all stopped 'MOCK_DOCKER_CONTAINERS=1 docker_start_all'
fx docker_start_all none 'docker_start_all'
fx docker_prune_all dry-run 'MOCK_DOCKER_CONTAINERS=1 MOCK_DOCKER_CUSTOM_NETWORKS=1 docker_prune_all --dry-run'
fx docker_prune_all declined 'MOCK_DOCKER_CONTAINERS=1 docker_prune_all' 'n'
fx docker_prune_all yes 'MOCK_DOCKER_CONTAINERS=1 MOCK_DOCKER_CUSTOM_NETWORKS=1 docker_prune_all -y'
fx docker_prune_all bad-option 'docker_prune_all --bogus'

# docker-compose.sh (docker and curl mocks: no registry is contacted)
fx docker_compose_check updates 'printf "services:\n  web:\n    image: nginx:latest\n  db:\n    image: postgres:15.2.1\n  app:\n    image: ghcr.io/some-org/a-really-long-application-image-name:main\n  pinned:\n    image: redis@sha256:abc\n" >docker-compose.yml; docker_compose_check'
fx docker_compose_check errors 'printf "services:\n  web:\n    image: nginx:latest\n  app:\n    image: ghcr.io/org/app:main\n" >docker-compose.yml; MOCK_CURL_REGISTRY_RATE_LIMITED=1 docker_compose_check'
fx docker_compose_check timeout 'printf "services:\n  web:\n    image: nginx:latest\n  app:\n    image: registry.example.com/app:main\n" >docker-compose.yml; MOCK_CURL_REGISTRY_TIMEOUT=1 docker_compose_check; MOCK_CURL_REGISTRY_NOT_FOUND=1 docker_compose_check; MOCK_CURL_REGISTRY_UNAUTHORIZED=1 docker_compose_check'
fx docker_compose_check no-color 'printf "services:\n  web:\n    image: nginx:latest\n" >docker-compose.yml; docker_compose_check --no-color'
fx docker_compose_check json 'printf "services:\n  web:\n    image: nginx:latest\n" >docker-compose.yml; docker_compose_check --json'
fx docker_compose_check no-file 'docker_compose_check'
fx docker_compose_check no-services 'printf "services:\n  web:\n    build: .\n" >compose.yml; docker_compose_check'
fx docker_compose_check bad-option 'docker_compose_check --bogus'
fx docker_compose_upgrade dry-run 'printf "services:\n  web:\n    image: nginx:latest\n  db:\n    image: postgres:15.2.1\n" >docker-compose.yml; docker_compose_upgrade --dry-run'
fx docker_compose_upgrade all-pinned 'printf "services:\n  db:\n    image: postgres:15.2.1\n" >docker-compose.yml; docker_compose_upgrade'
fx docker_compose_upgrade declined 'printf "services:\n  web:\n    image: nginx:latest\n" >docker-compose.yml; docker_compose_upgrade' 'n'
fx docker_compose_upgrade yes 'printf "services:\n  web:\n    image: nginx:latest\n" >docker-compose.yml; docker_compose_upgrade -y --no-color; docker_compose_upgrade -y'
fx docker_compose_upgrade no-file 'docker_compose_upgrade'
fx docker_compose_upgrade old-n 'docker_compose_upgrade -n'
fx docker_compose_scan found 'mkdir -p a b/c; printf "services:\n  web:\n    image: nginx:latest\n  db:\n    image: postgres:15\n" >a/compose.yml; printf "services:\n  x:\n    build: .\n" >b/c/docker-compose.yml; docker_compose_scan'
fx docker_compose_scan none 'docker_compose_scan'
fx docker_compose_scan json 'mkdir a; printf "services:\n  web:\n    image: nginx:latest\n" >a/compose.yml; docker_compose_scan --json --no-color'
fx llm_docker_check json 'printf "services:\n  web:\n    image: nginx:latest\n" >docker-compose.yml; llm_docker_check; llm_docker_check ./none'

# Worker dev: lib/modules/{git,remote,node,php,ssh}.sh.
# Git fixtures: a real repository "w" with a local bare repository "r.git"
# as origin; colors of git itself are off (color.ui false).

# git.sh
fx git_list_tags tags 'git config --global user.name T && git config --global user.email t@e.x && git config --global color.ui false && git config --global init.defaultBranch main && git init -q --bare r.git && git init -q w && cd w && git remote add origin ../r.git && echo a >f && git add f && git commit -qm init && git push -q -u origin main; git tag -a v1.0 -m one && git push -q origin v1.0 && git tag v1.1; git_list_tags'
fx git_list_tags no_remote 'git init -q x && cd x && git_list_tags'
fx git_add_tag created 'git config --global user.name T && git config --global user.email t@e.x && git config --global color.ui false && git config --global init.defaultBranch main && git init -q --bare r.git && git init -q w && cd w && git remote add origin ../r.git && echo a >f && git add f && git commit -qm init && git push -q -u origin main; git_add_tag v2.0 "Release two"'
fx git_add_tag exists 'git config --global user.name T && git config --global user.email t@e.x && git config --global color.ui false && git config --global init.defaultBranch main && git init -q --bare r.git && git init -q w && cd w && git remote add origin ../r.git && echo a >f && git add f && git commit -qm init && git push -q -u origin main; git tag v2.0; git_add_tag v2.0'
fx git_add_tag missing_name 'git config --global user.name T && git config --global user.email t@e.x && git config --global color.ui false && git config --global init.defaultBranch main && git init -q --bare r.git && git init -q w && cd w && git remote add origin ../r.git && echo a >f && git add f && git commit -qm init && git push -q -u origin main; git_add_tag'
fx git_delete_tag deleted 'git config --global user.name T && git config --global user.email t@e.x && git config --global color.ui false && git config --global init.defaultBranch main && git init -q --bare r.git && git init -q w && cd w && git remote add origin ../r.git && echo a >f && git add f && git commit -qm init && git push -q -u origin main; git tag -a v1 -m one && git push -q origin v1; git_delete_tag v1'
fx git_delete_tag unknown 'git config --global user.name T && git config --global user.email t@e.x && git config --global color.ui false && git config --global init.defaultBranch main && git init -q --bare r.git && git init -q w && cd w && git remote add origin ../r.git && echo a >f && git add f && git commit -qm init && git push -q -u origin main; git_delete_tag nope'
fx git_dump_revisions dumped 'git config --global user.name T && git config --global user.email t@e.x && git config --global color.ui false && git config --global init.defaultBranch main && git init -q w && cd w && echo a >f && git add f && git commit -qm one && echo b >>f && git commit -qam two; git_dump_revisions f'
fx git_dump_revisions missing_file 'git init -q w && cd w && git_dump_revisions nofile'
fx git_dump_revisions missing_arg 'git init -q w && cd w && git_dump_revisions'
fx git_apply_patch applied 'git config --global user.name T && git config --global user.email t@e.x && git config --global color.ui false && git config --global init.defaultBranch main && git init -q --bare r.git && git init -q w && cd w && git remote add origin ../r.git && echo a >f && git add f && git commit -qm init && git push -q -u origin main; git checkout -q -b feat && echo g >g && git add g && git commit -qm feat && git checkout -q main; git_apply_patch main feat "$(git rev-parse feat~1)"'
fx git_apply_patch missing_args 'git config --global init.defaultBranch main && git init -q w && cd w && git_apply_patch'
fx git_apply_patch dirty 'git config --global user.name T && git config --global user.email t@e.x && git config --global color.ui false && git config --global init.defaultBranch main && git init -q w && cd w && echo a >f && git add f && git commit -qm init && git branch feat && echo z >>f; git_apply_patch main feat HEAD'
fx git_repair clean 'git config --global user.name T && git config --global user.email t@e.x && git config --global color.ui false && git config --global init.defaultBranch main && git init -q --bare r.git && git init -q w && cd w && git remote add origin ../r.git && echo a >f && git add f && git commit -qm init && git push -q -u origin main; git_repair'
fx git_repair dry_run 'git config --global user.name T && git config --global user.email t@e.x && git config --global color.ui false && git config --global init.defaultBranch main && git init -q --bare r.git && git init -q w && cd w && git remote add origin ../r.git && echo a >f && git add f && git commit -qm init && git push -q -u origin main; mkdir -p .git/objects/ab && : >.git/objects/ab/cdef0123456789abcdef0123456789abcdef01; git_repair --dry-run'
fx git_repair declined 'git config --global user.name T && git config --global user.email t@e.x && git config --global color.ui false && git config --global init.defaultBranch main && git init -q --bare r.git && git init -q w && cd w && git remote add origin ../r.git && echo a >f && git add f && git commit -qm init && git push -q -u origin main; mkdir -p .git/objects/ab && : >.git/objects/ab/cdef0123456789abcdef0123456789abcdef01; git_repair' 'n'
fx git_repair repaired 'git config --global user.name T && git config --global user.email t@e.x && git config --global color.ui false && git config --global init.defaultBranch main && git init -q --bare r.git && git init -q w && cd w && git remote add origin ../r.git && echo a >f && git add f && git commit -qm init && git push -q -u origin main; mkdir -p .git/objects/ab && : >.git/objects/ab/cdef0123456789abcdef0123456789abcdef01; git_repair -y'
fx git_repair bad_option 'git_repair --bogus'
fx git_sync dry_run 'git config --global user.name T && git config --global user.email t@e.x && git config --global color.ui false && git config --global init.defaultBranch main && git init -q --bare r.git && git init -q w && cd w && git remote add origin ../r.git && echo a >f && git add f && git commit -qm init && git push -q -u origin main; echo n >new; git_sync --dry-run'
fx git_sync pushed 'git config --global user.name T && git config --global user.email t@e.x && git config --global color.ui false && git config --global init.defaultBranch main && git init -q --bare r.git && git init -q w && cd w && git remote add origin ../r.git && echo a >f && git add f && git commit -qm init && git push -q -u origin main; git checkout -q -b topic && echo n >new; git_sync -m "add new"'
fx git_sync nothing 'git config --global user.name T && git config --global user.email t@e.x && git config --global color.ui false && git config --global init.defaultBranch main && git init -q --bare r.git && git init -q w && cd w && git remote add origin ../r.git && echo a >f && git add f && git commit -qm init && git push -q -u origin main; git_sync'
fx git_sync detached 'git config --global user.name T && git config --global user.email t@e.x && git config --global color.ui false && git config --global init.defaultBranch main && git init -q w && cd w && echo a >f && git add f && git commit -qm init && git checkout -q --detach; git_sync'

# remote.sh: ssh, scp and rsync are stub functions (no network).
fx remote_hosts list 'printf "HOST:web=deploy@web.example.com\nHOST:db=admin@10.0.0.5\n" >.gash_env && chmod 600 .gash_env; __GASH_ENV_LOADED=""; remote_hosts'
fx remote_hosts empty '__GASH_ENV_LOADED=""; remote_hosts'
fx remote_exec ran 'ssh() { printf "hello from the remote host\n"; }; remote_exec deploy@web.example.com "uptime"'
fx remote_exec unreachable 'ssh() { return 255; }; remote_exec deploy@web.example.com "uptime"'
fx remote_exec unknown_alias '__GASH_ENV_LOADED=""; remote_exec nohost "uptime"'
fx remote_exec extra_args 'remote_exec deploy@web.example.com ls -la'
fx remote_push copied 'scp() { :; }; echo x >f; remote_push deploy@web.example.com f /tmp/f'
fx remote_push sensitive_declined 'scp() { :; }; echo x >f; remote_push deploy@web.example.com f /etc/app.conf' 'n'
fx remote_push denied 'remote_push deploy@web.example.com f /etc/shadow'
fx remote_pull copied 'scp() { :; }; remote_pull deploy@web.example.com /tmp/f ./f'
fx remote_pull denied 'remote_pull deploy@web.example.com /etc/shadow ./s'
fx remote_pull missing_args 'remote_pull deploy@web.example.com'
fx remote_sync_push synced 'rsync() { printf "sending incremental file list\n"; }; mkdir d; remote_sync_push deploy@web.example.com d/ /tmp/d/ --no-progress'
fx remote_sync_push delete_declined 'rsync() { :; }; mkdir d; remote_sync_push deploy@web.example.com d/ /srv/d/ --delete' 'n'
fx remote_sync_pull synced 'rsync() { :; }; remote_sync_pull deploy@web.example.com /tmp/d/ ./d/'
fx remote_sync_pull denied 'remote_sync_pull deploy@web.example.com /proc/1/ ./d/'

# node.sh: nvm is a stub function, NVM_DIR a directory tree.
fx node_detect_version detected 'export NVM_DIR="$HOME/.nvm"; mkdir -p "$NVM_DIR/versions/node/v20.11.0" "$NVM_DIR/versions/node/v22.1.0"; echo 20 >.nvmrc; node_detect_version'
fx node_detect_version not_installed 'export NVM_DIR="$HOME/.nvm"; mkdir -p "$NVM_DIR/versions/node/v22.1.0"; echo 18 >.nvmrc; node_detect_version'
fx node_detect_version json 'export NVM_DIR="$HOME/.nvm"; mkdir -p "$NVM_DIR/versions/node/v22.1.0"; node_detect_version --json'
fx node_detect_version no_nvm 'export NVM_DIR="$HOME/none"; node_detect_version'
fx node_detect_version bad_option 'export NVM_DIR="$HOME/.nvm"; mkdir -p "$NVM_DIR"; node_detect_version --bogus'
fx node_upgrade dry_run 'export NVM_DIR="$HOME/.nvm"; mkdir -p "$NVM_DIR/versions/node/v18.20.3" "$NVM_DIR/versions/node/v20.11.0" "$NVM_DIR/versions/node/v22.20.0"; nvm() { case "$1" in ls-remote) printf "        v18.20.8   (Latest LTS: Hydrogen)\n        v20.19.5   (Latest LTS: Iron)\n        v22.20.0   (Latest LTS: Jod)\n" ;; current) echo v20.11.0 ;; esac; }; node_upgrade --dry-run'
fx node_upgrade declined 'export NVM_DIR="$HOME/.nvm"; mkdir -p "$NVM_DIR/versions/node/v18.20.3" "$NVM_DIR/versions/node/v20.11.0" "$NVM_DIR/versions/node/v22.20.0"; nvm() { case "$1" in ls-remote) printf "        v18.20.8   (Latest LTS: Hydrogen)\n        v20.19.5   (Latest LTS: Iron)\n        v22.20.0   (Latest LTS: Jod)\n" ;; current) echo v20.11.0 ;; esac; }; node_upgrade' 'n'
fx node_upgrade upgraded 'export NVM_DIR="$HOME/.nvm"; mkdir -p "$NVM_DIR/versions/node/v18.20.3" "$NVM_DIR/versions/node/v20.11.0" "$NVM_DIR/versions/node/v22.20.0"; nvm() { case "$1" in ls-remote) printf "        v18.20.8   (Latest LTS: Hydrogen)\n        v20.19.5   (Latest LTS: Iron)\n        v22.20.0   (Latest LTS: Jod)\n" ;; current) echo v20.11.0 ;; esac; }; node_upgrade -y'
fx node_upgrade up_to_date 'export NVM_DIR="$HOME/.nvm"; mkdir -p "$NVM_DIR/versions/node/v22.20.0"; nvm() { case "$1" in ls-remote) printf "        v22.20.0   (Latest LTS: Jod)\n" ;; current) echo v22.20.0 ;; esac; }; node_upgrade'
fx node_upgrade offline 'export NVM_DIR="$HOME/.nvm"; mkdir -p "$NVM_DIR/versions/node/v22.20.0"; nvm() { return 1; }; node_upgrade'
fx node_upgrade no_nvm 'unset -f nvm; node_upgrade'
fx nvm_prune_versions dry_run 'nvm() { case "$1" in current) echo v20.11.0 ;; ls) printf "       v18.20.3\n       v18.20.8\n->     v20.11.0\n       v20.19.5\n       v20.9.0\n" ;; esac; }; nvm_prune_versions'
fx nvm_prune_versions force 'nvm() { case "$1" in current) echo v20.19.5 ;; ls) printf "       v18.20.3\n       v18.20.8\n       v20.11.0\n->     v20.19.5\n" ;; esac; }; nvm_prune_versions --force'
fx nvm_prune_versions none 'nvm() { case "$1" in current) echo v20.19.5 ;; ls) printf "%s\n" "->     v20.19.5" ;; esac; }; nvm_prune_versions'
fx nvm_prune_versions no_nvm 'unset -f nvm; nvm_prune_versions'

# php.sh
fx php_detect_version detected 'echo 8.2 >.target-php-version; php_detect_version'
fx php_detect_version json 'echo "{\"require\":{\"php\":\"^8.1\"}}" >composer.json; php_detect_version --json'
fx php_detect_version bad_option 'php_detect_version --bogus'

# ssh.sh: ssh-add, ssh-agent and expect are the mocks in tests/mocks/bin.
fx gash_ssh_auto_unlock unlocked 'export XDG_RUNTIME_DIR="$HOME/run" MOCK_SSH_AGENT=1; mkdir -p .ssh && : >.ssh/key1 && printf "SSH:~/.ssh/key1=pw\n" >.gash_env && chmod 600 .gash_env; __GASH_ENV_LOADED=""; unset GASH_SSH_AUTOUNLOCK_RAN; gash_ssh_auto_unlock'
fx gash_ssh_auto_unlock no_expect 'export XDG_RUNTIME_DIR="$HOME/run" MOCK_SSH_AGENT=1; mkdir -p .ssh bin && : >.ssh/key1 && printf "SSH:~/.ssh/key1=pw\n" >.gash_env && chmod 600 .gash_env && ln -s "$GASH_TEST_ROOT/tests/mocks/bin/ssh-add" bin/ssh-add; __GASH_ENV_LOADED=""; unset GASH_SSH_AUTOUNLOCK_RAN; PATH="$HOME/bin:/usr/bin:/bin" gash_ssh_auto_unlock'
fx gash_ssh_auto_unlock no_agent 'export XDG_RUNTIME_DIR="$HOME/run" MOCK_SSH_AGENT=0; mkdir -p .ssh run && : >.ssh/key1 && printf "SSH:~/.ssh/key1=pw\n" >.gash_env && chmod 600 .gash_env; __GASH_ENV_LOADED=""; unset GASH_SSH_AUTOUNLOCK_RAN; gash_ssh_auto_unlock'

# ai (worker ai): ai_ask, ai_query, ai_sysinfo, sysinfo. The HTTP client
# is tests/mocks/bin/curl; the API key is a fake one in the fixture's own
# ~/.gash_env (HOME is the fixture's temp dir).
fx ai_ask command 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_ask' 'how to list files?'
fx ai_ask explanation 'printf "AI:gemini=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_ask gemini' 'what is bash?'
fx ai_ask code 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_ask' 'write a script to back up my home'
fx ai_ask fallback 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_ask' 'hello'
fx ai_ask piped 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; printf "PHP Fatal error: x\n" | ai_ask claude'
fx ai_ask empty-query 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_ask'
fx ai_ask no-key 'ai_ask'
fx ai_ask no-token 'ai_ask gemini'
fx ai_ask bad-provider 'ai_ask openai'
fx ai_ask http-error 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; export MOCK_CURL_ERROR=1 MOCK_CURL_ERROR_CODE=401; ai_ask' 'what is bash?'
fx ai_ask confirm-no 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; GASH_AI_CONFIRM=1 ai_ask' $'what is bash?\nn'
fx ai_query command 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_query "how to list files?"'
fx ai_query explanation 'printf "AI:gemini=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_query gemini "what is bash?"'
fx ai_query code 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_query "write a script to back up my home"'
fx ai_query fallback 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_query hello'
fx ai_query piped 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; printf "PHP Fatal error: x\n" | ai_query "what is wrong?"'
fx ai_query no-query 'ai_query'
fx ai_query no-key 'ai_query hello'
fx ai_query no-token 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_query gemini hello'
fx ai_query http-error 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; export MOCK_CURL_ERROR=1 MOCK_CURL_ERROR_CODE=401; ai_query hello'
fx ai_query timeout 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; export MOCK_CURL_TIMEOUT=1; ai_query hello'
fx ai_query refusal 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; export MOCK_CURL_AI_RESPONSE="{\"content\":[],\"stop_reason\":\"refusal\"}"; ai_query hello'
fx ai_query confirm-yes 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; GASH_AI_CONFIRM=1 ai_query hello' 'y'
fx ai_query confirm-no 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; GASH_AI_CONFIRM=1 ai_query hello' 'n'
fx ai_query debug 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; GASH_AI_DEBUG=1 ai_query "how to list files?"'
fx ai_sysinfo raw 'ai_sysinfo --raw'
fx ai_sysinfo bad-arg 'ai_sysinfo --bogus'
fx ai_sysinfo no-key 'ai_sysinfo'
fx ai_sysinfo confirm-no 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_sysinfo' 'n'
fx ai_sysinfo report-quit 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_sysinfo' $'y\nq'
fx ai_sysinfo report-eof 'printf "AI:gemini=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_sysinfo gemini' 'y'
fx ai_sysinfo drilldown 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_sysinfo' $'y\n1'
fx ai_sysinfo question 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_sysinfo' $'y\n?\nwhat is listening on port 22?'
fx ai_sysinfo empty-question 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_sysinfo' $'y\n?\n'
fx ai_sysinfo save 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_sysinfo' $'y\ns'
fx ai_sysinfo bad-choice 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_sysinfo' $'y\nz'
fx ai_sysinfo no-color 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; ai_sysinfo --no-color' $'y\nq'
fx ai_sysinfo http-error 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; export MOCK_CURL_ERROR=1 MOCK_CURL_ERROR_CODE=401; ai_sysinfo' 'y'
fx ai_sysinfo drilldown-error 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; __sysinfo_call_drilldown_claude() { MOCK_CURL_ERROR=1 MOCK_CURL_ERROR_CODE=401 __ai_request claude "$1" "$__AI_SYSINFO_DRILLDOWN_PROMPT" "Section: $2" "$__AI_SYSINFO_DRILLDOWN_SCHEMA" 1024 60; }; ai_sysinfo' $'y\n2'
fx ai_sysinfo invalid-reply 'printf "AI:claude=test-key\n" >"$HOME/.gash_env"; chmod 600 "$HOME/.gash_env"; __gash_reload_env; __sysinfo_call_claude() { __AI_REPLY="not json"; }; ai_sysinfo' 'y'
fx sysinfo all 'sysinfo'
fx sysinfo overview 'sysinfo overview'
fx sysinfo overview-no-color 'sysinfo overview --no-color'
fx sysinfo identity 'sysinfo identity'
fx sysinfo services 'sysinfo services'
fx sysinfo network 'sysinfo network'
fx sysinfo system 'sysinfo system'
fx sysinfo llm 'sysinfo all --llm'
fx sysinfo layout '__sysinfo_collect_storage() { __sysinfo_section "STORAGE"; __sysinfo_sub "Empty"; true | __sysinfo_raw; __sysinfo_sub "fstab"; printf "UUID=1 / ext4 defaults 0 1\n" | __sysinfo_raw; __sysinfo_sub "Detected"; printf "/etc/nginx/\n/etc/php/\n" | while IFS= read -r x; do __sysinfo_item "$x"; done; __sysinfo_kv "LVM" "present"; __sysinfo_kv "ZFS Host-ID" "present"; }; __sysinfo_collect_mail() { __sysinfo_section "MAIL"; __sysinfo_sub "Postfix"; }; sysinfo storage; sysinfo mail'
fx sysinfo bad-arg 'sysinfo bogus'

# -----------------------------------------------------------------------------
# Harness
# -----------------------------------------------------------------------------

__render_dir="$(mktemp -d "${TMPDIR:-/tmp}/gash-render.XXXXXX")"
trap 'command rm -rf "$__render_dir"; trap - EXIT' EXIT
export __render_dir

# util-linux script on Linux, BSD script on macOS.
if script -qec true /dev/null >/dev/null 2>&1; then
  __render_pty=util-linux
else
  __render_pty=bsd
fi
export __render_pty

# The child: a terminal without echo or newline translation, Gash loaded as
# in color_gating_spec.sh, then the fixture.
cat >"$__render_dir/child.sh" <<'EOF'
dir="$1" mode="$2"
stty -echo -onlcr 2>/dev/null
: >"$dir/ready"
unset GASH_HEADLESS NO_COLOR GASH_NO_COLOR
[[ "$mode" == nocolor ]] && export NO_COLOR=1
export HOME="$dir/home" COLUMNS=100 LANG=C.UTF-8 LC_ALL=C.UTF-8 TERM=xterm-256color
export PATH="$GASH_TEST_ROOT/tests/mocks/bin:$PATH"
mkdir -p "$HOME"
cd "$HOME" || exit 99
source "$GASH_TEST_ROOT/tests/gash-test.sh"
gash_source_all "$GASH_TEST_ROOT"
if [[ "$mode" == pipe ]]; then
    { source "$dir/code.sh"; } | cat
else
    source "$dir/code.sh"
fi
EOF

# Run fixture ID in MODE (tty, pipe, nocolor); the bytes go to out.MODE.
__render_run() {
  local id="$1" mode="$2" n="$3"
  local dir="$__render_dir/$n.$mode"
  mkdir -p "$dir"
  printf '%s\n' "${__fx_code[$id]}" >"$dir/code.sh"
  local cmd="bash --noprofile --norc $__render_dir/child.sh $dir $mode"
  {
    local i=0
    while [[ ! -e "$dir/ready" ]] && ((i < 400)); do
      sleep 0.05
      i=$((i + 1))
    done
    [[ -n "${__fx_input[$id]}" ]] && printf '%s\n' "${__fx_input[$id]}"
    # End of input for every later read.
    for i in 1 2 3 4 5 6 7 8; do printf '\004'; done
  } | if [[ "$__render_pty" == util-linux ]]; then
    timeout 60 script -qec "$cmd" /dev/null
  else
    timeout 60 script -q /dev/null bash -c "$cmd"
  fi >"$__render_dir/$n.$mode.out" 2>&1
  printf '%s\n' "$?" >"$__render_dir/$n.$mode.rc"
}

# All runs, in parallel.
__render_jobs="$(nproc 2>/dev/null || getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)"
__render_n=0
for __render_id in "${__fx_ids[@]}"; do
  for __render_mode in tty pipe nocolor; do
    while (($(jobs -rp | wc -l) >= __render_jobs)); do
      wait -n 2>/dev/null || true
    done
    __render_run "$__render_id" "$__render_mode" "$__render_n" &
  done
  printf '%s\n' "$__render_id" >"$__render_dir/$__render_n.id"
  __render_n=$((__render_n + 1))
done
wait
unset __render_id __render_mode __render_n

# The SGR sequences R3 accepts, from output.sh itself.
(
  unset GASH_HEADLESS NO_COLOR GASH_NO_COLOR
  source "$GASH_TEST_ROOT/lib/core/output.sh"
  printf '%s\n' "$__GASH_COLOR_PRIMARY" "$__GASH_COLOR_SECONDARY" "$__GASH_COLOR_ACCENT" \
    "$__GASH_COLOR_MUTED" "$__GASH_COLOR_SUCCESS" "$__GASH_COLOR_WARNING" \
    "$__GASH_COLOR_ERROR" "$__GASH_COLOR_INFO" "$__GASH_BOLD" "$__GASH_DIM" \
    "$__GASH_UNDERLINE" "$__GASH_COLOR_OFF"
  __gash_sgr_extra
) | LC_ALL=C sort -u >"$__render_dir/sgr.allowed"

# Check one fixture's captures (number N). Prints every violation.
__render_check() {
  local n="$1" id fails=0 f seq line
  id="$(cat "$__render_dir/$n.id")"
  local tty="$__render_dir/$n.tty.out"
  local esc=$'\e'

  for f in tty pipe nocolor; do
    if [[ "$(cat "$__render_dir/$n.$f.rc")" == 124 ]]; then
      printf '%s: %s run timed out (waiting for input?)\n' "$id" "$f"
      fails=1
    fi
  done

  # R3: SGR allowlist.
  while IFS= read -r seq; do
    [[ -z "$seq" ]] && continue
    if ! grep -qxF -- "$seq" "$__render_dir/sgr.allowed"; then
      printf '%s: R3 SGR not in the palette: ' "$id"
      printf '%s' "$seq" | od -c | head -n 1
      fails=1
    fi
  done < <(LC_ALL=C grep -aoE "${esc}\\[[0-9;]*m" "$tty" | LC_ALL=C sort -u)

  # R4: other escape sequences.
  while IFS= read -r seq; do
    [[ -z "$seq" ]] && continue
    case "$seq" in
      "${esc}[K" | "${esc}[?25l" | "${esc}[?25h" | "${esc}[H" | "${esc}[2J") ;;
      *)
        printf '%s: R4 escape sequence not allowed: ' "$id"
        printf '%s' "$seq" | od -c | head -n 1
        fails=1
        ;;
    esac
  done < <(LC_ALL=C sed "s/${esc}\\[[0-9;]*m//g" "$tty" | LC_ALL=C grep -aoE "${esc}(\\[[0-9;?]*[@-~]|.)" | LC_ALL=C sort -u)

  # R5: status lines start with a glyph.
  while IFS= read -r line; do
    printf '%s: R5 status line without a glyph: %s\n' "$id" "$line"
    fails=1
  done < <(LC_ALL=C sed "s/${esc}\\[[0-9;?]*[@-~]//g" "$tty" | tr '\r' '\n' |
    grep -aE '^[[:space:]]*((Info|OK|Warning|Error|Debug|SSH):|\[(!|\+|i|OK|x|X)\])' || true)

  # R6: plain modes.
  for f in pipe nocolor; do
    if LC_ALL=C grep -aq "$esc" "$__render_dir/$n.$f.out"; then
      printf '%s: R6 escape bytes with %s: ' "$id" "$f"
      LC_ALL=C grep -ao "${esc}.\{0,8\}" "$__render_dir/$n.$f.out" | head -n 1 | od -c | head -n 1
      fails=1
    fi
    while IFS= read -r line; do
      printf '%s: R6 status line without its word prefix (%s): %s\n' "$id" "$f" "$line"
      fails=1
    done < <(tr '\r' '\n' <"$__render_dir/$n.$f.out" | grep -aE '^[[:space:]]*(✓|✗|! )' || true)
  done

  if ((fails)); then
    printf '\n--- %s on the terminal ---\n' "$id"
    head -c 3000 "$tty" | cat -v
    return 1
  fi
  return 0
}
export -f __render_check

# -----------------------------------------------------------------------------
# Checks
# -----------------------------------------------------------------------------

__render_list="$(
  for __render_f in "$__render_dir"/*.id; do
    printf '%s %s\n' "$(basename "$__render_f" .id)" "$(cat "$__render_f")"
  done | sort -n
)"
printf '%s\n' "${__fx_func[@]}" | LC_ALL=C sort -u >"$__render_dir/covered"

it "R1: every public human-facing function has a fixture" bash -c '
    set -euo pipefail
    want="$(bash -c "
        unset GASH_HEADLESS NO_COLOR GASH_NO_COLOR
        source \"\$GASH_TEST_ROOT/tests/gash-test.sh\"
        gash_source_all \"\$GASH_TEST_ROOT\"
        for k in \"\${!__GASH_HELP_REGISTRY[@]}\"; do
            [[ \"\$k\" == llm_* ]] || echo \"\$k\"
        done
        echo gash_username
    " | LC_ALL=C sort -u)"
    missing="$(LC_ALL=C comm -23 <(printf "%s\n" "$want") "$__render_dir/covered")"
    if [[ -n "$missing" ]]; then
        echo "functions without a fixture:"
        printf "  %s\n" $missing
        exit 1
    fi
    echo "$(wc -l <<<"$want") functions covered"
'

while read -r __render_k __render_id; do
  [[ -n "${__render_k-}" ]] || continue
  it "$__render_id: R2-R6" bash -c "__render_check $__render_k"
done <<<"$__render_list"
unset __render_k __render_id __render_list

it "R7: headless loading prints nothing, on a terminal or not" bash -c '
    set -euo pipefail
    out="$(GASH_HEADLESS=1 bash --noprofile --norc -c "source \"$GASH_TEST_ROOT/gash.sh\"" 2>&1)"
    [[ -z "$out" ]] || { printf "%s\n" "$out" | od -c | head; exit 1; }
    cmd="GASH_HEADLESS=1 bash --noprofile --norc -c \"source $GASH_TEST_ROOT/gash.sh\""
    if [[ "$__render_pty" == util-linux ]]; then
        out="$(script -qec "$cmd" /dev/null </dev/null 2>&1)"
    else
        out="$(script -q /dev/null bash -c "$cmd" </dev/null 2>&1)"
    fi
    [[ -z "$out" ]] || { printf "%s\n" "$out" | od -c | head; exit 1; }
'
