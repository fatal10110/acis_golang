//go:build integration

package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
)

func TestGameClientLinkFullFlow(t *testing.T) {
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, nil, nil, 0)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeCharCreateOk {
		t.Fatalf("opcode = %#x, want CharCreateOk (%#x)", reply[0], serverpackets.OpcodeCharCreateOk)
	}
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("opcode = %#x, want CharSelectInfo (%#x)", reply[0], serverpackets.OpcodeCharSelectInfo)
	}
	if count := wire.NewReader(reply[1:]).ReadInt32(); count != 1 {
		t.Fatalf("char count = %d, want 1", count)
	}
	objID := sqlSoleObjectID(t, chars)

	c.Send(encodeRequestGameStart(0))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo (%#x)", reply[0], serverpackets.OpcodeSSQInfo)
	}
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected (%#x)", reply[0], serverpackets.OpcodeCharSelected)
	}

	c.Send(encodeRequestManorList())
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeExtended {
		t.Fatalf("opcode = %#x, want extended packet (%#x)", reply[0], serverpackets.OpcodeExtended)
	}
	if second := wire.NewReader(reply[1:]).ReadUint16(); second != serverpackets.OpcodeExSendManorList {
		t.Fatalf("extended opcode = %#x, want ExSendManorList (%#x)", second, serverpackets.OpcodeExSendManorList)
	}

	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)
	if _, ok := state.Player(objID); !ok {
		t.Fatalf("world.Player(%d) missing after EnterWorld", objID)
	}
	if _, ok := state.Object(objID); !ok {
		t.Fatalf("world.Object(%d) missing after EnterWorld", objID)
	}
}

func TestGameClientLinkChargeFeedbackFrames(t *testing.T) {
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, nil, nil, 0)
	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	live, ok := state.Player(sqlSoleObjectID(t, chars))
	if !ok {
		t.Fatal("world player missing after EnterWorld")
	}
	character := live.(*livePlayer).Character

	character.IncreaseCharges(2, 5)
	assertForceChargeMessage(t, c.Read(), serverpackets.SystemMessageForceIncreasedToS1, 2)
	if frame := c.Read(); frame[0] != serverpackets.OpcodeEtcStatusUpdate {
		t.Fatalf("partial-add frame opcode = %#x, want EtcStatusUpdate (%#x)", frame[0], serverpackets.OpcodeEtcStatusUpdate)
	}

	character.IncreaseCharges(3, 5)
	assertForceChargeMessage(t, c.Read(), serverpackets.SystemMessageForceMaxLevelReached, 0)
	if frame := c.Read(); frame[0] != serverpackets.OpcodeEtcStatusUpdate {
		t.Fatalf("clamped-add frame opcode = %#x, want EtcStatusUpdate (%#x)", frame[0], serverpackets.OpcodeEtcStatusUpdate)
	}

	character.IncreaseCharges(1, 5)
	assertForceChargeMessage(t, c.Read(), serverpackets.SystemMessageForceMaxLevelReached, 0)
}

// TestGameClientLinkEnterWorldRecomputesRestoredWeight is the regression test
// for issue #1144: RestorePlayerInventory rebuilds the inventory from
// persisted rows without queuing update notifications, so totalWeight stays
// 0 unless attachLivePlayer recomputes it, matching the reference's
// ItemList constructor invoking PcInventory.updateWeight() on every send,
// including the one EnterWorld makes at login (ItemList.java:14-24,
// PcInventory.java:101-113, EnterWorld.java:223).
func TestGameClientLinkEnterWorldRecomputesRestoredWeight(t *testing.T) {
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, nil, func(chars *gamesql.CharacterStore, items *gamesql.ItemStore) {
		objID := seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 1, 0).ID
		if err := items.Create(context.Background(), objID, item.Instance{
			ObjectID: 500, TemplateID: 9500, OwnerID: objID, Count: 5, Location: item.LocationInventory,
		}); err != nil {
			t.Fatalf("seed item: %v", err)
		}
	}, 1)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected

	objID := sqlSoleObjectID(t, chars)

	// attachLivePlayer's explicit weight recompute detects restore's still-
	// zero totalWeight changing to the real carried weight and fires the
	// weight notifier immediately — before EnterWorld's own fixed frame
	// burst — so the login StatusUpdate(CUR_LOAD) arrives first.
	c.Send(encodeEnterWorld())
	reply := c.Read()
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
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, nil, nil, 0)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objID := sqlSoleObjectID(t, chars)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	live, ok := state.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing after EnterWorld", objID)
	}
	live.(*livePlayer).Inventory().AddNew(9500, 5, 501)

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeStatusUpdate {
		t.Fatalf("opcode = %#x, want StatusUpdate (%#x) for the RequestItemList weight recompute", reply[0], serverpackets.OpcodeStatusUpdate)
	}
	assertStatusAttrs(t, reply, objID, []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusCurrentLoad, Value: 50},
	})
	reply = c.Read()
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
	c, _, _, _, _, _ := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 50, 0)
	}, 1)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
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

func TestGameClientLinkEnterWorldRestoresDeathPenaltyPassiveStats(t *testing.T) {
	skills := skillstate.NewPersistence(nil, skillTable(modelskill.Definition{
		ID: 5076, Level: 2, Activation: modelskill.ActivationPassive,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 7}},
	}))
	var basePAtk float64
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		ch := seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 1, 0)
		ch.SetDeathPenaltyLevel(2)
		basePAtk = ch.PAtk()
		if err := chars.Save(context.Background(), ch); err != nil {
			t.Fatalf("save death penalty: %v", err)
		}
	}, 1)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	live, ok := state.Player(sqlSoleObjectID(t, chars))
	if !ok {
		t.Fatal("world player missing after EnterWorld")
	}
	character := live.(*livePlayer).Character
	if got, want := character.PAtk(), basePAtk+7; got != want {
		t.Fatalf("PAtk() after restoring death penalty = %v, want %v", got, want)
	}
	if got := character.SkillLevel(5076); got != 0 {
		t.Fatalf("SkillLevel(5076) = %d, want 0 for transient death penalty", got)
	}
}

func TestGameClientLinkEnterWorldSkipsDeathPenaltyPassiveAtZero(t *testing.T) {
	skills := skillstate.NewPersistence(nil, skillTable(modelskill.Definition{
		ID: 5076, Level: 1, Activation: modelskill.ActivationPassive,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 7}},
	}))
	var basePAtk float64
	c, chars, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		ch := seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 1, 0)
		basePAtk = ch.PAtk()
	}, 1)

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	live, ok := state.Player(sqlSoleObjectID(t, chars))
	if !ok {
		t.Fatal("world player missing after EnterWorld")
	}
	character := live.(*livePlayer).Character
	if got := character.PAtk(); got != basePAtk {
		t.Fatalf("PAtk() at death penalty level 0 = %v, want %v", got, basePAtk)
	}
	if got := character.SkillLevel(5076); got != 0 {
		t.Fatalf("SkillLevel(5076) = %d, want 0 for transient death penalty", got)
	}
}

func TestGameClientLinkCreateInvalidNameKeepsConnectionOpen(t *testing.T) {
	c, _, _, _, _, _ := newLinkedSQLGameClient(t, nil, nil, 0)

	c.Send(encodeRequestCharacterCreate("bad name!", 0, 0, 0, 1, 0, 0))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeCharCreateFail {
		t.Fatalf("opcode = %#x, want CharCreateFail (%#x)", reply[0], serverpackets.OpcodeCharCreateFail)
	}

	// The connection must still be usable: a valid create now succeeds.
	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeCharCreateOk {
		t.Fatalf("opcode = %#x, want CharCreateOk (%#x)", reply[0], serverpackets.OpcodeCharCreateOk)
	}
}

func TestGameClientLinkDeleteAndRestore(t *testing.T) {
	c, chars, _, _, _, _ := newLinkedSQLGameClient(t, nil, nil, 0)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo

	objID := sqlSoleObjectID(t, chars)

	c.Send(encodeRequestCharacterDelete(0))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeCharDeleteOk {
		t.Fatalf("opcode = %#x, want CharDeleteOk (%#x)", reply[0], serverpackets.OpcodeCharDeleteOk)
	}
	c.Read() // CharSelectInfo refresh

	ch, err := chars.Get(context.Background(), objID)
	if err != nil {
		t.Fatalf("load deleted character: %v", err)
	}
	if ch.DeleteAt == 0 {
		t.Fatal("expected character to be scheduled for deletion")
	}

	c.Send(encodeCharacterRestore(0))
	c.Read() // CharSelectInfo refresh

	ch, err = chars.Get(context.Background(), objID)
	if err != nil {
		t.Fatalf("load restored character: %v", err)
	}
	if ch.DeleteAt != 0 {
		t.Fatal("expected character's scheduled deletion to be cleared")
	}
}

func seedSelectableSQLCharacter(t *testing.T, chars *gamesql.CharacterStore, account, name string, level, sp int) *player.Character {
	t.Helper()
	tmpl, ok := testTemplates(t).Get(0)
	if !ok {
		t.Fatal("missing test class template")
	}
	ch, err := player.NewCharacter(100, tmpl, account, name, 1, 0, 0, player.SexMale)
	if err != nil {
		t.Fatalf("seed character: %v", err)
	}
	ch.CharLevel = level
	ch.SP = sp
	if err := chars.Create(context.Background(), ch); err != nil {
		t.Fatalf("seed character store: %v", err)
	}
	return ch
}

func sqlSoleObjectID(t *testing.T, chars *gamesql.CharacterStore) int32 {
	t.Helper()
	characters, err := chars.ListByAccount(context.Background(), "player1")
	if err != nil {
		t.Fatalf("list characters: %v", err)
	}
	if len(characters) != 1 {
		t.Fatalf("character count = %d, want 1", len(characters))
	}
	return characters[0].ID
}
