# Help: a topic card, search, and the full page of a function.
# Record with docs/tapes/render.sh help (see docs/tapes/README.md).
Output docs/assets/help.gif

Set Shell "bash"
Set FontSize 15
Set Width 1100
Set Height 860
Set Padding 18
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set WaitTimeout 90s
Set WaitPattern /\$ ?$/

Hide
Type "source docs/tapes/env.sh"
Enter
Wait
Show

Type "gash git"
Sleep 300ms
Enter
Wait
Sleep 4.5s

Type "clear; gash_help --search tag"
Sleep 300ms
Enter
Wait
Sleep 3.5s

Type "clear; gash_help gat"
Sleep 300ms
Enter
Wait
Sleep 5s
