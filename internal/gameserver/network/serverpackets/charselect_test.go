package serverpackets

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// TestFrameAuthLoginFail checks the framed and unframed builders agree; the
// bytes are pinned by the AuthLoginFail row of testdata/packets.golden.
func TestFrameAuthLoginFail(t *testing.T) {
	frame := FrameAuthLoginFail(LoginFailSystemErrorTryLater)
	defer frame.Release()

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
