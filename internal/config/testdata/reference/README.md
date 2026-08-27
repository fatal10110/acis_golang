Snapshot of the shipped `.properties` files, vendored from `aCis_gameserver/config/`
so `internal/config/gen` and its drift test are self-contained in CI (which checks
out only this repository). Refresh by re-copying from `aCis_gameserver/config/`
whenever those files change, then run `go generate ./internal/config/...`.
