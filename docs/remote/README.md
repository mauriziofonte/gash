# Remote hosts

Run commands on, and copy and sync files with, the SSH hosts named in ~/.gash_env, and unlock SSH keys at login.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):

- [ssh](../requirements.md#tool-ssh) (feature), for `remote_exec`, `remote_sync_pull` and `remote_sync_push`
- [scp](../requirements.md#tool-scp) (feature), for `remote_pull` and `remote_push`
- [rsync](../requirements.md#tool-rsync) (feature), for `remote_sync_pull` and `remote_sync_push`
- [ssh-add](../requirements.md#tool-ssh-add) (feature), for `gash_ssh_auto_unlock`
- [ssh-agent](../requirements.md#tool-ssh-agent) (feature), for `gash_ssh_auto_unlock`
<!-- END generated: prerequisites -->

Every function, alias and command of this area, with all its options: [reference.md](reference.md).
