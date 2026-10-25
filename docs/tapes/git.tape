# Git: history, a commit-and-push with its plan, an annotated tag.
# Record with docs/tapes/render.sh git (see docs/tapes/README.md).
Output docs/assets/git.gif

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

Type "gl"
Sleep 300ms
Enter
Wait
Sleep 3.5s

Type "clear; git_sync -m 'Document npm run dev'"
Sleep 300ms
Enter
Sleep 2.5s

Type "y"
Sleep 300ms
Enter
Wait
Sleep 3s

Type "clear; gat v1.2.0 'Shop 1.2.0'"
Sleep 300ms
Enter
Sleep 2.5s

Type "y"
Sleep 300ms
Enter
Wait
Sleep 3s

Type "clear; git_list_tags"
Sleep 300ms
Enter
Wait
Sleep 3.5s
