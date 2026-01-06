#!/usr/bin/env bash

# Gash Aliases: Safety
# Safer versions of common commands and system utilities.

# Safer versions of file operations
alias cp='cp -iv'
alias mv='mv -iv'
alias rm='rm -Iv --one-file-system --preserve-root'
alias mkdir='mkdir -pv'

# Calculator with math library
alias bc='bc -l'

# Network ports listing
alias ports='netstat -tulanp'

# Safety nets for system commands (prevent accidental root filesystem damage)
alias chown='chown --preserve-root'
alias chmod='chmod --preserve-root'
alias chgrp='chgrp --preserve-root'

# Safe reboot and shutdown (require sudo)
alias reboot='sudo /sbin/reboot'
alias shutdown='sudo /sbin/shutdown'
