package serverpackets

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

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
