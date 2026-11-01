package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// TestFrameCharInfoAndUserInfoWriteLiveSpeedMultipliers pins
// CharInfo.java:102-103 and UserInfo.java:152-153: both write the live
// movement and attack speed multipliers as doubles of their float values,
// ahead of the collision block, so a speed buff reaches the client.
func TestFrameCharInfoAndUserInfoWriteLiveSpeedMultipliers(t *testing.T) {
	tmpl := &player.Template{
		DEX: 30, RunSpeed: 120, WalkSpeed: 80, SwimSpeed: 50,
		CollisionRadius: 9, CollisionHeight: 23,
	}
	c := &player.Character{ID: 0x10000001, Race: player.RaceHuman}
	c.SetName("Observer")
	c.AttachRuntime(tmpl, nil)
	c.AddStatFuncs([]effect.Mod{
		{Stat: stat.RunSpeed, Op: effect.OpAdd, Value: 33},
		{Stat: stat.PowerAttackSpeed, Op: effect.OpMul, Value: 1.2},
	})
	move := float32(120*statbonus.DEXBonus[tmpl.DEX]+33) / 120
	attack := float32(1.1 * float64(c.AttackSpeed()) / 300)
	if move <= 1 || attack <= 1.1 {
		t.Fatalf("buffed multipliers = (%v, %v), want both above their unbuffed values", move, attack)
	}
	want := appendF64(nil, float64(move))
	want = appendF64(want, float64(attack))
	want = appendF64(want, tmpl.CollisionRadius)
	want = appendF64(want, tmpl.CollisionHeight)

	for name, got := range map[string][]byte{
		"CharInfo": framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: tmpl})),
		"UserInfo": framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: tmpl})),
	} {
		if !bytes.Contains(got, want) {
			t.Fatalf("%s missing multiplier/collision block %x", name, want)
		}
	}
}

// ---- from relationchanged_test.go ----
func TestFrameRelationChanged(t *testing.T) {
	got := framePayload(t, FrameRelationChanged(RelationChangedInfo{
		ObjectID:         12345,
		Relation:         RelationPvPFlag | RelationHasKarma,
		IsAutoAttackable: true,
		Karma:            500,
		PvPFlag:          1,
	}))

	want := []byte{OpcodeRelationChanged}
	for _, v := range []uint32{12345, RelationPvPFlag | RelationHasKarma, 1, 500, 1} {
		want = binary.LittleEndian.AppendUint32(want, v)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameRelationChanged() = %x, want %x", got, want)
	}
}

// liveStatsCharacter is a level 1 fighter wearing a weapon, a chest piece
// and a ring, with a buff-shaped stat func on every status-window stat, for
// the UserInfo/CharInfo live stat oracles.
func liveStatsCharacter() (*player.Character, *player.Template) {
	tmpl := &player.Template{
		FistsItemID: 1,
		STR:         40, CON: 43, DEX: 30, INT: 21, WIT: 11, MEN: 25,
		PAtk: 4, PDef: 30, MAtk: 3, MDef: 15,
		RunSpeed: 120, WalkSpeed: 80, SwimSpeed: 50,
		CollisionRadius: 9, CollisionHeight: 23,
	}
	items := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponFist}},
		{ID: 2, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponSword}, Modifiers: []item.StatModifier{
			{Op: item.FuncSet, Stat: "pAtk", Value: 100},
			{Op: item.FuncSet, Stat: "mAtk", Value: 50},
			{Op: item.FuncSet, Stat: "pAtkSpd", Value: 433},
			{Op: item.FuncSet, Stat: "rCrit", Value: 7},
		}},
		{ID: 3, Kind: item.KindArmor, Slot: item.SlotChest, Armor: &item.ArmorDetail{}},
		{ID: 4, Kind: item.KindArmor, Slot: item.SlotLFinger, Armor: &item.ArmorDetail{}},
	})
	c := &player.Character{ID: 0x10000001, Race: player.RaceHuman, CharLevel: 1}
	c.SetName("Buffed")
	c.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventory(c.ID, items, []*item.Instance{
		{ObjectID: 100, TemplateID: 2, Count: 1, Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand},
		{ObjectID: 101, TemplateID: 3, Count: 1, Location: item.LocationPaperdoll, LocationData: itemcontainer.Chest},
		{ObjectID: 102, TemplateID: 4, Count: 1, Location: item.LocationPaperdoll, LocationData: itemcontainer.LFinger},
	}))
	c.AddStatFuncs([]effect.Mod{
		{Stat: stat.StatSTR, Op: effect.OpAdd, Value: 2},
		{Stat: stat.PowerAttack, Op: effect.OpMul, Value: 1.2},
		{Stat: stat.PowerDefence, Op: effect.OpBaseAdd, Value: 50},
		{Stat: stat.PowerAttackSpeed, Op: effect.OpMul, Value: 1.33},
		{Stat: stat.EvasionRate, Op: effect.OpAdd, Value: 3},
		{Stat: stat.AccuracyCombat, Op: effect.OpAdd, Value: 2},
		{Stat: stat.CriticalRate, Op: effect.OpMul, Value: 1.5},
		{Stat: stat.MagicAttackSpeed, Op: effect.OpMul, Value: 1.3},
	})
	return c, tmpl
}

// TestFrameUserInfoWritesLiveStats pins UserInfo.java:42-47 and :127-136:
// the attributes and the combat block are the character's live values with
// gear and stat funcs applied, truncated to int. The expected values come
// from the reference formulas at level 1 (level mod 0.9) with the bonus
// tables STR 42 = 1.29, DEX 30 = 1.1, INT 21 = 0.81, WIT 11 = 0.64,
// MEN 25 = 1.28:
//
//	P.Atk   100 * 1.29 * 0.9 * 1.2              = 139.32 -> 139
//	P.Spd   433 * 1.1 * 1.33                     = 633.48 -> 633
//	P.Def   (30 + 50 - 31 chest) * 0.9           = 44.1   -> 44
//	Evasion sqrt(30) * 6 + level 1 + 3           = 36.86  -> 36
//	Acc     sqrt(30) * 6 + level 1 + 2           = 35.86  -> 35
//	Crit    7 * 1.1 * 10 * 1.5                   = 115.5  -> 115
//	M.Atk   50 * 0.9^2 * 0.81^2                  = 26.57  -> 26
//	M.Spd   333 * 0.64 * 1.3                     = 277.06 -> 277
//	M.Def   (15 - 5 ring) * 1.28 * 0.9           = 11.52  -> 11
func TestFrameUserInfoWritesLiveStats(t *testing.T) {
	c, tmpl := liveStatsCharacter()
	got := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: tmpl}))

	attrs := 1 + 5*4 + (len(c.CharacterName())+1)*2 + 4*4 + 8
	wantAttrs := []uint32{42, 30, 43, 21, 11, 25}
	for i, want := range wantAttrs {
		if v := binary.LittleEndian.Uint32(got[attrs+4*i:]); v != want {
			t.Errorf("attribute %d = %d, want %d", i, v, want)
		}
	}

	combat := attrs + 6*4 + 8*4 + 2*len(paperdollWriteOrder)*4 + 14*2 + 4 + 12*2 + 4 + 4*2
	wantCombat := []struct {
		name string
		want uint32
	}{
		{"P.Atk", 139},
		{"P.Atk. speed", 633},
		{"P.Def", 44},
		{"evasion", 36},
		{"accuracy", 35},
		{"critical rate", 115},
		{"M.Atk", 26},
		{"M.Atk. speed", 277},
		{"P.Atk. speed (repeated)", 633},
		{"M.Def", 11},
	}
	for i, field := range wantCombat {
		if v := binary.LittleEndian.Uint32(got[combat+4*i:]); v != field.want {
			t.Errorf("%s = %d, want %d", field.name, v, field.want)
		}
	}
}

// TestFrameCharInfoWritesLiveCastAndAttackSpeed pins CharInfo.java:84-85:
// the M.Atk. and P.Atk. speeds between the two PvP flag/karma pairs are the
// live values.
func TestFrameCharInfoWritesLiveCastAndAttackSpeed(t *testing.T) {
	c, tmpl := liveStatsCharacter()
	got := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: tmpl}))
	offset := 1 + 5*4 + (len(c.CharacterName())+1)*2 + 3*4 + len(charInfoPaperdollOrder)*4 + 4*2 + 4 + 12*2 + 4 + 4*2 + 2*4
	if v := binary.LittleEndian.Uint32(got[offset:]); v != 277 {
		t.Errorf("M.Atk. speed = %d, want 277", v)
	}
	if v := binary.LittleEndian.Uint32(got[offset+4:]); v != 633 {
		t.Errorf("P.Atk. speed = %d, want 633", v)
	}
}

func TestFrameUserInfo_DwarfUsesDwarfInventoryLimit(t *testing.T) {
	tmpl := &player.Template{}
	humanChar := &player.Character{Race: player.RaceHuman}
	humanChar.SetName("H")
	human := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: humanChar, Template: tmpl}))
	dwarfChar := &player.Character{Race: player.RaceDwarf}
	dwarfChar.SetName("H")
	dwarf := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: dwarfChar, Template: tmpl}))

	if len(human) != len(dwarf) {
		t.Fatalf("human and dwarf encodings differ in length: %d vs %d", len(human), len(dwarf))
	}
	wantHuman := binary.LittleEndian.AppendUint16(nil, 80)
	wantDwarf := binary.LittleEndian.AppendUint16(nil, 100)
	if !bytes.Contains(human, wantHuman) {
		t.Errorf("human encoding did not contain the non-dwarf inventory limit 80")
	}
	if !bytes.Contains(dwarf, wantDwarf) {
		t.Errorf("dwarf encoding did not contain the dwarf inventory limit 100")
	}
}

// TestStorageLimitPacketsReportLiveInventoryLimit pins UserInfo's and
// ExStorageMaxCount's inventory-limit field to the character's live limit:
// the configured race base plus the inventoryLimit stat.
func TestStorageLimitPacketsReportLiveInventoryLimit(t *testing.T) {
	c := &player.Character{Race: player.RaceDwarf}
	c.SetName("D")
	c.Configure(player.Runtime{Rules: player.NewRuleBook(player.Rules{InventorySlots: player.InventorySlots{NoDwarf: 90, Dwarf: 117, Configured: true}})})
	c.AddStatFuncs([]effect.Mod{{Stat: stat.InvLim, Op: effect.OpAdd, Value: 1}})

	storage := framePayload(t, FrameExStorageMaxCount(c))
	if got := binary.LittleEndian.Uint32(storage[3:7]); got != 118 {
		t.Fatalf("ExStorageMaxCount inventory limit = %d, want 118", got)
	}

	full := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: &player.Template{}}))
	plain := &player.Character{Race: player.RaceDwarf}
	plain.SetName("D")
	base := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: plain, Template: &player.Template{}}))
	if len(full) != len(base) {
		t.Fatalf("UserInfo lengths differ: %d vs %d", len(full), len(base))
	}
	// The two characters differ only in their inventory limit (118 vs the
	// default 100), so the first differing byte starts that field.
	idx := 0
	for idx < len(base) && base[idx] == full[idx] {
		idx++
	}
	if idx+2 > len(base) {
		t.Fatal("UserInfo does not change with the inventory limit")
	}
	if got := binary.LittleEndian.Uint16(base[idx : idx+2]); got != 100 {
		t.Fatalf("default dwarf UserInfo inventory limit = %d, want 100", got)
	}
	if got := binary.LittleEndian.Uint16(full[idx : idx+2]); got != 118 {
		t.Fatalf("UserInfo inventory limit = %d, want 118", got)
	}
	if !bytes.Equal(base[idx+2:], full[idx+2:]) {
		t.Fatal("UserInfo differs beyond the inventory-limit field")
	}
}
