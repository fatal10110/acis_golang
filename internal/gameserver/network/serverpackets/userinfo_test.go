package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestFrameUserInfo(t *testing.T) {
	c := &player.Character{
		ID:      0x10000001,
		Name:    "Newbie",
		ClassID: 0,
		Race:    player.RaceHuman,
		Sex:     player.SexMale,
		Level:   1,
		Exp:     0,
		SP:      0,
		Face:    0, HairStyle: 1, HairColor: 2,
		Location:    location.Location{X: 10, Y: 20, Z: 30},
		LastHeading: 100,
		Karma:       0, PKKills: 1, PvPKills: 2,
		ClanID: 5, Title: "Hero", AccessLevel: 1,
	}
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
	items := []*item.Instance{
		{ObjectID: 100, TemplateID: 2369, Location: item.LocationPaperdoll, LocationData: rhandPaperdollIndex, EnchantLevel: 200},
	}

	got := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: tmpl, Items: items}))
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
	want = binary.LittleEndian.AppendUint32(want, uint32(c.ClassID))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Level))
	want = binary.LittleEndian.AppendUint64(want, uint64(c.Exp))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.STR))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.DEX))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.CON))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.INT))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.WIT))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.MEN))
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
	want = binary.LittleEndian.AppendUint32(want, 0) // rhand augmentation
	for i := 0; i < 12; i++ {
		want = binary.LittleEndian.AppendUint16(want, 0)
	}
	want = binary.LittleEndian.AppendUint32(want, 0) // lhand augmentation
	for i := 0; i < 4; i++ {
		want = binary.LittleEndian.AppendUint16(want, 0)
	}

	want = binary.LittleEndian.AppendUint32(want, uint32(int32(tmpl.PAtk)))
	want = binary.LittleEndian.AppendUint32(want, 0) // p.atk speed
	want = binary.LittleEndian.AppendUint32(want, uint32(int32(tmpl.PDef)))
	want = binary.LittleEndian.AppendUint32(want, 0) // evasion
	want = binary.LittleEndian.AppendUint32(want, 0) // accuracy
	want = binary.LittleEndian.AppendUint32(want, 0) // critical rate
	want = binary.LittleEndian.AppendUint32(want, uint32(int32(tmpl.MAtk)))
	want = binary.LittleEndian.AppendUint32(want, 0) // m.atk speed
	want = binary.LittleEndian.AppendUint32(want, 0) // p.atk speed (repeated)
	want = binary.LittleEndian.AppendUint32(want, uint32(int32(tmpl.MDef)))
	want = binary.LittleEndian.AppendUint32(want, 0) // pvp flag
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Karma))

	want = binary.LittleEndian.AppendUint32(want, uint32(int32(tmpl.RunSpeed)))
	want = binary.LittleEndian.AppendUint32(want, uint32(int32(tmpl.WalkSpeed)))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.SwimSpeed))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.SwimSpeed))
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0) // flying run speed
	want = binary.LittleEndian.AppendUint32(want, 0) // flying walk speed

	want = appendF64(want, 1) // movement speed multiplier
	want = appendF64(want, 1) // attack speed multiplier
	want = appendF64(want, tmpl.CollisionRadius)
	want = appendF64(want, tmpl.CollisionHeight)

	want = binary.LittleEndian.AppendUint32(want, uint32(c.HairStyle))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.HairColor))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Face))
	want = binary.LittleEndian.AppendUint32(want, 1) // access level > 0 -> GM flag

	want = append(want, encodeUTF16Z(c.Title)...)

	want = binary.LittleEndian.AppendUint32(want, uint32(c.ClanID))
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

	want = binary.LittleEndian.AppendUint16(want, nonDwarfInventoryLimit)
	want = binary.LittleEndian.AppendUint32(want, uint32(c.ClassID))
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

	want = binary.LittleEndian.AppendUint32(want, defaultNameColor)
	want = append(want, 1) // running

	want = binary.LittleEndian.AppendUint32(want, 0) // pledge class
	want = binary.LittleEndian.AppendUint32(want, 0) // pledge type
	want = binary.LittleEndian.AppendUint32(want, defaultTitleColor)
	want = binary.LittleEndian.AppendUint32(want, 0) // cursed weapon stage

	if !bytes.Equal(got, want) {
		t.Errorf("FrameUserInfo mismatch:\n got  %x\n want %x", got, want)
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
	wantHuman := binary.LittleEndian.AppendUint16(nil, nonDwarfInventoryLimit)
	wantDwarf := binary.LittleEndian.AppendUint16(nil, dwarfInventoryLimit)
	if !bytes.Contains(human, wantHuman) {
		t.Errorf("human encoding did not contain the non-dwarf inventory limit %d", nonDwarfInventoryLimit)
	}
	if !bytes.Contains(dwarf, wantDwarf) {
		t.Errorf("dwarf encoding did not contain the dwarf inventory limit %d", dwarfInventoryLimit)
	}
}
