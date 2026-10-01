package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// ---- from charinfo_test.go ----
func TestFrameCharInfoCoreFields(t *testing.T) {
	c := &player.Character{
		ID: 0x10000001, Name: "Observer",
		Race: player.RaceHuman, Sex: player.SexMale,
		Location:    location.Location{X: 10, Y: 20, Z: 30},
		LastHeading: 123,
	}
	tmpl := &player.Template{CollisionRadius: 9, CollisionHeight: 23, RunSpeed: 120, WalkSpeed: 80, SwimSpeed: 50}
	items := []*item.Instance{{ObjectID: 100, TemplateID: 2369, Location: item.LocationPaperdoll, LocationData: rhandPaperdollIndex}}

	got := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: tmpl, Items: items}))
	if got[0] != OpcodeCharInfo {
		t.Fatalf("opcode = %#x, want %#x", got[0], OpcodeCharInfo)
	}

	offset := 1
	for _, want := range []uint32{10, 20, 30, 0, uint32(c.ObjectID())} {
		if v := binary.LittleEndian.Uint32(got[offset:]); v != want {
			t.Fatalf("field at offset %d = %d, want %d", offset, v, want)
		}
		offset += 4
	}

	// Skip UTF-16 name, race, sex, class id; the first 12 equipment template
	// ids follow. RHAND is the third entry in CharInfo's paperdoll order.
	for got[offset] != 0 || got[offset+1] != 0 {
		offset += 2
	}
	offset += 2 + 4 + 4 + 4
	if v := binary.LittleEndian.Uint32(got[offset+2*4:]); v != 2369 {
		t.Fatalf("right-hand template id = %d, want 2369", v)
	}
}

func TestFrameCharInfoMirrorsPvPFlag(t *testing.T) {
	c := &player.Character{Name: "Observer"}
	c.UpdatePvPFlag(task.PvPFlagOn)

	got := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: &player.Template{}}))
	offset := 1 + 5*4 + (len(c.Name)+1)*2 + 3*4 + len(charInfoPaperdollOrder)*4 + 4*2 + 4 + 12*2 + 4 + 4*2
	for _, fieldOffset := range []int{offset, offset + 4*4} {
		if v := binary.LittleEndian.Uint32(got[fieldOffset:]); v != uint32(task.PvPFlagOn) {
			t.Fatalf("PvP flag at offset %d = %d, want %d", fieldOffset, v, task.PvPFlagOn)
		}
	}
}

func TestFrameCharInfoUsesDoublePrecisionFloatFields(t *testing.T) {
	c := &player.Character{
		ID: 0x10000001, Name: "Observer",
		Race: player.RaceHuman, Sex: player.SexMale,
		Location: location.Location{X: 10, Y: 20, Z: 30},
	}
	tmpl := &player.Template{
		CollisionRadius: 9, CollisionHeight: 23,
		RunSpeed: 120, WalkSpeed: 80, SwimSpeed: 50,
	}

	got := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: tmpl}))
	want := appendF64(nil, float64(c.MovementSpeedMultiplier()))
	want = appendF64(want, float64(c.AttackSpeedMultiplier()))
	want = appendF64(want, tmpl.CollisionRadius)
	want = appendF64(want, tmpl.CollisionHeight)
	if !bytes.Contains(got, want) {
		t.Fatalf("CharInfo missing double-width movement/collision block %x", want)
	}
}

// TestFrameCharInfoAndUserInfoWriteLiveSpeedMultipliers pins
// CharInfo.java:102-103 and UserInfo.java:152-153: both write the live
// movement and attack speed multipliers as doubles of their float values,
// ahead of the collision block, so a speed buff reaches the client.
func TestFrameCharInfoAndUserInfoWriteLiveSpeedMultipliers(t *testing.T) {
	tmpl := &player.Template{
		DEX: 30, RunSpeed: 120, WalkSpeed: 80, SwimSpeed: 50,
		CollisionRadius: 9, CollisionHeight: 23,
	}
	c := &player.Character{ID: 0x10000001, Name: "Observer", Race: player.RaceHuman, Sex: player.SexMale}
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

func TestFrameCharInfo_CubicsSerializeCountAndIDs(t *testing.T) {
	tmpl := &player.Template{}
	empty := &player.Character{Name: "C"}
	withCubics := &player.Character{Name: "C"}
	withCubics.SetSkillLevel(143, 5) // Cubic Mastery: room for more than one cubic
	withCubics.AddOrRefreshCubic(1, false)
	withCubics.AddOrRefreshCubic(3, false)

	base := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: empty, Template: tmpl}))
	got := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: withCubics, Template: tmpl}))

	// The cubic field is the only difference between the two encodings, so
	// its exact position is the point the two payloads diverge, and the
	// bytes after it must resync with base once the field is skipped.
	prefixLen := 0
	for prefixLen < len(base) && prefixLen < len(got) && base[prefixLen] == got[prefixLen] {
		prefixLen++
	}
	want := binary.LittleEndian.AppendUint16(nil, 2)
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = binary.LittleEndian.AppendUint16(want, 3)
	if prefixLen+len(want) > len(got) || !bytes.Equal(got[prefixLen:prefixLen+len(want)], want) {
		t.Fatalf("cubic field at offset %d = % x, want count 2 followed by ids [1 3] (% x)", prefixLen, got[prefixLen:], want)
	}
	if suffix := got[prefixLen+len(want):]; !bytes.Equal(suffix, base[prefixLen+2:]) {
		t.Fatalf("bytes after cubic field don't resync with the no-cubic encoding: got %x, want %x", suffix, base[prefixLen+2:])
	}
}

func TestFrameCharInfoCarriesAbnormalEffectMask(t *testing.T) {
	tmpl := &player.Template{}
	plain := &player.Character{Name: "C"}
	bigHead := &player.Character{Name: "C"}
	bigHead.StartAbnormalEffect(0x002000)

	base := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: plain, Template: tmpl}))
	got := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: bigHead, Template: tmpl}))

	// Same-length encodings that must differ in exactly the abnormal-effect
	// int32 field, and resync everywhere else.
	if len(base) != len(got) {
		t.Fatalf("payload length changed: base %d, got %d", len(base), len(got))
	}
	prefixLen := 0
	for prefixLen < len(base) && base[prefixLen] == got[prefixLen] {
		prefixLen++
	}
	// 0x002000's only non-zero byte is its little-endian byte 1, so the diff
	// starts one byte into the field.
	fieldStart := prefixLen - 1
	if v := binary.LittleEndian.Uint32(got[fieldStart:]); v != 0x002000 {
		t.Fatalf("abnormal effect field at offset %d = %#x, want %#x", fieldStart, v, 0x002000)
	}
	if suffix := got[fieldStart+4:]; !bytes.Equal(suffix, base[fieldStart+4:]) {
		t.Fatalf("bytes after the abnormal effect field don't resync: got %x, want %x", suffix, base[fieldStart+4:])
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

// ---- from userinfo_test.go ----
func TestFrameUserInfo(t *testing.T) {
	c := &player.Character{
		ID:        0x10000001,
		Name:      "Newbie",
		Race:      player.RaceHuman,
		Sex:       player.SexMale,
		CharLevel: 1,
		Exp:       0,
		SP:        0,
		Face:      0, HairStyle: 1, HairColor: 2,
		Location:    location.Location{X: 10, Y: 20, Z: 30},
		LastHeading: 100,
		KarmaPoints: 0, PKKills: 1, PvPKills: 2,
		AccessLevel: 1,
	}
	c.SetTitle("Hero")
	c.SetClanID(5)
	c.SetColors(0x00CCFF, 0x0033CC)
	c.SetResourceValues(player.Resources{
		MaxHP: 80, CurrentHP: 75,
		MaxMP: 30, CurrentMP: 30,
		MaxCP: 40, CurrentCP: 40,
	})
	tmpl := &player.Template{
		STR: 40, CON: 43, DEX: 30, INT: 21, WIT: 11, MEN: 25,
		PAtk: 4, PDef: 30, MAtk: 3, MDef: 15,
		RunSpeed: 120, WalkSpeed: 80, SwimSpeed: 50,
		CollisionRadius: 9, CollisionHeight: 23,
	}
	c.AttachRuntime(tmpl, nil)
	items := []*item.Instance{
		{
			ObjectID: 100, TemplateID: 2369, Location: item.LocationPaperdoll, LocationData: rhandPaperdollIndex, EnchantLevel: 200,
			Augmentation: &item.Augmentation{Attributes: 8191<<16 | 3},
		},
	}

	got := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: tmpl, Items: items, IsGM: true}))
	resources := c.ResourceValues()

	want := []byte{OpcodeUserInfo}
	x, y, z := c.Position()
	want = binary.LittleEndian.AppendUint32(want, uint32(x))
	want = binary.LittleEndian.AppendUint32(want, uint32(y))
	want = binary.LittleEndian.AppendUint32(want, uint32(z))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.LastHeading))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.ObjectID()))
	want = append(want, encodeUTF16Z(c.Name)...)
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Race))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Sex))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.ClassID()))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.CharLevel))
	want = binary.LittleEndian.AppendUint64(want, uint64(c.Exp))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.STR()))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.DEX()))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.CON()))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.INT()))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.WIT()))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.MEN()))
	want = binary.LittleEndian.AppendUint32(want, uint32(resources.MaxHP))
	want = binary.LittleEndian.AppendUint32(want, uint32(resources.CurrentHP))
	want = binary.LittleEndian.AppendUint32(want, uint32(resources.MaxMP))
	want = binary.LittleEndian.AppendUint32(want, uint32(resources.CurrentMP))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.SP))
	want = binary.LittleEndian.AppendUint32(want, 0)  // current weight
	want = binary.LittleEndian.AppendUint32(want, 0)  // weight limit
	want = binary.LittleEndian.AppendUint32(want, 40) // talisman slots: weapon equipped

	paperdoll := item.Paperdoll(items)
	for _, pos := range paperdollWriteOrder {
		want = binary.LittleEndian.AppendUint32(want, uint32(paperdoll[pos].ObjectID))
	}
	for _, pos := range paperdollWriteOrder {
		want = binary.LittleEndian.AppendUint32(want, uint32(paperdoll[pos].TemplateID))
	}

	for i := 0; i < 14; i++ {
		want = binary.LittleEndian.AppendUint16(want, 0)
	}
	want = binary.LittleEndian.AppendUint32(want, 8191<<16|3) // rhand augmentation
	for i := 0; i < 12; i++ {
		want = binary.LittleEndian.AppendUint16(want, 0)
	}
	want = binary.LittleEndian.AppendUint32(want, 0) // lhand augmentation
	for i := 0; i < 4; i++ {
		want = binary.LittleEndian.AppendUint16(want, 0)
	}

	// The live combat block; TestFrameUserInfoWritesLiveStats pins its values.
	want = binary.LittleEndian.AppendUint32(want, uint32(int32(c.PAtk())))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.AttackSpeed()))
	want = binary.LittleEndian.AppendUint32(want, uint32(int32(c.PDef())))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Evasion()))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Accuracy()))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.CriticalRate()))
	want = binary.LittleEndian.AppendUint32(want, uint32(int32(c.MAtk())))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.MagicAttackSpeed()))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.AttackSpeed()))
	want = binary.LittleEndian.AppendUint32(want, uint32(int32(c.MDef())))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.PvPFlagState()))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Karma()))

	want = binary.LittleEndian.AppendUint32(want, uint32(int32(tmpl.RunSpeed)))
	want = binary.LittleEndian.AppendUint32(want, uint32(int32(tmpl.WalkSpeed)))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.SwimSpeed))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.SwimSpeed))
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0) // flying run speed
	want = binary.LittleEndian.AppendUint32(want, 0) // flying walk speed

	want = appendF64(want, float64(c.MovementSpeedMultiplier()))
	want = appendF64(want, float64(c.AttackSpeedMultiplier()))
	want = appendF64(want, tmpl.CollisionRadius)
	want = appendF64(want, tmpl.CollisionHeight)

	want = binary.LittleEndian.AppendUint32(want, uint32(c.HairStyle))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.HairColor))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Face))
	want = binary.LittleEndian.AppendUint32(want, 1) // IsGM flag

	want = append(want, encodeUTF16Z(c.Title())...)

	want = binary.LittleEndian.AppendUint32(want, uint32(5))
	want = binary.LittleEndian.AppendUint32(want, 0) // clan crest id
	want = binary.LittleEndian.AppendUint32(want, 0) // ally id
	want = binary.LittleEndian.AppendUint32(want, 0) // ally crest id
	want = binary.LittleEndian.AppendUint32(want, 0) // relation
	want = append(want, 0)                           // mount type
	want = append(want, 0)                           // operate type
	want = append(want, 0)                           // crystallize

	want = binary.LittleEndian.AppendUint32(want, uint32(c.PKKills))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.PvPKills))

	want = binary.LittleEndian.AppendUint16(want, 0) // cubic count

	want = append(want, 0)                           // party match room
	want = binary.LittleEndian.AppendUint32(want, 0) // abnormal effect
	want = append(want, 0)                           // reserved
	want = binary.LittleEndian.AppendUint32(want, 0) // clan privileges
	want = binary.LittleEndian.AppendUint16(want, 0) // recommendations left
	want = binary.LittleEndian.AppendUint16(want, 0) // recommendations received
	want = binary.LittleEndian.AppendUint32(want, 0) // mount npc id

	want = binary.LittleEndian.AppendUint16(want, 80) // MaximumSlotsForNoDwarf default
	want = binary.LittleEndian.AppendUint32(want, uint32(c.ClassID()))
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, uint32(resources.MaxCP))
	want = binary.LittleEndian.AppendUint32(want, uint32(resources.CurrentCP))
	want = append(want, 127) // enchant effect, capped

	want = append(want, 0)                           // team
	want = binary.LittleEndian.AppendUint32(want, 0) // large clan crest
	want = append(want, 0)                           // noble
	want = append(want, 0)                           // hero
	want = append(want, 0)                           // fishing

	want = binary.LittleEndian.AppendUint32(want, 0) // fishing stance x
	want = binary.LittleEndian.AppendUint32(want, 0) // fishing stance y
	want = binary.LittleEndian.AppendUint32(want, 0) // fishing stance z

	want = binary.LittleEndian.AppendUint32(want, 0x00CCFF) // name color
	want = append(want, 1)                                  // running

	want = binary.LittleEndian.AppendUint32(want, 0)        // pledge class
	want = binary.LittleEndian.AppendUint32(want, 0)        // pledge type
	want = binary.LittleEndian.AppendUint32(want, 0x0033CC) // title color
	want = binary.LittleEndian.AppendUint32(want, 0)        // cursed weapon stage

	if !bytes.Equal(got, want) {
		t.Errorf("FrameUserInfo mismatch:\n got  %x\n want %x", got, want)
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
	c := &player.Character{ID: 0x10000001, Name: "Buffed", Race: player.RaceHuman, Sex: player.SexMale, CharLevel: 1}
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

	attrs := 1 + 5*4 + (len(c.Name)+1)*2 + 4*4 + 8
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
	offset := 1 + 5*4 + (len(c.Name)+1)*2 + 3*4 + len(charInfoPaperdollOrder)*4 + 4*2 + 4 + 12*2 + 4 + 4*2 + 2*4
	if v := binary.LittleEndian.Uint32(got[offset:]); v != 277 {
		t.Errorf("M.Atk. speed = %d, want 277", v)
	}
	if v := binary.LittleEndian.Uint32(got[offset+4:]); v != 633 {
		t.Errorf("P.Atk. speed = %d, want 633", v)
	}
}

// TestFrameUserInfo_PvPFlagByteFollowsPvPFlagState is the regression test
// for the pvp-flag field staying hardcoded to 0: the byte at that slot must
// track Character.PvPFlagState() (0/1/2 for none/on/blinking), or the
// client never redraws the name color/PvP icon UpdatePvPFlag is supposed
// to refresh.
func TestFrameUserInfo_PvPFlagByteFollowsPvPFlagState(t *testing.T) {
	tmpl := &player.Template{}
	c := &player.Character{Name: "M"}

	unflagged := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: tmpl}))
	c.UpdatePvPFlag(task.PvPFlagOn)
	flagged := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: tmpl}))

	if bytes.Equal(unflagged, flagged) {
		t.Fatal("unflagged/flagged encodings are identical, want the pvp-flag byte to differ")
	}
}

// TestFrameUserInfo_GMByteFollowsIsGMNotAccessLevel pins the GM byte to
// UserInfoSnapshot.IsGM (accessLevels.xml's isGM flag) rather than a raw
// AccessLevel > 0 check: a level 1-6 character (Chat Moderator .. Head GM,
// isGM="false" in the reference data) must not broadcast the GM flag even
// though its AccessLevel is positive.
func TestFrameUserInfo_GMByteFollowsIsGMNotAccessLevel(t *testing.T) {
	tmpl := &player.Template{}
	c := &player.Character{Name: "M", AccessLevel: 3}

	notGM := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: tmpl, IsGM: false}))
	gm := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: tmpl, IsGM: true}))

	if bytes.Equal(notGM, gm) {
		t.Fatal("IsGM false/true encodings are identical, want the GM byte to differ")
	}
}

func TestFrameUserInfo_FemaleUsesFemaleCollision(t *testing.T) {
	tmpl := &player.Template{
		CollisionRadius: 9, CollisionHeight: 23,
		CollisionRadiusFemale: 17.5, CollisionHeightFemale: 42.25,
	}
	male := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: &player.Character{Sex: player.SexMale, Name: "M"}, Template: tmpl}))
	female := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: &player.Character{Sex: player.SexFemale, Name: "M"}, Template: tmpl}))

	if bytes.Equal(male, female) {
		t.Fatal("male and female encodings are identical, want different collision fields")
	}
	if !bytes.Contains(female, appendF64(nil, tmpl.CollisionRadiusFemale)) {
		t.Errorf("female encoding did not contain the female collision radius %v", tmpl.CollisionRadiusFemale)
	}
	if bytes.Contains(male, appendF64(nil, tmpl.CollisionRadiusFemale)) {
		t.Errorf("male encoding unexpectedly contained the female collision radius %v", tmpl.CollisionRadiusFemale)
	}
}

func TestFrameUserInfo_DwarfUsesDwarfInventoryLimit(t *testing.T) {
	tmpl := &player.Template{}
	human := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: &player.Character{Race: player.RaceHuman, Name: "H"}, Template: tmpl}))
	dwarf := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: &player.Character{Race: player.RaceDwarf, Name: "H"}, Template: tmpl}))

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
	c := &player.Character{Race: player.RaceDwarf, Name: "D"}
	c.Configure(player.Runtime{Rules: player.Rules{InventorySlots: player.InventorySlots{NoDwarf: 90, Dwarf: 117, Configured: true}}})
	c.AddStatFuncs([]effect.Mod{{Stat: stat.InvLim, Op: effect.OpAdd, Value: 1}})

	storage := framePayload(t, FrameExStorageMaxCount(c))
	if got := binary.LittleEndian.Uint32(storage[3:7]); got != 118 {
		t.Fatalf("ExStorageMaxCount inventory limit = %d, want 118", got)
	}

	full := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: &player.Template{}}))
	plain := &player.Character{Race: player.RaceDwarf, Name: "D"}
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

func TestFrameUserInfo_CubicsSerializeCountAndIDs(t *testing.T) {
	tmpl := &player.Template{}
	empty := &player.Character{Name: "C"}
	withCubics := &player.Character{Name: "C"}
	withCubics.SetSkillLevel(143, 5) // Cubic Mastery: room for more than one cubic
	withCubics.AddOrRefreshCubic(1, false)
	withCubics.AddOrRefreshCubic(3, false)

	base := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: empty, Template: tmpl}))
	got := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: withCubics, Template: tmpl}))

	// The cubic field is the only difference between the two encodings, so
	// its exact position is the point the two payloads diverge, and the
	// bytes after it must resync with base once the field is skipped.
	prefixLen := 0
	for prefixLen < len(base) && prefixLen < len(got) && base[prefixLen] == got[prefixLen] {
		prefixLen++
	}
	want := binary.LittleEndian.AppendUint16(nil, 2)
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = binary.LittleEndian.AppendUint16(want, 3)
	if prefixLen+len(want) > len(got) || !bytes.Equal(got[prefixLen:prefixLen+len(want)], want) {
		t.Fatalf("cubic field at offset %d = % x, want count 2 followed by ids [1 3] (% x)", prefixLen, got[prefixLen:], want)
	}
	if suffix := got[prefixLen+len(want):]; !bytes.Equal(suffix, base[prefixLen+2:]) {
		t.Fatalf("bytes after cubic field don't resync with the no-cubic encoding: got %x, want %x", suffix, base[prefixLen+2:])
	}
}

func TestFrameUserInfoCarriesAbnormalEffectMask(t *testing.T) {
	tmpl := &player.Template{}
	plain := &player.Character{Name: "C"}
	bigHead := &player.Character{Name: "C"}
	bigHead.StartAbnormalEffect(0x002000)

	base := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: plain, Template: tmpl}))
	got := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: bigHead, Template: tmpl}))

	if len(base) != len(got) {
		t.Fatalf("payload length changed: base %d, got %d", len(base), len(got))
	}
	prefixLen := 0
	for prefixLen < len(base) && base[prefixLen] == got[prefixLen] {
		prefixLen++
	}
	// 0x002000's only non-zero byte is its little-endian byte 1, so the diff
	// starts one byte into the field.
	fieldStart := prefixLen - 1
	if v := binary.LittleEndian.Uint32(got[fieldStart:]); v != 0x002000 {
		t.Fatalf("abnormal effect field at offset %d = %#x, want %#x", fieldStart, v, 0x002000)
	}
	if suffix := got[fieldStart+4:]; !bytes.Equal(suffix, base[fieldStart+4:]) {
		t.Fatalf("bytes after the abnormal effect field don't resync: got %x, want %x", suffix, base[fieldStart+4:])
	}
}

// TestFrameUserInfo_MountNpcIdCarriesOffset pins the mount-id encoding: a
// mounted character reports its mount npc id shifted into the client's
// mount id space (+1000000); an unmounted character reports 0.
func TestFrameUserInfo_MountNpcIdCarriesOffset(t *testing.T) {
	tmpl := &player.Template{}
	c := &player.Character{Name: "M"}

	unmounted := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: tmpl}))
	if bytes.Contains(unmounted, binary.LittleEndian.AppendUint32(nil, 12621)) {
		t.Fatal("unmounted UserInfo contains the raw wyvern npc id")
	}

	c.Mount(12621, 555)
	mounted := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: tmpl}))
	if !bytes.Contains(mounted, binary.LittleEndian.AppendUint32(nil, 12621+1000000)) {
		t.Fatalf("mounted UserInfo does not contain npc id + 1000000 (%d)", 12621+1000000)
	}
}

// TestFrameUserInfo_TeamByteBlueWhileSpawnProtected pins the team byte:
// while spawn protection holds the client sees TeamType.BLUE (1), not the
// unassigned team.
func TestFrameUserInfo_TeamByteBlueWhileSpawnProtected(t *testing.T) {
	tmpl := &player.Template{}
	c := &player.Character{Name: "M"}

	unprotected := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: tmpl}))
	protected := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: tmpl, SpawnProtectedTeam: true}))

	if len(unprotected) != len(protected) {
		t.Fatalf("payload lengths differ: %d vs %d", len(unprotected), len(protected))
	}
	diffs := 0
	teamAt := -1
	for i := range unprotected {
		if unprotected[i] != protected[i] {
			diffs++
			teamAt = i
		}
	}
	if diffs != 1 || teamAt < 0 {
		t.Fatalf("expected exactly one differing byte, got %d (at %d)", diffs, teamAt)
	}
	if unprotected[teamAt] != 0 {
		t.Fatalf("unprotected team byte = %d, want 0", unprotected[teamAt])
	}
	if protected[teamAt] != 1 {
		t.Fatalf("spawn-protected team byte = %d, want TeamType.BLUE (1)", protected[teamAt])
	}
}

// TestFrameUserAndCharInfoCarryOperateType pins the operate byte UserInfo
// and CharInfo write right after the mount type: a character running a
// package sale (operate type 8) differs from one with no store in that byte
// alone.
func TestFrameUserAndCharInfoCarryOperateType(t *testing.T) {
	tmpl := &player.Template{}
	c := &player.Character{Name: "M"}
	encode := map[string]func() []byte{
		"UserInfo": func() []byte { return framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: tmpl})) },
		"CharInfo": func() []byte { return framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: tmpl})) },
	}
	for name, enc := range encode {
		c.SetOperateType(privatestore.OperateNone)
		idle := enc()
		c.SetOperateType(privatestore.OperatePackageSell)
		selling := enc()
		if len(idle) != len(selling) {
			t.Fatalf("%s payload lengths differ: %d vs %d", name, len(idle), len(selling))
		}
		at := -1
		for i := range idle {
			if idle[i] != selling[i] {
				if at >= 0 {
					t.Fatalf("%s differs at %d and %d, want one operate byte", name, at, i)
				}
				at = i
			}
		}
		if at < 0 || idle[at] != 0 || selling[at] != 8 {
			t.Fatalf("%s operate byte at %d = %d -> %d, want 0 -> 8", name, at, idle[at], selling[at])
		}
		// The byte before it is the mount type, 0 for a character on foot;
		// TestFrameUserInfo pins the full UserInfo layout around both.
		if idle[at-1] != 0 {
			t.Fatalf("%s byte before the operate type = %d, want mount type 0", name, idle[at-1])
		}
	}
}
