package items

import (
	"slices"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// riggedSwordPassiveHP is the maxHp bonus of the rigged sword's item skill
// in TestItemPassiveStatChangesSendOwnUserInfo.
const riggedSwordPassiveHP = 50

// statUserInfoHPs returns the MaxHP of every UserInfo ahead of the first
// SkillList among frames: the stat refreshes the equip listeners send
// before the item-skill listener's SkillList.
func statUserInfoHPs(t *testing.T, frames [][]byte) []int32 {
	t.Helper()
	var hps []int32
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeSkillList:
			return hps
		case serverpackets.OpcodeUserInfo:
			hps = append(hps, userInfoMaxHP(t, f))
		}
	}
	t.Fatalf("no SkillList among %d frames", len(frames))
	return nil
}

// TestItemPassiveStatChangesSendOwnUserInfo equips and unequips a sword
// with a maxHp modifier and an item skill whose passive adds maxHp too.
// The item's own funcs and its skill's funcs have separate owners (the
// ItemInstance and the L2Skill): StatsListener runs ahead of
// ItemPassiveSkillsListener (Inventory.java:64, PcInventory.java:42-45), and
// each stat change sends its own UserInfo (Creature.addStatFuncs /
// removeStatsByOwner -> broadcastModifiedStats -> updateAndBroadcastStatus(1),
// Player.addSkill / removeSkill, Player.java:4541-4625). Equipping sends the
// item's UserInfo, then the skill's; unequipping sends one with only the
// passive left, then one with neither, both ahead of the SkillList.
func TestItemPassiveStatChangesSendOwnUserInfo(t *testing.T) {
	t.Parallel()
	srv, sword := bootRiggedSword(t, modelskill.FuncTemplate{Op: modelskill.FuncAdd, Stat: "maxHp", Value: riggedSwordPassiveHP})
	c := srv.Client
	baseHP := int32(-1)
	for _, f := range startInWorld(t, c) {
		if f[0] == serverpackets.OpcodeUserInfo {
			baseHP = userInfoMaxHP(t, f)
		}
	}
	if baseHP < 0 {
		t.Fatal("EnterWorld burst has no UserInfo")
	}

	c.Send(encodeUseItem(sword, false))
	on := collectUntilQuiet(t, c)
	if got, want := statUserInfoHPs(t, on), []int32{baseHP + riggedSwordHPBonus, baseHP + riggedSwordHPBonus + riggedSwordPassiveHP}; !slices.Equal(got, want) {
		t.Fatalf("equip stat UserInfo MaxHPs = %v, want %v (item, then its skill)", got, want)
	}
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)

	c.Send(encodeUseItem(sword, false))
	off := collectUntilQuiet(t, c)
	if got, want := statUserInfoHPs(t, off), []int32{baseHP + riggedSwordPassiveHP, baseHP}; !slices.Equal(got, want) {
		t.Fatalf("unequip stat UserInfo MaxHPs = %v, want %v (item gone with the passive left, then the passive gone)", got, want)
	}
	if got := readEquipSideEffects(t, off); got.swordSkill || got.userInfoHP != baseHP {
		t.Fatalf("unequip SkillList lists item skill=%v, closing UserInfo MaxHP=%d; want no item skill and base %d", got.swordSkill, got.userInfoHP, baseHP)
	}
}
