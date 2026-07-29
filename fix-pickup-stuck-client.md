# Adena Pickup Fix

## Issue Description
Clicking on Adena (or Ancient Adena) on the ground after killing mobs results in:
- Client shows "picking" animation briefly
- Item disappears from ground
- Character stops responding to any clicks/commands
- Server logs show connection reset by peer

This bug causes the client to hang waiting for attention notifications that never arrive.

## Root Cause
The `broadcastPickupAttention()` function in `internal/gameserver/network/pickup.go` only broadcasts attention messages for armor and weapon items (lines 113-117):

```go
switch ground.Template.Kind {
case item.KindArmor, item.KindWeapon:
    // broadcast logic
default:
    return // No broadcast for other kinds
}
```

Adena and Ancient Adena are classified as `KindEtcItem`, so they are silently excluded from the attention broadcast. This breaks the protocol expectation that the client receives pickup notifications, causing it to timeout and disconnect.

## Solution
Modified `broadcastPickupAttention()` to explicitly handle currency items:

1. **Early Adena check**: Added a check for `ground.Template.ID == item.AdenaID || ground.Template.ID == item.AncientAdenaID`

2. **Currency attention frame**: For Adena, broadcast using the count-based attention message:
   - Message ID: `SystemMessageAttentionS1PickedUpS2` (1533)
   - Parameters: Player name and item count
   - Example: "Adena picked up by [PlayerName] (500)"

3. **Early return**: Skip the armor/weapon check after handling currency items

## Changes Made

### File: `internal/gameserver/network/pickup.go`

Modified `broadcastPickupAttention()` function (lines 109-162):

**Before**: Only armor/weapon items received attention broadcasts
**After**: Currency items get special handling with count-based attention messages

Key code changes:
- Added Adena detection logic before the armor/weapon check
- For Adena, use `serverpackets.FrameSystemMessageNumber()` with item count
- Return early for currency items to avoid armor/weapon filtering

## Behavior for Different Item Types

### Armor/Weapon Items (KindArmor, KindWeapon)
- Message format: "[PlayerName] picked up [ItemName]"
- Uses `FrameSystemMessageStringItemName()`
- Enchant level support: "[PlayerName] picked up [ItemName] (enchant level X)"

### Adena/Ancient Adena (KindEtcItem, ID=57/5575)
- Message format: "[PlayerName] picked up (count)" (count in parentheses)
- Uses `FrameSystemMessageNumber()` with count
- No enchant level for currency items

## Impact
- **Fixes**: Connection resets when clicking Adena on ground
- **Fixes**: "Accepted action packet answered with nothing" for currency items
- **Preserves**: Existing behavior for armor/weapon items
- **Preserves**: All pickup logic and validation

## Technical Details
- **Adena ID**: 57 (item.AdenaID)
- **Ancient Adena ID**: 5575 (item.AncientAdenaID)
- **Attention message ID**: SystemMessageAttentionS1PickedUpS2 (1533)
- **Broadcast radius**: pickupAttentionRadius (1400 units)
- **Lock duration**: pickupParalyzeLock (200ms after pickup)

## Testing
The existing test `TestGameClientLinkPickupGroundItemFullClientFlow()` needs to be expanded to test Adena pickup (currently only tests armor/weapon items). The fix ensures:
- Client receives proper attention notification for Adena pickup
- Player movement commands continue to work after pickup
- Item is properly removed from ground world state