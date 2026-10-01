package character

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// userInfoSpeeds is the speed block of a UserInfo frame: the template run
// and swim speeds and the live movement speed multiplier.
type userInfoSpeeds struct {
	runSpd, swimSpd int32
	moveMult        float64
}

func decodeUserInfoSpeeds(t *testing.T, frame []byte) userInfoSpeeds {
	t.Helper()
	if frame[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("opcode = %#x, want UserInfo", frame[0])
	}
	r := wire.NewReader(frame[1:])
	for range 5 { // x, y, z, heading, object id
		r.ReadInt32()
	}
	r.ReadString()
	for range 4 { // race, sex, class, level
		r.ReadInt32()
	}
	r.ReadInt64()
	for range 6 + 8 + 2*17 { // STR..MEN, vitals/SP/weight/slots, paperdoll ids
		r.ReadInt32()
	}
	for _, n := range []int{14, 12, 4} { // augmentation pairs, with the hand ids between
		for range n {
			r.ReadUint16()
		}
		if n != 4 {
			r.ReadInt32()
		}
	}
	for range 12 { // combat stats, PvP flag, karma
		r.ReadInt32()
	}
	var s userInfoSpeeds
	s.runSpd = r.ReadInt32()
	r.ReadInt32() // walk
	s.swimSpd = r.ReadInt32()
	for range 5 { // swim again, unused x2, fly x2
		r.ReadInt32()
	}
	s.moveMult = r.ReadFloat64()
	return s
}

// userInfoSpeedsAmong decodes every UserInfo among frames, failing when
// there is none.
func userInfoSpeedsAmong(t *testing.T, frames [][]byte, when string) []userInfoSpeeds {
	t.Helper()
	var all []userInfoSpeeds
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeUserInfo {
			all = append(all, decodeUserInfoSpeeds(t, f))
		}
	}
	if len(all) == 0 {
		t.Fatalf("%s: no UserInfo among %d frames", when, len(frames))
	}
	return all
}

func assertMoveMult(t *testing.T, when string, got []userInfoSpeeds, want float64) {
	t.Helper()
	for i, s := range got {
		if math.Abs(s.moveMult-want) > 1e-5 {
			t.Fatalf("%s: UserInfo %d move multiplier = %v, want %v", when, i, s.moveMult, want)
		}
	}
}

func assertLiveMoveSpeed(t *testing.T, when string, character *player.Character, want float64) {
	t.Helper()
	if got := character.Move().Speed(); math.Abs(got-want) > 1e-3 {
		t.Fatalf("%s: simulated move speed = %v, want %v", when, got, want)
	}
}

// TestSwampSlowsTheMoveSpeedMultiplier pins SwampZone.onEnter/onExit
// (SwampZone.java:36-51): crossing a swamp boundary broadcasts the player's
// info, and inside it PlayerStatus.getMoveSpeed (PlayerStatus.java:936-941)
// scales the base speed by (100 + move_bonus) / 100, which the UserInfo
// movement multiplier and the server's own movement both follow.
func TestSwampSlowsTheMoveSpeedMultiplier(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(2_000, 4_000, -1_000, 1_000, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	set := commons.NewStatSet()
	set.Set("move_bonus", -80)
	swamp, err := zone.NewSwamp(1, form, set)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(swamp)
	srv, character, objID := bootInZones(t, zones)
	x, y, z := srv.PlayerPosition(t, objID)
	land := userInfoSpeedsAmong(t, appear(t, srv.Client), "on land")[0]
	landSpeed := character.Move().Speed()

	in := userInfoSpeedsAmong(t, teleportIntoWater(t, srv, character, 3_000, y, z), "into the swamp")
	if len(in) != 2 {
		t.Fatalf("teleport into the swamp sent %d UserInfo, want the swamp entry one and the Appearing answer", len(in))
	}
	assertMoveMult(t, "in the swamp", in, land.moveMult*0.2)
	assertLiveMoveSpeed(t, "in the swamp", character, landSpeed*0.2)

	// The teleport itself leaves the swamp, before Appearing.
	character.TeleportTo(x, y, z, 0)
	out := userInfoSpeedsAmong(t, append(readUntilQuiet(srv.Client), appear(t, srv.Client)...), "out of the swamp")
	if len(out) != 2 {
		t.Fatalf("teleport out of the swamp sent %d UserInfo, want the swamp exit one and the Appearing answer", len(out))
	}
	assertMoveMult(t, "out of the swamp", out, land.moveMult)
	assertLiveMoveSpeed(t, "out of the swamp", character, landSpeed)
}

// TestWaterMoveSpeedMultiplierUsesSwimSpeed pins
// CreatureStatus.getMovementSpeedMultiplier over PlayerStatus.getMoveSpeed
// in water: the numerator is the swim speed while the base stays the run
// speed (CreatureStatus.java:776-790, PlayerStatus.java:934), so the water
// entry UserInfo scales the land multiplier by swim/run, and the server
// moves the player at its swim speed.
func TestWaterMoveSpeedMultiplierUsesSwimSpeed(t *testing.T) {
	t.Parallel()
	srv, character, objID := bootInZones(t, waterZones(t, 2_000, 4_000))
	_, y, z := srv.PlayerPosition(t, objID)
	land := userInfoSpeedsAmong(t, appear(t, srv.Client), "on land")[0]
	landSpeed := character.Move().Speed()
	ratio := float64(land.swimSpd) / float64(land.runSpd)

	in := userInfoSpeedsAmong(t, teleportIntoWater(t, srv, character, 3_000, y, min(z, 100)), "into the water")
	assertMoveMult(t, "in the water", in, land.moveMult*ratio)
	assertLiveMoveSpeed(t, "in the water", character, landSpeed*ratio)
}
