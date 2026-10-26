# Files and archives

Find what fills a disk, summarize a tree, extract any archive and back up a file.

<!-- BEGIN generated: prerequisites (go test ./internal/docgen -update) -->
Beyond what Gash itself needs, this area uses (install commands for each system: [requirements](../requirements.md)):

- [unzip](../requirements.md#tool-unzip) (feature), for `archive_extract` (.zip)
- [bzip2](../requirements.md#tool-bzip2) (feature), for `archive_extract` (.bz2)
- [xz](../requirements.md#tool-xz) (feature), for `archive_extract` (.xz)
- [zstd](../requirements.md#tool-zstd) (feature), for `archive_extract` (.zst)
- [7z](../requirements.md#tool-7z) (feature), for `archive_extract` (.7z)
- [unrar](../requirements.md#tool-unrar) (feature), for `archive_extract` (.rar)
- [uncompress](../requirements.md#tool-uncompress) (feature), for `archive_extract` (.Z)
<!-- END generated: prerequisites -->

Every function, alias and command of this area, with all its options: [reference.md](reference.md).

<!-- example
mkdir -p project/src project/docs
printf 'hello world\n' >project/src/main.go
printf '# Title\n' >project/docs/README.md
mask: Scan time: +[0-9]+ms
mask: _backup_[0-9]{14}
-->
```console
$ cd project
$ tree_stats .
Tree statistics for ~/project
  Files:       2
  Directories: 3
  Total size:  20 B (20 bytes)
  Avg file:    10 B
  Max depth:   1
  Empty dirs:  0
  Scan time:   0ms

  Top 5 extensions by count:
    go                1 files  12 B
    md                1 files  8 B

  Top 5 extensions by size:
    go           12 B
    md           8 B
$ file_backup src/main.go
'src/main.go' -> 'src/main.go_backup_20261026082008'
```
