package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
)

func TestGameClientLinkFullFlow(t *testing.T) {
	c, chars, _, state := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeCharCreateOk {
		t.Fatalf("opcode = %#x, want CharCreateOk (%#x)", reply[0], serverpackets.OpcodeCharCreateOk)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("opcode = %#x, want CharSelectInfo (%#x)", reply[0], serverpackets.OpcodeCharSelectInfo)
	}
	if count := wire.NewReader(reply[1:]).ReadInt32(); count != 1 {
		t.Fatalf("char count = %d, want 1", count)
	}
	objID := chars.soleObjectID(t)

	c.send(encodeRequestGameStart(0))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo (%#x)", reply[0], serverpackets.OpcodeSSQInfo)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected (%#x)", reply[0], serverpackets.OpcodeCharSelected)
	}

	c.send(encodeRequestManorList())
	reply = c.read()
	if reply[0] != serverpackets.OpcodeExtended {
		t.Fatalf("opcode = %#x, want extended packet (%#x)", reply[0], serverpackets.OpcodeExtended)
	}
	if second := wire.NewReader(reply[1:]).ReadUint16(); second != serverpackets.OpcodeExSendManorList {
		t.Fatalf("extended opcode = %#x, want ExSendManorList (%#x)", second, serverpackets.OpcodeExSendManorList)
	}

	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)
	if _, ok := state.Player(objID); !ok {
		t.Fatalf("world.Player(%d) missing after EnterWorld", objID)
	}
	if _, ok := state.Object(objID); !ok {
		t.Fatalf("world.Object(%d) missing after EnterWorld", objID)
	}
}

// TestGameClientLinkEnterWorldRecomputesRestoredWeight is the regression test
// for issue #1144: RestorePlayerInventory rebuilds the inventory from
// persisted rows without queuing update notifications, so totalWeight stays
// 0 unless attachLivePlayer recomputes it, matching the reference's
// ItemList constructor invoking PcInventory.updateWeight() on every send,
// including the one EnterWorld makes at login (ItemList.java:14-24,
// PcInventory.java:101-113, EnterWorld.java:223).
func TestGameClientLinkEnterWorldRecomputesRestoredWeight(t *testing.T) {
	c, chars, _, state := newLinkedGameClientWithSkillsSeed(t, nil, func(chars *fakeCharStore, items *fakeItemStore) {
		objID := seedSelectableCharacter(t, chars, "player1", "Newbie", 1, 0)
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: 500, TemplateID: 9500, OwnerID: objID, Count: 5, Location: item.LocationInventory,
		}); err != nil {
			t.Fatalf("seed item: %v", err)
		}
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected

	objID := chars.soleObjectID(t)

	// attachLivePlayer's explicit weight recompute detects restore's still-
	// zero totalWeight changing to the real carried weight and fires the
	// weight notifier immediately — before EnterWorld's own fixed frame
	// burst — so the login StatusUpdate(CUR_LOAD) arrives first.
	c.send(encodeEnterWorld())
	reply := c.read()
	if reply[0] != serverpackets.OpcodeStatusUpdate {
		t.Fatalf("opcode = %#x, want StatusUpdate (%#x) for the login weight recompute", reply[0], serverpackets.OpcodeStatusUpdate)
	}
	assertStatusAttrs(t, reply, objID, []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusCurrentLoad, Value: 50},
	})
	readEnterWorldBurst(t, c, false)

	live, ok := state.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing after EnterWorld", objID)
	}
	if got := live.(*livePlayer).Inventory().TotalWeight(); got != 50 {
		t.Fatalf("TotalWeight() = %d, want 50", got)
	}
}

// TestGameClientLinkRequestItemListRecomputesWeight covers issue #1144's
// second scope item: the reference's ItemList constructor invokes
// PcInventory.updateWeight() on every send, not only at login
// (ItemList.java:14-24, PcInventory.java:101-113), so RequestItemList must
// recompute totalWeight too. Weight-change notification for an ordinary
// pickup is issue #1137's separate scope, so this test bypasses that path
// entirely and adds the weight directly to the live inventory, the same way
// weight_notifier_test.go's unit-level tests do, to isolate the
// RequestItemList handler's own recompute.
func TestGameClientLinkRequestItemListRecomputesWeight(t *testing.T) {
	c, chars, _, state := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	live, ok := state.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing after EnterWorld", objID)
	}
	live.(*livePlayer).Inventory().AddNew(9500, 5, 501)

	c.send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeStatusUpdate {
		t.Fatalf("opcode = %#x, want StatusUpdate (%#x) for the RequestItemList weight recompute", reply[0], serverpackets.OpcodeStatusUpdate)
	}
	assertStatusAttrs(t, reply, objID, []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusCurrentLoad, Value: 50},
	})
	reply = c.read()
	if reply[0] != serverpackets.OpcodeItemList {
		t.Fatalf("opcode = %#x, want ItemList (%#x)", reply[0], serverpackets.OpcodeItemList)
	}
}

// TestGameClientLinkEnterWorldReGrantsFreeSkills is the regression test for
// issue #1149: Player.giveSkills() runs again on every login, right after
// restoreCharData() (Player.java:4139), so a free level-unlocked grant —
// which a prior in-session level-up handed out in memory only, per
// GiveSkills's own doc comment — comes back on relog instead of staying
// dropped. testTemplates' template grants skill 900001 for free from level
// 50; no other test's character reaches that level, so this is the only
// enter-world path affected by the added grant.
func TestGameClientLinkEnterWorldReGrantsFreeSkills(t *testing.T) {
	skills := skillstate.NewPersistence(nil, skillTable(modelskill.Definition{ID: 900001, Level: 1}))
	c, _, _, _ := newLinkedGameClientWithSkillsSeed(t, skills, func(chars *fakeCharStore, items *fakeItemStore) {
		seedSelectableCharacter(t, chars, "player1", "Newbie", 50, 0)
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, c, false)

	skillList := frames[5]
	if skillList[0] != serverpackets.OpcodeSkillList {
		t.Fatalf("frame[5] opcode = %#x, want SkillList (%#x)", skillList[0], serverpackets.OpcodeSkillList)
	}
	r := wire.NewReader(skillList[1:])
	count := r.ReadInt32()
	for range count {
		if _, level, id := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id == 900001 {
			if level != 1 {
				t.Fatalf("skill 900001 level = %d, want 1", level)
			}
			return
		}
	}
	t.Fatalf("SkillList (%d entries) missing free grant skill 900001 re-derived on login", count)
}

func TestGameClientLinkCreateInvalidNameKeepsConnectionOpen(t *testing.T) {
	c, _, _, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("bad name!", 0, 0, 0, 1, 0, 0))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeCharCreateFail {
		t.Fatalf("opcode = %#x, want CharCreateFail (%#x)", reply[0], serverpackets.OpcodeCharCreateFail)
	}

	// The connection must still be usable: a valid create now succeeds.
	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeCharCreateOk {
		t.Fatalf("opcode = %#x, want CharCreateOk (%#x)", reply[0], serverpackets.OpcodeCharCreateOk)
	}
}

func TestGameClientLinkDeleteAndRestore(t *testing.T) {
	c, chars, _, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo

	objID := chars.soleObjectID(t)

	c.send(encodeRequestCharacterDelete(0))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeCharDeleteOk {
		t.Fatalf("opcode = %#x, want CharDeleteOk (%#x)", reply[0], serverpackets.OpcodeCharDeleteOk)
	}
	c.read() // CharSelectInfo refresh

	if chars.deleteAt(objID) == 0 {
		t.Fatal("expected character to be scheduled for deletion")
	}

	c.send(encodeCharacterRestore(0))
	c.read() // CharSelectInfo refresh

	if chars.deleteAt(objID) != 0 {
		t.Fatal("expected character's scheduled deletion to be cleared")
	}
}
