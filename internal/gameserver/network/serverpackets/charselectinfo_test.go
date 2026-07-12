package serverpackets

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func appendF64(b []byte, v float64) []byte {
	return binary.LittleEndian.AppendUint64(b, math.Float64bits(v))
}

func encodeUTF16Z(s string) []byte {
	var out []byte
	for _, u := range utf16.Encode([]rune(s)) {
		out = binary.LittleEndian.AppendUint16(out, u)
	}
	return binary.LittleEndian.AppendUint16(out, 0)
}

func TestNewCharacterSlot_DeleteTimer(t *testing.T) {
	now := time.UnixMilli(2_000_000_000_000)

	tests := []struct {
		name        string
		accessLevel int
		deleteAt    int64
		want        int32
	}{
		{"no deletion scheduled", 0, 0, 0},
		{"deletion scheduled in the future", 0, now.UnixMilli() + 10_000, 10},
		{"deletion deadline already passed", 0, now.UnixMilli() - 10_000, 0},
		{"banned character", -1, 0, -1},
	}
	for _, tt := range tests {
		c := &player.Character{AccessLevel: tt.accessLevel, DeleteAt: tt.deleteAt}
		slot := NewCharacterSlot(c, nil, now)
		if slot.DeleteTimerSeconds != tt.want {
			t.Errorf("%s: DeleteTimerSeconds = %d, want %d", tt.name, slot.DeleteTimerSeconds, tt.want)
		}
	}
}

func TestNewCharacterSlot_Paperdoll(t *testing.T) {
	items := []*item.Instance{
		{ObjectID: 100, TemplateID: 2369, Location: item.LocationPaperdoll, LocationData: 7, EnchantLevel: 5},
		{ObjectID: 101, TemplateID: 1146, Location: item.LocationPaperdoll, LocationData: 10},
		{ObjectID: 102, TemplateID: 5588, Location: item.LocationInventory},
	}
	slot := NewCharacterSlot(&player.Character{}, items, time.Now())

	if slot.Paperdoll[7].ObjectID != 100 || slot.Paperdoll[7].EnchantLevel != 5 {
		t.Errorf("Paperdoll[7] = %+v, want weapon with enchant 5", slot.Paperdoll[7])
	}
	if slot.Paperdoll[10].ObjectID != 101 {
		t.Errorf("Paperdoll[10] = %+v, want chest item", slot.Paperdoll[10])
	}
	for i, entry := range slot.Paperdoll {
		if i == 7 || i == 10 {
			continue
		}
		if entry != (item.PaperdollEntry{}) {
			t.Errorf("Paperdoll[%d] = %+v, want empty", i, entry)
		}
	}
}

func TestFrameCharSelectInfo(t *testing.T) {
	slot := CharacterSlot{
		Name: "Newbie", ObjectID: 0x10000001, ClanID: 0,
		Sex: player.SexMale, Race: player.RaceHuman, ClassID: 0,
		X: 10, Y: 20, Z: 30,
		CurHP: 80, CurMP: 30, MaxHP: 80, MaxMP: 30,
		SP: 0, Exp: 0, Level: 1,
		Karma: 0, PKKills: 0, PvPKills: 0,
		HairStyle: 1, HairColor: 2, Face: 0,
		DeleteTimerSeconds: 0,
	}
	slot.Paperdoll[rhandPaperdollIndex] = item.PaperdollEntry{ObjectID: 100, TemplateID: 2369, EnchantLevel: 5}
	slot.Paperdoll[10] = item.PaperdollEntry{ObjectID: 101, TemplateID: 1146}

	got := framePayload(t, FrameCharSelectInfo("acct1", 999, []CharacterSlot{slot}, 0))

	want := []byte{OpcodeCharSelectInfo}
	want = binary.LittleEndian.AppendUint32(want, 1) // slot count

	want = append(want, encodeUTF16Z(slot.Name)...)
	want = binary.LittleEndian.AppendUint32(want, uint32(slot.ObjectID))
	want = append(want, encodeUTF16Z("acct1")...)
	want = binary.LittleEndian.AppendUint32(want, 999)
	want = binary.LittleEndian.AppendUint32(want, uint32(slot.ClanID))
	want = binary.LittleEndian.AppendUint32(want, 0)

	want = binary.LittleEndian.AppendUint32(want, uint32(slot.Sex))
	want = binary.LittleEndian.AppendUint32(want, uint32(slot.Race))
	want = binary.LittleEndian.AppendUint32(want, uint32(slot.ClassID))

	want = binary.LittleEndian.AppendUint32(want, 1)

	want = binary.LittleEndian.AppendUint32(want, uint32(slot.X))
	want = binary.LittleEndian.AppendUint32(want, uint32(slot.Y))
	want = binary.LittleEndian.AppendUint32(want, uint32(slot.Z))

	want = appendF64(want, slot.CurHP)
	want = appendF64(want, slot.CurMP)

	want = binary.LittleEndian.AppendUint32(want, uint32(slot.SP))
	want = binary.LittleEndian.AppendUint64(want, uint64(slot.Exp))
	want = binary.LittleEndian.AppendUint32(want, uint32(slot.Level))

	want = binary.LittleEndian.AppendUint32(want, uint32(slot.Karma))
	want = binary.LittleEndian.AppendUint32(want, uint32(slot.PKKills))
	want = binary.LittleEndian.AppendUint32(want, uint32(slot.PvPKills))

	for i := 0; i < 7; i++ {
		want = binary.LittleEndian.AppendUint32(want, 0)
	}

	for _, pos := range paperdollWriteOrder {
		want = binary.LittleEndian.AppendUint32(want, uint32(slot.Paperdoll[pos].ObjectID))
	}
	for _, pos := range paperdollWriteOrder {
		want = binary.LittleEndian.AppendUint32(want, uint32(slot.Paperdoll[pos].TemplateID))
	}

	want = binary.LittleEndian.AppendUint32(want, uint32(slot.HairStyle))
	want = binary.LittleEndian.AppendUint32(want, uint32(slot.HairColor))
	want = binary.LittleEndian.AppendUint32(want, uint32(slot.Face))

	want = appendF64(want, slot.MaxHP)
	want = appendF64(want, slot.MaxMP)

	want = binary.LittleEndian.AppendUint32(want, uint32(slot.DeleteTimerSeconds))
	want = binary.LittleEndian.AppendUint32(want, uint32(slot.ClassID))
	want = binary.LittleEndian.AppendUint32(want, 1) // active slot (activeID=0, i=0)

	want = append(want, 5)                           // enchant effect from the RHAND weapon
	want = binary.LittleEndian.AppendUint32(want, 0) // augmentation id

	if !bytes.Equal(got, want) {
		t.Errorf("FrameCharSelectInfo mismatch:\n got  %x\n want %x", got, want)
	}
}

func TestFrameCharSelectInfo_AutoPicksMostRecentlyAccessed(t *testing.T) {
	older := CharacterSlot{Name: "Older", ObjectID: 1, LastAccess: 100}
	newer := CharacterSlot{Name: "Newer", ObjectID: 2, LastAccess: 200}

	payload := framePayload(t, FrameCharSelectInfo("acct1", 1, []CharacterSlot{older, newer}, -1))

	// The active flag sits right after the (name, objectId, loginName,
	// sessionId, clanId, builderLevel, sex, race, classId, 0x01, x, y, z,
	// curHp, curMp, sp, exp, level, karma, pkKills, pvpKills, 7 zeros, 34
	// paperdoll fields, hairStyle, hairColor, face, maxHp, maxMp,
	// deleteTimer, classId) run for each slot; rather than compute that
	// offset by hand, decode both slots back out using known-good sibling
	// behavior: re-encode with an explicit activeID and compare.
	wantOlderActive := framePayload(t, FrameCharSelectInfo("acct1", 1, []CharacterSlot{older, newer}, 1))
	if !bytes.Equal(payload, wantOlderActive) {
		t.Error("FrameCharSelectInfo with activeID=-1 did not pick the slot with the highest LastAccess")
	}
}

func TestNewCharacterSlot_PositionAndAppearance(t *testing.T) {
	c := &player.Character{
		Location: location.Location{X: 1, Y: 2, Z: 3},
	}
	slot := NewCharacterSlot(c, nil, time.Now())
	if slot.X != 1 || slot.Y != 2 || slot.Z != 3 {
		t.Errorf("position = (%d,%d,%d), want (1,2,3)", slot.X, slot.Y, slot.Z)
	}
}
