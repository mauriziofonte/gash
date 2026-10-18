// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

// hgrepHelp is the inline help of hgrep -h (the 2.x heredoc, which differs
// from the registered help: LEDGER SYS-47).
const hgrepHelp = `hgrep - Smart history search with timestamps and deduplication

USAGE:
  hgrep PATTERN [OPTIONS]

OPTIONS:
  -n, --limit N       Show only last N results
  -a, --all           Include hgrep/history_grep in results
  -r, --reverse       Show oldest first (default: newest last)
  -j, --json          Output as JSON array
  -H, --no-highlight  Disable pattern highlighting / force plain output
  -E, --regex         Use extended regex (default: fixed string match)
  -c, --count         Only show count of unique matching commands
  -g, --group         Collapse similar commands under a shared chain prefix
      --pick          Fuzzy-select a matching command via fzf and print it
      --no-color      Force-disable colored output for this invocation

EXAMPLES:
  hgrep git                    Search for "git" commands
  hgrep "composer.*install"    Search with pattern
  hgrep docker -n 10           Last 10 docker commands
  hgrep -E "^git (push|pull)"  Regex: git push or pull at start
  hgrep npm -j                 JSON output for scripting
  hgrep make -c                Count unique make commands
  hgrep cd -g                  Group "cd X && ..." variants under one prefix
  hgrep git --pick             Fuzzy-pick a past git command (needs fzf)

OUTPUT FORMAT:
  [2025-01-15 14:32:01] git push origin main
  [2025-01-15 15:10:22] git commit -m "fix"

  With --json:
  [{"timestamp":"2025-01-15 14:32:01","epoch":1705330321,"count":1,"command":"git push origin main"},...]
`
