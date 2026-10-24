## What changes for the user

Before:

After:

## How

## Checklist

- [ ] `go test ./...` passes (and a new unit test or `testdata/script` txtar covers the change)
- [ ] `go vet ./...` and golangci-lint are clean; `shellcheck` and `shfmt -d` are clean on the generated Bash
- [ ] Help text updated and `go run ./cmd/gash __compat generate --out .` re-run, if a function changed (`--check` passes)
- [ ] `CHANGELOG.md` updated under the unreleased version
- [ ] No breaking change to a documented function, envelope or setting (or it is called out above)
