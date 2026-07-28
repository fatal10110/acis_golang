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

func TestConsumeAICastItemLeavesControllerCastingOnSuccess(t *testing.T) {
	def := modelskill.Definition{ID: 9, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf}
	caster := newCastAICharacter(10)
	ctrl := startedCastAIController(t, caster, def)

	inv := itemcontainer.NewPlayerInventory(caster.ID, modelitem.NewTable(nil))
	inst := &modelitem.Instance{ObjectID: 20, TemplateID: 1}
	destroyer := &fakeDestroyer{}

	if consumed := ConsumeAICastItem(ConsumeAICastItemRequest{
		Controller: ctrl,
		Definition: def,
		Inventory:  inv,
		Item:       inst,
		Destroyer:  destroyer,
	}); consumed.Err != nil {
		t.Fatalf("ConsumeAICastItem() error: %v", consumed.Err)
	}
	if destroyer.calls != 1 {
		t.Fatalf("DestroyItem calls = %d, want 1", destroyer.calls)
	}
	// The caller drives the cast's Launch/Hit/Finish phases through
	// Controller.Schedule after consuming the item (network/item_skill_cast.go);
	// consuming must not itself clear the cast.
	if !ctrl.CastingNow() {
		t.Fatal("controller CastingNow() = false after a successful consume, want still casting")
	}
}

func TestConsumeAICastItemStopsControllerWhenItemMissing(t *testing.T) {
	def := modelskill.Definition{ID: 9, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf}
	caster := newCastAICharacter(10)
	ctrl := startedCastAIController(t, caster, def)

	inv := itemcontainer.NewPlayerInventory(caster.ID, modelitem.NewTable(nil))
	inst := &modelitem.Instance{ObjectID: 20, TemplateID: 1}
	destroyer := &fakeDestroyer{fail: true}

	consumed := ConsumeAICastItem(ConsumeAICastItemRequest{
		Controller: ctrl,
		Definition: def,
		Inventory:  inv,
		Item:       inst,
		Destroyer:  destroyer,
	})

	if !errors.Is(consumed.Err, actorcast.ErrNotEnoughItems) {
		t.Fatalf("ConsumeAICastItem() error = %v, want ErrNotEnoughItems", consumed.Err)
	}
	if ctrl.CastingNow() {
		t.Fatal("controller CastingNow() = true after a rejection, want stopped/cleared")
	}
}

func TestConsumeAICastItemReportsSharedReuseGroup(t *testing.T) {
	def := modelskill.Definition{ID: 9, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, ReuseDelay: 5000}
	inv := itemcontainer.NewPlayerInventory(10, modelitem.NewTable(nil))
	inst := &modelitem.Instance{ObjectID: 20, TemplateID: 1}
	destroyer := &fakeDestroyer{}

	t.Run("no group defined", func(t *testing.T) {
		caster := newCastAICharacter(10)
		ctrl := startedCastAIController(t, caster, def)
		tmpl := &modelitem.Template{ID: 1, EtcItem: &modelitem.EtcItemDetail{SharedReuseGroup: -1}}
		res := ConsumeAICastItem(ConsumeAICastItemRequest{
			Controller: ctrl, Definition: def, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer,
		})
		if res.Err != nil {
			t.Fatalf("ConsumeAICastItem() error: %v", res.Err)
		}
		if res.SharedReuseGroup != -1 {
			t.Fatalf("SharedReuseGroup = %d, want -1", res.SharedReuseGroup)
		}
	})

	t.Run("group defined, item reuse longer than skill's", func(t *testing.T) {
		caster := newCastAICharacter(11)
		ctrl := startedCastAIController(t, caster, def)
		tmpl := &modelitem.Template{ID: 1, EtcItem: &modelitem.EtcItemDetail{SharedReuseGroup: 3, ReuseDelay: 8000}}
		res := ConsumeAICastItem(ConsumeAICastItemRequest{
			Controller: ctrl, Definition: def, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer,
		})
		if res.Err != nil {
			t.Fatalf("ConsumeAICastItem() error: %v", res.Err)
		}
		if res.SharedReuseGroup != 3 {
			t.Fatalf("SharedReuseGroup = %d, want 3", res.SharedReuseGroup)
		}
		if res.ReuseMillis != 8000 {
			t.Fatalf("ReuseMillis = %d, want 8000 (item's reuse, longer than the skill's 5000)", res.ReuseMillis)
		}
	})
}
