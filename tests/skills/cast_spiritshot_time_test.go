package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// readLongCastStart reads the start of a cast requested at start whose hit
// time is past the client's read window: the MagicSkillUse with hit and
// reuse, the use message and a gauge of hit, then lets the driven clock run
// to the launch, which comes at hit-400 and not a millisecond earlier.
func readLongCastStart(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, start time.Time, objID, skillID, hit, reuse int32) {
	t.Helper()
	use := c.Read()
	assertFrameOpcode(t, use, serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
	r := wireReader(use[1:])
	if caster, _, sid := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != objID || sid != skillID {
		t.Fatalf("MagicSkillUse caster %d skill %d, want %d/%d", caster, sid, objID, skillID)
	}
	r.ReadInt32() // level
	if gotHit, gotReuse := r.ReadInt32(), r.ReadInt32(); gotHit != hit || gotReuse != reuse {
		t.Fatalf("MagicSkillUse timing = hit %d reuse %d, want %d/%d", gotHit, gotReuse, hit, reuse)
	}
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageUseS1, skillID, 1)
	gauge := c.Read()
	assertFrameOpcode(t, gauge, serverpackets.OpcodeSetupGauge, "SetupGauge")
	r = wireReader(gauge[1:])
	if _, current, maxTime := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); current != hit || maxTime != hit {
		t.Fatalf("SetupGauge = %d/%d, want %d/%d", current, maxTime, hit, hit)
	}

	launch := start.Add(time.Duration(hit-400) * time.Millisecond)
	srv.Advance(t, launch.Add(-time.Millisecond).Sub(c.Now()))
	c.ExpectNoFrame()
	srv.Advance(t, time.Millisecond)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
	if got := c.Now(); !got.Equal(launch) {
		t.Fatalf("MagicSkillLaunched at %v after the request, want %v", got.Sub(start), launch.Sub(start))
	}
}

// TestCastSpiritshotShortensMagicHitTime casts a Wind Strike shaped magic
// skill with no shot, a spiritshot and a blessed spiritshot charged on the
// player's weapon (#4858): a charged spiritshot of either kind cuts the
// scaled hit time to (int)(0.70 * hitTime) (CreatureCast.doCast,
// CreatureCast.java:112-116), so the MagicSkillUse hit time, the gauge and
// the launch at hit-400 all move with it.
func TestCastSpiritshotShortensMagicHitTime(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		shot    item.ShotKind
		charged bool
		wantHit int32
	}{
		// MAtkSpd 133: int(4000*333/133) = 10015.
		{name: "no shot", wantHit: 10015},
		// int(0.70 * 10015) = 7010.
		{name: "spiritshot", shot: item.ShotSpirit, charged: true, wantHit: 7010},
		{name: "blessed spiritshot", shot: item.ShotBlessedSpirit, charged: true, wantHit: 7010},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := windStrikeShape()
			def.StaticReuse = true
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Mage", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
			)
			if !srv.DrivesClock() {
				t.Skip("exact cast phase timing needs the driven clock")
			}
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, windStrikeID, 1)
			weapon := srv.GiveItem(t, objID, signetWeaponID, 1)
			startInWorld(t, c)
			c.Send(encodeSignetUseItem(weapon))
			drainUntilQuiet(t, c)
			if tc.charged {
				onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.SetChargedShot(tc.shot, true) })
			}
			if spd, _ := liveSpeeds(t, srv, objID); spd != 133 {
				t.Fatalf("fixture MAtkSpd = %d, want 133", spd)
			}

			start := c.Now()
			c.Send(encodeRequestMagicSkillUse(windStrikeID, false, false))
			readLongCastStart(t, srv, c, start, objID, windStrikeID, tc.wantHit, int32(def.ReuseDelay))
		})
	}
}
