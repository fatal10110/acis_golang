package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

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
	got := framePayload(t, FrameHennaInfo(2))
	want := []byte{OpcodeHennaInfo, 0, 0, 0, 0, 0, 0}
	want = appendD(want, 3)
	want = appendD(want, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameHennaInfo() = %x, want %x", got, want)
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
