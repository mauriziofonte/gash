# nav

Directory navigation and listings.

_Written from the help registry (`gash_help NAME`); `go test ./internal/registry` checks it against the help text._

## Aliases

| Alias | Runs |
|---|---|
| `ll` | `ls -l` |
| `la` | `ls -la` |
| `l` | `ls -CF` |
| `lash` | `ls -lash` |
| `sl` | `ls` |
| `..` | `cd ..` |
| `...` | `cd ../..` |
| `cd..` | `cd ..` |
| `....` | `cd ../../..` |
| `.....` | `cd ../../../..` |
| `.4` | `cd ../../../..` |
| `.5` | `cd ../../../../..` |
| `cls` | `clear` |
| `path` | `tr ":" "\n" <<<"$PATH"` |

