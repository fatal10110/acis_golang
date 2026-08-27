Snapshot of the shipped `.properties` files, vendored from `aCis_gameserver/config/`
so `internal/config/gen` and its drift test are self-contained in CI (which checks
out only this repository). Only key names matter here — every value has been replaced
with `REDACTED` (or left blank where the source was blank) so no host, credential, or
other real value from the source config ships in this repo.

Refresh by re-copying from `aCis_gameserver/config/` whenever the *key set* changes
(new/renamed/removed keys), then re-redact values the same way before running
`go generate ./internal/config/...`.
