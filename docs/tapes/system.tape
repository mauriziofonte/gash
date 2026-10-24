# System: who holds a port, free it, find that command again.
# Record with docs/tapes/render.sh system (see docs/tapes/README.md).
Output docs/assets/system.gif

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
Type "source docs/tapes/env.sh"
Enter
Wait
Show

Hide
Type "php -S 127.0.0.1:8080 -t public >/dev/null 2>&1 & disown; sleep 1; clear"
Enter
Sleep 1.5s
Show

Type "process_find php"
Sleep 300ms
Enter
Wait
Sleep 3s

Type "port_kill 8080"
Sleep 300ms
Enter
Sleep 2.5s

Type "y"
Sleep 300ms
Enter
Wait
Sleep 2.5s

Type "clear; hgrep artisan"
Sleep 300ms
Enter
Wait
Sleep 4.5s
