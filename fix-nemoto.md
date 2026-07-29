# Adena Pickup Bug Fix

## Issue Description

When a player clicks on Adena (item ID 57) on the ground after killing a mob:

1. **First click** - Item is selected, `MyTargetSelected` packet sent ✓
2. **Second click** - "Picking" animation plays, `GetItem` packet sent ✓
3. **Bug** - Item **stays visible on the ground** (no `DeleteObject` packet) ✗
4. **Bug** - **Character freezes** - unresponsive to all clicks/commands ✗
5. Client eventually disconnects with "connection reset by peer"

### Server Logs
```json
{"level":"error","error":"read tcp 192.168.50.144:7777->192.168.50.250:53830: read: connection reset by peer","time":"2026-07-29T23:22:08+03:00","message":"Read frame"}
{"level":"warn","error":"writev tcp 192.168.50.144:7777->192.168.50.250:53830: writev: broken pipe","time":"2026-07-29T23:22:08+03:00","message":"game connection write failed, closing"}
```

## Root Cause

The bug occurred specifically when **Adena merged into an existing stack** in the player's inventory (the `absorbed=true` path in `PickupGround`).

In `internal/gameserver/network/pickup.go`, the `pickupLiveGroundItem` function had incorrect operation ordering:

```go
// BEFORE (buggy)
l.broadcastGroundPickup(ground, live.ObjectID())  // GetItem
l.broadcastPickupAttention(live, ground)          // SystemMessage (optional)
l.groundItems.Remove(ground)                      // Removed from tracker FIRST
l.world.Despawn(ground)                           // DeleteObject - CALLED TOO LATE
```

When the ground item was absorbed into an existing stack:
1. `groundItems.Remove(ground)` cleaned up the tracker
2. `world.Despawn(ground)` was called but couldn't properly broadcast `DeleteObject` because the item was no longer tracked
3. Client never received `DeleteObject` → item stayed visible
4. Client never received `InventoryUpdate` → action never resolved → input locked

## Fix Applied

**File:** `internal/gameserver/network/pickup.go` (lines 78-85)

**Reordered operations** to ensure `DeleteObject` is broadcast BEFORE tracker cleanup:

```go
// AFTER (fixed)
l.broadcastGroundPickup(ground, live.ObjectID())  // GetItem (1st)
l.world.Despawn(ground)                           // DeleteObject (2nd) - MOVED UP
l.groundItems.Remove(ground)                      // Tracker cleanup (3rd)
l.broadcastPickupAttention(live, ground)          // SystemMessage (4th)
l.lockPickupParalysis(live)                       // Paralysis (5th)

l.applyPersistActions(ctx, res.Persist)           // DB I/O (last)
l.sendInventoryUpdate(live, inv)                  // InventoryUpdate
```

### Key Changes
| Order | Operation | Purpose |
|-------|-----------|---------|
| 1 | `broadcastGroundPickup` | Sends `GetItem` (pickup animation) |
| 2 | `world.Despawn` | **Sends `DeleteObject` - NOW BEFORE REMOVE** |
| 3 | `groundItems.Remove` | Cleans up cleanup tracker |
| 4 | `broadcastPickupAttention` | Sends attention SystemMessage (weapons/armor) |
| 5 | `lockPickupParalysis` | Brief 200ms paralysis anti-spam |
| 6 | `applyPersistActions` | DB persistence (blocking) |
| 7 | `sendInventoryUpdate` | Sends `InventoryUpdate` |

## Test Coverage Added

### New Integration Test
**File:** `internal/gameserver/network/pickup_test.go`

**Test:** `TestGameClientLinkPickupAdenaMergeFullClientFlow`

**Scenario:**
1. Player has 100 Adena in inventory (existing stack, ObjectID: 800)
2. Mob drops 40 Adena on ground (ObjectID: 5000)
3. Player clicks ground Adena twice (Action packets)
4. Verify full packet sequence and state changes

**Assertions:**
- Packet sequence: `MyTargetSelected` → `GetItem` → `DeleteObject` → `InventoryUpdate`
- Ground item removed from world registry
- Inventory has merged stack: 140 Adena in ObjectID 800
- Movement works after pickup (character not frozen)

### Existing Tests Verified
All 10 pickup-related tests pass:
- `TestGameClientLinkPickupGroundItemFullClientFlow` (fresh pickup)
- `TestGameClientLinkPickupAdenaMergeFullClientFlow` (merge scenario - **NEW**)
- `TestPickupLiveGroundItemMovesItemAndDespawns`
- `TestPickupLiveGroundItemLocksAndReleasesTransientParalysis`
- `TestPickupLiveGroundItemMergesStackAndDeletesGroundRow`
- `TestPickupLiveGroundItemBroadcastsAttentionForWeaponAndArmor`
- `TestPickupLiveGroundItemSkipsAttentionForEtcItem`
- `TestPickupLiveGroundItemRejectsOutOfRange`
- `TestPickupLiveGroundItemRejectsLootLockedByOtherOwner`
- `TestPickupLiveGroundItemRejectsWhenSlotsFull`

## Verification

```bash
# Run specific pickup tests
go test -v -run "Pickup" ./internal/gameserver/network/

# Run full network test suite
go test ./internal/gameserver/network/

# Run all internal tests
go test ./internal/...
```

All tests pass with no regressions.

## Impact

- **Fixes:** Adena (ID 57) and Ancient Adena (ID 5575) pickup when merging
- **Fixes:** Any stackable item pickup when merging into existing stack
- **No impact:** Non-stackable items, weapons, armor (different code path)
- **No impact:** Rejection paths (out of range, loot locked, slots full) - already had proper `ActionFailed` responses

## Related Files

| File | Purpose |
|------|---------|
| `internal/gameserver/network/pickup.go` | Main fix - reordered operations |
| `internal/gameserver/network/pickup_test.go` | New integration test + existing unit tests |
| `internal/gameserver/inventory/service.go` | `PickupGround` returns `absorbed=true` for merges |
| `internal/gameserver/task/grounditems.go` | Ground item tracker cleanup |
| `internal/gameserver/world/visibility.go` | `Despawn` broadcasts `DeleteObject` |

## Reference Behavior (aCis Java)

The fix matches aCis `PlayerAI.thinkPickUp()` behavior:
```java
// Java reference (PlayerAI.java:365)
item.pickupMe(_actor);  // Broadcasts GetItem + sets invisible
ItemsOnGroundTaskManager.getInstance().remove(item);  // Tracker cleanup AFTER
ThreadPool.schedule(() -> _actor.setIsParalyzed(false), 200);
_actor.setIsParalyzed(true);
```

The key insight: **Despawn (broadcast DeleteObject) must happen before tracker removal**, exactly as the Java reference does.