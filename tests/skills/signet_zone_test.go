package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// readNPCInfoMoveTypes reads c until quiet and returns the move type byte
// of every NpcInfo of objID: the field after the ally crest, followed by
// the team byte, the collision radius and height (two doubles), the enchant
// effect and the flying flag.
func readNPCInfoMoveTypes(c *testsupport.ScriptedClient, objID int32) []byte {
	var out []byte
	for f := c.ReadWithTimeout(300 * time.Millisecond); f != nil; f = c.ReadWithTimeout(300 * time.Millisecond) {
		if len(f) >= 5 && f[0] == serverpackets.OpcodeNPCInfo && wire.NewReader(f[1:]).ReadInt32() == objID {
			out = append(out, f[len(f)-1-(1+8+8+4+4)])
		}
	}
	return out
}

// TestSignetPointJoinsTheZonesItStandsIn pins a signet effect point's zone
// membership (EffectPoint is an Npc, so Creature.setRegion enters and
// leaves its zones): a point placed in water is the zone's occupant, and
// its known players see its NpcInfo again, swimming, for the crossing
// (WaterZone.java:28-39: NpcInfo, as the shipped point templates move at
// 20/50, not 0). Despawned when its signet ends, it leaves the zone,
// showing its NpcInfo out of the water before it goes (WaterZone.java:48-59).
func TestSignetPointJoinsTheZonesItStandsIn(t *testing.T) {
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
			ID: 13018, Type: "EffectPoint", CollisionRadius: 5, RunSpeed: 20, WalkSpeed: 50,
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
	assertOccupant(t, water, point.ObjectID())
	if !point.InsideZone(zone.FlagWater) {
		t.Fatal("signet point placed in water is not in it")
	}
	if got := readNPCInfoMoveTypes(c, point.ObjectID()); countMoveType(got, move.MoveSwim) != 1 {
		t.Fatalf("NpcInfo move types of the point entering the water = %v, want one swimming", got)
	}

	srv.Advance(t, 1100*time.Millisecond)
	srv.TickEffects()
	if _, ok := srv.State.Object(point.ObjectID()); ok {
		t.Fatal("signet point still in the world after its signet ended")
	}
	for _, a := range water.Occupants() {
		if a.ObjectID() == point.ObjectID() {
			t.Fatal("despawned signet point is still an occupant of the water zone")
		}
	}
	if got := readNPCInfoMoveTypes(c, point.ObjectID()); len(got) != 1 || countMoveType(got, move.MoveGround) != 1 {
		t.Fatalf("NpcInfo move types of the point leaving the water = %v, want one on the ground", got)
	}
}

// countMoveType counts the move types among types equal to want.
func countMoveType(types []byte, want move.MoveType) int {
	n := 0
	for _, mt := range types {
		if mt == byte(want) {
			n++
		}
	}
	return n
}

// assertOccupant fails unless objID is among z's occupants.
func assertOccupant(t *testing.T, z *zone.Water, objID int32) {
	t.Helper()
	for _, a := range z.Occupants() {
		if a.ObjectID() == objID {
			return
		}
	}
	t.Fatalf("object %d is not an occupant of the water zone", objID)
}
