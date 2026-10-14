# README demo: Gash at a terminal.
# Render from the repository root: vhs docs/tapes/demo.tape
Output docs/assets/demo.gif

Set Shell "bash"
Set FontSize 15
Set Width 1100
Set Height 620
Set Padding 18
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Env LANG "C.UTF-8"
Env LC_ALL "C.UTF-8"
Env GASH_BANNER "0"

Hide
Type "source ./gash.sh && clear"
Enter
Sleep 1s
Show

Type "gash"
Sleep 300ms
Enter
Sleep 3.5s

Type "clear; files_largest -n 8"
Sleep 300ms
Enter
Sleep 3.5s

Type "clear; tree_stats"
Sleep 300ms
Enter
Sleep 3.5s

Type "clear; disk_usage"
Sleep 300ms
Enter
Sleep 3s

Type "clear; sysinfo overview"
Sleep 300ms
Enter
Sleep 4s
