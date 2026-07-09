package serverpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func frameTestCharacter() *player.Character {
	return &player.Character{
		ObjectID: 0x10000001,
		Name:     "Newbie",
		ClassID:  0,
		Race:     player.RaceHuman,
		Sex:      player.SexMale,
		Level:    1,
		MaxHP:    80, CurHP: 75,
		MaxMP: 30, CurMP: 30,
		MaxCP: 40, CurCP: 40,
		Face: 0, HairStyle: 1, HairColor: 2,
		Position: location.Location{X: 10, Y: 20, Z: 30},
		Heading:  100,
		PKKills:  1, PvPKills: 2,
		ClanID: 5, Title: "Hero", AccessLevel: 1,
	}
}

func frameTestTemplate() *player.Template {
	return &player.Template{
		STR: 40, CON: 43, DEX: 30, INT: 21, WIT: 11, MEN: 25,
		PAtk: 4, PDef: 30, MAtk: 3, MDef: 15,
		RunSpeed: 120, WalkSpeed: 80, SwimSpeed: 50,
		CollisionRadius: 9, CollisionHeight: 23,
	}
}

func frameTestItems() []*item.Instance {
	return []*item.Instance{
		{ObjectID: 100, TemplateID: 2368, Count: 1, Location: item.LocationPaperdoll, LocationData: rhandPaperdollIndex, EnchantLevel: 5},
		{ObjectID: 102, TemplateID: item.AdenaID, Count: 500, Location: item.LocationInventory},
	}
}

func frameTestItemTable() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: 2368, Kind: item.KindWeapon, Slot: item.SlotLRHand},
		{ID: item.AdenaID, Kind: item.KindEtcItem, Slot: item.SlotNone},
	})
}

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

// BenchmarkUserInfoPooled measures the pooled frame path end to end,
// including returning the writer to the pool the way a connection writer
// does after the frame is written.
func BenchmarkUserInfoPooled(b *testing.B) {
	snap := UserInfoSnapshot{Character: frameTestCharacter(), Template: frameTestTemplate(), Items: frameTestItems()}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		frame := FrameUserInfo(snap)
		frame.Release()
	}
}
