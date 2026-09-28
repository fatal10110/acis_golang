package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// edgeSkillID is the skill every cast edge-case scenario below casts.
const edgeSkillID = 1854

// bootEdgeCaster boots the fixture player knowing def and enters the world.
// The scenarios read frame times off the harness's driven clock, so they
// require it.
func bootEdgeCaster(t *testing.T, def modelskill.Definition) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	if !srv.DrivesClock() {
		t.Fatal("cast edge-case scenarios need the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
	startInWorld(t, c)
	drainUntilQuiet(t, c)
	return srv, c, objID
}

// readMagicSkillUseHitTime reads the cast's MagicSkillUse ack and the use
// message, returning the ack's hit-time field.
func readMagicSkillUseHitTime(t *testing.T, c *testsupport.ScriptedClient, level int32) int32 {
	t.Helper()
	reply := c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
	r := wireReader(reply[1:])
	r.ReadInt32() // caster
	r.ReadInt32() // target
	if sid, lvl := r.ReadInt32(), r.ReadInt32(); sid != edgeSkillID || lvl != level {
		t.Fatalf("MagicSkillUse skill = %d/%d, want %d/%d", sid, lvl, edgeSkillID, level)
	}
	hit := r.ReadInt32()
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageUseS1, edgeSkillID, level)
	return hit
}

// readCastPhases reads the rest of a started cast — the optional cast bar,
// MagicSkillLaunched, and the hit's MP StatusUpdate — and returns whether a
// SetupGauge was sent, its duration, and how far the driven clock had moved
// past start when the launch and hit frames arrived.
func readCastPhases(t *testing.T, c *testsupport.ScriptedClient, start time.Time) (gauge bool, gaugeMs int32, launchAt, hitAt time.Duration) {
	t.Helper()
	frame := c.Read()
	if frame[0] == serverpackets.OpcodeSetupGauge {
		gauge = true
		r := wireReader(frame[1:])
		r.ReadInt32() // color
		r.ReadInt32() // current
		gaugeMs = r.ReadInt32()
		frame = c.Read()
	}
	assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
	launchAt = c.Now().Sub(start)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeStatusUpdate, "hit StatusUpdate")
	hitAt = c.Now().Sub(start)
	return gauge, gaugeMs, launchAt, hitAt
}

// TestCastScaledHitTimeFloorsAtFiveHundred pins CreatureCast.doCast's floor
// (CreatureCast.java:118-119): a skill configured with at least 500 ms of
// hit time never casts faster than 500 ms however far casting speed scales
// it, while a skill configured below 500 ms keeps its scaled time and so
// can drop under the 410 ms gauge threshold.
func TestCastScaledHitTimeFloorsAtFiveHundred(t *testing.T) {
	t.Parallel()
	// Casting speed 1332 is four times the base 333, quartering every
	// non-static hit time (hitTime * 333 / MAtkSpd). The fixture caster's
	// WIT bonus finalizes a set 3330 to that speed.
	const setSpeed, castSpeed = 3330, 1332
	for _, tt := range []struct {
		name       string
		configured int
		wantHit    int32
		wantGauge  bool
	}{
		{name: "scaled above floor", configured: 4000, wantHit: 1000, wantGauge: true},
		{name: "scaled below floor is raised", configured: 1000, wantHit: 500, wantGauge: true},
		{name: "configured below floor stays scaled", configured: 499, wantHit: 124},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			def := modelskill.Definition{
				ID: edgeSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				Magic: true, HitTime: tt.configured, MPConsume: 3, SkillType: "DUMMY",
			}
			srv, c, objID := bootEdgeCaster(t, def)
			onPlayerQueue(t, srv, objID, func(pc *player.Character) {
				pc.AddStatFuncs([]effect.Mod{{Stat: stat.MagicAttackSpeed, Op: effect.OpSet, Value: setSpeed}})
				if got := pc.MagicAttackSpeed(); got != castSpeed {
					t.Errorf("caster MAtkSpd = %d, want %d", got, castSpeed)
				}
			})
			if t.Failed() {
				t.FailNow()
			}
			drainUntilQuiet(t, c)

			c.Send(encodeRequestMagicSkillUse(edgeSkillID, false, false))
			if hit := readMagicSkillUseHitTime(t, c, 1); hit != tt.wantHit {
				t.Fatalf("MagicSkillUse hit time = %d, want %d", hit, tt.wantHit)
			}
			gauge, gaugeMs, _, _ := readCastPhases(t, c, c.Now())
			if gauge != tt.wantGauge {
				t.Fatalf("SetupGauge sent = %v, want %v", gauge, tt.wantGauge)
			}
			if gauge && gaugeMs != tt.wantHit {
				t.Fatalf("SetupGauge duration = %d, want %d", gaugeMs, tt.wantHit)
			}
			drainUntilQuiet(t, c)
		})
	}
}

// TestCastAtOrBelow410CollapsesPhases pins CreatureCast's 410 ms threshold
// (CreatureCast.java:157-165, 232, 288): a hit time above it sends the cast
// bar and runs launch at hitTime-400, the hit 400 ms later and the finalizer
// after the cool time; at or below it no bar is sent and launch, hit and
// finalizer all run at once, even with a cool time configured. The
// MagicSkillUse ack still reports the configured hit time.
func TestCastAtOrBelow410CollapsesPhases(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		hitTime   int
		collapsed bool
	}{
		{name: "410 collapses", hitTime: 410, collapsed: true},
		{name: "411 keeps phases", hitTime: 411},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			def := modelskill.Definition{
				ID: edgeSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				Magic: true, HitTime: tt.hitTime, CoolTime: 1000, StaticHitTime: true, MPConsume: 3, SkillType: "DUMMY",
			}
			_, c, _ := bootEdgeCaster(t, def)

			c.Send(encodeRequestMagicSkillUse(edgeSkillID, false, false))
			if hit := readMagicSkillUseHitTime(t, c, 1); hit != int32(tt.hitTime) {
				t.Fatalf("MagicSkillUse hit time = %d, want %d", hit, tt.hitTime)
			}
			start := c.Now()
			gauge, gaugeMs, launchAt, hitAt := readCastPhases(t, c, start)

			if tt.collapsed {
				if gauge {
					t.Fatalf("SetupGauge sent (%d ms) for a %d ms hit time, want none", gaugeMs, tt.hitTime)
				}
				if launchAt != 0 || hitAt != 0 {
					t.Fatalf("launch at +%v, hit at +%v, want both at +0", launchAt, hitAt)
				}
				// The finalizer ignored the 1000 ms cool time too: the cast is
				// already over, so a recast starts on the same instant.
				c.Send(encodeRequestMagicSkillUse(edgeSkillID, false, false))
				readMagicSkillUseHitTime(t, c, 1)
				if at := c.Now().Sub(start); at != 0 {
					t.Fatalf("recast accepted at +%v, want +0", at)
				}
				drainUntilQuiet(t, c)
				return
			}
			if !gauge || gaugeMs != int32(tt.hitTime) {
				t.Fatalf("SetupGauge sent = %v duration %d, want %d", gauge, gaugeMs, tt.hitTime)
			}
			wantLaunch := time.Duration(tt.hitTime-400) * time.Millisecond
			wantHit := time.Duration(tt.hitTime) * time.Millisecond
			if launchAt != wantLaunch || hitAt != wantHit {
				t.Fatalf("launch at +%v, hit at +%v, want +%v and +%v", launchAt, hitAt, wantLaunch, wantHit)
			}
			drainUntilQuiet(t, c)
		})
	}
}

// castOutcome records what the caster's client saw between a hit landing on
// it mid-cast and the end of that cast's hit time.
type castOutcome struct {
	canceled    bool
	interrupted bool
	launched    bool
}

// watchCast reads the caster's frames until the driven clock passes until.
func watchCast(t *testing.T, c *testsupport.ScriptedClient, until time.Time) castOutcome {
	t.Helper()
	var out castOutcome
	for c.Now().Before(until) {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			continue
		}
		switch frame[0] {
		case serverpackets.OpcodeMagicSkillCanceled:
			out.canceled = true
		case serverpackets.OpcodeMagicSkillLaunched:
			out.launched = true
		case serverpackets.OpcodeSystemMessage:
			if wireReader(frame[1:]).ReadInt32() == serverpackets.SystemMessageCastingInterrupted {
				out.interrupted = true
			}
		}
	}
	return out
}

// TestDamageCastBreakRules drives a real NPC auto-attack into a player
// mid-cast and pins Formulas.calcCastBreak (Formulas.java:725-750): an
// invulnerable caster is never broken; a FUSION cast breaks on any hit
// before the magic-only rule and without the break roll, so even a physical
// fusion skill under a roll no ordinary cast can fail is interrupted; any
// other physical cast is never broken; a magic cast breaks when the roll
// falls under the clamped rate.
//
// The raid-related half of calcCastBreak's first guard needs a non-player
// caster taking damage, which no damage path forwards to the cast yet
// (#2650); the controller-level TestInterruptOnDamageImmuneOverridesFusion
// covers it.
func TestDamageCastBreakRules(t *testing.T) {
	t.Parallel()
	const hitTime = 10_000
	for _, tt := range []struct {
		name      string
		magic     bool
		skillType string
		invul     bool
		// roll is the caster's [0,100) break roll; the break rate is clamped
		// to [1,99], so 0 always breaks and 99 never does.
		roll        int
		interrupted bool
	}{
		{name: "magic breaks under the rate", magic: true, skillType: "DUMMY", interrupted: true},
		{name: "magic survives a roll over the rate", magic: true, skillType: "DUMMY", roll: 99},
		{name: "invulnerable caster never breaks", magic: true, skillType: "DUMMY", invul: true},
		{name: "physical never breaks", skillType: "DUMMY"},
		{name: "physical fusion always breaks", skillType: "FUSION", roll: 99, interrupted: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			def := modelskill.Definition{
				ID: edgeSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				Magic: tt.magic, HitTime: hitTime, StaticHitTime: true, SkillType: tt.skillType,
			}
			if tt.skillType == "FUSION" {
				def.Target, def.CastRange = modelskill.TargetOne, 900
			}
			srv, c, objID := bootEdgeCaster(t, def)
			attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, c)
			if tt.skillType == "FUSION" {
				targetHostile(t, c, attacker.ObjectID())
				drainUntilQuiet(t, c)
			}
			var victim attackable.Combatant
			onPlayerQueue(t, srv, objID, func(pc *player.Character) {
				pc.SetRollSource(func(int) int { return tt.roll })
				if tt.invul {
					pc.SetInvul(true)
				}
				victim = pc
			})

			// A non-offensive single-target skill reaches a monster only
			// when forced.
			c.Send(encodeRequestMagicSkillUse(edgeSkillID, tt.skillType == "FUSION", false))
			readMagicSkillUseHitTime(t, c, 1)
			end := c.Now().Add(hitTime*time.Millisecond + time.Second)

			attacker.DoAttack(t, victim)
			got := watchCast(t, c, end)

			if got.interrupted != tt.interrupted || got.canceled != tt.interrupted {
				t.Fatalf("CASTING_INTERRUPTED = %v, MagicSkillCanceled = %v, want both %v", got.interrupted, got.canceled, tt.interrupted)
			}
			// A cast the hit left alone runs on to its launch; fusion casts
			// have no launch phase to observe.
			if tt.skillType != "FUSION" && got.launched == tt.interrupted {
				t.Fatalf("MagicSkillLaunched = %v, want %v", got.launched, !tt.interrupted)
			}
		})
	}
}
