# Help: a namespace page, Tab completion, search, and the full page of a
# command found by its shortcut.
# Record with dev/tapes/render.sh help (see dev/tapes/README.md).
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
Type "source dev/tapes/env.sh"
Enter
Wait
Show

Type "gash git"
Sleep 300ms
Enter
Wait
Sleep 4.5s

Type "clear; gash git t"
Sleep 300ms
Tab
Sleep 800ms
Tab
Tab
Sleep 3.5s
Ctrl+C
Sleep 300ms

Type "clear; gash help --search tag"
Sleep 300ms
Enter
Wait
Sleep 3.5s

Type "clear; gash help gat"
Sleep 300ms
Enter
Wait
Sleep 5s
