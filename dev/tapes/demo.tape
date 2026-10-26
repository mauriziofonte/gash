# README hero: Gash at a terminal.
# Record with dev/tapes/render.sh demo (see dev/tapes/README.md).
Output docs/assets/demo.gif

Set Shell "bash"
Set FontSize 15
Set Width 1100
Set Height 640
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

Type "gash"
Sleep 300ms
Enter
Wait
Sleep 4s

Type "clear; files_largest ~ -n 8"
Sleep 300ms
Enter
Wait
Sleep 4s

Type "clear; tree_stats"
Sleep 300ms
Enter
Wait
Sleep 4s

Type "clear; sysinfo overview"
Sleep 300ms
Enter
Wait
Sleep 4.5s
