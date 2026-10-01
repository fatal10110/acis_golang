package serverpackets

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

func TestFrameExStorageMaxCount(t *testing.T) {
	got := framePayload(t, FrameExStorageMaxCount(&player.Character{Race: player.RaceDwarf}))
	want := []byte{OpcodeExtended}
	want = appendH(want, OpcodeExStorageMaxCount)
	want = appendD(want, 100) // MaximumSlotsForDwarf
	want = appendD(want, 120) // MaximumWarehouseSlotsForDwarf
	want = appendD(want, 20)  // MaximumFreightSlots
	want = appendD(want, 5)   // MaxPvtStoreSlotsDwarf (sell)
	want = appendD(want, 5)   // MaxPvtStoreSlotsDwarf (buy)
	want = appendD(want, 50)  // DwarfRecipeLimit
	want = appendD(want, 50)  // CommonRecipeLimit
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExStorageMaxCount() = %x, want %x", got, want)
	}
}

// TestFrameExStorageMaxCountFollowsConfigAndLimitStats pins every field to
// its configured base for the character's race plus its own limit stat,
// truncated, in the reference field order.
func TestFrameExStorageMaxCountFollowsConfigAndLimitStats(t *testing.T) {
	slots := player.StorageSlots{
		WarehouseNoDwarf: 11, WarehouseDwarf: 12, Freight: 13,
		PrivateStoreNoDwarf: 14, PrivateStoreDwarf: 15,
		DwarfRecipe: 16, CommonRecipe: 17, Configured: true,
	}
	mods := []effect.Mod{
		{Stat: stat.WhLim, Op: effect.OpAdd, Value: 1.9},
		{Stat: stat.FreightLim, Op: effect.OpAdd, Value: 2},
		{Stat: stat.PSellLim, Op: effect.OpAdd, Value: 3},
		{Stat: stat.PBuyLim, Op: effect.OpAdd, Value: 4},
		{Stat: stat.RecDLim, Op: effect.OpAdd, Value: 5},
		{Stat: stat.RecCLim, Op: effect.OpAdd, Value: 6},
	}
	for _, tc := range []struct {
		race player.Race
		want []int32
	}{
		{player.RaceHuman, []int32{90, 12, 15, 17, 18, 21, 23}},
		{player.RaceDwarf, []int32{117, 13, 15, 18, 19, 21, 23}},
	} {
		c := &player.Character{Race: tc.race, Name: "S"}
		c.Configure(player.Runtime{Rules: player.Rules{
			InventorySlots: player.InventorySlots{NoDwarf: 90, Dwarf: 117, Configured: true},
			StorageSlots:   slots,
		}})
		c.AddStatFuncs(mods)
		got := framePayload(t, FrameExStorageMaxCount(c))
		want := appendH([]byte{OpcodeExtended}, OpcodeExStorageMaxCount)
		for _, v := range tc.want {
			want = appendD(want, v)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("race %v: FrameExStorageMaxCount() = %x, want %x", tc.race, got, want)
		}
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

// ---- from exregenmax_test.go ----
func TestFrameExSetCompassZoneCode(t *testing.T) {
	got := framePayload(t, FrameExSetCompassZoneCode(0x0c))
	want := []byte{0xfe, 0x32, 0x00, 0x0c, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExSetCompassZoneCode() = %x, want %x", got, want)
	}
	r := wire.NewReader(got[3:])
	if code := r.ReadInt32(); code != 0x0c || r.Err() != nil || r.Remaining() != 0 {
		t.Fatalf("decoded compass code = %#x, err = %v, trailing = %d", code, r.Err(), r.Remaining())
	}
}

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

// ---- from ssqinfo_test.go ----
func TestFrameSSQInfo(t *testing.T) {
	got := framePayload(t, FrameSSQInfo())

	want := []byte{OpcodeSSQInfo}
	want = binary.LittleEndian.AppendUint16(want, regularSkyState)

	if !bytes.Equal(got, want) {
		t.Errorf("FrameSSQInfo() = % x, want % x", got, want)
	}
}
