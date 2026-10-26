# System

Processes, ports, services, disks, sudo history and a system report.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):

- [ps](../requirements.md#tool-ps) (feature), for `process_find`
- [pgrep](../requirements.md#tool-pgrep) (feature), for `process_kill`
- [lsof](../requirements.md#tool-lsof) (feature), for `port_kill`
- [sudo](../requirements.md#tool-sudo) (feature), for `services_stop`, `sudo_last` and `sysinfo` (the privileged sections)
- [systemctl](../requirements.md#tool-systemctl) (feature), for `services_stop`
<!-- END generated: prerequisites -->

Every function, alias and command of this area, with all its options: [reference.md](reference.md).
