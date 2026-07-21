package network

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// readWithTimeout reads one frame within d, returning nil on timeout instead
// of failing the test. Used by TestGameClientLinkNeverGoesSilentOnActionRequests
// to drain however many frames a rejected request produces (a system message
// plus ActionFailed, or ActionFailed alone) without hard-coding an exact
// count, while still treating "nothing at all" as a failure.
func (f *fakeGameClient) readWithTimeout(d time.Duration) []byte {
	f.t.Helper()
	f.conn.SetReadDeadline(time.Now().Add(d))
	payload, err := wire.ReadFrame(f.conn)
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return nil
		}
		f.t.Fatalf("ReadFrame: %v", err)
	}
	if f.cipher != nil {
		f.cipher.Decrypt(payload)
	}
	return payload
}

// TestGameClientLinkNeverGoesSilentOnActionRequests is the guardrail against
// the bug class behind #828/#829/#873: an accepted client action packet that
// a handler quietly drops, leaving the client's pending action unresolved —
// which presented as a character that walks up to a target and freezes, a
// picked-up item that never leaves the ground, or an item-window click that
// does nothing. Every case here sends a request built to be rejected (a
// nonexistent object id, an unclaimed action id, a command with no target to
// act on) and asserts at least one frame comes back. It intentionally does
// not assert which frame — the point is that the handler answered at all,
// not what it said — so this test keeps working (and keeps catching
// regressions) as new rejection reasons and messages are added.
func TestGameClientLinkNeverGoesSilentOnActionRequests(t *testing.T) {
	c, chars, _, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	chars.soleObjectID(t)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	const missingObjectID = 999999

	cases := []struct {
		name    string
		payload []byte
	}{
		{"UseItem on an object the player doesn't hold", encodeUseItem(missingObjectID, false)},
		{"RequestEnchantItem on an object the player doesn't hold", encodeRequestEnchantItem(missingObjectID)},
		{"RequestDestroyItem on an object the player doesn't hold", encodeRequestDestroyItem(missingObjectID, 1)},
		{"RequestCrystallizeItem on an object the player doesn't hold", encodeRequestCrystallizeItem(missingObjectID, 1)},
		{"RequestDropItem on an object the player doesn't hold", encodeRequestDropItem(missingObjectID, 1, location.Location{})},
		{"RequestActionUse with an action id no handler claims", encodeRequestActionUse(9999, false, false)},
		{"RequestActionUse pet command with no active summon", encodeRequestActionUse(16, false, false)},
		// Fishing trainer info for a skill the trainer can't offer: the
		// general trainer branch already answered with NoMoreSkillsToLearn,
		// the fishing branch used to drop the request silently. Same
		// trainer-window rejection, now the same reply.
		{"RequestAcquireSkillInfo fishing for a skill the player can't obtain", encodeRequestAcquireSkillInfo(99999, 1, 1)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c.send(tc.payload)
			// The first read is the actual assertion: it must prove the
			// server answered at all, so it gets a generous timeout. Any
			// further frames from the same rejection (e.g. a system
			// message followed by ActionFailed) are already in flight by
			// the time the first one arrives, so a short timeout is
			// enough to drain them before the next case's send.
			first := c.readWithTimeout(2 * time.Second)
			if first == nil {
				t.Fatalf("%s: no reply at all — the request was silently dropped, leaving the client's action unresolved", tc.name)
			}
			for c.readWithTimeout(100*time.Millisecond) != nil {
			}
		})
	}
}

// TestGameClientLinkUseItemPotionRejectionReplies is the silent-drop
// guardrail for the consumable-use branch useItem now dispatches. A
// healing potion whose reuse delay is still cooling must still answer the
// client (a reuse system message plus ActionFailed) rather than going
// quiet, the way the #873 guardrail already requires of every other
// rejected item-window action. The first use succeeds and installs the
// reuse; the second use, sent while the reuse window is open, is the case
// under test.
func TestGameClientLinkUseItemPotionRejectionReplies(t *testing.T) {
	skills := consumableSkillTable(t)
	const objectID int32 = 720
	c, chars, _, _ := newLinkedGameClientWithSkillsSeed(t, skills, func(chars *fakeCharStore, items *fakeItemStore) {
		objID := seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 0)
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: objectID, TemplateID: 1060, OwnerID: objID,
			Count: 5, Location: item.LocationInventory, ManaLeft: -1,
		}); err != nil {
			t.Fatalf("seed potion: %v", err)
		}
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	// First use succeeds: drains the InventoryUpdate, MagicSkillUse and
	// USE_S1 frames so the second use's reply is the next thing read.
	c.send(encodeUseItem(objectID, false))
	for c.readWithTimeout(time.Second) != nil {
	}

	c.send(encodeUseItem(objectID, false))
	first := c.readWithTimeout(2 * time.Second)
	if first == nil {
		t.Fatal("reuse-rejected potion use produced no reply at all — the request was silently dropped")
	}
	for c.readWithTimeout(100*time.Millisecond) != nil {
	}

	_ = chars // keep the character-store handle live for the seeded player
}
