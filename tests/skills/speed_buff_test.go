package skills

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// userInfoSpeeds is the speed block of a UserInfo frame: the template run
// speed and the live movement and attack speed multipliers.
type userInfoSpeeds struct {
	runSpd     int32
	moveMult   float64
	attackMult float64
}

func decodeUserInfoSpeeds(t *testing.T, frame []byte) userInfoSpeeds {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeUserInfo, "UserInfo")
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
	for range 7 { // walk, swim x2, unused x2, fly x2
		r.ReadInt32()
	}
	s.moveMult, s.attackMult = r.ReadFloat64(), r.ReadFloat64()
	return s
}

// lastUserInfoSpeeds decodes the last UserInfo among frames.
func lastUserInfoSpeeds(t *testing.T, frames [][]byte, when string) userInfoSpeeds {
	t.Helper()
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i][0] == serverpackets.OpcodeUserInfo {
			return decodeUserInfoSpeeds(t, frames[i])
		}
	}
	t.Fatalf("%s: no UserInfo among %d frames", when, len(frames))
	return userInfoSpeeds{}
}

// TestRunSpeedBuffUpdatesUserInfoMoveMultiplier casts a Wind Walk-shaped
// self-buff (a flat RUN_SPEED add). Its stat funcs changing RUN_SPEED
// rebroadcast the player's info (Creature.broadcastModifiedStats →
// Player.updateAndBroadcastStatus(2), Creature.java:1243-1253), and that
// UserInfo carries the raised movement speed multiplier
// (UserInfo.java:152, CreatureStatus.java:784-790); the buff wearing off
// sends the plain multiplier again.
func TestRunSpeedBuffUpdatesUserInfoMoveMultiplier(t *testing.T) {
	t.Parallel()
	const (
		skillID = 1204
		bonus   = 33
	)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
			SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{
				Name: "Buff", Time: 2, Icon: true, StackType: "speed_up", StackOrder: 1,
				Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "runSpd", Value: bonus}},
			}},
		}})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, 1)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, c, objID, skillID, 1, 500, 60_000, objID)
	srv.Advance(t, 700*time.Millisecond)
	buffed := lastUserInfoSpeeds(t, collectUntilQuiet(t, c), "buff landed")

	srv.Advance(t, 2200*time.Millisecond)
	srv.TickEffects()
	plain := lastUserInfoSpeeds(t, collectUntilQuiet(t, c), "buff wore off")

	if want := plain.moveMult + bonus/float64(buffed.runSpd); math.Abs(buffed.moveMult-want) > 1e-5 {
		t.Fatalf("buffed move multiplier = %v, want %v (plain %v + %d/%d)", buffed.moveMult, want, plain.moveMult, bonus, buffed.runSpd)
	}
	if plain.runSpd <= 0 || plain.moveMult < 0.5 || plain.moveMult > 2 || plain.attackMult < 0.5 || plain.attackMult > 2 {
		t.Fatalf("plain speeds = %+v, want a positive run speed and live multipliers near 1 (speed block misread?)", plain)
	}
}
