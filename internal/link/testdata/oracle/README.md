# Link packet oracle

`../link.golden` is the expected output of `TestLinkPacketsMatchReferenceProbe`
(`../../link_oracle_test.go`): the game server to login server packets of the
reference, written by `LinkProbe.java` (the reference classes compiled
unmodified; `stub/` replaces the id/value holder, which reaches the skill table).

Lines:

- `serverstatus <attribute>:<value>,... <payload hex>`: attributes in the order
  the reference adds them. Alone: one per call of `setServerType`/`setMaxPlayer`.
  Six at once: the packet sent on `AuthResponse` (status, clock, brackets, age
  limit, test server, pvp server).
- `playeringame <accounts,...> <payload hex>`: the roster (or one entering
  account) sent after it.

Regenerate with `run.sh`.
