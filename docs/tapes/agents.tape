# README demo: the same functions for an LLM agent, as JSON.
# Render from the repository root: vhs docs/tapes/agents.tape
Output docs/assets/agents.gif

Set Shell "bash"
Set FontSize 15
Set Width 1100
Set Height 560
Set Padding 18
Set Theme "Catppuccin Mocha"
Set TypingSpeed 35ms
Env LANG "C.UTF-8"
Env LC_ALL "C.UTF-8"

Hide
Type `export GASH_HEADLESS=1 PS1='$ ' && clear`
Enter
Sleep 500ms
Show

Type `bash -c 'source ./gash.sh; files_largest --json -n 3' | jq .`
Sleep 300ms
Enter
Sleep 4s

Type `clear; bash -c 'source ./gash.sh; llm_db_query "DELETE FROM users" -f app.db'`
Sleep 300ms
Enter
Sleep 4s

Type `clear; bash -c 'source ./gash.sh; llm_exec "rm -rf build"'`
Sleep 300ms
Enter
Sleep 4s
