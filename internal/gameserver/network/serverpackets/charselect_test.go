package serverpackets

import (
	"bytes"
	"encoding/binary"
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
