# Fix: Client Freeze on Ground Item Pickup

## Bug

Clicking Adena (or any ground item) after killing a mob shows the picking
animation, but the item stays on the floor and the character becomes
unresponsive. Server logs show "connection reset by peer".

## Root Cause

`broadcastGroundPickup` used `broadcastFrame`, which calls the **blocking**
`SendFrame` sequentially on **every** observer in `ForEachKnown` order. The
same goroutine that called `broadcastGroundPickup` still had to run
`world.Despawn(ground)` (sends `DeleteObject`) and `sendInventoryUpdate`
afterward.

If **any** observer had a full write buffer, the blocking `SendFrame` would
hang that goroutine **permanently**. The client never received the
`DeleteObject` for the ground item (it stayed visible) or the inventory
update — the session timed out, causing "connection reset by peer".

## Fix

Split the broadcast into two delivery paths:

- **Picker**: blocking `SendFrame` — the picking player must receive
  `GetItem` reliably.
- **Other observers**: non-blocking `sendVisibilityFrame` — best-effort
  delivery. If a dropped `GetItem` frame on a nearby player is acceptable;
  it was already the same pattern used by visibility broadcasts
  (`Discover`/`Forget` via `trySendFrame`).

The frame is serialized once and copied per recipient via
`serverpackets.CopyFrame` to avoid shared mutable state.

## Files Changed

| File | Change |
|------|--------|
| `internal/gameserver/network/pickup.go` | Added `broadcastGroundPickup` with split blocking/non-blocking send |
| `internal/gameserver/network/pet.go` | Removed old `broadcastGroundPickup` (moved to `pickup.go`), removed unused `wire` and `world` imports |

## Verification

- `go build ./internal/gameserver/network/` — clean
- `go test ./internal/gameserver/network/ -count=1` — 17s, all tests pass
- `go vet ./internal/gameserver/network/` — clean
