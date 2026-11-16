package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: CreatureCast.doCast (CreatureCast.java:122-142) scales a
// non-static reuse delay by the caster's MAGIC_REUSE_RATE (mReuse) or
// P_REUSE (pReuse) stat, truncating to int, then by 333.0 / its casting or
// attack speed, truncating again. The result is the MagicSkillUse reuse, the
// skill's disable timer and, above 30 s, its reuse timestamp.

const (
	quickRecoveryID = 164  // passive, mul mReuse 0.9/0.85/0.8
	windStrikeID    = 1177 // magic, hitTime 4000, reuseDelay 6000
	longReuseID     = 9301 // magic, reuseDelay 60000: past the timestamp threshold
	pReusePassiveID = 9302 // passive, mul pReuse 0.9
	powerStrikeID   = 9303 // physical, reuseDelay 6000
	npcReuseSkill   = 9304 // NPC magic, hitTime 2000, reuseDelay 10000
	npcReuseTmpl    = 101  // casting monster template
)

func quickRecovery() modelskill.Definition {
	return modelskill.Definition{
		ID: quickRecoveryID, Level: 3, Activation: modelskill.ActivationPassive, Target: modelskill.TargetSelf,
		SkillType: "BUFF", Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncMul, Stat: "mReuse", Value: 0.8}},
	}
}

func windStrikeShape() modelskill.Definition {
	return modelskill.Definition{
		ID: windStrikeID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		Magic: true, HitTime: 4000, ReuseDelay: 6000, SkillType: "DUMMY",
	}
}

// liveSpeeds reads the player's casting and attack speeds on its queue.
func liveSpeeds(t *testing.T, srv *gameservertest.Server, objID int32) (mAtkSpd, pAtkSpd int) {
	t.Helper()
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		mAtkSpd, pAtkSpd = pc.MagicAttackSpeed(), pc.AttackSpeed()
	})
	return mAtkSpd, pAtkSpd
}

// TestCastReuseFollowsMagicReuseRate casts a Wind Strike shaped skill with
// and without Quick Recovery lvl 3 (mReuse x0.8) known (#4859): the passive
// cuts the MagicSkillUse reuse, the disable timer and the reuse timestamp.
func TestCastReuseFollowsMagicReuseRate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name               string
		passive            bool
		wantReuse, wantRec int32
	}{
		// MAtkSpd 133: int(4000*333/133) = 10015 hit;
		// int(6000 * (333.0/133)) = 15022; int(60000 * (333.0/133)) = 150225.
		{name: "no passive", wantReuse: 15022, wantRec: 150225},
		// int(6000*0.8) = 4800, int(4800 * (333.0/133)) = 12018;
		// int(60000*0.8) = 48000, int(48000 * (333.0/133)) = 120180.
		{name: "quick recovery", passive: true, wantReuse: 12018, wantRec: 120180},
	} {
		t.Run(tc.name, func(t *testing.T) {
			long := windStrikeShape()
			long.ID, long.ReuseDelay = longReuseID, 60_000
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Mage", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{quickRecovery(), windStrikeShape(), long})),
			)
			if !srv.DrivesClock() {
				t.Skip("exact reuse timing needs the driven clock")
			}
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, windStrikeID, 1)
			seedKnownSkill(t, srv, objID, longReuseID, 1)
			if tc.passive {
				seedKnownSkill(t, srv, objID, quickRecoveryID, 3)
			}
			startInWorld(t, c)
			drainUntilQuiet(t, c)
			if spd, _ := liveSpeeds(t, srv, objID); spd != 133 {
				t.Fatalf("fixture MAtkSpd = %d, want 133", spd)
			}

			start := c.Now()
			c.Send(encodeRequestMagicSkillUse(windStrikeID, false, false))
			readLongCastStart(t, srv, c, start, objID, windStrikeID, 10015, tc.wantReuse)
			srv.AdvanceUntil(t, "cast end", func() bool { return !srv.PlayerCastingNow(t, objID) })
			drainUntilQuiet(t, c)

			// The disable timer runs out exactly at the planned reuse.
			srv.Advance(t, start.Add(time.Duration(tc.wantReuse-1)*time.Millisecond).Sub(c.Now()))
			c.Send(encodeRequestMagicSkillUse(windStrikeID, false, false))
			assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageS1PreparedForReuse, windStrikeID, 1)
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "recast before the reuse ran out")
			drainUntilQuiet(t, c)
			srv.Advance(t, time.Millisecond)
			c.Send(encodeRequestMagicSkillUse(windStrikeID, false, false))
			readUntil(t, c, serverpackets.OpcodeMagicSkillUse)
			srv.Advance(t, 10015*time.Millisecond)
			if srv.PlayerCastingNow(t, objID) {
				t.Fatal("the recast is still running past its hit time")
			}
			drainUntilQuiet(t, c)

			// A reuse past 30 s is also stamped with the scaled delay.
			c.Send(encodeRequestMagicSkillUse(longReuseID, false, false))
			readUntil(t, c, serverpackets.OpcodeMagicSkillUse)
			key := cast.ReuseKey(&long)
			onPlayerQueue(t, srv, objID, func(pc *player.Character) {
				left, ok := pc.SkillReuseRemaining(key, pc.Now())
				if !ok || left != time.Duration(tc.wantRec)*time.Millisecond {
					t.Errorf("reuse timestamp = %v (recorded %v), want %dms", left, ok, tc.wantRec)
				}
			})
		})
	}
}

// TestCastReuseFollowsPhysicalReuseRate casts a physical skill with and
// without a pReuse x0.9 passive known (#4859).
func TestCastReuseFollowsPhysicalReuseRate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		passive   bool
		wantReuse int32
	}{
		// PAtkSpd 255: int(6000 * (333.0/255)) = 7835.
		{name: "no passive", wantReuse: 7835},
		// int(6000*0.9) = 5400, int(5400 * (333.0/255)) = 7051.
		{name: "pReuse passive", passive: true, wantReuse: 7051},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
					{
						ID: pReusePassiveID, Level: 1, Activation: modelskill.ActivationPassive, Target: modelskill.TargetSelf,
						SkillType: "BUFF", Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncMul, Stat: "pReuse", Value: 0.9}},
					},
					{
						ID: powerStrikeID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
						HitTime: 1000, StaticHitTime: true, ReuseDelay: 6000, SkillType: "DUMMY",
					},
				})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, powerStrikeID, 1)
			if tc.passive {
				seedKnownSkill(t, srv, objID, pReusePassiveID, 1)
			}
			startInWorld(t, c)
			drainUntilQuiet(t, c)
			if _, spd := liveSpeeds(t, srv, objID); spd != 255 {
				t.Fatalf("fixture PAtkSpd = %d, want 255", spd)
			}

			c.Send(encodeRequestMagicSkillUse(powerStrikeID, false, false))
			readCastStartFrames(t, c, objID, powerStrikeID, 1, 1000, tc.wantReuse, objID)
		})
	}
}

// TestNPCCastReuseFollowsTemplatePassive casts from a monster whose template
// carries an mReuse x0.8 passive (#4859): its MagicSkillUse reuse is cut too.
// A slow caster's reuse divides the speed ratio out before multiplying it in
// (#4865).
func TestNPCCastReuseFollowsTemplatePassive(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name               string
		passives           []modelskill.Ref
		wit                int
		reuse              int
		wantHit, wantReuse int32
	}{
		// WIT 20: casting speed 333, so only the rate moves the reuse.
		{name: "no passive", wit: 20, reuse: 10000, wantHit: 2000, wantReuse: 10000},
		{name: "mReuse passive", passives: []modelskill.Ref{{ID: quickRecoveryID, Level: 3}}, wit: 20, reuse: 10000, wantHit: 2000, wantReuse: 8000},
		// WIT 4: bonus 0.46, casting speed int(333*0.46) = 153;
		// int(2000*333/153) = 4352; int(340000 * (333.0/153)) = 739999,
		// where 340000*333/153 rounds to 740000.
		{name: "slow caster", wit: 4, reuse: 340000, wantHit: 4352, wantReuse: 739999},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c := srv.Client
			startInWorld(t, c)
			drainUntilQuiet(t, c)
			hostile, aiCtl := srv.SpawnCastingHostileNPC(t, &npc.Template{
				ID: npcReuseTmpl, TemplateID: npcReuseTmpl, Type: "Monster", Level: 1, HPMax: 1000, WIT: tc.wit,
				AtkSpd: 253, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
				Passives: tc.passives,
			}, modelskill.NewTable([]modelskill.Definition{quickRecovery(), {
				ID: npcReuseSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				Magic: true, SkillType: "BUFF", HitTime: 2000, ReuseDelay: tc.reuse,
			}}))
			drainUntilQuiet(t, c)

			onNPCQueue(t, hostile, func() { aiCtl.Cast(hostile, modelskill.Ref{ID: npcReuseSkill, Level: 1}) })
			r := wireReader(readUntil(t, c, serverpackets.OpcodeMagicSkillUse)[1:])
			if caster := r.ReadInt32(); caster != hostile.ObjectID() {
				t.Fatalf("MagicSkillUse caster = %d, want monster %d", caster, hostile.ObjectID())
			}
			r.ReadInt32() // target
			r.ReadInt32() // skill
			r.ReadInt32() // level
			if hit, reuse := r.ReadInt32(), r.ReadInt32(); hit != tc.wantHit || reuse != tc.wantReuse {
				t.Fatalf("MagicSkillUse hit/reuse = %d/%d, want %d/%d", hit, reuse, tc.wantHit, tc.wantReuse)
			}
		})
	}
}
