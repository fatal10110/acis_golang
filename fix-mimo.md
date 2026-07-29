# Fix: Character stops responding after picking up ground items

## Bug

After killing a mob and picking up dropped Adena (or any ground item), the
character becomes permanently unresponsive to all clicks and commands.

**Reproduction steps:**

1. Kill a mob — loot drops on the ground
2. Click on Adena — nothing happens (first click selects target)
3. Click again — picking animation plays, but item stays on the floor
4. After that — character stops responding to any input

## Root cause

`sendInventoryUpdate` (`inventory.go:305`) silently drops the `InventoryUpdate`
packet when `FrameInventoryUpdate` fails to build (e.g. missing item template).

After a successful pickup, the server sends this packet sequence:

| Packet | Purpose | Sent? |
|--------|---------|-------|
| `GetItem` | Picking animation | Yes |
| `DeleteObject` | Remove item from ground | Yes |
| `InventoryUpdate` | Update client inventory | **Dropped on error** |

The client receives `GetItem` (animation plays) and `DeleteObject` (item removed
from world), but never receives `InventoryUpdate`. The client's action state
machine is stuck waiting for the inventory response that never arrives — blocking
**all** subsequent input.

The L2 client locks its input on every action click until the server responds.
If the response is lost, the client enters a permanent pending state.

## Changes

### 1. `internal/gameserver/network/inventory.go`

```diff
 func (l *GameClientLink) sendInventoryUpdate(live *livePlayer, inv *itemcontainer.Inventory) {
     ...
     frame, err := serverpackets.FrameInventoryUpdate(updates, items, inv.Templates())
     if err != nil {
         l.log.Error().Err(err).Msg("build InventoryUpdate")
+        // The inventory state was mutated but the client never learns
+        // about it. Sending ActionFailed unblocks the client's pending
+        // action instead of leaving it frozen forever — a visual desync
+        // (open inventory to resync) is recoverable; a frozen client is not.
+        live.SendFrame(serverpackets.FrameActionFailed())
         return
     }
     live.SendFrame(frame)
 }
```

**Why ActionFailed:** The inventory change succeeded server-side, but the client
never learns about it. Sending `ActionFailed` unblocks the client's pending
action. The player sees a minor visual desync (open inventory to resync), which
is recoverable — a frozen client is not.

### 2. `internal/gameserver/network/lifecycle.go`

```diff
 func (l *GameClientLink) detachLivePlayer(ctx context.Context, live *livePlayer) {
     ...
     live.Stop()
+    // Clear any transient paralysis lock left by a pickup or other action.
+    // A scheduled release goroutine may have panicked or not yet fired;
+    // leaving the lock set would block every item op on the next login.
+    live.SetParalyzed(false)
     l.cancelActiveTrade(live)
     ...
 }
```

**Why:** The 200ms pickup paralysis lock (`lockPickupParalysis`) is cleared by a
scheduled goroutine. If the connection closes during this window, the deferred
`detachLivePlayer` now explicitly clears the lock so it doesn't persist into the
next session.

### 3. `internal/gameserver/network/pickup.go`

Added diagnostic logging for `PickupNoop` rejections and paralysis-release
no-ops so future occurrences are visible in server logs.

## Design principle

Every client action that registers a pending state **must** receive a response.
Silent drops freeze the client. The fix follows the existing pattern from issue
#873: every rejection path sends `ActionFailed` to release the client's pending
action.
