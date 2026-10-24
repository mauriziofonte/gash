# gash hte: local HTTPS sites with Apache and PHP-FPM, run as your user.
# Changes the machine (one sudo at install): render.sh records it only with
# GASH_DEMO_HTE=1, and the hidden tail uninstalls again.
# Record with docs/tapes/render.sh hte (see docs/tapes/README.md).
Output docs/assets/hte.gif

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
Type "source docs/tapes/env.sh"
Enter
Wait
Show

Type "gash hte install --yes --autostart none"
Sleep 300ms
Enter
Wait
Sleep 4s

Type "clear; gash hte site create --domain shop.test --docroot public"
Sleep 300ms
Enter
Wait
Sleep 3.5s

Type "curl -sk https://shop.test/"
Sleep 300ms
Enter
Wait
Sleep 3s

Type "clear; gash hte site list"
Sleep 300ms
Enter
Wait
Sleep 4s

Type "gash hte service status"
Sleep 300ms
Enter
Wait
Sleep 4s

Hide
Type "clear; gash hte site remove --domain shop.test --yes >/dev/null 2>&1; gash hte uninstall --yes >/dev/null 2>&1"
Enter
Wait

