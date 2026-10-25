# Agents: Gash headless, as an AI coding agent calls it. JSON in, JSON out,
# and the guardrails refuse writes, secrets and commands outside the allowlist
# (the error envelope goes to stderr, hence 2>&1).
# Record with docs/tapes/render.sh agents (see docs/tapes/README.md).
Output docs/assets/agents.gif

Set Shell "bash"
Set FontSize 15
Set Width 1100
Set Height 700
Set Padding 18
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set WaitTimeout 90s
Set WaitPattern /\$ ?$/

Hide
Type "source docs/tapes/env.sh --headless"
Enter
Wait
Show

Type "export GASH_HEADLESS=1; source ~/.gash/gash.sh"
Sleep 300ms
Enter
Wait
Sleep 1.5s

Type "llm_project | jq -c ."
Sleep 300ms
Enter
Wait
Sleep 2.5s

Type "llm_git_status | jq -c ."
Sleep 300ms
Enter
Wait
Sleep 2.5s

Type "llm_db_query 'SELECT name, email FROM users' -f database/app.sqlite | jq -c .data"
Sleep 300ms
Enter
Wait
Sleep 3s

Type "clear; llm_db_query 'DELETE FROM users' -f database/app.sqlite 2>&1 | jq ."
Sleep 300ms
Enter
Wait
Sleep 4s

Type "llm_config .env 2>&1 | jq -c '{error, hint}'"
Sleep 300ms
Enter
Wait
Sleep 3s

Type "llm_exec 'rm -rf storage' 2>&1 | jq -c '{error, hint}'"
Sleep 300ms
Enter
Wait
Sleep 4.5s
