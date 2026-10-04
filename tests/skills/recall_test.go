package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// recallTown is a restart table whose one point covers the fixture spawn's
// map region and lands at town.
func recallTown(town location.Location) *restart.Table {
	region := location.Point{
		X: (0-world.MinX)/world.TileSize + world.TileXMin,
		Y: (0-world.MinY)/world.TileSize + world.TileYMin,
	}
	return &restart.Table{Points: []restart.Point{{
		Name: "TestTown", MapRegions: []location.Point{region}, Points: []location.Location{town},
	}}}
}

// bootRecallCaster boots one caster knowing def at its level.
func bootRecallCaster(t *testing.T, def modelskill.Definition, opts ...gameservertest.Option) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	}, opts...)...)
	objID := srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
	return srv, srv.Client, objID
}

func near(x, y int, at location.Location) bool {
	return x >= at.X-20 && x <= at.X+20 && y >= at.Y-20 && y <= at.Y+20
}

// TestTeleportSkillGoesToTeleCoords casts the shipped Escape - to X Town
// (2213) level 1, a TELEPORT skill whose level picks "-84200;244544;-3728"
// from its teleCoords table: at the cast's end (20 s hit time) the caster
// stands within 20 of those coordinates, whatever its restart point says.
func TestTeleportSkillGoesToTeleCoords(t *testing.T) {
	t.Parallel()
	def := shippedSkill(t, 2213, 1)
	srv, c, objID := bootRecallCaster(t, def, gameservertest.WithRestartPoints(recallTown(location.Location{X: -5000, Y: 2000, Z: 30})))
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
	drainUntilQuiet(t, c)
	if !srv.PlayerCastingNow(t, objID) {
		t.Fatal("2213 did not start a cast")
	}
	srv.Advance(t, 19*time.Second)
	want := location.Location{X: -84200, Y: 244544, Z: -3728}
	if x, y, _ := srv.PlayerPosition(t, objID); near(x, y, want) {
		t.Fatal("teleported before the cast's end")
	}
	srv.AdvanceUntil(t, "teleCoords teleport", func() bool {
		x, y, _ := srv.PlayerPosition(t, objID)
		return near(x, y, want)
	})
}

// TestCastleRecallWithoutCastleGoesToTown pins RestartPointData's fallback
// for a Castle recall (recallType Castle, no teleCoords) by a player whose
// clan owns no castle: it lands at its nearest town restart point.
func TestCastleRecallWithoutCastleGoesToTown(t *testing.T) {
	t.Parallel()
	town := location.Location{X: -5000, Y: 2000, Z: 30}
	def := modelskill.Definition{
		ID: 9301, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "RECALL", RecallType: modelskill.RecallCastle, HitTime: 500, StaticHitTime: true, MagicLevel: 1,
	}
	srv, c, objID := bootRecallCaster(t, def, gameservertest.WithRestartPoints(recallTown(town)))
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
	srv.AdvanceUntil(t, "castle recall to town", func() bool {
		x, y, _ := srv.PlayerPosition(t, objID)
		return near(x, y, town)
	})
}

// TestRecallRefusedInBossZone pins L2SkillTeleport's caster gate: a player
// casting a recall inside a boss zone finishes the cast and stays put.
func TestRecallRefusedInBossZone(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(-1000000, 1000000, -1000000, 1000000, -100000, 100000)
	if err != nil {
		t.Fatalf("build zone form: %v", err)
	}
	set := commons.NewStatSet()
	set.Set("InvadeTime", "600000")
	boss, err := zone.NewBoss(1, form, set)
	if err != nil {
		t.Fatalf("build boss zone: %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(boss)
	town := location.Location{X: -5000, Y: 2000, Z: 30}
	def := modelskill.Definition{
		ID: 9302, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "RECALL", HitTime: 500, StaticHitTime: true, MagicLevel: 1,
	}
	srv, c, objID := bootRecallCaster(t, def, gameservertest.WithZones(zones), gameservertest.WithRestartPoints(recallTown(town)))
	boss.AllowEntry(objID, time.Hour)
	startInWorld(t, c)
	x0, y0, _ := srv.PlayerPosition(t, objID)

	c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
	drainUntilQuiet(t, c)
	if !srv.PlayerCastingNow(t, objID) {
		t.Fatal("recall did not start a cast in the boss zone")
	}
	srv.AdvanceUntil(t, "boss-zone recall cast end", func() bool { return !srv.PlayerCastingNow(t, objID) })
	srv.Advance(t, time.Second)
	if x, y, _ := srv.PlayerPosition(t, objID); x != x0 || y != y0 {
		t.Fatalf("player moved from %d, %d to %d, %d; want a boss-zone recall refused", x0, y0, x, y)
	}
}
