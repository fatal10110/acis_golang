# Fix: Ground Item Pickup Causing Client Freeze and Connection Reset

## Problem

When picking up Adena (or any ground item) from the floor after killing a mob, the following symptoms occurred:

1. **First click does nothing** — the ground item is selected as target but no pickup feedback is shown.
2. **Second click shows "picking" animation but item stays on the floor** — `GetItem` (0x0D) plays the pickup animation on the client, but the item remains visually on the ground.
3. **Character stops responding to any clicks/commands** — the client becomes completely unresponsive.
4. **Server logs show `connection reset by peer` + `broken pipe`** — the client forcibly closes the TCP connection.

## Root Cause

In `pickupLiveGroundItem` (`acis_golang/internal/gameserver/network/pickup.go`), the pickup flow called `l.world.Despawn(ground)` synchronously immediately after `l.broadcastGroundPickup(ground, live.ObjectID())`.

`world.Despawn` sends `DeleteObject` (opcode 0x12) to all nearby players via `livePlayer.Forget()`. This meant the client received two removal packets for the same ground item in rapid succession:

1. `GetItem` (0x0D) — pickup animation, removes item from ground list
2. `DeleteObject` (0x12) — removes object from known list

The Lineage 2 client's ground-item handling and generic object-removal handling conflicted when both packets arrived synchronously in the same tick, causing the client to enter a broken state where it could no longer process input packets, leading to the connection being reset.

This differed from the Java reference implementation where `pickupMe()` only calls `setIsVisible(false)` (deferring `DeleteObject` to the async visibility system) and does not send `DeleteObject` synchronously in the pickup handler.

The same issue affected pet ground-item pickup (`petGetItem` in `pet.go`).

## Fix

Removed the synchronous `l.world.Despawn(ground)` call from both `pickupLiveGroundItem` (in `pickup.go`) and `petGetItem` (in `pet.go`).

- `l.groundItems.Remove(ground)` already removes the item from the ground-item cleanup tracker, so it will never re-appear after being picked up.
- The `GetItem` (0x0D) broadcast is sufficient to make the item disappear from all clients' ground lists — this matches the Java reference's behavior where `pickupMe()` sends `GetItem` + `setIsVisible(false)`.
- The ground item remains in the world state as an invisible object, which is harmless and consistent with Java's approach (Java's `setIsVisible(false)` + deferred async `DeleteObject`).

Also removed the duplicate `broadcastGroundPickup` function definition from `pet.go` (it was redundant with the one already in `pickup.go`).

## Changed Files

- `acis_golang/internal/gameserver/network/pickup.go` — removed `l.world.Despawn(ground)` from `pickupLiveGroundItem`
- `acis_golang/internal/gameserver/network/pet.go` — removed `l.world.Despawn(ground)` and the duplicate `broadcastGroundPickup` function from `petGetItem`
- `acis_golang/internal/gameserver/network/pickup_test.go` — updated `TestPickupLiveGroundItemMovesItemAndDespawns` and `TestGameClientLinkPickupGroundItemFullClientFlow` to no longer expect `DeleteObject` in the pickup packet sequence; `world.Object` checks now expect ground items to remain as invisible objects
- `acis_golang/internal/gameserver/network/pet_test.go` — updated pet pickup tests similarly

## Verification

All tests pass:
```
go test ./... 
```

The full client flow test (`TestGameClientLinkPickupGroundItemFullClientFlow`) validates the complete end-to-end packet sequence via a real TCP connection: `GetItem` → `InventoryUpdate`, with no `DeleteObject` in the synchronous pickup response.