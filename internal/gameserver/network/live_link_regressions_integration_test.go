package network

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
)

// The tests here are the white-box survivors of the character-flow pilot
// migration (issue #1670): each is triggered by reaching directly into live
// model state (IncreaseCharges, Inventory.AddNew, PAtk comparisons) that no
// client packet can drive, so they cannot become tests/character flows
// without changing observable behavior. They stay at the link level until
// their domains land — charge feedback and inventory weight under
// tests/items (#1671), death-penalty passive restore under tests/combat
// (#1673) — and are deleted with the rest of the package's covered unit
// tests in phase 3 (#1678).

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

// TestGameClientLinkEnterWorldRestoresDeathPenaltyPassiveStats and its
// level-0 sibling pin the death-penalty passive restore to the live
// character's computed stats, which no packet currently exposes (UserInfo
// reports template stats only).
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
