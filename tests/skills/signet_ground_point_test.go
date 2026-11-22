package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const (
	groundPointSignetID = 1600
	groundPointOtherID  = 1601
	groundPointBuffID   = 1602
	groundPointNpcID    = 13018
	// groundPointFar is far beyond every cast range here: the effect point
	// of #4991's report spawned this far from an accepted 900-range cast.
	groundPointFar = 30_000
)

// groundPointSignet is a ground-targeted SIGNET with the given cast range.
func groundPointSignet(id int32, castRange int) modelskill.Definition {
	return modelskill.Definition{
		ID:            modelskill.ID(id),
		Level:         1,
		Activation:    modelskill.ActivationActive,
		Target:        modelskill.TargetGround,
		CastRange:     castRange,
		HitTime:       1000,
		ReuseDelay:    60_000,
		StaticHitTime: true,
		StaticReuse:   true,
		SkillType:     "SIGNET",
		Radius:        180,
		Effects:       []modelskill.EffectTemplate{{Name: "Signet", Count: 1, Time: 10}},
	}.WithExtra(modelskill.Extra{
		EffectNpcID: groundPointNpcID,
	})
}

// bootGroundPointCaster boots an in-world Mage who knows defs and returns
// the caster's position.
func bootGroundPointCaster(t *testing.T, defs ...modelskill.Definition) (srv *gameservertest.Server, c *testsupport.ScriptedClient, objID int32, x, y, z int32) {
	t.Helper()
	srv = gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{ID: groundPointNpcID, Type: "EffectPoint", CollisionRadius: 5}})),
		gameservertest.WithSkills(skillPersistence(t, defs)),
	)
	if !srv.DrivesClock() {
		t.Skip("cast phase ordering needs the driven clock")
	}
	c, objID = srv.Client, srv.SoleObjectID(t)
	for _, def := range defs {
		seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
	}
	startInWorld(t, c)
	px, py, pz := srv.PlayerPosition(t, objID)
	return srv, c, objID, int32(px), int32(py), int32(pz)
}

// groundPoints lists the X, Y of every signet effect point in the world.
func groundPoints(srv *gameservertest.Server) [][2]int {
	var points [][2]int
	for _, obj := range srv.State.Objects() {
		if ep, ok := obj.(*npc.EffectPoint); ok {
			x, y, _ := ep.Position()
			points = append(points, [2]int{x, y})
		}
	}
	return points
}

func awaitCasting(t *testing.T, srv *gameservertest.Server, objID int32, casting bool) {
	t.Helper()
	srv.AdvanceUntil(t, "caster casting state", func() bool { return srv.PlayerCastingNow(t, objID) == casting })
}

// TestGroundSignetKeepsItsPointPastARefusedRequest accepts a ground signet
// cast, then sends a second ground request for the same skill far away,
// refused because the skill is now reusing: the accepted cast's effect point
// still spawns where it was accepted (#4991).
func TestGroundSignetKeepsItsPointPastARefusedRequest(t *testing.T) {
	t.Parallel()
	srv, c, objID, x, y, z := bootGroundPointCaster(t, groundPointSignet(groundPointSignetID, 900))

	c.Send(encodeRequestExMagicSkillUseGround(x+100, y, z, groundPointSignetID, false, false))
	awaitCasting(t, srv, objID, true)
	c.Send(encodeRequestExMagicSkillUseGround(x+groundPointFar, y, z, groundPointSignetID, false, false))
	awaitCasting(t, srv, objID, false)

	got := groundPoints(srv)
	if want := [2]int{int(x) + 100, int(y)}; len(got) != 1 || got[0] != want {
		t.Fatalf("effect points = %v, want one at %v", got, want)
	}
}

// TestGroundSignetKeepsItsPointPastADeferredRequest queues a second ground
// signet behind an accepted one: the accepted cast lands at its own point,
// and the queued one, run once the first ends, at the point it was
// requested with.
func TestGroundSignetKeepsItsPointPastADeferredRequest(t *testing.T) {
	t.Parallel()
	srv, c, objID, x, y, z := bootGroundPointCaster(t, groundPointSignet(groundPointSignetID, 900), groundPointSignet(groundPointOtherID, 900))

	c.Send(encodeRequestExMagicSkillUseGround(x+100, y, z, groundPointSignetID, false, false))
	awaitCasting(t, srv, objID, true)
	c.Send(encodeRequestExMagicSkillUseGround(x, y+200, z, groundPointOtherID, false, false))
	srv.AdvanceUntil(t, "both effect points", func() bool { return len(groundPoints(srv)) == 2 })

	got := groundPoints(srv)
	first, second := [2]int{int(x) + 100, int(y)}, [2]int{int(x), int(y) + 200}
	if !(got[0] == first && got[1] == second) && !(got[0] == second && got[1] == first) {
		t.Fatalf("effect points = %v, want %v and %v", got, first, second)
	}
}

// TestGroundSignetApproachKeepsItsPointPastARefusedRequest starts the
// approach walk of a ground signet requested out of range, then sends a
// ground request for a skill still reusing at a far point: the refusal
// leaves the walk alone, and on arrival the signet lands at the point it
// walked to.
func TestGroundSignetApproachKeepsItsPointPastARefusedRequest(t *testing.T) {
	t.Parallel()
	buff := modelskill.Definition{
		ID: groundPointBuffID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetGround,
		CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		SkillType: "BUFF", Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
	}
	srv, c, objID, x, y, z := bootGroundPointCaster(t, groundPointSignet(groundPointSignetID, 100), buff)

	// Put the buff on reuse.
	c.Send(encodeRequestExMagicSkillUseGround(x+50, y, z, groundPointBuffID, false, false))
	awaitCasting(t, srv, objID, true)
	awaitCasting(t, srv, objID, false)

	c.Send(encodeRequestExMagicSkillUseGround(x+400, y, z, groundPointSignetID, false, false))
	srv.AdvanceUntil(t, "approach walk", func() bool { return srv.PlayerMove(t, objID).Moving() })
	c.Send(encodeRequestExMagicSkillUseGround(x-groundPointFar, y, z, groundPointBuffID, false, false))
	srv.AdvanceUntil(t, "signet effect point", func() bool { return len(groundPoints(srv)) > 0 })

	got := groundPoints(srv)
	if want := [2]int{int(x) + 400, int(y)}; len(got) != 1 || got[0] != want {
		t.Fatalf("effect points = %v, want one at %v", got, want)
	}
}
