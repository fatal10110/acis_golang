package serverpackets

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

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

// ---- from charselected_test.go ----
func TestFrameCharSelected(t *testing.T) {
	c := &player.Character{
		ID:       0x10000001,
		Name:     "Newbie",
		Sex:      player.SexMale,
		Race:     player.RaceHuman,
		Location: location.Location{X: 10, Y: 20, Z: 30},
		SP:       7, Exp: 12345, CharLevel: 3,
		KarmaPoints: 1, PKKills: 2,
	}
	c.SetTitle("Hero")
	c.SetClanID(5)
	c.SetResourceValues(player.Resources{CurrentHP: 75, CurrentMP: 30})
	tmpl := &player.Template{STR: 40, CON: 43, DEX: 30, INT: 21, WIT: 11, MEN: 25}

	got := framePayload(t, FrameCharSelected(CharSelectedSnapshot{Character: c, Template: tmpl, SessionID: 999, GameTime: 1234}))
	resources := c.ResourceValues()

	want := []byte{OpcodeCharSelected}
	x, y, z := c.Position()
	want = append(want, encodeUTF16Z(c.Name)...)
	want = binary.LittleEndian.AppendUint32(want, uint32(c.ObjectID()))
	want = append(want, encodeUTF16Z("Hero")...)
	want = binary.LittleEndian.AppendUint32(want, 999) // session id
	want = binary.LittleEndian.AppendUint32(want, uint32(5))
	want = binary.LittleEndian.AppendUint32(want, 0) // unknown

	want = binary.LittleEndian.AppendUint32(want, uint32(c.Sex))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.Race))
	want = binary.LittleEndian.AppendUint32(want, uint32(c.ClassID()))

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
	want = binary.LittleEndian.AppendUint32(want, uint32(c.ClassID()))
	for i := 0; i < 4; i++ {
		want = binary.LittleEndian.AppendUint32(want, 0)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("FrameCharSelected mismatch:\n got  % x\n want % x", got, want)
	}
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
	slot.Paperdoll[rhandPaperdollIndex] = item.PaperdollEntry{ObjectID: 100, TemplateID: 2369, EnchantLevel: 5, AugmentationID: 8191<<16 | 3}
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

	want = append(want, 5)                                    // enchant effect from the RHAND weapon
	want = binary.LittleEndian.AppendUint32(want, 8191<<16|3) // the RHAND weapon's augmentation id

	if !bytes.Equal(got, want) {
		t.Errorf("FrameCharSelectInfo mismatch:\n got  %x\n want %x", got, want)
	}
}

// TestFrameCharSelectInfoUnsetAugmentationReadsZero pins that an
// augmentations row left at its column default (-1) shows as no
// augmentation on the character list.
func TestFrameCharSelectInfoUnsetAugmentationReadsZero(t *testing.T) {
	slot := CharacterSlot{Name: "Newbie", ObjectID: 1}
	slot.Paperdoll[rhandPaperdollIndex] = item.PaperdollEntry{ObjectID: 100, TemplateID: 2369, AugmentationID: -1}
	got := framePayload(t, FrameCharSelectInfo("acct1", 1, []CharacterSlot{slot}, 0))
	if tail := binary.LittleEndian.Uint32(got[len(got)-4:]); tail != 0 {
		t.Fatalf("augmentation id = %d, want 0", int32(tail))
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
