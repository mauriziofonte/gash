# Files: folder sizes, empty folders, safe extraction, backups.
# Record with dev/tapes/render.sh files (see dev/tapes/README.md).
Output docs/assets/files.gif

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

Type "gash files largest-dirs --depth 2 --limit 6"
Sleep 300ms
Enter
Wait
Sleep 3.5s

Type "clear; gash files empty-dirs"
Sleep 300ms
Enter
Wait
Sleep 3s

Type "clear; gash files extract ~/archives/suspicious.tar"
Sleep 300ms
Enter
Wait
Sleep 4s

Type "gash files extract ~/archives/site-1.4.2.tar.gz ~/sites"
Sleep 300ms
Enter
Wait
Sleep 3.5s

Type "clear; gash files backup .env && ls -a"
Sleep 300ms
Enter
Wait
Sleep 4s
