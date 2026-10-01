package serverpackets

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/npcstring"
)

// ---- from npcinfo_test.go ----
func TestFrameServerObjectInfo(t *testing.T) {
	got := framePayload(t, FrameServerObjectInfo(NPCInfoSnapshot{
		ObjectID: 0x01020304, TemplateID: 123, Name: "Goblin", Attackable: true,
		X: -1, Y: 2, Z: -3, Heading: 4, CollisionRadius: 5.5, CollisionHeight: 6.5,
		CurrentHP: 70, MaxHP: 100, MoveMultiplier: 1.5, AtkSpdMultiplier: 0.9,
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
	// Both speed multipliers are a literal 1.0, whatever the snapshot's.
	for _, value := range []float64{1, 1, 5.5, 6.5} {
		want = binary.LittleEndian.AppendUint64(want, math.Float64bits(value))
	}
	for _, value := range []uint32{70, 100, 1, 0} {
		want = binary.LittleEndian.AppendUint32(want, value)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameServerObjectInfo() = %x, want %x", got, want)
	}
}

// TestFrameNPCInfoWritesMovementSpeedMultiplier pins the movement
// multiplier the client scales the base run/walk speeds by.
func TestFrameNPCInfoWritesMovementSpeedMultiplier(t *testing.T) {
	want := float64(float32(66) / 60)
	payload := framePayload(t, FrameNPCInfo(NPCInfoSnapshot{RunSpd: 120, WalkSpd: 60, MoveMultiplier: want}))
	const multiplierOffset = 1 + 18*4
	if got := math.Float64frombits(binary.LittleEndian.Uint64(payload[multiplierOffset:])); got != want {
		t.Fatalf("movement speed multiplier = %v, want %v", got, want)
	}
}

// TestFrameNPCInfoWritesAttackSpeedMultiplier pins the attack speed
// multiplier the client scales the attack animation by:
// (float) (1.1 * P.Atk.Spd / base), written as a double. A 300-base NPC
// slowed to 229 carries float32(0.83966666) widened, not 0.83966666.
func TestFrameNPCInfoWritesAttackSpeedMultiplier(t *testing.T) {
	want := npcinfo.AttackSpeedMultiplier(229, 300)
	if want != float64(float32(1.1*229.0/300)) || want == 1.1*229.0/300 {
		t.Fatalf("AttackSpeedMultiplier(229, 300) = %v, want the float32-rounded quotient", want)
	}
	payload := framePayload(t, FrameNPCInfo(NPCInfoSnapshot{PAtkSpd: 229, AtkSpdMultiplier: want}))
	const multiplierOffset = 1 + 18*4
	if got := math.Float64frombits(binary.LittleEndian.Uint64(payload[multiplierOffset+8:])); got != want {
		t.Fatalf("attack speed multiplier = %v, want %v", got, want)
	}
	if got := npcinfo.AttackSpeedMultiplier(229, 0); got != 0 {
		t.Fatalf("AttackSpeedMultiplier with a zero base = %v, want 0", got)
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
		RunSpd: 120, WalkSpd: 60, MoveMultiplier: float64(float32(132) / 120),
		AtkSpdMultiplier: float64(float32(1.1 * 300.0 / 278)),
		CollisionRadius:  8, CollisionHeight: 20,
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
	want = appendPetInfoFloat64(want, s.MoveMultiplier)
	want = appendPetInfoFloat64(want, s.AtkSpdMultiplier)
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

// TestFrameRideMountTypes pins Ride's ride type switch (Ride.java:19-28):
// the three striders ride as type 1, the wyvern as type 2, any other NPC
// as 0.
func TestFrameRideMountTypes(t *testing.T) {
	for npcID, want := range map[int32]uint32{12526: 1, 12527: 1, 12528: 1, 12621: 2, 12077: 0} {
		got := framePayload(t, FrameRide(7, npcID))
		if v := binary.LittleEndian.Uint32(got[1+2*4:]); v != want {
			t.Fatalf("Ride(%d) ride type = %d, want %d", npcID, v, want)
		}
	}
}

// TestFrameDismount pins Ride(objectId, ACTION_DISMOUNT, 0): no ride type
// and the bare 1000000 class offset.
func TestFrameDismount(t *testing.T) {
	want := []byte{OpcodeRide}
	want = binary.LittleEndian.AppendUint32(want, 7)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 0)
	want = binary.LittleEndian.AppendUint32(want, 1000000)
	if got := framePayload(t, FrameDismount(7)); string(got) != string(want) {
		t.Fatalf("Dismount = %x, want %x", got, want)
	}
}
