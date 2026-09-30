package items

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	sqltest "github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The dual sword rig: the fixture's D-grade sword (template 30) carrying a
// +100 maxHp equip modifier, a +10 maxHp passive item skill and a +50 maxHp
// passive +4 enchant skill, so each of them shows in the live MaxHP and the
// two skills in SkillList.
const (
	dualRigHPBonus        = 100
	dualRigItemSkillID    = 3099
	dualRigItemSkillHP    = 10
	dualRigEnchantSkillID = 3098
	dualRigEnchantSkillHP = 50
)

// bootDualRig boots a character that knows Expertise and holds the dual
// sword rig at weaponEnchant plus one scroll (blessed when asked) and
// returns the server with the character, weapon and scroll object ids.
func bootDualRig(t *testing.T, roll float64, weaponEnchant int, blessed bool) (*gameservertest.Server, int32, int32, int32) {
	t.Helper()
	var templates []*item.Template
	for _, tmpl := range gameservertest.ItemTemplates().All() {
		if tmpl.ID == riggedSwordID {
			rigged := *tmpl
			weapon := *tmpl.Weapon
			weapon.Enchant4Skill = &item.SkillRef{ID: dualRigEnchantSkillID, Level: 1}
			rigged.Weapon = &weapon
			rigged.Modifiers = []item.StatModifier{{Op: item.FuncAdd, Stat: "maxHp", Value: dualRigHPBonus}}
			rigged.AttachedSkills = []item.SkillRef{{ID: dualRigItemSkillID, Level: 1}}
			tmpl = &rigged
		}
		templates = append(templates, tmpl)
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: expertiseSkillID, Level: 1},
		{ID: dualRigItemSkillID, Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "maxHp", Value: dualRigItemSkillHP},
		}},
		{ID: dualRigEnchantSkillID, Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "maxHp", Value: dualRigEnchantSkillHP},
		}},
	}), gamesql.NewCharacterSkillStore(db))
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithSkills(skills),
		gameservertest.WithEnchantRoll(func() float64 { return roll }),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	objID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, expertiseSkillID, 1); err != nil {
		t.Fatalf("grant Expertise: %v", err)
	}
	weapon := srv.GiveItem(t, objID, riggedSwordID, 1)
	if weaponEnchant > 0 {
		inst := mustFindItem(t, srv, objID, weapon)
		inst.EnchantLevel = weaponEnchant
		if err := srv.Items.Update(context.Background(), inst); err != nil {
			t.Fatalf("seed enchant level: %v", err)
		}
	}
	scrollTemplate := int32(955)
	if blessed {
		scrollTemplate = 6575
	}
	return srv, objID, weapon, srv.GiveItem(t, objID, scrollTemplate, 1)
}

// wearDualRig enters the world, equips the rig and returns the MaxHP the
// character had before, and whether the equip SkillList listed the +4
// enchant skill.
func wearDualRig(t *testing.T, srv *gameservertest.Server, objID, weapon int32) (baseHP int, enchantSkill bool) {
	t.Helper()
	c := srv.Client
	startInWorld(t, c)
	baseHP = srv.PlayerMaxHP(t, objID)
	c.Send(encodeUseItem(weapon, false))
	for _, f := range collectUntilQuiet(t, c) {
		if f[0] == serverpackets.OpcodeSkillList && skillListHas(t, f, dualRigEnchantSkillID) {
			enchantSkill = true
		}
	}
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	return baseHP, enchantSkill
}

func skillListHas(t *testing.T, frame []byte, id int32) bool {
	t.Helper()
	for _, e := range readSkillList(t, frame) {
		if e.id == id {
			return true
		}
	}
	return false
}

// frameIndex returns the index of the first frame matching opcode (and
// system message id, when message is not zero) at or after from, or -1.
func frameIndex(t *testing.T, frames [][]byte, from int, opcode byte, message int) int {
	t.Helper()
	for i := from; i < len(frames); i++ {
		if frames[i][0] != opcode {
			continue
		}
		if message != 0 && systemMessageID(t, frames[i]) != message {
			continue
		}
		return i
	}
	return -1
}

func opcodes(frames [][]byte) []byte {
	out := make([]byte, 0, len(frames))
	for _, f := range frames {
		out = append(out, f[0])
	}
	return out
}

// TestEnchantWornWeaponEnchantSkill pins the weapon +4 enchant skill as a
// real, live-granted skill (RequestEnchantItem, ItemPassiveSkillsListener):
// a success taking the worn weapon to exactly +4 adds it and resends
// SkillList ahead of EnchantResult; a failure at +4 or higher removes it and
// resends SkillList ahead of the failure's own messages; and equipping at +4
// grants it.
func TestEnchantWornWeaponEnchantSkill(t *testing.T) {
	t.Parallel()
	t.Run("success to +4 grants it", func(t *testing.T) {
		srv, objID, weapon, scroll := bootDualRig(t, 0, 3, false)
		c := srv.Client
		baseHP, granted := wearDualRig(t, srv, objID, weapon)
		if granted {
			t.Fatal("equip at +3 listed the +4 enchant skill")
		}
		if got, want := srv.PlayerMaxHP(t, objID), baseHP+dualRigHPBonus+dualRigItemSkillHP; got != want {
			t.Fatalf("MaxHP worn at +3 = %d, want %d", got, want)
		}

		openEnchantSelection(t, c, scroll, 955)
		c.Send(encodeRequestEnchantItem(weapon))
		frames := collectUntilQuiet(t, c)
		msg := frameIndex(t, frames, 0, serverpackets.OpcodeSystemMessage, serverpackets.SystemMessageS1S2SuccessfullyEnchanted)
		skillList := frameIndex(t, frames, 0, serverpackets.OpcodeSkillList, 0)
		result := frameIndex(t, frames, 0, serverpackets.OpcodeEnchantResult, 0)
		if msg != 0 || skillList < msg || result < skillList {
			t.Fatalf("frames %x: want the success message, then SkillList, then EnchantResult", opcodes(frames))
		}
		if !skillListHas(t, frames[skillList], dualRigEnchantSkillID) {
			t.Fatal("SkillList after reaching +4 does not list the +4 enchant skill")
		}
		assertEnchantResult(t, frames[result], serverpackets.EnchantResultSuccess)
		if got, want := srv.PlayerMaxHP(t, objID), baseHP+dualRigHPBonus+dualRigItemSkillHP+dualRigEnchantSkillHP; got != want {
			t.Fatalf("MaxHP worn at +4 = %d, want %d", got, want)
		}
	})

	t.Run("blessed failure at +4 revokes it", func(t *testing.T) {
		srv, objID, weapon, scroll := bootDualRig(t, 0.99, 4, true)
		c := srv.Client
		baseHP, granted := wearDualRig(t, srv, objID, weapon)
		if !granted {
			t.Fatal("equip at +4 did not list the +4 enchant skill")
		}
		if got, want := srv.PlayerMaxHP(t, objID), baseHP+dualRigHPBonus+dualRigItemSkillHP+dualRigEnchantSkillHP; got != want {
			t.Fatalf("MaxHP worn at +4 = %d, want %d", got, want)
		}

		openEnchantSelection(t, c, scroll, 6575)
		c.Send(encodeRequestEnchantItem(weapon))
		frames := collectUntilQuiet(t, c)
		skillList := frameIndex(t, frames, 0, serverpackets.OpcodeSkillList, 0)
		msg := frameIndex(t, frames, 0, serverpackets.OpcodeSystemMessage, serverpackets.SystemMessageBlessedEnchantFailed)
		result := frameIndex(t, frames, 0, serverpackets.OpcodeEnchantResult, 0)
		if skillList < 0 || msg < skillList || result < msg {
			t.Fatalf("frames %x: want SkillList, then the blessed failure message, then EnchantResult", opcodes(frames))
		}
		if skillListHas(t, frames[skillList], dualRigEnchantSkillID) {
			t.Fatal("SkillList after the failure still lists the +4 enchant skill")
		}
		assertEnchantResult(t, frames[result], serverpackets.EnchantResultUnsuccess)
		if got, want := srv.PlayerMaxHP(t, objID), baseHP+dualRigHPBonus+dualRigItemSkillHP; got != want {
			t.Fatalf("MaxHP worn at +0 = %d, want %d", got, want)
		}
	})
}

// TestEnchantBreakOnWornWeaponUndoesEquipSideEffects pins a normal scroll
// breaking the worn weapon: the +4 skill goes first with its SkillList, then
// the destroyed weapon comes off the paperdoll with its modifier and item
// skill — a second SkillList, as the unequip listener sends one for a +4
// weapon — all before the crystal reward, and none of its stats stay on
// the character.
func TestEnchantBreakOnWornWeaponUndoesEquipSideEffects(t *testing.T) {
	t.Parallel()
	srv, objID, weapon, scroll := bootDualRig(t, 0.99, 4, false)
	c := srv.Client
	baseHP, _ := wearDualRig(t, srv, objID, weapon)

	openEnchantSelection(t, c, scroll, 955)
	c.Send(encodeRequestEnchantItem(weapon))
	frames := collectUntilQuiet(t, c)
	earned := frameIndex(t, frames, 0, serverpackets.OpcodeSystemMessage, serverpackets.SystemMessageEarnedS2S1S)
	if earned < 0 {
		t.Fatalf("frames %x: no crystal reward message", opcodes(frames))
	}
	var skillLists []int
	for i := range frames[:earned] {
		if frames[i][0] == serverpackets.OpcodeSkillList {
			skillLists = append(skillLists, i)
		}
	}
	if len(skillLists) != 2 {
		t.Fatalf("frames %x: want two SkillLists ahead of the crystal reward (+4 skill removal, then the unequip)", opcodes(frames))
	}
	last := frames[skillLists[1]]
	if skillListHas(t, last, dualRigEnchantSkillID) || skillListHas(t, last, dualRigItemSkillID) {
		t.Fatal("SkillList after the weapon broke still lists one of its skills")
	}
	if result := frameIndex(t, frames, earned, serverpackets.OpcodeEnchantResult, 0); result < 0 {
		t.Fatalf("frames %x: no EnchantResult after the reward", opcodes(frames))
	} else {
		assertEnchantResult(t, frames[result], serverpackets.EnchantResultBrokenWithCrystals)
	}
	if got := srv.PlayerMaxHP(t, objID); got != baseHP {
		t.Fatalf("MaxHP after the worn weapon broke = %d, want the unarmed %d", got, baseHP)
	}
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	assertItemGone(t, srv, objID, weapon)
}

// TestEnchantRequestGates pins the RequestEnchantItem gates that do not
// depend on the target: a store or trade in progress refuses the request
// and drops the selection; a scroll that left the inventory is no selection
// at all, so the request is silent and the next scroll opens with the
// selection prompt again; a dead player may still enchant.
func TestEnchantRequestGates(t *testing.T) {
	t.Parallel()
	t.Run("operating a store refuses and clears the selection", func(t *testing.T) {
		srv, objID, weapon, scroll := bootEnchanter(t, func() float64 { return 0 }, 0, false, nil)
		c := srv.Client
		openEnchantSelection(t, c, scroll, 955)
		srv.SetPlayerOperating(t, objID, true)
		c.Send(encodeRequestEnchantItem(weapon))
		assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotEnchantWhileStore)
		assertEnchantResult(t, c.Read(), serverpackets.EnchantResultCancelled)

		srv.SetPlayerOperating(t, objID, false)
		c.Send(encodeRequestEnchantItem(weapon))
		if reply := c.ReadWithTimeout(300 * time.Millisecond); reply != nil {
			t.Fatalf("enchant after the store refusal replied %x, want no reply", reply)
		}
		if inst := mustFindItem(t, srv, objID, scroll); inst.Count != 1 {
			t.Fatalf("scroll count after the store refusal = %d, want 1", inst.Count)
		}
	})

	t.Run("a destroyed scroll is no selection", func(t *testing.T) {
		srv, objID, weapon, blessed := bootDestroyedSelection(t)
		c := srv.Client
		c.Send(encodeRequestEnchantItem(weapon))
		if reply := c.ReadWithTimeout(300 * time.Millisecond); reply != nil {
			t.Fatalf("enchant with the selected scroll destroyed replied %x, want no reply", reply)
		}
		openEnchantSelection(t, c, blessed, 6575)
		if inst := mustFindItem(t, srv, objID, weapon); inst.EnchantLevel != 0 {
			t.Fatalf("weapon enchant = %d, want 0", inst.EnchantLevel)
		}
	})

	// PcInventory.removeItem drops the selection with the scroll, so
	// RequestRestart finds no active enchant item and lets the player out.
	t.Run("a destroyed scroll does not block restart", func(t *testing.T) {
		srv, _, _, _ := bootDestroyedSelection(t)
		c := srv.Client
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
		for {
			reply := c.Read()
			if len(reply) == 0 || reply[0] != serverpackets.OpcodeRestartResponse {
				continue
			}
			if ok := wire.NewReader(reply[1:]).ReadInt32(); ok != 1 {
				t.Fatalf("RestartResponse result = %d, want 1 (the destroyed scroll is no selection)", ok)
			}
			return
		}
	})

	t.Run("a dead player still enchants", func(t *testing.T) {
		srv, objID, weapon, scroll := bootEnchanter(t, func() float64 { return 0 }, 0, false, nil)
		c := srv.Client
		openEnchantSelection(t, c, scroll, 955)
		srv.MarkPlayerDead(t, objID)
		c.Send(encodeRequestEnchantItem(weapon))
		frame := c.Read()
		if id := systemMessageID(t, frame); id != serverpackets.SystemMessageS1SuccessfullyEnchanted {
			t.Fatalf("message id = %d, want S1SuccessfullyEnchanted (%d)", id, serverpackets.SystemMessageS1SuccessfullyEnchanted)
		}
		assertEnchantResult(t, c.Read(), serverpackets.EnchantResultSuccess)
	})
}

// bootDestroyedSelection boots an enchanter who selects their only scroll
// 955 and then destroys it. It returns the server, the player, the weapon
// and a held blessed scroll 6575.
func bootDestroyedSelection(t *testing.T) (*gameservertest.Server, int32, int32, int32) {
	t.Helper()
	var templates []*item.Template
	for _, tmpl := range gameservertest.ItemTemplates().All() {
		if tmpl.ID == 955 {
			destroyable := *tmpl
			destroyable.Destroyable = true
			tmpl = &destroyable
		}
		templates = append(templates, tmpl)
	}
	var blessed int32
	srv, objID, weapon, scroll := bootEnchanter(t, func() float64 { return 0 }, 0, false, func(t *testing.T, srv *gameservertest.Server, objID int32) {
		blessed = srv.GiveItem(t, objID, 6575, 1)
	}, gameservertest.WithItemTemplates(item.NewTable(templates)))
	c := srv.Client
	openEnchantSelection(t, c, scroll, 955)
	c.Send(encodeRequestDestroyItem(scroll, 1))
	drainUntilQuiet(t, c)
	if held := srv.PlayerInventory(t, objID).ItemByObjectID(scroll); held != nil {
		t.Fatal("the scroll is still held after the destroy")
	}
	return srv, objID, weapon, blessed
}

// TestTeleportCancelsActiveEnchant pins Player.teleportTo dropping the
// scroll selection once the teleport is under way.
func TestTeleportCancelsActiveEnchant(t *testing.T) {
	t.Parallel()
	srv, objID, weapon, scroll := bootEnchanter(t, func() float64 { return 0 }, 0, false, nil)
	c := srv.Client
	openEnchantSelection(t, c, scroll, 955)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	character, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	x, y, z := srv.PlayerPosition(t, objID)
	character.TeleportTo(x+2_000, y, z, 0)
	frames := collectUntilQuiet(t, c)
	teleport := frameIndex(t, frames, 0, serverpackets.OpcodeTeleportToLocation, 0)
	result := frameIndex(t, frames, 0, serverpackets.OpcodeEnchantResult, 0)
	cancelled := frameIndex(t, frames, 0, serverpackets.OpcodeSystemMessage, serverpackets.SystemMessageEnchantScrollCancelled)
	if teleport < 0 || result < teleport || cancelled != result+1 {
		t.Fatalf("frames %x: want TeleportToLocation, then EnchantResult and ENCHANT_SCROLL_CANCELLED", opcodes(frames))
	}
	assertEnchantResult(t, frames[result], serverpackets.EnchantResultCancelled)

	c.Send(encodeRequestEnchantItem(weapon))
	if reply := c.ReadWithTimeout(300 * time.Millisecond); reply != nil {
		t.Fatalf("enchant after the teleport replied %x, want no reply", reply)
	}
}
