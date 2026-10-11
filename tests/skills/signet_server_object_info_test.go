package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// readServerObjectInfos reads c until quiet and decodes the attackable flag
// and the HP pair of every ServerObjectInfo of objID, in order.
func readServerObjectInfos(c *testsupport.ScriptedClient, objID int32) [][3]int32 {
	var out [][3]int32
	for f := c.ReadWithTimeout(300 * time.Millisecond); f != nil; f = c.ReadWithTimeout(300 * time.Millisecond) {
		if len(f) < 5 || f[0] != serverpackets.OpcodeServerObjectInfo {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != objID {
			continue
		}
		r.ReadInt32() // template id
		r.ReadString()
		attackable := r.ReadInt32()
		for range 4 { // x, y, z, heading
			r.ReadInt32()
		}
		for range 4 { // the two multipliers, collision radius and height
			r.ReadFloat64()
		}
		out = append(out, [3]int32{attackable, r.ReadInt32(), r.ReadInt32()})
	}
	return out
}

// TestStillSignetPointInWaterShowsNotAttackable pins the stationary view
// of a signet effect point that cannot move: no creature may attack an
// effect point (EffectPoint.isAttackableBy), so crossing the water shows
// its known players ServerObjectInfo (WaterZone.java:28-59, its move speed
// is 0) attackable 0 with no HP sent, on placement and again when its
// signet ends (ServerObjectInfo.writeImpl).
func TestStillSignetPointInWaterShowsNotAttackable(t *testing.T) {
	t.Parallel()
	def := modelskill.Definition{
		ID: 454, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		SkillType: "SIGNET", EffectNpcID: 13018, Radius: 180,
		Effects: []modelskill.EffectTemplate{{Name: "Signet", Count: 1, Time: 1}},
	}
	form, err := zone.NewCuboid(-200_000, 200_000, -200_000, 200_000, -20_000, 20_000)
	if err != nil {
		t.Fatal(err)
	}
	water := zone.NewWater(1, form)
	zones := zone.NewIndex()
	zones.Add(water)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{
			ID: 13018, Type: "EffectPoint", CollisionRadius: 5, HPMax: 100,
		}})),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 454, 1)
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(454, false, false))
	var point *npc.EffectPoint
	srv.AdvanceUntil(t, "signet effect point", func() bool {
		for _, obj := range srv.State.Objects() {
			if ep, ok := obj.(*npc.EffectPoint); ok {
				point = ep
			}
		}
		return point != nil
	})
	want := [3]int32{0, 0, 0}
	if got := readServerObjectInfos(c, point.ObjectID()); len(got) != 1 || got[0] != want {
		t.Fatalf("placed still signet point ServerObjectInfo (attackable, HP, max HP) = %v, want one %v", got, want)
	}

	srv.Advance(t, 1100*time.Millisecond)
	srv.TickEffects()
	if _, ok := srv.State.Object(point.ObjectID()); ok {
		t.Fatal("signet point still in the world after its signet ended")
	}
	if got := readServerObjectInfos(c, point.ObjectID()); len(got) != 1 || got[0] != want {
		t.Fatalf("removed still signet point ServerObjectInfo (attackable, HP, max HP) = %v, want one %v", got, want)
	}
}
