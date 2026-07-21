package item

import (
	"errors"
	"testing"
	"time"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func newCastAICharacter(id int32) *player.Character {
	ch := &player.Character{ID: id}
	ch.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: 100, MaxMP: 100, CurrentMP: 100})
	return ch
}

func startedCastAIController(t *testing.T, caster *player.Character, def modelskill.Definition) *actorcast.Controller {
	t.Helper()
	ctrl := actorcast.NewController(actorcast.PlayerActor{Character: caster})
	if _, err := ctrl.Start(time.Unix(1000, 0), caster, def); err != nil {
		t.Fatalf("setup Start() error: %v", err)
	}
	return ctrl
}

func TestConsumeAndCompleteAICastAppliesEffects(t *testing.T) {
	def := modelskill.Definition{ID: 9, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf}
	caster := newCastAICharacter(10)
	ctrl := startedCastAIController(t, caster, def)

	inv := itemcontainer.NewPlayerInventory(caster.ID, modelitem.NewTable(nil))
	inst := &modelitem.Instance{ObjectID: 20, TemplateID: 1}
	destroyer := &fakeDestroyer{}

	if err := ConsumeAICastItem(ConsumeAICastItemRequest{
		Controller: ctrl,
		Inventory:  inv,
		Item:       inst,
		Destroyer:  destroyer,
	}); err != nil {
		t.Fatalf("ConsumeAICastItem() error: %v", err)
	}
	if destroyer.calls != 1 {
		t.Fatalf("DestroyItem calls = %d, want 1", destroyer.calls)
	}
	if !ctrl.CastingNow() {
		t.Fatal("controller CastingNow() = false after a successful consume, want still casting")
	}

	result := CompleteAICast(CompleteAICastRequest{
		Controller: ctrl,
		Definition: def,
		Caster:     caster,
		Target:     caster,
		Effects:    actorcast.EffectHandlers{},
	})
	if result.Err != nil {
		t.Fatalf("CompleteAICast() error: %v", result.Err)
	}
	if ctrl.CastingNow() {
		t.Fatal("controller CastingNow() = true after Finish, want cleared")
	}
}

func TestConsumeAICastItemStopsControllerWhenItemMissing(t *testing.T) {
	def := modelskill.Definition{ID: 9, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf}
	caster := newCastAICharacter(10)
	ctrl := startedCastAIController(t, caster, def)

	inv := itemcontainer.NewPlayerInventory(caster.ID, modelitem.NewTable(nil))
	inst := &modelitem.Instance{ObjectID: 20, TemplateID: 1}
	destroyer := &fakeDestroyer{fail: true}

	err := ConsumeAICastItem(ConsumeAICastItemRequest{
		Controller: ctrl,
		Inventory:  inv,
		Item:       inst,
		Destroyer:  destroyer,
	})

	if !errors.Is(err, actorcast.ErrNotEnoughItems) {
		t.Fatalf("ConsumeAICastItem() error = %v, want ErrNotEnoughItems", err)
	}
	if ctrl.CastingNow() {
		t.Fatal("controller CastingNow() = true after a rejection, want stopped/cleared")
	}
}

func TestCompleteAICastStopsControllerOnHitFailure(t *testing.T) {
	def := modelskill.Definition{ID: 9, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, MPConsume: 50}
	caster := newCastAICharacter(10)
	ctrl := startedCastAIController(t, caster, def)

	// Drain MP below the hit-phase cost after Start (which already
	// validated the cost against the caster's MP at that time), so Hit()
	// itself is what fails here, not Start().
	caster.ReduceCurrentMP(caster.CurrentMP())

	result := CompleteAICast(CompleteAICastRequest{
		Controller: ctrl,
		Definition: def,
		Caster:     caster,
		Target:     caster,
		Effects:    actorcast.EffectHandlers{},
	})

	if result.Err == nil {
		t.Fatal("CompleteAICast() error = nil, want a hit-cost failure (MP cost exceeds current MP)")
	}
	if ctrl.CastingNow() {
		t.Fatal("controller CastingNow() = true after a hit failure, want stopped/cleared")
	}
}
