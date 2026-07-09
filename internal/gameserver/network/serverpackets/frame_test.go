package serverpackets

import (
	"bytes"
	"testing"
	"time"

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

// TestFramePacketsMatchEncoders pins every pooled-frame builder to its
// encoder: same input, byte-identical wire frame.
func TestFramePacketsMatchEncoders(t *testing.T) {
	c, tmpl := frameTestCharacter(), frameTestTemplate()
	items, itemTable := frameTestItems(), frameTestItemTable()
	userSnap := UserInfoSnapshot{Character: c, Template: tmpl, Items: items}
	selectedSnap := CharSelectedSnapshot{Character: c, Template: tmpl, SessionID: 0x1122}
	slots := []CharacterSlot{NewCharacterSlot(c, items, time.Unix(0, 0))}
	skills := []SkillListEntry{{ID: 3, Level: 1, Passive: true}, {ID: 4, Level: 2, Disabled: true}}

	cases := []struct {
		name    string
		frame   func(t *testing.T) wire.Frame
		payload func(t *testing.T) []byte
	}{
		{
			"AuthLoginFail",
			func(t *testing.T) wire.Frame { return FrameAuthLoginFail(LoginFailSystemErrorTryLater) },
			func(t *testing.T) []byte { return EncodeAuthLoginFail(LoginFailSystemErrorTryLater) },
		},
		{
			"UserInfo",
			func(t *testing.T) wire.Frame { return FrameUserInfo(userSnap) },
			func(t *testing.T) []byte { return EncodeUserInfo(userSnap) },
		},
		{
			"CharSelected",
			func(t *testing.T) wire.Frame { return FrameCharSelected(selectedSnap) },
			func(t *testing.T) []byte { return EncodeCharSelected(selectedSnap) },
		},
		{
			"CharSelectInfo",
			func(t *testing.T) wire.Frame { return FrameCharSelectInfo("login", 7, slots, -1) },
			func(t *testing.T) []byte { return EncodeCharSelectInfo("login", 7, slots, -1) },
		},
		{
			"SkillList",
			func(t *testing.T) wire.Frame { return FrameSkillList(skills) },
			func(t *testing.T) []byte { return EncodeSkillList(skills) },
		},
		{
			"SSQInfo",
			func(t *testing.T) wire.Frame { return FrameSSQInfo() },
			func(t *testing.T) []byte { return EncodeSSQInfo() },
		},
		{
			"CharCreateOk",
			func(t *testing.T) wire.Frame { return FrameCharCreateOk() },
			func(t *testing.T) []byte { return EncodeCharCreateOk() },
		},
		{
			"CharCreateFail",
			func(t *testing.T) wire.Frame { return FrameCharCreateFail(CharCreateFailReasonNameAlreadyExists) },
			func(t *testing.T) []byte { return EncodeCharCreateFail(CharCreateFailReasonNameAlreadyExists) },
		},
		{
			"CharDeleteOk",
			func(t *testing.T) wire.Frame { return FrameCharDeleteOk() },
			func(t *testing.T) []byte { return EncodeCharDeleteOk() },
		},
		{
			"CharDeleteFail",
			func(t *testing.T) wire.Frame { return FrameCharDeleteFail(CharDeleteFailReasonClanMemberMayNotDelete) },
			func(t *testing.T) []byte { return EncodeCharDeleteFail(CharDeleteFailReasonClanMemberMayNotDelete) },
		},
		{
			"ItemList",
			func(t *testing.T) wire.Frame {
				frame, err := FrameItemList(items, itemTable, true)
				if err != nil {
					t.Fatalf("FrameItemList: %v", err)
				}
				return frame
			},
			func(t *testing.T) []byte {
				payload, err := EncodeItemList(items, itemTable, true)
				if err != nil {
					t.Fatalf("EncodeItemList: %v", err)
				}
				return payload
			},
		},
		{
			"NewCharacterSuccess",
			func(t *testing.T) wire.Frame {
				frame, err := FrameNewCharacterSuccess(allRootTemplates(t))
				if err != nil {
					t.Fatalf("FrameNewCharacterSuccess: %v", err)
				}
				return frame
			},
			func(t *testing.T) []byte {
				payload, err := EncodeNewCharacterSuccess(allRootTemplates(t))
				if err != nil {
					t.Fatalf("EncodeNewCharacterSuccess: %v", err)
				}
				return payload
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			frame := tc.frame(t)
			defer frame.Release()

			want := wire.FrameBytes(tc.payload(t))
			if !bytes.Equal(frame.Bytes(), want) {
				t.Errorf("Frame%s = % X, want % X", tc.name, frame.Bytes(), want)
			}
		})
	}
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

// BenchmarkUserInfoUnpooled measures the pre-pool send shape: encode into a
// fresh writer, then copy behind a fresh frame header.
func BenchmarkUserInfoUnpooled(b *testing.B) {
	snap := UserInfoSnapshot{Character: frameTestCharacter(), Template: frameTestTemplate(), Items: frameTestItems()}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		frame := wire.BorrowedFrame(wire.FrameBytes(EncodeUserInfo(snap)))
		frame.Release()
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
