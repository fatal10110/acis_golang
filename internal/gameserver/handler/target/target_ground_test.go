package target

import (
	"slices"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestGroundHandlerTargetsCaster(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable}
	handler := mustHandler(t, NewRegistry(knownList{}), modelskill.TargetGround)

	if got := handler.FinalTarget(caster, nil, &modelskill.Definition{}); got != caster {
		t.Fatalf("ground final target = %v, want caster", got)
	}
	if got := ids(handler.Targets(caster, nil, &modelskill.Definition{})); !slices.Equal(got, []int32{1}) {
		t.Fatalf("ground targets = %v, want [1]", got)
	}
	if !handler.CanCast(caster, nil, &modelskill.Definition{}, false) {
		t.Fatal("ground CanCast = false, want true")
	}
}

// groundTargetActor is a Creature that also implements GroundTargeter, so
// groundHandler.CanCast exercises its LOS/peace-zone gate instead of the
// permissive fallback.
type groundTargetActor struct {
	targetActor
	gx, gy, gz int
	canSee     bool
	inPeace    bool
}

func (g *groundTargetActor) GroundTarget() (x, y, z int)  { return g.gx, g.gy, g.gz }
func (g *groundTargetActor) CanSeePoint(x, y, z int) bool { return g.canSee }
func (g *groundTargetActor) EffectRangeInPeaceZone(x, y, z, effectRange int) bool {
	return g.inPeace
}

func TestGroundHandlerCanCastGating(t *testing.T) {
	handler := mustHandler(t, NewRegistry(knownList{}), modelskill.TargetGround)
	skill := &modelskill.Definition{EffectRange: 30}

	blind := &groundTargetActor{targetActor: targetActor{id: 1}, canSee: false, inPeace: false}
	if handler.CanCast(blind, nil, skill, false) {
		t.Fatal("CanCast = true without line of sight to the ground point")
	}

	peaceful := &groundTargetActor{targetActor: targetActor{id: 2}, canSee: true, inPeace: true}
	if handler.CanCast(peaceful, nil, skill, false) {
		t.Fatal("CanCast = true with the ground point's effect range inside a peace zone")
	}

	clear := &groundTargetActor{targetActor: targetActor{id: 3}, canSee: true, inPeace: false}
	if !handler.CanCast(clear, nil, skill, false) {
		t.Fatal("CanCast = false with a visible, non-peace ground point")
	}
}
