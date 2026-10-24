# Files: folder sizes, empty folders, safe extraction, backups.
# Record with docs/tapes/render.sh files (see docs/tapes/README.md).
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
Type "source docs/tapes/env.sh"
Enter
Wait
Show

Type "dirs_largest --depth 2 --limit 6"
Sleep 300ms
Enter
Wait
Sleep 3.5s

Type "clear; dirs_list_empty"
Sleep 300ms
Enter
Wait
Sleep 3s

Type "clear; archive_extract ~/archives/suspicious.tar"
Sleep 300ms
Enter
Wait
Sleep 4s

Type "archive_extract ~/archives/site-1.4.2.tar.gz ~/sites"
Sleep 300ms
Enter
Wait
Sleep 3.5s

Type "clear; file_backup .env && ls -a"
Sleep 300ms
Enter
Wait
Sleep 4s
