package serverpackets

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/npcstring"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// ---- from acquireskill_test.go ----
func TestFrameAcquireSkillInfo(t *testing.T) {
	got := framePayload(t, FrameAcquireSkillInfo(3, 1, 50, 0, []SkillRequirement{
		{Type: 99, ItemID: 57, Count: 1, Unknown: 50},
	}))

	want := []byte{OpcodeAcquireSkillInfo}
	want = binary.LittleEndian.AppendUint32(want, 3)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 50)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 99)
	want = binary.LittleEndian.AppendUint32(want, 57)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 50)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameAcquireSkillInfo() = %x, want %x", got, want)
	}
}

func TestFrameAcquireSkillList(t *testing.T) {
	got := framePayload(t, FrameAcquireSkillList(AcquireSkillTypeUsual, []AcquireSkillListEntry{
		{ID: 3, Level: 1, Cost: 50},
		{ID: 4, Level: 2, Cost: 100},
	}))

	want := []byte{OpcodeAcquireSkillList}
	want = binary.LittleEndian.AppendUint32(want, uint32(AcquireSkillTypeUsual))
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = binary.LittleEndian.AppendUint32(want, 3)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 50)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 4)
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = binary.LittleEndian.AppendUint32(want, 0)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameAcquireSkillList() = %x, want %x", got, want)
	}
}

func TestFrameAcquireSkillDone(t *testing.T) {
	got := framePayload(t, FrameAcquireSkillDone())
	want := []byte{OpcodeAcquireSkillDone}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameAcquireSkillDone() = %x, want %x", got, want)
	}
}

// ---- from actionfailed_test.go ----
func TestFrameActionFailed(t *testing.T) {
	got := framePayload(t, FrameActionFailed())
	want := []byte{OpcodeActionFailed}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameActionFailed() = %x, want %x", got, want)
	}
}

// ---- from attack_test.go ----
func TestFrameAttack(t *testing.T) {
	got := framePayload(t, FrameAttack(AttackSnapshot{
		AttackerID: 268476516,
		X:          -71440,
		Y:          258000,
		Z:          -3104,
		Hits: []AttackHit{
			{TargetID: 268480061, Damage: 37, Flags: AttackHitSoulshot | AttackHitCritical | AttackHitShield},
			{TargetID: 7, Damage: 0, Flags: AttackHitMiss},
		},
	}))
	want := []byte{
		0x05,
		0x64, 0xa0, 0x00, 0x10,
		0x3d, 0xae, 0x00, 0x10,
		0x25, 0x00, 0x00, 0x00,
		0x70,
		0xf0, 0xe8, 0xfe, 0xff,
		0xd0, 0xef, 0x03, 0x00,
		0xe0, 0xf3, 0xff, 0xff,
		0x01, 0x00,
		0x07, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x80,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameAttack() = %x, want %x", got, want)
	}
}

// ---- from authloginfail_test.go ----
func TestEncodeAuthLoginFail(t *testing.T) {
	got := EncodeAuthLoginFail(LoginFailSystemErrorTryLater)

	var want []byte
	want = append(want, OpcodeAuthLoginFail)
	want = binary.LittleEndian.AppendUint32(want, uint32(LoginFailSystemErrorTryLater))

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeAuthLoginFail(%v) = % X, want % X", LoginFailSystemErrorTryLater, got, want)
	}
}

func TestFrameAuthLoginFail(t *testing.T) {
	frame := FrameAuthLoginFail(LoginFailSystemErrorTryLater)
	defer frame.Release()

	want := []byte{0x07, 0x00, OpcodeAuthLoginFail, 0x01, 0x00, 0x00, 0x00}
	if !bytes.Equal(frame.Bytes(), want) {
		t.Fatalf("FrameAuthLoginFail(%v) = % X, want % X", LoginFailSystemErrorTryLater, frame.Bytes(), want)
	}

	payload := EncodeAuthLoginFail(LoginFailSystemErrorTryLater)
	if !bytes.Equal(frame.Bytes()[2:], payload) {
		t.Fatalf("framed payload = % X, want EncodeAuthLoginFail output % X", frame.Bytes()[2:], payload)
	}
}

// ---- from autoattack_test.go ----
func TestFrameAutoAttackStop(t *testing.T) {
	// Expected payloads generated by the reference Java implementation's
	// AutoAttackStop packet writer for each object id.
	tests := []struct {
		name     string
		objectID int32
		want     []byte
	}{
		{"player", 268476516, []byte{0x2c, 0x64, 0xa0, 0x00, 0x10}},
		{"zero", 0, []byte{0x2c, 0x00, 0x00, 0x00, 0x00}},
		{"max", 2147483647, []byte{0x2c, 0xff, 0xff, 0xff, 0x7f}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := framePayload(t, FrameAutoAttackStop(tc.objectID))
			if !bytes.Equal(got, tc.want) {
				t.Errorf("FrameAutoAttackStop(%d) = %x, want %x", tc.objectID, got, tc.want)
			}
		})
	}
}

// ---- from charcreatefail_test.go ----
func TestFrameCharCreateFail(t *testing.T) {
	tests := []struct {
		reason CharCreateFailReason
		want   int32
	}{
		{CharCreateFailReasonCreationFailed, 0},
		{CharCreateFailReasonTooManyCharacters, 1},
		{CharCreateFailReasonNameAlreadyExists, 2},
		{CharCreateFailReason16EngChars, 3},
		{CharCreateFailReasonIncorrectName, 4},
		{CharCreateFailReasonCreateNotAllowed, 5},
		{CharCreateFailReasonChooseAnotherServer, 6},
	}
	for _, tt := range tests {
		got := framePayload(t, FrameCharCreateFail(tt.reason))

		want := []byte{OpcodeCharCreateFail}
		want = binary.LittleEndian.AppendUint32(want, uint32(tt.want))

		if !bytes.Equal(got, want) {
			t.Errorf("FrameCharCreateFail(%v) = %x, want %x", tt.reason, got, want)
		}
	}
}

// ---- from charcreateok_test.go ----
func TestFrameCharCreateOk(t *testing.T) {
	got := framePayload(t, FrameCharCreateOk())

	want := []byte{OpcodeCharCreateOk}
	want = binary.LittleEndian.AppendUint32(want, 1)

	if !bytes.Equal(got, want) {
		t.Errorf("FrameCharCreateOk = %x, want %x", got, want)
	}
}

// ---- from chardeletefail_test.go ----
func TestFrameCharDeleteFail(t *testing.T) {
	tests := []struct {
		reason CharDeleteFailReason
		want   int32
	}{
		{CharDeleteFailReasonDeletionFailed, 1},
		{CharDeleteFailReasonClanMemberMayNotDelete, 2},
		{CharDeleteFailReasonClanLeaderMayNotDelete, 3},
	}
	for _, tt := range tests {
		got := framePayload(t, FrameCharDeleteFail(tt.reason))

		want := []byte{OpcodeCharDeleteFail}
		want = binary.LittleEndian.AppendUint32(want, uint32(tt.want))

		if !bytes.Equal(got, want) {
			t.Errorf("FrameCharDeleteFail(%v) = %x, want %x", tt.reason, got, want)
		}
	}
}

// ---- from chardeleteok_test.go ----
func TestFrameCharDeleteOk(t *testing.T) {
	got := framePayload(t, FrameCharDeleteOk())
	want := []byte{OpcodeCharDeleteOk}
	if !bytes.Equal(got, want) {
		t.Errorf("FrameCharDeleteOk = %x, want %x", got, want)
	}
}

// ---- from charinfo_test.go ----
func TestFrameCharInfoCoreFields(t *testing.T) {
	c := &player.Character{
		ID: 0x10000001, Name: "Observer", ClassID: 0,
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
		ID: 0x10000001, Name: "Observer", ClassID: 0,
		Race: player.RaceHuman, Sex: player.SexMale,
		Location: location.Location{X: 10, Y: 20, Z: 30},
	}
	tmpl := &player.Template{
		CollisionRadius: 9, CollisionHeight: 23,
		RunSpeed: 120, WalkSpeed: 80, SwimSpeed: 50,
	}

	got := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: tmpl}))
	want := appendF64(nil, 1)
	want = appendF64(want, 1)
	want = appendF64(want, tmpl.CollisionRadius)
	want = appendF64(want, tmpl.CollisionHeight)
	if !bytes.Contains(got, want) {
		t.Fatalf("CharInfo missing double-width movement/collision block %x", want)
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

// ---- from charselected_test.go ----
func TestFrameCharSelected(t *testing.T) {
	c := &player.Character{
		ID:       0x10000001,
		Name:     "Newbie",
		Title:    "Hero",
		ClanID:   5,
		Sex:      player.SexMale,
		Race:     player.RaceHuman,
		ClassID:  0,
		Location: location.Location{X: 10, Y: 20, Z: 30},
		SP:       7, Exp: 12345, CharLevel: 3,
		KarmaPoints: 1, PKKills: 2,
	}
	c.SetResourceValues(player.Resources{CurrentHP: 75, CurrentMP: 30})
	tmpl := &player.Template{STR: 40, CON: 43, DEX: 30, INT: 21, WIT: 11, MEN: 25}

	got := framePayload(t, FrameCharSelected(CharSelectedSnapshot{Character: c, Template: tmpl, SessionID: 999, GameTime: 1234}))
	resources := c.ResourceValues()

	want := []byte{OpcodeCharSelected}
	x, y, z := c.Position()
	want = append(want, encodeUTF16Z(c.Name)...)
	want = binary.LittleEndian.AppendUint32(want, uint32(c.ObjectID()))
	want = append(want, encodeUTF16Z(c.Title)...)
	want = binary.LittleEndian.AppendUint32(want, 999) // session id
	want = binary.LittleEndian.AppendUint32(want, uint32(c.ClanID))
	want = binary.LittleEndian.AppendUint32(want, 0) // unknown

	want = binary.LittleEndian.AppendUint32(want, uint32(c.Sex))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Race))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.ClassID))

	want = binary.LittleEndian.AppendUint32(want, 1)

	want = binary.LittleEndian.AppendUint32(want, uint32(x))
	want = binary.LittleEndian.AppendUint32(want, uint32(y))
	want = binary.LittleEndian.AppendUint32(want, uint32(z))
	want = binary.LittleEndian.AppendUint64(want, math.Float64bits(resources.CurrentHP))
	want = binary.LittleEndian.AppendUint64(want, math.Float64bits(resources.CurrentMP))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.SP))
	want = binary.LittleEndian.AppendUint64(want, uint64(c.Exp))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.CharLevel))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Karma()))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.PKKills))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.INT))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.STR))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.CON))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.MEN))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.DEX))
	want = binary.LittleEndian.AppendUint32(want, uint32(tmpl.WIT))

	for i := 0; i < 32; i++ { // 30 padding zeros + 2 reserved
		want = binary.LittleEndian.AppendUint32(want, 0)
	}
	want = binary.LittleEndian.AppendUint32(want, 1234) // game time
	want = binary.LittleEndian.AppendUint32(want, 0)    // reserved
	want = binary.LittleEndian.AppendUint32(want, uint32(c.ClassID))
	for i := 0; i < 4; i++ {
		want = binary.LittleEndian.AppendUint32(want, 0)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("FrameCharSelected mismatch:\n got  % x\n want % x", got, want)
	}
}

// ---- from charselectinfo_test.go ----
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

// ---- from confirmdlg_test.go ----
func TestFrameConfirmDlgSummonFriendRequest(t *testing.T) {
	got := framePayload(t, FrameConfirmDlgSummonFriendRequest("Bob", 12345, 10, 20, 30, 30*time.Second))
	want := []byte{
		OpcodeConfirmDlg,
		0x32, 0x07, 0x00, 0x00, // 1842
		0x02, 0x00, 0x00, 0x00, // 2 info entries
		confirmDlgTypeText, 0x00, 0x00, 0x00,
		'B', 0x00, 'o', 0x00, 'b', 0x00, 0x00, 0x00,
		confirmDlgTypeZoneName, 0x00, 0x00, 0x00,
		10, 0x00, 0x00, 0x00,
		20, 0x00, 0x00, 0x00,
		30, 0x00, 0x00, 0x00,
		0x30, 0x75, 0x00, 0x00, // 30000ms
		0x39, 0x30, 0x00, 0x00, // 12345
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameConfirmDlgSummonFriendRequest() = %x, want %x", got, want)
	}
}

// ---- from crest_test.go ----
func TestFramePledgeCrest(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03}
	got := framePayload(t, FramePledgeCrest(101, data))

	want := []byte{OpcodePledgeCrest}
	want = binary.LittleEndian.AppendUint32(want, 101)
	want = binary.LittleEndian.AppendUint32(want, uint32(len(data)))
	want = append(want, data...)

	if !bytes.Equal(got, want) {
		t.Fatalf("FramePledgeCrest() = %x, want %x", got, want)
	}
}

func TestFrameAllyCrest(t *testing.T) {
	data := []byte{0x04, 0x05, 0x06}
	got := framePayload(t, FrameAllyCrest(103, data))

	want := []byte{OpcodeAllyCrest}
	want = binary.LittleEndian.AppendUint32(want, 103)
	want = binary.LittleEndian.AppendUint32(want, uint32(len(data)))
	want = append(want, data...)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameAllyCrest() = %x, want %x", got, want)
	}
}

func TestFrameExPledgeCrestLarge(t *testing.T) {
	data := []byte{0x07, 0x08, 0x09}
	got := framePayload(t, FrameExPledgeCrestLarge(105, data))

	want := []byte{OpcodeExtended}
	want = binary.LittleEndian.AppendUint16(want, OpcodeExPledgeCrestLarge)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 105)
	want = binary.LittleEndian.AppendUint32(want, uint32(len(data)))
	want = append(want, data...)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExPledgeCrestLarge() = %x, want %x", got, want)
	}
}

func TestFramePledgeCrestMissingData(t *testing.T) {
	got := framePayload(t, FramePledgeCrest(101, nil))

	want := []byte{OpcodePledgeCrest}
	want = binary.LittleEndian.AppendUint32(want, 101)
	want = binary.LittleEndian.AppendUint32(want, 0)

	if !bytes.Equal(got, want) {
		t.Fatalf("FramePledgeCrest(nil) = %x, want %x", got, want)
	}
}

// ---- from deleteobject_test.go ----
func TestFrameDeleteObject(t *testing.T) {
	// Expected payloads generated by the reference Java implementation's
	// DeleteObject packet writer for each (objectID, seated) pair.
	tests := []struct {
		name     string
		objectID int32
		seated   bool
		want     []byte
	}{
		{"standing", 268476516, false, []byte{0x12, 0x64, 0xa0, 0x00, 0x10, 0x01, 0x00, 0x00, 0x00}},
		{"seated", 268476516, true, []byte{0x12, 0x64, 0xa0, 0x00, 0x10, 0x00, 0x00, 0x00, 0x00}},
		{"small id", 1, false, []byte{0x12, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00}},
		{"max id seated", 2147483647, true, []byte{0x12, 0xff, 0xff, 0xff, 0x7f, 0x00, 0x00, 0x00, 0x00}},
		{"zero id", 0, false, []byte{0x12, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := framePayload(t, FrameDeleteObject(tc.objectID, tc.seated))
			if !bytes.Equal(got, tc.want) {
				t.Errorf("FrameDeleteObject(%d, %v) = %x, want %x", tc.objectID, tc.seated, got, tc.want)
			}
		})
	}
}

// ---- from enchant_test.go ----
func TestFrameEnchantResult(t *testing.T) {
	got := framePayload(t, FrameEnchantResult(EnchantResultCancelled))
	want := []byte{OpcodeEnchantResult, 0x02, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameEnchantResult() = %x, want %x", got, want)
	}
}

func TestFrameChooseInventoryItem(t *testing.T) {
	got := framePayload(t, FrameChooseInventoryItem(955))
	want := []byte{OpcodeChooseInventoryItem, 0xbb, 0x03, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameChooseInventoryItem() = %x, want %x", got, want)
	}
}

// ---- from enterworld_packets_test.go ----
func appendD(b []byte, v int32) []byte {
	return binary.LittleEndian.AppendUint32(b, uint32(v))
}

func appendH(b []byte, v uint16) []byte {
	return binary.LittleEndian.AppendUint16(b, v)
}

func TestFrameExStorageMaxCount(t *testing.T) {
	got := framePayload(t, FrameExStorageMaxCount(&player.Character{Race: player.RaceDwarf}))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExStorageMaxCount)
	want = appendD(want, dwarfInventoryLimit)
	want = appendD(want, warehouseSlotsDwarf)
	want = appendD(want, freightSlots)
	want = appendD(want, privateStoreSlotsDwarf)
	want = appendD(want, privateStoreSlotsDwarf)
	want = appendD(want, dwarfRecipeLimit)
	want = appendD(want, commonRecipeLimit)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExStorageMaxCount() = %x, want %x", got, want)
	}
}

func TestFrameHennaInfo(t *testing.T) {
	got := framePayload(t, FrameHennaInfo(henna.Snapshot{MaxSlots: 3}))
	want := []byte{OpcodeHennaInfo, 0, 0, 0, 0, 0, 0}
	want = appendD(want, 3)
	want = appendD(want, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameHennaInfo() empty = %x, want %x", got, want)
	}

	got = framePayload(t, FrameHennaInfo(henna.Snapshot{
		INT: 1, STR: 2, CON: 0, MEN: 255, DEX: 3, WIT: 4,
		MaxSlots: 2,
		Equipped: []henna.Equipped{
			{SymbolID: 7, ActiveSymbolID: 7},
			{SymbolID: 3, ActiveSymbolID: 0},
		},
	}))
	want = []byte{OpcodeHennaInfo, 1, 2, 0, 255, 3, 4}
	want = appendD(want, 2)
	want = appendD(want, 2)
	want = appendD(want, 7)
	want = appendD(want, 7)
	want = appendD(want, 3)
	want = appendD(want, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameHennaInfo() filled = %x, want %x", got, want)
	}
}

func TestFrameEtcStatusUpdate(t *testing.T) {
	got := framePayload(t, FrameEtcStatusUpdate(EtcStatus{Charges: 3, Blocked: true, GradePenalty: true, DeathPenaltyLevel: 2}))
	want := []byte{OpcodeEtcStatusUpdate}
	want = appendD(want, 3)
	want = appendD(want, 0)
	want = appendD(want, 1)
	want = appendD(want, 0)
	want = appendD(want, 1)
	want = appendD(want, 0)
	want = appendD(want, 2)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameEtcStatusUpdate() = %x, want %x", got, want)
	}
}

func TestFramePledgeSkillList(t *testing.T) {
	got := framePayload(t, FramePledgeSkillList([]SkillListEntry{{ID: 370, Level: 2}}))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExPledgeSkillList)
	want = appendD(want, 1)
	want = appendD(want, 370)
	want = appendD(want, 2)
	if !bytes.Equal(got, want) {
		t.Fatalf("FramePledgeSkillList() = %x, want %x", got, want)
	}
}

func TestFrameExCursedWeaponList(t *testing.T) {
	got := framePayload(t, FrameExCursedWeaponList([]int32{8190, 8689}))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExCursedWeaponList)
	want = appendD(want, 2)
	want = appendD(want, 8190)
	want = appendD(want, 8689)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExCursedWeaponList() = %x, want %x", got, want)
	}
}

func TestFrameExCursedWeaponLocationEmpty(t *testing.T) {
	got := framePayload(t, FrameExCursedWeaponLocation(nil))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExCursedWeaponLocation)
	want = appendD(want, 0)
	want = appendD(want, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExCursedWeaponLocation() = %x, want %x", got, want)
	}
}

func TestFrameQuestList(t *testing.T) {
	got := framePayload(t, FrameQuestList([]QuestListEntry{{QuestID: 255, Flags: 7}}))
	want := []byte{OpcodeQuestList}
	want = appendH(want, 1)
	want = appendD(want, 255)
	want = appendD(want, 7)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameQuestList() = %x, want %x", got, want)
	}
}

func TestFrameQuestListRejectsOversizedCount(t *testing.T) {
	if err := FrameQuestList(make([]QuestListEntry, 1<<16)).Err(); err == nil {
		t.Fatal("FrameQuestList oversized count error = nil, want error")
	}
}

func TestFrameFriendList(t *testing.T) {
	got := framePayload(t, FrameFriendList([]FriendListEntry{{ObjectID: 11, Name: "Buddy", Online: true}}))
	want := []byte{OpcodeFriendList}
	want = appendD(want, 1)
	want = appendD(want, 11)
	want = append(want, encodeUTF16Z("Buddy")...)
	want = appendD(want, 1)
	want = appendD(want, 11)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameFriendList() = %x, want %x", got, want)
	}
}

func TestFrameShortCutInit(t *testing.T) {
	got := framePayload(t, FrameShortCutInit([]Shortcut{{Slot: 0, Type: ShortcutAction, ID: 2, CharacterType: 1}}))
	want := []byte{OpcodeShortCutInit}
	want = appendD(want, 1)
	want = appendD(want, int32(ShortcutAction))
	want = appendD(want, 0)
	want = appendD(want, 2)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameShortCutInit() = %x, want %x", got, want)
	}
}

func TestFrameShortCutRegisterSkill(t *testing.T) {
	got := framePayload(t, FrameShortCutRegister(Shortcut{Slot: 3, Page: 1, Type: ShortcutSkill, ID: 248, Level: 1, CharacterType: 1}))
	want := []byte{OpcodeShortCutRegister}
	want = appendD(want, int32(ShortcutSkill))
	want = appendD(want, 15)
	want = appendD(want, 248)
	want = appendD(want, 1)
	want = append(want, 0)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameShortCutRegister(skill) = %x, want %x", got, want)
	}
}

func TestFrameShortCutRegisterItem(t *testing.T) {
	got := framePayload(t, FrameShortCutRegister(Shortcut{
		Slot:             2,
		Page:             0,
		Type:             ShortcutItem,
		ID:               57,
		CharacterType:    1,
		SharedReuseGroup: -1,
		RemainingSeconds: 4,
		ReuseSeconds:     12,
		AugmentationID:   12345,
	}))
	want := []byte{OpcodeShortCutRegister}
	want = appendD(want, int32(ShortcutItem))
	want = appendD(want, 2)
	want = appendD(want, 57)
	want = appendD(want, 1)
	want = appendD(want, -1)
	want = appendD(want, 4)
	want = appendD(want, 12)
	want = appendD(want, 12345)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameShortCutRegister(item) = %x, want %x", got, want)
	}
}

func TestFrameShortCutDelete(t *testing.T) {
	got := framePayload(t, FrameShortCutDelete(3, 1))
	want := []byte{OpcodeShortCutDelete}
	want = appendD(want, 15)
	want = appendD(want, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameShortCutDelete() = %x, want %x", got, want)
	}
}

func TestFrameDie(t *testing.T) {
	got := framePayload(t, FrameDie(123, DieOptions{Castle: true}))
	want := []byte{OpcodeDie}
	want = appendD(want, 123)
	want = appendD(want, 1)
	want = appendD(want, 0)
	want = appendD(want, 1)
	want = appendD(want, 0)
	want = appendD(want, 0)
	want = appendD(want, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameDie() = %x, want %x", got, want)
	}
}

func TestFrameExMailArrived(t *testing.T) {
	got := framePayload(t, FrameExMailArrived())
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExMailArrived)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExMailArrived() = %x, want %x", got, want)
	}
}

func TestFramePlaySound(t *testing.T) {
	got := framePayload(t, FramePlaySound("systemmsg_e.1233"))
	want := []byte{OpcodePlaySound}
	want = appendD(want, 0)
	want = append(want, encodeUTF16Z("systemmsg_e.1233")...)
	want = appendD(want, 0)
	want = appendD(want, 0)
	want = appendD(want, 0)
	want = appendD(want, 0)
	want = appendD(want, 0)
	want = appendD(want, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FramePlaySound() = %x, want %x", got, want)
	}
}

func TestFrameNpcHtmlMessage(t *testing.T) {
	got := framePayload(t, FrameNpcHtmlMessage(7, "<html></html>", 57))
	want := []byte{OpcodeNpcHtmlMessage}
	want = appendD(want, 7)
	want = append(want, encodeUTF16Z("<html></html>")...)
	want = appendD(want, 57)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameNpcHtmlMessage() = %x, want %x", got, want)
	}
}

func TestFrameSkillCoolTime(t *testing.T) {
	got := framePayload(t, FrameSkillCoolTime([]SkillCoolTimeEntry{{SkillID: 1, Level: 2, ReuseSeconds: 30, RemainingSeconds: 20}}))
	want := []byte{OpcodeSkillCoolTime}
	want = appendD(want, 1)
	want = appendD(want, 1)
	want = appendD(want, 2)
	want = appendD(want, 30)
	want = appendD(want, 20)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSkillCoolTime() = %x, want %x", got, want)
	}
}

// ---- from exautosoulshot_test.go ----
func TestFrameExAutoSoulShot(t *testing.T) {
	got := framePayload(t, FrameExAutoSoulShot(1463, true))
	want := []byte{
		OpcodeExtended,
		0x12, 0x00,
		0xb7, 0x05, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExAutoSoulShot() = %x, want %x", got, want)
	}
}

func TestFrameExUseSharedGroupItem(t *testing.T) {
	got := framePayload(t, FrameExUseSharedGroupItem(1463, 4, 12_000, 60_000))
	want := []byte{
		OpcodeExtended,
		0x49, 0x00,
		0xb7, 0x05, 0x00, 0x00,
		0x04, 0x00, 0x00, 0x00,
		0x0c, 0x00, 0x00, 0x00,
		0x3c, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExUseSharedGroupItem() = %x, want %x", got, want)
	}
}

// ---- from exregenmax_test.go ----
func TestFrameExRegenMax(t *testing.T) {
	got := framePayload(t, FrameExRegenMax(14, 2, 16))
	want := []byte{OpcodeExtended}
	want = binary.LittleEndian.AppendUint16(want, OpcodeExRegenMax)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 14)
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = binary.LittleEndian.AppendUint64(want, math.Float64bits(16*0.66))
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExRegenMax() = %x, want %x", got, want)
	}
}

// ---- from exsendmanorlist_test.go ----
func TestFrameExSendManorList(t *testing.T) {
	got := framePayload(t, FrameExSendManorList())

	var want []byte
	want = append(want, OpcodeExtended)
	want = binary.LittleEndian.AppendUint16(want, OpcodeExSendManorList)
	want = binary.LittleEndian.AppendUint32(want, uint32(len(manorNames)))
	for i, name := range manorNames {
		want = binary.LittleEndian.AppendUint32(want, uint32(i+1))
		var w wire.Writer
		w.WriteString(name)
		want = append(want, w.Bytes()...)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("FrameExSendManorList() = % x, want % x", got, want)
	}
}

// ---- from flytolocation_test.go ----
func TestFrameFlyToLocation(t *testing.T) {
	got := framePayload(t, FrameFlyToLocation(
		268476516,
		location.Location{X: 46160, Y: 41237, Z: -3534},
		location.Location{X: 12345, Y: -6789, Z: 100},
		skill.FlightThrowUp,
	))
	want := []byte{
		OpcodeFlyToLocation,
		0x64, 0xa0, 0x00, 0x10,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
		0x39, 0x30, 0x00, 0x00,
		0x7b, 0xe5, 0xff, 0xff,
		0x64, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameFlyToLocation() = %x, want %x", got, want)
	}
}

// ---- from frame_test.go ----
func frameBytes(t *testing.T, frame wire.Frame) []byte {
	t.Helper()
	t.Cleanup(frame.Release)
	return frame.Bytes()
}

func framePayload(t *testing.T, frame wire.Frame) []byte {
	t.Helper()
	bytes := frameBytes(t, frame)
	if len(bytes) < 2 {
		t.Fatalf("frame length = %d, want header", len(bytes))
	}
	return bytes[2:]
}

func TestFrameItemListErrorReturnsNoFrame(t *testing.T) {
	items := []*item.Instance{{ObjectID: 1, TemplateID: 999, Count: 1, Location: item.LocationInventory}}

	frame, err := FrameItemList(items, item.NewTable(nil), true)
	if err == nil {
		t.Fatal("FrameItemList err = nil, want an error for a missing template")
	}
	frame.Release() // must be a no-op on the zero frame
	if frame.Bytes() != nil {
		t.Errorf("frame.Bytes() = % X, want nil", frame.Bytes())
	}
}

func TestFrameNewCharacterSuccessErrorReturnsNoFrame(t *testing.T) {
	table, err := player.NewTemplateTable(map[int]*player.Template{0: rootTemplate(0, 1, 2, 3, 4, 5, 6)})
	if err != nil {
		t.Fatalf("build template table: %v", err)
	}

	frame, err := FrameNewCharacterSuccess(table)
	if err == nil {
		t.Fatal("FrameNewCharacterSuccess err = nil, want an error for a missing profession")
	}
	frame.Release()
	if frame.Bytes() != nil {
		t.Errorf("frame.Bytes() = % X, want nil", frame.Bytes())
	}
}

// ---- from grounditems_test.go ----
func TestFrameSpawnItem(t *testing.T) {
	ground := packetGroundItem{id: 100, itemID: 57, count: 500, stackable: true, x: 10, y: 20, z: -30}

	got := framePayload(t, FrameSpawnItem(ground))

	want := []byte{OpcodeSpawnItem}
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = binary.LittleEndian.AppendUint32(want, 57)
	want = binary.LittleEndian.AppendUint32(want, 10)
	want = binary.LittleEndian.AppendUint32(want, 20)
	want = appendInt32(want, -30)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 500)
	want = binary.LittleEndian.AppendUint32(want, 0)
	if string(got) != string(want) {
		t.Fatalf("FrameSpawnItem() = % x, want % x", got, want)
	}
}

func TestFrameDropItem(t *testing.T) {
	ground := packetGroundItem{id: 100, itemID: 10, count: 1, x: 10, y: 20, z: -30}

	got := framePayload(t, FrameDropItem(ground, 200))

	want := []byte{OpcodeDropItem}
	want = binary.LittleEndian.AppendUint32(want, 200)
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = binary.LittleEndian.AppendUint32(want, 10)
	want = binary.LittleEndian.AppendUint32(want, 10)
	want = binary.LittleEndian.AppendUint32(want, 20)
	want = appendInt32(want, -30)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 1)
	if string(got) != string(want) {
		t.Fatalf("FrameDropItem() = % x, want % x", got, want)
	}
}

func TestFrameGetItem(t *testing.T) {
	ground := packetGroundItem{id: 100, itemID: 57, count: 500, stackable: true, x: 10, y: 20, z: -30}

	got := framePayload(t, FrameGetItem(ground, 200))

	want := []byte{OpcodeGetItem}
	want = binary.LittleEndian.AppendUint32(want, 200)
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = binary.LittleEndian.AppendUint32(want, 10)
	want = binary.LittleEndian.AppendUint32(want, 20)
	want = appendInt32(want, -30)
	if string(got) != string(want) {
		t.Fatalf("FrameGetItem() = % x, want % x", got, want)
	}
}

func appendInt32(b []byte, v int32) []byte {
	return binary.LittleEndian.AppendUint32(b, uint32(v))
}

type packetGroundItem struct {
	id, itemID int32
	count      int
	stackable  bool
	x, y, z    int
}

func (p packetGroundItem) ObjectID() int32 { return p.id }
func (p packetGroundItem) ItemID() int32   { return p.itemID }
func (p packetGroundItem) Count() int      { return p.count }
func (p packetGroundItem) Stackable() bool { return p.stackable }
func (p packetGroundItem) Position() (int, int, int) {
	return p.x, p.y, p.z
}

// ---- from inventoryupdate_test.go ----
func TestFrameInventoryUpdate(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand, Duration: -1},
		{ID: 1146, Kind: item.KindArmor, Slot: item.SlotChest, Duration: -1},
	})
	items := []*item.Instance{
		{ObjectID: 100, TemplateID: 2368, Count: 1, Location: item.LocationPaperdoll, LocationData: 7, EnchantLevel: 5, CustomType1: 3, CustomType2: 4, ManaLeft: -1, Augmentation: &item.Augmentation{Attributes: 777}},
		{ObjectID: 101, TemplateID: 1146, Count: 1, Location: item.LocationInventory, ManaLeft: -1},
	}
	updates := []itemcontainer.Update{
		{ObjectID: 100, TemplateID: 2368, Count: 1, State: itemcontainer.UpdateModified},
		{ObjectID: 101, TemplateID: 1146, Count: 1, State: itemcontainer.UpdateRemoved},
	}

	frame, err := FrameInventoryUpdate(updates, items, templates)
	if err != nil {
		t.Fatalf("FrameInventoryUpdate: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeInventoryUpdate}
	want = binary.LittleEndian.AppendUint16(want, 2)

	want = binary.LittleEndian.AppendUint16(want, uint16(itemcontainer.UpdateModified))
	want = binary.LittleEndian.AppendUint16(want, uint16(item.CategoryWeaponOrJewelry))
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = binary.LittleEndian.AppendUint32(want, 2368)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint16(want, uint16(item.SubCategoryWeapon))
	want = binary.LittleEndian.AppendUint16(want, 3)
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = binary.LittleEndian.AppendUint32(want, uint32(item.SlotLRHand))
	want = binary.LittleEndian.AppendUint16(want, 5)
	want = binary.LittleEndian.AppendUint16(want, 4)
	want = binary.LittleEndian.AppendUint32(want, 777)
	want = binary.LittleEndian.AppendUint32(want, uint32(noManaLeft))

	want = binary.LittleEndian.AppendUint16(want, uint16(itemcontainer.UpdateRemoved))
	want = binary.LittleEndian.AppendUint16(want, uint16(item.CategoryArmor))
	want = binary.LittleEndian.AppendUint32(want, 101)
	want = binary.LittleEndian.AppendUint32(want, 1146)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint16(want, uint16(item.SubCategoryArmor))
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint32(want, uint32(item.SlotChest))
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, uint32(noManaLeft))

	if !bytes.Equal(got, want) {
		t.Errorf("FrameInventoryUpdate mismatch:\n got  %x\n want %x", got, want)
	}
}

func TestFrameInventoryUpdateMissingTemplate(t *testing.T) {
	_, err := FrameInventoryUpdate([]itemcontainer.Update{{ObjectID: 1, TemplateID: 999, Count: 1}}, nil, item.NewTable(nil))
	if err == nil {
		t.Fatal("FrameInventoryUpdate: want error for missing template")
	}
}

// ---- from itemlist_test.go ----
// noManaLeft is the displayed-mana-left placeholder FrameItemList always
// writes (item.Instance carries no shadow-item duration state), kept as a
// variable so converting it to uint32 below is a runtime, not constant,
// conversion.
var noManaLeft int32 = -1

func TestFrameItemList(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand},
		{ID: 1146, Kind: item.KindArmor, Slot: item.SlotChest},
		{ID: item.AdenaID, Kind: item.KindEtcItem, Slot: item.SlotNone},
	})
	items := []*item.Instance{
		{ObjectID: 100, TemplateID: 2368, Count: 1, Location: item.LocationPaperdoll, LocationData: 7, EnchantLevel: 5},
		{ObjectID: 101, TemplateID: 1146, Count: 1, Location: item.LocationPaperdoll, LocationData: 10},
		{ObjectID: 102, TemplateID: item.AdenaID, Count: 500, Location: item.LocationInventory},
		{ObjectID: 103, TemplateID: 1146, Count: 1, Location: item.LocationWarehouse}, // excluded: not carried
	}

	frame, err := FrameItemList(items, templates, true)
	if err != nil {
		t.Fatalf("FrameItemList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeItemList}
	want = binary.LittleEndian.AppendUint16(want, 1) // show window
	want = binary.LittleEndian.AppendUint16(want, 3) // carried item count

	want = binary.LittleEndian.AppendUint16(want, uint16(item.CategoryWeaponOrJewelry))
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = binary.LittleEndian.AppendUint32(want, 2368)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint16(want, uint16(item.SubCategoryWeapon))
	want = binary.LittleEndian.AppendUint16(want, 0) // custom type 1
	want = binary.LittleEndian.AppendUint16(want, 1) // equipped
	want = binary.LittleEndian.AppendUint32(want, uint32(item.SlotLRHand))
	want = binary.LittleEndian.AppendUint16(want, 5) // enchant level
	want = binary.LittleEndian.AppendUint16(want, 0) // custom type 2
	want = binary.LittleEndian.AppendUint32(want, 0) // augmentation id
	want = binary.LittleEndian.AppendUint32(want, uint32(noManaLeft))

	want = binary.LittleEndian.AppendUint16(want, uint16(item.CategoryArmor))
	want = binary.LittleEndian.AppendUint32(want, 101)
	want = binary.LittleEndian.AppendUint32(want, 1146)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint16(want, uint16(item.SubCategoryArmor))
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = binary.LittleEndian.AppendUint32(want, uint32(item.SlotChest))
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, uint32(noManaLeft))

	want = binary.LittleEndian.AppendUint16(want, uint16(item.CategoryMoneyOrEtcItem))
	want = binary.LittleEndian.AppendUint32(want, 102)
	want = binary.LittleEndian.AppendUint32(want, uint32(item.AdenaID))
	want = binary.LittleEndian.AppendUint32(want, 500)
	want = binary.LittleEndian.AppendUint16(want, uint16(item.SubCategoryMoney))
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint16(want, 0) // not equipped
	want = binary.LittleEndian.AppendUint32(want, uint32(item.SlotNone))
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint16(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, uint32(noManaLeft))

	if !bytes.Equal(got, want) {
		t.Errorf("FrameItemList mismatch:\n got  %x\n want %x", got, want)
	}
}

func TestFrameItemList_HideWindow(t *testing.T) {
	frame, err := FrameItemList(nil, item.NewTable(nil), false)
	if err != nil {
		t.Fatalf("FrameItemList: %v", err)
	}
	got := framePayload(t, frame)
	want := []byte{OpcodeItemList, 0, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("FrameItemList (empty, hidden) = %x, want %x", got, want)
	}
}

// TestWriteItemListNoAuxiliaryAllocation guards against writeItemList
// reintroducing a filtered snapshot slice: with a pre-sized Writer and
// already-locked item.Instances, encoding must not allocate.
func TestWriteItemListNoAuxiliaryAllocation(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand},
		{ID: 1146, Kind: item.KindArmor, Slot: item.SlotChest},
		{ID: item.AdenaID, Kind: item.KindEtcItem, Slot: item.SlotNone},
	})
	items := []*item.Instance{
		{ObjectID: 100, TemplateID: 2368, Count: 1, Location: item.LocationPaperdoll, LocationData: 7, EnchantLevel: 5},
		{ObjectID: 101, TemplateID: 1146, Count: 1, Location: item.LocationPaperdoll, LocationData: 10},
		{ObjectID: 102, TemplateID: item.AdenaID, Count: 500, Location: item.LocationInventory},
		{ObjectID: 103, TemplateID: 1146, Count: 1, Location: item.LocationWarehouse}, // excluded: not carried
	}

	var w wire.Writer
	w.ResetFrame(256) // pre-size so append growth doesn't allocate during the measured run
	if err := writeItemList(&w, items, templates, true); err != nil {
		t.Fatalf("writeItemList: %v", err) // warm up per-item mutex lazy-init before measuring
	}
	warm := append([]byte(nil), w.Bytes()...)

	allocs := testing.AllocsPerRun(100, func() {
		w.ResetFrame(256)
		if err := writeItemList(&w, items, templates, true); err != nil {
			t.Fatalf("writeItemList: %v", err)
		}
	})
	if allocs != 0 {
		t.Errorf("writeItemList allocs/run = %v, want 0 (no auxiliary filter/snapshot slice)", allocs)
	}

	w.ResetFrame(256)
	if err := writeItemList(&w, items, templates, true); err != nil {
		t.Fatalf("writeItemList: %v", err)
	}
	if !bytes.Equal(w.Bytes(), warm) {
		t.Errorf("writeItemList output changed across repeated runs:\n got  %x\n want %x", w.Bytes(), warm)
	}
}

// ---- from lifecycle_test.go ----
func TestFrameRestartResponse(t *testing.T) {
	tests := []struct {
		name string
		ok   bool
		want []byte
	}{
		{"success", true, []byte{OpcodeRestartResponse, 1, 0, 0, 0}},
		{"failure", false, []byte{OpcodeRestartResponse, 0, 0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := framePayload(t, FrameRestartResponse(tt.ok))
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("FrameRestartResponse(%v) = %x, want %x", tt.ok, got, tt.want)
			}
		})
	}
}

func TestFrameLeaveWorld(t *testing.T) {
	got := framePayload(t, FrameLeaveWorld())
	want := []byte{OpcodeLeaveWorld}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameLeaveWorld() = %x, want %x", got, want)
	}
}

func TestFrameRevive(t *testing.T) {
	got := framePayload(t, FrameRevive(100))
	want := []byte{OpcodeRevive, 100, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameRevive() = %x, want %x", got, want)
	}
}

// ---- from magic_skill_test.go ----
func TestFrameMagicSkillUse(t *testing.T) {
	got := framePayload(t, FrameMagicSkillUse(
		SkillCastObject{ObjectID: 100, Location: location.Location{X: 10, Y: 20, Z: 30}},
		SkillCastObject{ObjectID: 200, Location: location.Location{X: 40, Y: 50, Z: 60}},
		3, 1, 500, 1200, false,
	))

	want := []byte{OpcodeMagicSkillUse}
	for _, v := range []uint32{100, 200, 3, 1, 500, 1200, 10, 20, 30, 0, 40, 50, 60} {
		want = binary.LittleEndian.AppendUint32(want, v)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMagicSkillUse() = %x, want %x", got, want)
	}
}

func TestFrameMagicSkillLaunched(t *testing.T) {
	got := framePayload(t, FrameMagicSkillLaunched(100, 3, 1, []int32{200, 300}))

	want := []byte{OpcodeMagicSkillLaunched}
	for _, v := range []uint32{100, 3, 1, 2, 200, 300} {
		want = binary.LittleEndian.AppendUint32(want, v)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMagicSkillLaunched() = %x, want %x", got, want)
	}
}

func TestFrameMagicSkillLaunchedNoTargets(t *testing.T) {
	got := framePayload(t, FrameMagicSkillLaunched(100, 3, 1, nil))

	want := []byte{OpcodeMagicSkillLaunched}
	for _, v := range []uint32{100, 3, 1, 0, 0} {
		want = binary.LittleEndian.AppendUint32(want, v)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMagicSkillLaunched(nil) = %x, want %x", got, want)
	}
}

func TestFrameSetupGauge(t *testing.T) {
	got := framePayload(t, FrameSetupGauge(GaugeBlue, 500, 1200))

	want := []byte{OpcodeSetupGauge}
	for _, v := range []uint32{uint32(GaugeBlue), 500, 1200} {
		want = binary.LittleEndian.AppendUint32(want, v)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSetupGauge() = %x, want %x", got, want)
	}
}

func TestFrameMagicSkillCanceled(t *testing.T) {
	got := framePayload(t, FrameMagicSkillCanceled(100))
	want := []byte{OpcodeMagicSkillCanceled, 100, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMagicSkillCanceled() = %x, want %x", got, want)
	}
}

// ---- from move_test.go ----
func TestFrameMoveLocationEvent(t *testing.T) {
	event := move.Event{
		Origin:      location.Location{X: 10, Y: 20, Z: 30},
		Destination: location.Location{X: 100, Y: 200, Z: 300},
	}
	got := framePayload(t, FrameMove(101, event))
	want := []byte{
		0x01,
		0x65, 0x00, 0x00, 0x00,
		0x64, 0x00, 0x00, 0x00,
		0xc8, 0x00, 0x00, 0x00,
		0x2c, 0x01, 0x00, 0x00,
		0x0a, 0x00, 0x00, 0x00,
		0x14, 0x00, 0x00, 0x00,
		0x1e, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMove() = %x, want MoveToLocation %x", got, want)
	}
}

func TestFrameMoveFollowEvent(t *testing.T) {
	event := move.Event{
		Origin:       location.Location{X: 10, Y: 20, Z: 30},
		FollowTarget: 202,
		FollowOffset: 40,
	}
	got := framePayload(t, FrameMove(101, event))
	want := []byte{
		0x60,
		0x65, 0x00, 0x00, 0x00,
		0xca, 0x00, 0x00, 0x00,
		0x28, 0x00, 0x00, 0x00,
		0x0a, 0x00, 0x00, 0x00,
		0x14, 0x00, 0x00, 0x00,
		0x1e, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMove() = %x, want MoveToPawn %x", got, want)
	}
}

// ---- from movement_control_test.go ----
func TestFrameStopMove(t *testing.T) {
	got := framePayload(t, FrameStopMove(268476516, location.Location{X: 46160, Y: 41237, Z: -3534}, 32768))
	want := []byte{
		OpcodeStopMove,
		0x64, 0xa0, 0x00, 0x10,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
		0x00, 0x80, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameStopMove() = %x, want %x", got, want)
	}
}

func TestFrameValidateLocation(t *testing.T) {
	got := framePayload(t, FrameValidateLocation(268476516, location.Location{X: 46160, Y: 41237, Z: -3534}, 32768))
	want := []byte{
		OpcodeValidateLocation,
		0x64, 0xa0, 0x00, 0x10,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
		0x00, 0x80, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameValidateLocation() = %x, want %x", got, want)
	}
}

func TestFrameStartRotation(t *testing.T) {
	got := framePayload(t, FrameStartRotation(268476516, 32768, 1, 0))
	want := []byte{
		OpcodeStartRotation,
		0x64, 0xa0, 0x00, 0x10,
		0x00, 0x80, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameStartRotation() = %x, want %x", got, want)
	}
}

func TestFrameStopRotation(t *testing.T) {
	got := framePayload(t, FrameStopRotation(268476516, 0x1234, 0))
	want := []byte{
		OpcodeStopRotation,
		0x64, 0xa0, 0x00, 0x10,
		0x34, 0x12, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x34,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameStopRotation() = %x, want %x", got, want)
	}
}

// ---- from movetolocation_test.go ----
func TestFrameMoveToLocation(t *testing.T) {
	got := framePayload(t, FrameMoveToLocation(
		268476516,
		location.Location{X: 46160, Y: 41237, Z: -3534},
		location.Location{X: 46117, Y: 41247, Z: -3532},
	))
	want := []byte{
		0x01,
		0x64, 0xa0, 0x00, 0x10,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
		0x25, 0xb4, 0x00, 0x00,
		0x1f, 0xa1, 0x00, 0x00,
		0x34, 0xf2, 0xff, 0xff,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMoveToLocation() = %x, want %x", got, want)
	}
}

// ---- from movetopawn_test.go ----
func TestFrameMoveToPawn(t *testing.T) {
	got := framePayload(t, FrameMoveToPawn(268476516, 268480061, 70, location.Location{X: -71440, Y: 258000, Z: -3104}))
	want := []byte{
		0x60,
		0x64, 0xa0, 0x00, 0x10,
		0x3d, 0xae, 0x00, 0x10,
		0x46, 0x00, 0x00, 0x00,
		0xf0, 0xe8, 0xfe, 0xff,
		0xd0, 0xef, 0x03, 0x00,
		0xe0, 0xf3, 0xff, 0xff,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMoveToPawn() = %x, want %x", got, want)
	}
}

// ---- from newcharactersuccess_test.go ----
func rootTemplate(id, str, dex, con, intl, wit, men int) *player.Template {
	return &player.Template{
		ID: id, BaseLevel: 1,
		STR: str, DEX: dex, CON: con, INT: intl, WIT: wit, MEN: men,
	}
}

func allRootTemplates(t *testing.T) *player.TemplateTable {
	t.Helper()
	templates := map[int]*player.Template{
		0:  rootTemplate(0, 40, 30, 43, 21, 11, 25),
		10: rootTemplate(10, 21, 22, 23, 24, 25, 26),
		18: rootTemplate(18, 1, 2, 3, 4, 5, 6),
		25: rootTemplate(25, 1, 2, 3, 4, 5, 6),
		31: rootTemplate(31, 1, 2, 3, 4, 5, 6),
		38: rootTemplate(38, 1, 2, 3, 4, 5, 6),
		44: rootTemplate(44, 1, 2, 3, 4, 5, 6),
		49: rootTemplate(49, 1, 2, 3, 4, 5, 6),
		53: rootTemplate(53, 1, 2, 3, 4, 5, 6),
	}
	table, err := player.NewTemplateTable(templates)
	if err != nil {
		t.Fatalf("build template table: %v", err)
	}
	return table
}

func TestFrameNewCharacterSuccess(t *testing.T) {
	frame, err := FrameNewCharacterSuccess(allRootTemplates(t))
	if err != nil {
		t.Fatalf("FrameNewCharacterSuccess: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeNewCharacterSuccess}
	want = binary.LittleEndian.AppendUint32(want, uint32(len(creationScreenClassIDs)))
	for _, id := range creationScreenClassIDs {
		race, _ := player.ClassRace(id)
		want = binary.LittleEndian.AppendUint32(want, uint32(race))
		want = binary.LittleEndian.AppendUint32(want, uint32(id))

		tmpl := map[int][6]int{
			0:  {40, 30, 43, 21, 11, 25},
			10: {21, 22, 23, 24, 25, 26},
			18: {1, 2, 3, 4, 5, 6},
			25: {1, 2, 3, 4, 5, 6},
			31: {1, 2, 3, 4, 5, 6},
			38: {1, 2, 3, 4, 5, 6},
			44: {1, 2, 3, 4, 5, 6},
			49: {1, 2, 3, 4, 5, 6},
			53: {1, 2, 3, 4, 5, 6},
		}[id]
		for _, v := range tmpl {
			want = binary.LittleEndian.AppendUint32(want, 0x46)
			want = binary.LittleEndian.AppendUint32(want, uint32(v))
			want = binary.LittleEndian.AppendUint32(want, 0x0a)
		}
	}

	if !bytes.Equal(got, want) {
		t.Errorf("FrameNewCharacterSuccess mismatch:\n got  %x\n want %x", got, want)
	}
}

func TestFrameNewCharacterSuccess_MissingTemplate(t *testing.T) {
	table, err := player.NewTemplateTable(map[int]*player.Template{0: rootTemplate(0, 1, 1, 1, 1, 1, 1)})
	if err != nil {
		t.Fatalf("build template table: %v", err)
	}
	frame, err := FrameNewCharacterSuccess(table)
	frame.Release()
	if err == nil {
		t.Error("FrameNewCharacterSuccess: want error for missing profession, got nil")
	}
}

// ---- from npcinfo_test.go ----
func TestFrameServerObjectInfo(t *testing.T) {
	got := framePayload(t, FrameServerObjectInfo(NPCInfoSnapshot{
		ObjectID: 0x01020304, TemplateID: 123, Name: "Goblin", Attackable: true,
		X: -1, Y: 2, Z: -3, Heading: 4, CollisionRadius: 5.5, CollisionHeight: 6.5,
		CurrentHP: 70, MaxHP: 100,
	}))
	want := []byte{OpcodeServerObjectInfo}
	for _, value := range []uint32{0x01020304, 1000123} {
		want = binary.LittleEndian.AppendUint32(want, value)
	}
	for _, char := range []uint16{'G', 'o', 'b', 'l', 'i', 'n', 0} {
		want = binary.LittleEndian.AppendUint16(want, char)
	}
	for _, value := range []uint32{1, 0xffffffff, 2, 0xfffffffd, 4} {
		want = binary.LittleEndian.AppendUint32(want, value)
	}
	for _, value := range []float64{1, 1.1, 5.5, 6.5} {
		want = binary.LittleEndian.AppendUint64(want, math.Float64bits(value))
	}
	for _, value := range []uint32{70, 100, 1, 0} {
		want = binary.LittleEndian.AppendUint32(want, value)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameServerObjectInfo() = %x, want %x", got, want)
	}
}

func TestFrameNPCInfoWritesAttackSpeedMultiplier(t *testing.T) {
	payload := framePayload(t, FrameNPCInfo(NPCInfoSnapshot{}))
	const multiplierOffset = 1 + 18*4
	if got := math.Float64frombits(binary.LittleEndian.Uint64(payload[multiplierOffset+8:])); got != 1.1 {
		t.Fatalf("attack speed multiplier = %v, want 1.1", got)
	}
}

func TestFrameNPCInfoWritesPvpFlagAndKarma(t *testing.T) {
	payload := framePayload(t, FrameNPCInfo(NPCInfoSnapshot{
		Name: "N", Title: "T", Summon: true, PvpFlag: 1, Karma: 500,
	}))
	fields := []byte{'N', 0, 0, 0, 'T', 0, 0, 0}
	offset := bytes.Index(payload, fields)
	if offset < 0 {
		t.Fatal("name/title fields missing")
	}
	got := payload[offset+len(fields):]
	if len(got) < 12 || binary.LittleEndian.Uint32(got[:4]) != 1 || binary.LittleEndian.Uint32(got[4:]) != 1 || binary.LittleEndian.Uint32(got[8:]) != 500 {
		t.Fatalf("summon/pvp/karma fields = %x, want 1/1/500", got)
	}
}

func TestFrameNPCInfoWritesAbnormalEffect(t *testing.T) {
	payload := framePayload(t, FrameNPCInfo(NPCInfoSnapshot{Name: "N", Title: "T", AbnormalEffect: 0x010000}))
	fields := []byte{'N', 0, 0, 0, 'T', 0, 0, 0}
	offset := bytes.Index(payload, fields)
	if offset < 0 {
		t.Fatal("name/title fields missing")
	}
	got := payload[offset+len(fields):]
	if len(got) < 16 || binary.LittleEndian.Uint32(got[12:16]) != 0x010000 {
		t.Fatalf("abnormal effect = %x, want 01000000", got)
	}
}

// ---- from npcsay_test.go ----
func TestFrameNpcSay(t *testing.T) {
	got := framePayload(t, FrameNpcSay(500, 12564, SayTypeAll, "Hello"))
	want := []byte{OpcodeNpcSay}
	want = appendD(want, 500)
	want = appendD(want, 0)
	want = appendD(want, 1012564)
	want = append(want, encodeUTF16Z("Hello")...)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameNpcSay() = %x, want %x", got, want)
	}
}

// TestFrameNpcSayResolvesNpcStringId pins issue #2028: a walkerRoutes.xml
// node's fstring resolves through the ported npcstring table (aCis
// NpcStringId.getMessage()) before broadcasting via NpcSay.
func TestFrameNpcSayResolvesNpcStringId(t *testing.T) {
	text, ok := npcstring.Text(3)
	if !ok || text != "Opening" {
		t.Fatalf("npcstring.Text(3) = (%q, %v), want (\"Opening\", true)", text, ok)
	}

	got := framePayload(t, FrameNpcSay(12345, 31357, SayTypeAll, text))
	want := []byte{OpcodeNpcSay}
	want = appendD(want, 12345)
	want = appendD(want, 0)
	want = appendD(want, 1_000_000+31357)
	want = append(want, encodeUTF16Z(text)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameNpcSay() = %x, want %x", got, want)
	}
}

// ---- from pet_test.go ----
func petPacketTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{
			ID:        2375,
			Kind:      item.KindWeapon,
			Slot:      item.SlotWolf,
			Stackable: false,
			Weapon:    &item.WeaponDetail{Type: item.WeaponPet},
		},
		{
			ID:        57,
			Kind:      item.KindEtcItem,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{},
		},
	})
}

func TestFramePetStatusShow(t *testing.T) {
	got := framePayload(t, FramePetStatusShow(2))
	want := []byte{OpcodePetStatusShow, 0x02, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FramePetStatusShow() = %x, want %x", got, want)
	}
}

func TestFramePetDelete(t *testing.T) {
	got := framePayload(t, FramePetDelete(2, 0x01020304))
	want := []byte{OpcodePetDelete, 0x02, 0x00, 0x00, 0x00, 0x04, 0x03, 0x02, 0x01}
	if !bytes.Equal(got, want) {
		t.Fatalf("FramePetDelete() = %x, want %x", got, want)
	}
}

func TestFramePetItemList(t *testing.T) {
	items := []*item.Instance{{ObjectID: 0x01020304, TemplateID: 57, Count: 10, Location: item.LocationPet}}
	frame, err := FramePetItemList(items, petPacketTemplates())
	if err != nil {
		t.Fatalf("FramePetItemList: %v", err)
	}
	got := framePayload(t, frame)
	want := []byte{
		OpcodePetItemList,
		0x01, 0x00,
		0x04, 0x00,
		0x04, 0x03, 0x02, 0x01,
		0x39, 0x00, 0x00, 0x00,
		0x0a, 0x00, 0x00, 0x00,
		0x04, 0x00,
		0x00, 0x00,
		0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00,
		0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FramePetItemList() = %x, want %x", got, want)
	}
}

func TestFramePetItemListRejectsOversizedCount(t *testing.T) {
	items := make([]*item.Instance, 1<<16)
	if _, err := FramePetItemList(items, item.NewTable(nil)); err == nil {
		t.Fatal("FramePetItemList oversized count error = nil, want error")
	}
}

func TestFramePetInventoryUpdate(t *testing.T) {
	templates := petPacketTemplates()
	items := []*item.Instance{{ObjectID: 0x01020304, TemplateID: 57, Count: 10, Location: item.LocationPet}}
	updates := []itemcontainer.Update{{ObjectID: 0x01020304, TemplateID: 57, Count: 10, State: itemcontainer.UpdateModified}}

	frame, err := FramePetInventoryUpdate(updates, items, templates)
	if err != nil {
		t.Fatalf("FramePetInventoryUpdate: %v", err)
	}
	got := framePayload(t, frame)
	want := []byte{
		OpcodePetInventoryUpdate,
		0x01, 0x00,
		0x02, 0x00,
		0x04, 0x00,
		0x04, 0x03, 0x02, 0x01,
		0x39, 0x00, 0x00, 0x00,
		0x0a, 0x00, 0x00, 0x00,
		0x04, 0x00,
		0x00, 0x00,
		0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00,
		0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FramePetInventoryUpdate() = %x, want %x", got, want)
	}
}

// ---- from petinfo_test.go ----
func appendPetInfoInt32(b []byte, v int32) []byte {
	return binary.LittleEndian.AppendUint32(b, uint32(v))
}

func appendPetInfoInt64(b []byte, v int64) []byte {
	return binary.LittleEndian.AppendUint64(b, uint64(v))
}

func appendPetInfoFloat64(b []byte, v float64) []byte {
	return binary.LittleEndian.AppendUint64(b, math.Float64bits(v))
}

func appendPetInfoUint16(b []byte, v uint16) []byte {
	return binary.LittleEndian.AppendUint16(b, v)
}

func appendPetInfoString(b []byte, s string) []byte {
	for _, r := range s {
		b = appendPetInfoUint16(b, uint16(r))
	}
	return appendPetInfoUint16(b, 0)
}

func TestFramePetInfo(t *testing.T) {
	s := PetInfoSnapshot{
		SummonType: 2, ObjectID: 20, TemplateID: 12077,
		X: 100, Y: 200, Z: -50, Heading: 123,
		MAtkSpd: 333, PAtkSpd: 300,
		RunSpd: 120, WalkSpd: 60,
		CollisionRadius: 8, CollisionHeight: 20,
		InCombat: true, AlikeDead: false,
		Name: "Wolf", Title: "",
		PvpFlag: 0, Karma: 5,
		CurFed: 90, MaxFed: 100,
		CurHP: 50, MaxHP: 100, CurMP: 20, MaxMP: 30,
		SP: 0, Level: 5,
		Exp: 1000, ExpForThisLevel: 1000, ExpForNextLevel: 2000,
		TotalWeight: 10, WeightLimit: 5000,
		PAtk: 10, PDef: 20, MAtk: 5, MDef: 15,
		Accuracy: 30, EvasionRate: 25, CriticalHit: 4, MoveSpeed: 120,
		Mountable:         true,
		AbnormalEffect:    0x010000,
		Team:              0,
		SoulShotsPerHit:   1,
		SpiritShotsPerHit: 1,
	}
	got := framePayload(t, FramePetInfo(s))

	var want []byte
	want = append(want, OpcodePetInfo)
	want = appendPetInfoInt32(want, int32(s.SummonType))
	want = appendPetInfoInt32(want, s.ObjectID)
	want = appendPetInfoInt32(want, int32(s.TemplateID+1000000))
	want = appendPetInfoInt32(want, 0)
	want = appendPetInfoInt32(want, int32(s.X))
	want = appendPetInfoInt32(want, int32(s.Y))
	want = appendPetInfoInt32(want, int32(s.Z))
	want = appendPetInfoInt32(want, int32(s.Heading))
	want = appendPetInfoInt32(want, 0)
	want = appendPetInfoInt32(want, int32(s.MAtkSpd))
	want = appendPetInfoInt32(want, int32(s.PAtkSpd))
	for range 4 {
		want = appendPetInfoInt32(want, int32(s.RunSpd))
		want = appendPetInfoInt32(want, int32(s.WalkSpd))
	}
	want = appendPetInfoFloat64(want, 1)
	want = appendPetInfoFloat64(want, 1)
	want = appendPetInfoFloat64(want, s.CollisionRadius)
	want = appendPetInfoFloat64(want, s.CollisionHeight)
	want = appendPetInfoInt32(want, 0)
	want = appendPetInfoInt32(want, 0)
	want = appendPetInfoInt32(want, 0)
	want = append(want, 1) // owner present
	want = append(want, 1) // literal
	want = append(want, 1) // InCombat
	want = append(want, 0) // AlikeDead
	want = append(want, 2) // always-true show-summon-animation
	want = appendPetInfoString(want, s.Name)
	want = appendPetInfoString(want, s.Title)
	want = appendPetInfoInt32(want, 1)
	want = appendPetInfoInt32(want, int32(s.PvpFlag))
	want = appendPetInfoInt32(want, int32(s.Karma))
	want = appendPetInfoInt32(want, int32(s.CurFed))
	want = appendPetInfoInt32(want, int32(s.MaxFed))
	want = appendPetInfoInt32(want, int32(s.CurHP))
	want = appendPetInfoInt32(want, int32(s.MaxHP))
	want = appendPetInfoInt32(want, int32(s.CurMP))
	want = appendPetInfoInt32(want, int32(s.MaxMP))
	want = appendPetInfoInt32(want, int32(s.SP))
	want = appendPetInfoInt32(want, int32(s.Level))
	want = appendPetInfoInt64(want, s.Exp)
	want = appendPetInfoInt64(want, s.ExpForThisLevel)
	want = appendPetInfoInt64(want, s.ExpForNextLevel)
	want = appendPetInfoInt32(want, int32(s.TotalWeight))
	want = appendPetInfoInt32(want, int32(s.WeightLimit))
	want = appendPetInfoInt32(want, int32(s.PAtk))
	want = appendPetInfoInt32(want, int32(s.PDef))
	want = appendPetInfoInt32(want, int32(s.MAtk))
	want = appendPetInfoInt32(want, int32(s.MDef))
	want = appendPetInfoInt32(want, int32(s.Accuracy))
	want = appendPetInfoInt32(want, int32(s.EvasionRate))
	want = appendPetInfoInt32(want, int32(s.CriticalHit))
	want = appendPetInfoInt32(want, int32(s.MoveSpeed))
	want = appendPetInfoInt32(want, int32(s.PAtkSpd))
	want = appendPetInfoInt32(want, int32(s.MAtkSpd))
	want = appendPetInfoInt32(want, int32(s.AbnormalEffect))
	want = appendPetInfoUint16(want, 1) // mountable
	want = append(want, 0)              // move type
	want = appendPetInfoUint16(want, 0)
	want = append(want, byte(s.Team))
	want = appendPetInfoInt32(want, int32(s.SoulShotsPerHit))
	want = appendPetInfoInt32(want, int32(s.SpiritShotsPerHit))

	if !bytes.Equal(got, want) {
		t.Fatalf("FramePetInfo() =\n% x\nwant\n% x", got, want)
	}
}

func TestFramePetStatusUpdate(t *testing.T) {
	s := PetInfoSnapshot{
		SummonType: 2, ObjectID: 20, X: 100, Y: 200, Z: -50, Title: "Companion",
		CurFed: 80, MaxFed: 120, CurHP: 450, MaxHP: 500, CurMP: 90, MaxMP: 100,
		Level: 44, Exp: 1_000, ExpForThisLevel: 900, ExpForNextLevel: 1_100,
	}
	got := framePayload(t, FramePetStatusUpdate(s))

	want := []byte{OpcodePetStatusUpdate}
	for _, value := range []int32{2, 20, 100, 200, -50} {
		want = appendPetInfoInt32(want, value)
	}
	want = appendPetInfoString(want, "Companion")
	for _, value := range []int32{80, 120, 450, 500, 90, 100, 44} {
		want = appendPetInfoInt32(want, value)
	}
	for _, value := range []int64{1_000, 900, 1_100} {
		want = appendPetInfoInt64(want, value)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FramePetStatusUpdate() = %x, want %x", got, want)
	}
}

// ---- from pledgememberlist_test.go ----
func TestFramePledgeShowMemberListUpdate(t *testing.T) {
	got := framePayload(t, FramePledgeShowMemberListUpdate(PledgeMemberListMember{
		Name:           "Rhea",
		Level:          52,
		ClassID:        16,
		Sex:            1,
		Race:           2,
		OnlineObjectID: 9981,
		PledgeType:     1001,
		HasSponsor:     true,
	}))

	want := []byte{OpcodePledgeShowMemberListUpdate}
	want = append(want, encodeUTF16Z("Rhea")...)
	want = appendD(want, 52)
	want = appendD(want, 16)
	want = appendD(want, 1)
	want = appendD(want, 2)
	want = appendD(want, 9981)
	want = appendD(want, 1001)
	want = appendD(want, 1)

	if !bytes.Equal(got, want) {
		t.Fatalf("FramePledgeShowMemberListUpdate() = %x, want %x", got, want)
	}
}

func TestFramePledgeShowMemberListAll(t *testing.T) {
	got := framePayload(t, FramePledgeShowMemberListAll(PledgeMemberList{
		ClanID:      501,
		PledgeType:  1001,
		PledgeName:  "Knights",
		LeaderName:  "Captain",
		CrestID:     77,
		Level:       5,
		CastleID:    1,
		ClanHallID:  2,
		Rank:        3,
		Reputation:  4500,
		Dissolving:  true,
		AllyID:      88,
		AllyName:    "Alliance",
		AllyCrestID: 99,
		AtWar:       true,
		Members: []PledgeMemberListMember{
			{Name: "Rhea", Level: 52, ClassID: 16, Sex: 1, Race: 2, OnlineObjectID: 9981, PledgeType: 1001, HasSponsor: true},
			{Name: "Main", Level: 60, ClassID: 22, PledgeType: 0},
		},
	}))

	want := []byte{OpcodePledgeShowMemberListAll}
	want = appendD(want, 1)
	want = appendD(want, 501)
	want = appendD(want, 1001)
	want = append(want, encodeUTF16Z("Knights")...)
	want = append(want, encodeUTF16Z("Captain")...)
	want = appendD(want, 77)
	want = appendD(want, 5)
	want = appendD(want, 1)
	want = appendD(want, 2)
	want = appendD(want, 3)
	want = appendD(want, 4500)
	want = appendD(want, 3)
	want = appendD(want, 0)
	want = appendD(want, 88)
	want = append(want, encodeUTF16Z("Alliance")...)
	want = appendD(want, 99)
	want = appendD(want, 1)
	want = appendD(want, 1)
	want = append(want, encodeUTF16Z("Rhea")...)
	want = appendD(want, 52)
	want = appendD(want, 16)
	want = appendD(want, 1)
	want = appendD(want, 2)
	want = appendD(want, 9981)
	want = appendD(want, 1)

	if !bytes.Equal(got, want) {
		t.Fatalf("FramePledgeShowMemberListAll() = %x, want %x", got, want)
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

// ---- from ride_test.go ----
func TestFrameRideMountWyvern(t *testing.T) {
	want := []byte{OpcodeRide}
	want = binary.LittleEndian.AppendUint32(want, 7)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = binary.LittleEndian.AppendUint32(want, 1012621)
	if got := framePayload(t, FrameRide(7, 12621)); string(got) != string(want) {
		t.Fatalf("Ride = %x, want %x", got, want)
	}
}

// ---- from shop_trade_test.go ----
func TestFrameBuyList(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 57, Kind: item.KindEtcItem, Slot: item.SlotNone},
		{ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand},
	})
	list := buylist.List{ID: 101, Products: []buylist.Product{
		{ItemID: 57, Price: 1, MaxCount: -1},
		{ItemID: 2368, Price: 625, MaxCount: 3},
	}}

	frame, err := FrameBuyList(list, 123456, 0.10, 1.0, templates)
	if err != nil {
		t.Fatalf("FrameBuyList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeBuyList}
	want = binary.LittleEndian.AppendUint32(want, 123456)
	want = binary.LittleEndian.AppendUint32(want, 101)
	want = binary.LittleEndian.AppendUint16(want, 2)
	want = appendShopTradeItem(want, item.CategoryMoneyOrEtcItem, 57, 57, 0, item.SubCategoryMoney, item.SlotNone, 0, 0, 0, 1)
	want = appendShopTradeItem(want, item.CategoryWeaponOrJewelry, 2368, 2368, 3, item.SubCategoryWeapon, item.SlotLRHand, 0, 0, 0, 687)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameBuyList() = %x, want %x", got, want)
	}
}

// TestFrameBuyListSiegeGuardPrice proves siege-guard buylist items (IDs
// 3960-4026) price with Config.RATE_SIEGE_GUARDS_PRICE in addition to tax,
// per BuyList.java:47-50, while items outside that range use the plain
// price*(1+taxRate) formula.
func TestFrameBuyListSiegeGuardPrice(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 3960, Kind: item.KindEtcItem, Slot: item.SlotNone},
		{ID: 4026, Kind: item.KindEtcItem, Slot: item.SlotNone},
		{ID: 4027, Kind: item.KindEtcItem, Slot: item.SlotNone},
	})
	list := buylist.List{ID: 1, Products: []buylist.Product{
		{ItemID: 3960, Price: 1000, MaxCount: -1},
		{ItemID: 4026, Price: 1000, MaxCount: -1},
		{ItemID: 4027, Price: 1000, MaxCount: -1},
	}}

	frame, err := FrameBuyList(list, 0, 0.10, 2.0, templates)
	if err != nil {
		t.Fatalf("FrameBuyList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeBuyList}
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint16(want, 3)
	// Siege-guard range: price * rate(2.0) * (1+tax) = 1000*2.0*1.10 = 2200.
	want = appendShopTradeItem(want, item.CategoryMoneyOrEtcItem, 3960, 3960, 0, item.SubCategoryOther, item.SlotNone, 0, 0, 0, 2200)
	want = appendShopTradeItem(want, item.CategoryMoneyOrEtcItem, 4026, 4026, 0, item.SubCategoryOther, item.SlotNone, 0, 0, 0, 2200)
	// Outside range: price * (1+tax) = 1000*1.10 = 1100, rate not applied.
	want = appendShopTradeItem(want, item.CategoryMoneyOrEtcItem, 4027, 4027, 0, item.SubCategoryOther, item.SlotNone, 0, 0, 0, 1100)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameBuyList() = %x, want %x", got, want)
	}
}

func TestFrameSellList(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 57, Kind: item.KindEtcItem, Slot: item.SlotNone, ReferencePrice: 1},
		{ID: 1146, Kind: item.KindArmor, Slot: item.SlotChest, ReferencePrice: 2500},
	})
	items := []*item.Instance{
		{ObjectID: 500, TemplateID: 57, Count: 1000, Location: item.LocationInventory},
		{ObjectID: 501, TemplateID: 1146, Count: 1, EnchantLevel: 3, CustomType1: 4, CustomType2: 5, Location: item.LocationPaperdoll},
	}

	frame, err := FrameSellList(3000, items, templates)
	if err != nil {
		t.Fatalf("FrameSellList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeSellList}
	want = binary.LittleEndian.AppendUint32(want, 3000)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint16(want, 2)
	want = appendShopTradeItem(want, item.CategoryMoneyOrEtcItem, 500, 57, 1000, item.SubCategoryMoney, item.SlotNone, 0, 0, 0, 0)
	want = appendShopTradeItem(want, item.CategoryArmor, 501, 1146, 1, item.SubCategoryArmor, item.SlotChest, 3, 4, 5, 1250)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSellList() = %x, want %x", got, want)
	}
}

func TestFrameTradePackets(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"request", framePayload(t, FrameSendTradeRequest(42)), []byte{OpcodeSendTradeRequest, 42, 0, 0, 0}},
		{"done success", framePayload(t, FrameSendTradeDone(true)), []byte{OpcodeSendTradeDone, 1, 0, 0, 0}},
		{"done failure", framePayload(t, FrameSendTradeDone(false)), []byte{OpcodeSendTradeDone, 0, 0, 0, 0}},
		{"press own", framePayload(t, FrameTradePressOwnOk()), []byte{OpcodeTradePressOwnOk}},
		{"press other", framePayload(t, FrameTradePressOtherOk()), []byte{OpcodeTradePressOtherOk}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("%s = %x, want %x", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestFrameTradeStartAndAdd(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 57, Kind: item.KindEtcItem, Slot: item.SlotNone, Tradable: true},
		{ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand, Tradable: true},
		{ID: 1146, Kind: item.KindArmor, Slot: item.SlotChest, Tradable: true},
	})
	items := []*item.Instance{
		{ObjectID: 500, TemplateID: 57, Count: 1000, Location: item.LocationInventory},
		{ObjectID: 501, TemplateID: 2368, Count: 1, EnchantLevel: 3, Location: item.LocationInventory},
		{ObjectID: 502, TemplateID: 2368, Count: 1, Location: item.LocationWarehouse},
		{ObjectID: 503, TemplateID: 1146, Count: 1, Location: item.LocationPaperdoll},
	}

	frame, err := FrameTradeStart(42, items, templates)
	if err != nil {
		t.Fatalf("FrameTradeStart: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeTradeStart}
	want = binary.LittleEndian.AppendUint32(want, 42)
	want = binary.LittleEndian.AppendUint16(want, 2)
	want = appendTradeItem(want, item.CategoryMoneyOrEtcItem, 500, 57, 1000, item.SubCategoryMoney, item.SlotNone, 0)
	want = appendTradeItem(want, item.CategoryWeaponOrJewelry, 501, 2368, 1, item.SubCategoryWeapon, item.SlotLRHand, 3)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameTradeStart() = %x, want %x", got, want)
	}

	add := TradeItemSnapshot{ObjectID: 501, TemplateID: 2368, Count: 1, EnchantLevel: 3}
	own, err := FrameTradeOwnAdd(add, 1, templates)
	if err != nil {
		t.Fatalf("FrameTradeOwnAdd: %v", err)
	}
	other, err := FrameTradeOtherAdd(add, 1, templates)
	if err != nil {
		t.Fatalf("FrameTradeOtherAdd: %v", err)
	}
	addPayload := append([]byte{OpcodeTradeOwnAdd}, binary.LittleEndian.AppendUint16(nil, 1)...)
	addPayload = appendTradeItem(addPayload, item.CategoryWeaponOrJewelry, 501, 2368, 1, item.SubCategoryWeapon, item.SlotLRHand, 3)
	if got := framePayload(t, own); !bytes.Equal(got, addPayload) {
		t.Fatalf("FrameTradeOwnAdd() = %x, want %x", got, addPayload)
	}
	addPayload[0] = OpcodeTradeOtherAdd
	if got := framePayload(t, other); !bytes.Equal(got, addPayload) {
		t.Fatalf("FrameTradeOtherAdd() = %x, want %x", got, addPayload)
	}
}

func TestFrameTradeUpdatePackets(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 57, Kind: item.KindEtcItem, Slot: item.SlotNone, Stackable: true},
		{ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand},
	})
	stack := TradeItemSnapshot{ObjectID: 500, TemplateID: 57, Count: 10}
	weapon := TradeItemSnapshot{ObjectID: 501, TemplateID: 2368, Count: 1, EnchantLevel: 3}

	frame, err := FrameTradeUpdate(stack, 90, templates)
	if err != nil {
		t.Fatalf("FrameTradeUpdate: %v", err)
	}
	got := framePayload(t, frame)
	want := []byte{OpcodeTradeUpdate}
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = binary.LittleEndian.AppendUint16(want, 3)
	want = appendTradeItem(want, item.CategoryMoneyOrEtcItem, 500, 57, 90, item.SubCategoryMoney, item.SlotNone, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameTradeUpdate(stack) = %x, want %x", got, want)
	}

	frame, err = FrameTradeItemUpdate([]TradeItemUpdateEntry{
		{Item: stack, AvailableCount: 90},
		{Item: weapon, AvailableCount: 0},
	}, templates)
	if err != nil {
		t.Fatalf("FrameTradeItemUpdate: %v", err)
	}
	got = framePayload(t, frame)
	want = []byte{OpcodeTradeItemUpdate}
	want = binary.LittleEndian.AppendUint16(want, 2)
	want = binary.LittleEndian.AppendUint16(want, 3)
	want = appendTradeItem(want, item.CategoryMoneyOrEtcItem, 500, 57, 90, item.SubCategoryMoney, item.SlotNone, 0)
	want = binary.LittleEndian.AppendUint16(want, 2)
	want = appendTradeItem(want, item.CategoryWeaponOrJewelry, 501, 2368, 1, item.SubCategoryWeapon, item.SlotLRHand, 3)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameTradeItemUpdate() = %x, want %x", got, want)
	}
}

func TestFrameShopTradeMissingTemplate(t *testing.T) {
	if _, err := FrameBuyList(buylist.List{ID: 1, Products: []buylist.Product{{ItemID: 9, MaxCount: -1}}}, 0, 0, 1.0, item.NewTable(nil)); err == nil {
		t.Fatal("FrameBuyList: want missing-template error")
	}
	if _, err := FrameSellList(0, []*item.Instance{{TemplateID: 9}}, item.NewTable(nil)); err == nil {
		t.Fatal("FrameSellList: want missing-template error")
	}
	if _, err := FrameTradeStart(1, []*item.Instance{{TemplateID: 9, Location: item.LocationInventory}}, item.NewTable(nil)); err == nil {
		t.Fatal("FrameTradeStart: want missing-template error")
	}
	if _, err := FrameTradeOwnAdd(TradeItemSnapshot{TemplateID: 9}, 1, item.NewTable(nil)); err == nil {
		t.Fatal("FrameTradeOwnAdd: want missing-template error")
	}
	if _, err := FrameTradeUpdate(TradeItemSnapshot{TemplateID: 9}, 1, item.NewTable(nil)); err == nil {
		t.Fatal("FrameTradeUpdate: want missing-template error")
	}
	if _, err := FrameTradeItemUpdate([]TradeItemUpdateEntry{{Item: TradeItemSnapshot{TemplateID: 9}}}, item.NewTable(nil)); err == nil {
		t.Fatal("FrameTradeItemUpdate: want missing-template error")
	}
}

func appendShopTradeItem(dst []byte, category item.Category, objectID, templateID, count int32, subCategory item.SubCategory, slot item.Slot, enchant, custom1, custom2 int, price int32) []byte {
	dst = binary.LittleEndian.AppendUint16(dst, uint16(category))
	dst = binary.LittleEndian.AppendUint32(dst, uint32(objectID))
	dst = binary.LittleEndian.AppendUint32(dst, uint32(templateID))
	dst = binary.LittleEndian.AppendUint32(dst, uint32(count))
	dst = binary.LittleEndian.AppendUint16(dst, uint16(subCategory))
	dst = binary.LittleEndian.AppendUint16(dst, uint16(custom1))
	dst = binary.LittleEndian.AppendUint32(dst, uint32(slot))
	dst = binary.LittleEndian.AppendUint16(dst, uint16(enchant))
	dst = binary.LittleEndian.AppendUint16(dst, uint16(custom2))
	dst = binary.LittleEndian.AppendUint16(dst, 0)
	dst = binary.LittleEndian.AppendUint32(dst, uint32(price))
	return dst
}

func appendTradeItem(dst []byte, category item.Category, objectID, templateID, count int32, subCategory item.SubCategory, slot item.Slot, enchant int) []byte {
	dst = binary.LittleEndian.AppendUint16(dst, uint16(category))
	dst = binary.LittleEndian.AppendUint32(dst, uint32(objectID))
	dst = binary.LittleEndian.AppendUint32(dst, uint32(templateID))
	dst = binary.LittleEndian.AppendUint32(dst, uint32(count))
	dst = binary.LittleEndian.AppendUint16(dst, uint16(subCategory))
	dst = binary.LittleEndian.AppendUint16(dst, 0)
	dst = binary.LittleEndian.AppendUint32(dst, uint32(slot))
	dst = binary.LittleEndian.AppendUint16(dst, uint16(enchant))
	dst = binary.LittleEndian.AppendUint16(dst, 0)
	dst = binary.LittleEndian.AppendUint16(dst, 0)
	return dst
}

// ---- from skill_enchant_test.go ----
func TestFrameExEnchantSkillList(t *testing.T) {
	got := framePayload(t, FrameExEnchantSkillList([]EnchantSkillEntry{
		{ID: 124, Level: 101, SPCost: 250000, XPCost: 123456789},
		{ID: 125, Level: 102, SPCost: 350000, XPCost: 987654321},
	}))

	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExEnchantSkillList)
	want = appendD(want, 2)
	want = appendD(want, 124)
	want = appendD(want, 101)
	want = appendD(want, 250000)
	want = appendQ(want, 123456789)
	want = appendD(want, 125)
	want = appendD(want, 102)
	want = appendD(want, 350000)
	want = appendQ(want, 987654321)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExEnchantSkillList() = %x, want %x", got, want)
	}
}

func TestFrameExEnchantSkillInfo(t *testing.T) {
	got := framePayload(t, FrameExEnchantSkillInfo(EnchantSkillInfo{
		ID:     124,
		Level:  101,
		SPCost: 250000,
		XPCost: 123456789,
		Rate:   82,
		Requirements: []EnchantSkillRequirement{
			{Type: 4, ItemID: 6622, Count: 1, Unknown: 0},
		},
	}))

	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExEnchantSkillInfo)
	want = appendD(want, 124)
	want = appendD(want, 101)
	want = appendD(want, 250000)
	want = appendQ(want, 123456789)
	want = appendD(want, 82)
	want = appendD(want, 1)
	want = appendD(want, 4)
	want = appendD(want, 6622)
	want = appendD(want, 1)
	want = appendD(want, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExEnchantSkillInfo() = %x, want %x", got, want)
	}
}

// ---- from skilllist_test.go ----
func TestFrameSkillList_Empty(t *testing.T) {
	got := framePayload(t, FrameSkillList(nil))
	want := []byte{OpcodeSkillList, 0, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("FrameSkillList(nil) = %x, want %x", got, want)
	}
}

func TestFrameSkillList_Entries(t *testing.T) {
	skills := []SkillListEntry{
		{ID: 1001, Level: 3, Passive: false, Disabled: false},
		{ID: 1002, Level: 1, Passive: true, Disabled: true},
	}
	got := framePayload(t, FrameSkillList(skills))

	want := []byte{OpcodeSkillList}
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = binary.LittleEndian.AppendUint32(want, 0) // not passive
	want = binary.LittleEndian.AppendUint32(want, 3) // level
	want = binary.LittleEndian.AppendUint32(want, 1001)
	want = append(want, 0)                           // not disabled
	want = binary.LittleEndian.AppendUint32(want, 1) // passive
	want = binary.LittleEndian.AppendUint32(want, 1) // level
	want = binary.LittleEndian.AppendUint32(want, 1002)
	want = append(want, 1) // disabled

	if !bytes.Equal(got, want) {
		t.Errorf("FrameSkillList() = %x, want %x", got, want)
	}
}

// ---- from ssqinfo_test.go ----
func TestFrameSSQInfo(t *testing.T) {
	got := framePayload(t, FrameSSQInfo())

	want := []byte{OpcodeSSQInfo}
	want = binary.LittleEndian.AppendUint16(want, regularSkyState)

	if !bytes.Equal(got, want) {
		t.Errorf("FrameSSQInfo() = % x, want % x", got, want)
	}
}

// ---- from stance_social_test.go ----
func TestFrameAutoAttackStart(t *testing.T) {
	got := framePayload(t, FrameAutoAttackStart(12345))
	want := []byte{OpcodeAutoAttackStart, 0x39, 0x30, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameAutoAttackStart() = %x, want %x", got, want)
	}
}

func TestFrameSocialAction(t *testing.T) {
	got := framePayload(t, FrameSocialAction(12345, 13))
	want := []byte{
		OpcodeSocialAction,
		0x39, 0x30, 0x00, 0x00,
		0x0d, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSocialAction() = %x, want %x", got, want)
	}
}

func TestFrameChangeMoveType(t *testing.T) {
	got := framePayload(t, FrameChangeMoveType(12345, false, false))
	want := []byte{
		OpcodeChangeMoveType,
		0x39, 0x30, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameChangeMoveType() = %x, want %x", got, want)
	}
}

func TestFrameChangeWaitType(t *testing.T) {
	got := framePayload(t, FrameChangeWaitType(12345, WaitSitting, location.Location{X: 46160, Y: 41237, Z: -3534}))
	want := []byte{
		OpcodeChangeWaitType,
		0x39, 0x30, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameChangeWaitType() = %x, want %x", got, want)
	}
}

// ---- from status_effects_test.go ----
func TestFrameAbnormalStatusUpdate(t *testing.T) {
	got := framePayload(t, FrameAbnormalStatusUpdate([]AbnormalStatusEffect{
		{SkillID: 1040, Level: 3, DurationMillis: 15_000},
		{SkillID: 1068, Level: 2, DurationMillis: -1},
		{SkillID: 1002, Level: 1, DurationMillis: 30_000, Toggle: true},
		{SkillID: 1001, Level: 4, DurationMillis: 30_000, Toggle: true},
	}))

	want := []byte{OpcodeAbnormalStatusUpdate}
	want = binary.LittleEndian.AppendUint16(want, 4)
	want = appendEffect(want, 1040, 3, 15)
	want = appendEffect(want, 1068, 2, -1)
	want = appendEffect(want, 1001, 4, -1)
	want = appendEffect(want, 1002, 1, -1)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameAbnormalStatusUpdate() = %x, want %x", got, want)
	}
}

func TestFrameShortBuffStatusUpdate(t *testing.T) {
	got := framePayload(t, FrameShortBuffStatusUpdate(1323, 1, 120))

	want := []byte{OpcodeShortBuffStatusUpdate}
	for _, v := range []uint32{1323, 1, 120} {
		want = binary.LittleEndian.AppendUint32(want, v)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameShortBuffStatusUpdate() = %x, want %x", got, want)
	}
}

func appendEffect(out []byte, skillID uint32, level uint16, duration int32) []byte {
	out = binary.LittleEndian.AppendUint32(out, skillID)
	out = binary.LittleEndian.AppendUint16(out, level)
	return binary.LittleEndian.AppendUint32(out, uint32(duration))
}

// ---- from systemmessage_test.go ----
func TestFrameSystemMessage(t *testing.T) {
	got := framePayload(t, FrameSystemMessage(SystemMessagePetRefusingOrder))
	want := []byte{
		OpcodeSystemMessage,
		0x48, 0x07, 0x00, 0x00, // 1864
		0x00, 0x00, 0x00, 0x00, // no params
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSystemMessage() = %x, want %x", got, want)
	}
}

func TestFrameSystemMessageCorpseTargetFailures(t *testing.T) {
	for _, id := range []int{
		SystemMessageSweeperFailedTargetNotSpoiled,
		SystemMessageHarvestFailedSeedNotSown,
		SystemMessageCorpseTooOldSkillNotUsed,
	} {
		got := framePayload(t, FrameSystemMessage(id))
		want := []byte{OpcodeSystemMessage, byte(id), byte(id >> 8), 0, 0, 0, 0, 0, 0}
		if !bytes.Equal(got, want) {
			t.Fatalf("FrameSystemMessage(%d) = %x, want %x", id, got, want)
		}
	}
}

func TestFrameSystemMessageCounterattackFeedback(t *testing.T) {
	tests := []struct {
		name string
		id   int
	}{
		{name: "performing", id: SystemMessageS1PerformingCounterattack},
		{name: "countered", id: SystemMessageCounteredS1Attack},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := framePayload(t, FrameSystemMessageString(tt.id, "Target"))
			want := []byte{
				OpcodeSystemMessage,
				byte(tt.id), byte(tt.id >> 8), 0x00, 0x00,
				0x01, 0x00, 0x00, 0x00,
				SystemMessageParamText, 0x00, 0x00, 0x00,
				'T', 0x00, 'a', 0x00, 'r', 0x00, 'g', 0x00, 'e', 0x00, 't', 0x00, 0x00, 0x00,
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("counterattack frame = %x, want %x", got, want)
			}
		})
	}
}

func TestFrameSystemMessageForceChargeFeedback(t *testing.T) {
	t.Run("increased", func(t *testing.T) {
		got := framePayload(t, FrameSystemMessageNumber(SystemMessageForceIncreasedToS1, 3))
		want := []byte{
			OpcodeSystemMessage,
			0x43, 0x01, 0x00, 0x00, // 323
			0x01, 0x00, 0x00, 0x00, // one parameter
			0x01, 0x00, 0x00, 0x00, // number parameter
			0x03, 0x00, 0x00, 0x00, // three charges
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("force-increased frame = %x, want %x", got, want)
		}
	})

	t.Run("maximum", func(t *testing.T) {
		got := framePayload(t, FrameSystemMessage(SystemMessageForceMaxLevelReached))
		want := []byte{
			OpcodeSystemMessage,
			0x44, 0x01, 0x00, 0x00, // 324
			0x00, 0x00, 0x00, 0x00, // no parameters
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("force-maximum frame = %x, want %x", got, want)
		}
	})
}

func TestFrameSystemMessageTwoNumbers(t *testing.T) {
	got := framePayload(t, FrameSystemMessageTwoNumbers(SystemMessageYouEarnedS1ExpAndS2SP, 1000, 25))
	want := []byte{
		OpcodeSystemMessage,
		0x5f, 0x00, 0x00, 0x00, // 95
		0x02, 0x00, 0x00, 0x00, // two params
		0x01, 0x00, 0x00, 0x00, // number param
		0xe8, 0x03, 0x00, 0x00, // 1000 exp
		0x01, 0x00, 0x00, 0x00, // number param
		0x19, 0x00, 0x00, 0x00, // 25 sp
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSystemMessageTwoNumbers() = %x, want %x", got, want)
	}
}

func TestFrameSystemMessageSkillName(t *testing.T) {
	got := framePayload(t, FrameSystemMessageSkillName(SystemMessageNightSkillEffectApplies, 294, 1))
	want := []byte{
		OpcodeSystemMessage,
		0x6b, 0x04, 0x00, 0x00, // 1131
		0x01, 0x00, 0x00, 0x00, // one param
		0x04, 0x00, 0x00, 0x00, // skill-name param
		0x26, 0x01, 0x00, 0x00, // skill 294
		0x01, 0x00, 0x00, 0x00, // level 1
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSystemMessageSkillName() = %x, want %x", got, want)
	}
}

func TestFrameSystemMessageStringSkillName(t *testing.T) {
	got := framePayload(t, FrameSystemMessageStringSkillName(SystemMessageS1ResistedYourS2, "Target", 123, 1))
	want := []byte{
		OpcodeSystemMessage,
		0x8b, 0x00, 0x00, 0x00, // 139
		0x02, 0x00, 0x00, 0x00, // two params
		0x00, 0x00, 0x00, 0x00, // text parameter
		'T', 0x00, 'a', 0x00, 'r', 0x00, 'g', 0x00, 'e', 0x00, 't', 0x00, 0x00, 0x00,
		0x04, 0x00, 0x00, 0x00, // skill-name parameter
		0x7b, 0x00, 0x00, 0x00, // skill 123
		0x01, 0x00, 0x00, 0x00, // level 1
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSystemMessageStringSkillName() = %x, want %x", got, want)
	}
}

func TestFrameSystemMessageStringNumber(t *testing.T) {
	got := framePayload(t, FrameSystemMessageStringNumber(1016, "Attacker", 12))
	want := []byte{
		OpcodeSystemMessage,
		0xf8, 0x03, 0x00, 0x00, // 1016
		0x02, 0x00, 0x00, 0x00, // two params
		0x00, 0x00, 0x00, 0x00, // text parameter
		'A', 0x00, 't', 0x00, 't', 0x00, 'a', 0x00, 'c', 0x00, 'k', 0x00, 'e', 0x00, 'r', 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00, // number parameter
		0x0c, 0x00, 0x00, 0x00, // 12
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameSystemMessageStringNumber() = %x, want %x", got, want)
	}
}

// ---- from targeting_test.go ----
func TestFrameMyTargetSelected(t *testing.T) {
	got := framePayload(t, FrameMyTargetSelected(12345, 0x0010))
	want := []byte{
		OpcodeMyTargetSelected,
		0x39, 0x30, 0x00, 0x00,
		0x10, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameMyTargetSelected() = %x, want %x", got, want)
	}
}

func TestFrameTargetSelected(t *testing.T) {
	got := framePayload(t, FrameTargetSelected(100, 200, location.Location{X: 46160, Y: 41237, Z: -3534}))
	want := []byte{
		OpcodeTargetSelected,
		0x64, 0x00, 0x00, 0x00,
		0xc8, 0x00, 0x00, 0x00,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameTargetSelected() = %x, want %x", got, want)
	}
}

func TestFrameTargetUnselected(t *testing.T) {
	got := framePayload(t, FrameTargetUnselected(100, location.Location{X: 46160, Y: 41237, Z: -3534}))
	want := []byte{
		OpcodeTargetUnselected,
		0x64, 0x00, 0x00, 0x00,
		0x50, 0xb4, 0x00, 0x00,
		0x15, 0xa1, 0x00, 0x00,
		0x32, 0xf2, 0xff, 0xff,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameTargetUnselected() = %x, want %x", got, want)
	}
}

func TestFrameStatusUpdate(t *testing.T) {
	got := framePayload(t, FrameStatusUpdate(12345, []StatusAttribute{
		{Type: StatusCurrentHP, Value: 75},
		{Type: StatusMaxHP, Value: 100},
	}))
	want := []byte{
		OpcodeStatusUpdate,
		0x39, 0x30, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x00,
		0x09, 0x00, 0x00, 0x00,
		0x4b, 0x00, 0x00, 0x00,
		0x0a, 0x00, 0x00, 0x00,
		0x64, 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameStatusUpdate() = %x, want %x", got, want)
	}
}

// ---- from teleporttolocation_test.go ----
func TestFrameTeleportToLocation(t *testing.T) {
	to := location.Location{X: -71440, Y: 258000, Z: -3104}

	tests := []struct {
		name         string
		fastTeleport bool
		want         []byte
	}{
		{
			name:         "black screen",
			fastTeleport: false,
			want: []byte{
				0x28,
				0x64, 0xa0, 0x00, 0x10,
				0xf0, 0xe8, 0xfe, 0xff,
				0xd0, 0xef, 0x03, 0x00,
				0xe0, 0xf3, 0xff, 0xff,
				0x00, 0x00, 0x00, 0x00,
			},
		},
		{
			name:         "fast teleport",
			fastTeleport: true,
			want: []byte{
				0x28,
				0x64, 0xa0, 0x00, 0x10,
				0xf0, 0xe8, 0xfe, 0xff,
				0xd0, 0xef, 0x03, 0x00,
				0xe0, 0xf3, 0xff, 0xff,
				0x01, 0x00, 0x00, 0x00,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := framePayload(t, FrameTeleportToLocation(268476516, to, tc.fastTeleport))
			if !bytes.Equal(got, tc.want) {
				t.Errorf("FrameTeleportToLocation(_, _, %v) = %x, want %x", tc.fastTeleport, got, tc.want)
			}
		})
	}
}

// ---- from userinfo_test.go ----
func TestFrameUserInfo(t *testing.T) {
	c := &player.Character{
		ID:        0x10000001,
		Name:      "Newbie",
		ClassID:   0,
		Race:      player.RaceHuman,
		Sex:       player.SexMale,
		CharLevel: 1,
		Exp:       0,
		SP:        0,
		Face:      0, HairStyle: 1, HairColor: 2,
		Location:    location.Location{X: 10, Y: 20, Z: 30},
		LastHeading: 100,
		KarmaPoints: 0, PKKills: 1, PvPKills: 2,
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
	want = binary.LittleEndian.AppendUint32(want, uint32(c.ClassID))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.CharLevel))
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

	want = appendF64(want, 1) // movement speed multiplier
	want = appendF64(want, 1) // attack speed multiplier
	want = appendF64(want, tmpl.CollisionRadius)
	want = appendF64(want, tmpl.CollisionHeight)

	want = binary.LittleEndian.AppendUint32(want, uint32(c.HairStyle))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.HairColor))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Face))
	want = binary.LittleEndian.AppendUint32(want, 1) // IsGM flag

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
	wantHuman := binary.LittleEndian.AppendUint16(nil, nonDwarfInventoryLimit)
	wantDwarf := binary.LittleEndian.AppendUint16(nil, dwarfInventoryLimit)
	if !bytes.Contains(human, wantHuman) {
		t.Errorf("human encoding did not contain the non-dwarf inventory limit %d", nonDwarfInventoryLimit)
	}
	if !bytes.Contains(dwarf, wantDwarf) {
		t.Errorf("dwarf encoding did not contain the dwarf inventory limit %d", dwarfInventoryLimit)
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

// ---- from variation_test.go ----
func appendQ(b []byte, v int64) []byte {
	return binary.LittleEndian.AppendUint64(b, uint64(v))
}

func TestFrameExShowVariationWindows(t *testing.T) {
	got := framePayload(t, FrameExShowVariationMakeWindow())
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExShowVariationMakeWindow)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExShowVariationMakeWindow() = %x, want %x", got, want)
	}

	got = framePayload(t, FrameExShowVariationCancelWindow())
	want = []byte{OpcodeExtended}
	want = appendH(want, OpcodeExShowVariationCancelWindow)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExShowVariationCancelWindow() = %x, want %x", got, want)
	}
}

func TestFrameExConfirmVariationItem(t *testing.T) {
	got := framePayload(t, FrameExConfirmVariationItem(1000))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExConfirmVariationItem)
	want = appendD(want, 1000)
	want = appendD(want, 1)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExConfirmVariationItem() = %x, want %x", got, want)
	}
}

func TestFrameExConfirmVariationRefiner(t *testing.T) {
	got := framePayload(t, FrameExConfirmVariationRefiner(2000, 8723, 2130, 20))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExConfirmVariationRefiner)
	want = appendD(want, 2000)
	want = appendD(want, 8723)
	want = appendD(want, 2130)
	want = appendD(want, 20)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExConfirmVariationRefiner() = %x, want %x", got, want)
	}
}

func TestFrameExConfirmVariationGemstone(t *testing.T) {
	got := framePayload(t, FrameExConfirmVariationGemstone(3000, 36))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExConfirmVariationGemstone)
	want = appendD(want, 3000)
	want = appendD(want, 1)
	want = appendD(want, 36)
	want = appendD(want, 1)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExConfirmVariationGemstone() = %x, want %x", got, want)
	}
}

func TestFrameExConfirmCancelItem(t *testing.T) {
	got := framePayload(t, FrameExConfirmCancelItem(1000, 7575, 0x12345678, 390000))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExConfirmCancelItem)
	want = appendD(want, 1000)
	want = appendD(want, 7575)
	want = appendD(want, 0x5678)
	want = appendD(want, 0x1234)
	want = appendQ(want, 390000)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExConfirmCancelItem() = %x, want %x", got, want)
	}
}

func TestFrameExVariationResult(t *testing.T) {
	got := framePayload(t, FrameExVariationResult(0x1111, 0x2222, 1))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExVariationResult)
	want = appendD(want, 0x1111)
	want = appendD(want, 0x2222)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExVariationResult() = %x, want %x", got, want)
	}
}

func TestFrameExVariationResultFailed(t *testing.T) {
	got := framePayload(t, FrameExVariationResultFailed())
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExVariationResult)
	want = appendD(want, 0)
	want = appendD(want, 0)
	want = appendD(want, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExVariationResultFailed() = %x, want %x", got, want)
	}
}

func TestFrameExVariationCancelResult(t *testing.T) {
	got := framePayload(t, FrameExVariationCancelResult(1))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExVariationCancelResult)
	want = appendD(want, 1)
	want = appendD(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExVariationCancelResult() = %x, want %x", got, want)
	}
}

// ---- from versioncheck_test.go ----
func TestFrameVersionCheck(t *testing.T) {
	key := bytes.Repeat([]byte{0xcc}, 16)

	for _, cipherEnabled := range []bool{false, true} {
		frame := FrameVersionCheck(key, cipherEnabled)
		var want []byte
		want = binary.LittleEndian.AppendUint16(want, uint16(2+1+1+versionCheckKeySize+4+4))
		want = append(want, OpcodeVersionCheck)
		want = append(want, 0x01)
		want = append(want, key[:versionCheckKeySize]...)
		if cipherEnabled {
			want = binary.LittleEndian.AppendUint32(want, 1)
		} else {
			want = binary.LittleEndian.AppendUint32(want, 0)
		}
		want = binary.LittleEndian.AppendUint32(want, 1)

		if !bytes.Equal(frame.Bytes(), want) {
			t.Errorf("FrameVersionCheck(%t) = %x, want %x", cipherEnabled, frame.Bytes(), want)
		}
		frame.Release()
	}
}

// ---- from warehouse_test.go ----
func TestFramePackageToList(t *testing.T) {
	got := framePayload(t, FramePackageToList([]PackageRecipient{
		{ObjectID: 100, Name: "Alpha"},
		{ObjectID: 200, Name: "Beta"},
	}))

	want := []byte{OpcodePackageToList}
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = binary.LittleEndian.AppendUint32(want, 100)
	want = appendUTF16Z(want, "Alpha")
	want = binary.LittleEndian.AppendUint32(want, 200)
	want = appendUTF16Z(want, "Beta")

	if !bytes.Equal(got, want) {
		t.Fatalf("FramePackageToList = %x, want %x", got, want)
	}
}

func TestFramePackageSendableList(t *testing.T) {
	templates := warehousePacketTemplates()
	items := []*item.Instance{
		{ObjectID: 500, TemplateID: 30, Count: 1, Location: item.LocationInventory, EnchantLevel: 2, CustomType1: 7, CustomType2: 8},
		{ObjectID: 501, TemplateID: item.AdenaID, Count: 100, Location: item.LocationInventory},
	}

	frame, err := FramePackageSendableList(200, 777, items, templates)
	if err != nil {
		t.Fatalf("FramePackageSendableList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodePackageSendableList}
	want = binary.LittleEndian.AppendUint32(want, 200)
	want = binary.LittleEndian.AppendUint32(want, 777)
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = appendWarehouseVisibleItem(want, items[0], templates, false)
	want = appendWarehouseVisibleItem(want, items[1], templates, false)

	if !bytes.Equal(got, want) {
		t.Fatalf("FramePackageSendableList = %x, want %x", got, want)
	}
}

func TestFrameWarehouseDepositList(t *testing.T) {
	templates := warehousePacketTemplates()
	items := []*item.Instance{
		{
			ObjectID: 500, TemplateID: 30, Count: 1, Location: item.LocationInventory,
			EnchantLevel: 2, CustomType1: 7, CustomType2: 8,
			Augmentation: &item.Augmentation{Attributes: 0x12345678},
		},
	}

	frame, err := FrameWarehouseDepositList(WarehousePrivate, 777, items, templates)
	if err != nil {
		t.Fatalf("FrameWarehouseDepositList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeWarehouseDepositList}
	want = binary.LittleEndian.AppendUint16(want, uint16(WarehousePrivate))
	want = binary.LittleEndian.AppendUint32(want, 777)
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = appendWarehouseVisibleItem(want, items[0], templates, true)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameWarehouseDepositList = %x, want %x", got, want)
	}
}

func TestFrameWarehouseWithdrawList(t *testing.T) {
	templates := warehousePacketTemplates()
	items := []*item.Instance{{ObjectID: 501, TemplateID: item.AdenaID, Count: 100, Location: item.LocationWarehouse}}

	frame, err := FrameWarehouseWithdrawList(WarehouseFreight, 777, items, templates)
	if err != nil {
		t.Fatalf("FrameWarehouseWithdrawList: %v", err)
	}
	got := framePayload(t, frame)

	want := []byte{OpcodeWarehouseWithdrawList}
	want = binary.LittleEndian.AppendUint16(want, uint16(WarehouseFreight))
	want = binary.LittleEndian.AppendUint32(want, 777)
	want = binary.LittleEndian.AppendUint16(want, 1)
	want = appendWarehouseVisibleItem(want, items[0], templates, true)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameWarehouseWithdrawList = %x, want %x", got, want)
	}
}

func TestFramePackageSendableListMissingTemplate(t *testing.T) {
	_, err := FramePackageSendableList(200, 777, []*item.Instance{{ObjectID: 500, TemplateID: 999, Count: 1}}, item.NewTable(nil))
	if err == nil {
		t.Fatal("FramePackageSendableList: want error for missing template")
	}
}

func warehousePacketTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: item.AdenaID, Kind: item.KindEtcItem, Slot: item.SlotNone, Duration: -1, Stackable: true, EtcItem: &item.EtcItemDetail{}},
		{ID: 30, Kind: item.KindWeapon, Slot: item.SlotRHand, Duration: -1, Weapon: &item.WeaponDetail{}},
	})
}

func appendWarehouseVisibleItem(out []byte, inst *item.Instance, templates *item.Table, includeAugmentation bool) []byte {
	tmpl, _ := templates.Get(inst.TemplateID)
	category, subCategory := tmpl.Category()

	out = binary.LittleEndian.AppendUint16(out, uint16(category))
	out = binary.LittleEndian.AppendUint32(out, uint32(inst.ObjectID))
	out = binary.LittleEndian.AppendUint32(out, uint32(inst.TemplateID))
	out = binary.LittleEndian.AppendUint32(out, uint32(inst.Count))
	out = binary.LittleEndian.AppendUint16(out, uint16(subCategory))
	out = binary.LittleEndian.AppendUint16(out, uint16(inst.CustomType1))
	out = binary.LittleEndian.AppendUint32(out, uint32(tmpl.Slot))
	out = binary.LittleEndian.AppendUint16(out, uint16(inst.EnchantLevel))
	out = binary.LittleEndian.AppendUint16(out, uint16(inst.CustomType2))
	out = binary.LittleEndian.AppendUint16(out, 0)
	out = binary.LittleEndian.AppendUint32(out, uint32(inst.ObjectID))
	if includeAugmentation {
		if inst.Augmentation != nil {
			out = binary.LittleEndian.AppendUint32(out, uint32(inst.Augmentation.Attributes&0x0000ffff))
			return binary.LittleEndian.AppendUint32(out, uint32(inst.Augmentation.Attributes>>16))
		}
		return binary.LittleEndian.AppendUint64(out, 0)
	}
	return out
}

func appendUTF16Z(out []byte, s string) []byte {
	for _, unit := range utf16.Encode([]rune(s)) {
		out = binary.LittleEndian.AppendUint16(out, unit)
	}
	return binary.LittleEndian.AppendUint16(out, 0)
}

// ---- from worldobjects_test.go ----
type doorShape struct{}

func (doorShape) GeoX() int               { return 0 }
func (doorShape) GeoY() int               { return 0 }
func (doorShape) GeoZ() int               { return 0 }
func (doorShape) Height() int             { return 32 }
func (doorShape) GeoData() [][]block.NSWE { return [][]block.NSWE{{block.NoDirections}} }

func TestFrameDoorInfo(t *testing.T) {
	gate := testDoor(t)

	got := framePayload(t, FrameDoorInfo(gate, true))

	want := []byte{OpcodeDoorInfo}
	want = binary.LittleEndian.AppendUint32(want, uint32(1000))
	want = binary.LittleEndian.AppendUint32(want, uint32(19210001))
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 253200)
	want = binary.LittleEndian.AppendUint32(want, 253200)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	if string(got) != string(want) {
		t.Fatalf("FrameDoorInfo() = % x, want % x", got, want)
	}
}

func TestFrameDoorStatusUpdate(t *testing.T) {
	gate := testDoor(t)

	got := framePayload(t, FrameDoorStatusUpdate(gate, false))

	want := []byte{OpcodeDoorStatusUpdate}
	want = binary.LittleEndian.AppendUint32(want, uint32(1000))
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, uint32(19210001))
	want = binary.LittleEndian.AppendUint32(want, 253200)
	want = binary.LittleEndian.AppendUint32(want, 253200)
	if string(got) != string(want) {
		t.Fatalf("FrameDoorStatusUpdate() = % x, want % x", got, want)
	}
}

func TestFrameStaticObjectInfo(t *testing.T) {
	sign, err := staticobject.NewObject(1001, &staticobject.Template{ID: 41001})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	got := framePayload(t, FrameStaticObjectInfo(sign))

	want := []byte{OpcodeStaticObjectInfo}
	want = binary.LittleEndian.AppendUint32(want, 41001)
	want = binary.LittleEndian.AppendUint32(want, 1001)
	if string(got) != string(want) {
		t.Fatalf("FrameStaticObjectInfo() = % x, want % x", got, want)
	}
}

func TestFrameChairSit(t *testing.T) {
	got := framePayload(t, FrameChairSit(0x1000a064, 1234))

	want := []byte{OpcodeChairSit}
	want = binary.LittleEndian.AppendUint32(want, 0x1000a064)
	want = binary.LittleEndian.AppendUint32(want, 1234)
	if string(got) != string(want) {
		t.Fatalf("FrameChairSit() = % x, want % x", got, want)
	}
}

func testDoor(t *testing.T) *door.Object {
	t.Helper()
	gate, err := door.NewObject(1000, &door.Template{
		ID:       19210001,
		Name:     "gludio_castle_outter_001",
		Kind:     door.KindDoor,
		Level:    1,
		Position: location.Location{X: -18408, Y: 113064, Z: -2768},
		Coordinates: []location.Point{
			{X: -18481, Y: 113059},
			{X: -18351, Y: 113059},
			{X: -18351, Y: 113071},
			{X: -18481, Y: 113071},
		},
		HP: 253200, PDef: 644, MDef: 518, Height: 320,
	}, doorShape{})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}
	return gate
}

// ---- from writer_test.go ----
func TestFrameWriterPoolability(t *testing.T) {
	normal := wire.NewFrameWriter(packetWriterCapacity)
	if !poolable(normal) {
		t.Fatal("normal writer should be retained")
	}

	oversized := wire.NewFrameWriter(packetWriterCapacity)
	oversized.WriteBytes(make([]byte, maxPacketWriterCapacity))
	if got := oversized.Cap(); got <= maxPacketWriterCapacity {
		t.Fatalf("oversized writer capacity = %d, want > %d", got, maxPacketWriterCapacity)
	}
	if poolable(oversized) {
		t.Fatal("oversized writer should be dropped")
	}
}

func TestCopyFrameShortSourceReportsFailure(t *testing.T) {
	frame, ok := CopyFrame(wire.BorrowedFrame([]byte{1}))
	defer frame.Release()
	if ok {
		t.Fatalf("CopyFrame short source = %x, true; want empty frame, false", frame.Bytes())
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
