package manager

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// fakeHostileCastDefs is a minimal non-nil actorcast.Definitions: this test
// only needs newLiveHostile to take the castDefs != nil branch, not to
// resolve a real skill.
type fakeHostileCastDefs struct{}

func (fakeHostileCastDefs) Definition(modelskill.Ref) (modelskill.Definition, bool) {
	return modelskill.Definition{}, false
}

// TestNewLiveHostileWiresOnHitResultFromEffects pins the other half of issue
// #2350's fix: newLiveHostile must copy EffectHandlers.OnHitResult onto the
// AIController it installs, or a hostile NPC's MagicResist/ManaDrain
// messages silently stop reaching real online targets again. Reverting
// npcs_hostile.go's `OnHitResult: castEffects.OnHitResult` to the old unset
// form fails this test.
func TestNewLiveHostileWiresOnHitResultFromEffects(t *testing.T) {
	inst := &npc.Instance{
		ObjectID: 1,
		Template: &npc.Template{
			ID:          9001,
			Type:        "Monster",
			RunSpeed:    100,
			AIParams:    commons.NewStatSet(),
			NoSleepMode: true,
		},
	}

	state := world.New()
	positions := task.NewPositionUpdates(state)

	var got actorcast.EffectResult
	var calls int
	effects := actorcast.EffectHandlers{
		OnHitResult: func(result actorcast.EffectResult) {
			got = result
			calls++
		},
	}

	hostile, _, err := newLiveHostile(inst, 100, blockedHomeGeo{}, positions, zerolog.Nop(), fakeHostileCastDefs{}, effects, nil, 20, 0, nil, effect.Env{}, npcQueues().NewQueue("npc"))
	if err != nil {
		t.Fatalf("newLiveHostile() error: %v", err)
	}

	controller := hostile.AI().CastController()
	aiController, ok := controller.(*actorcast.AIController)
	if !ok || aiController == nil {
		t.Fatalf("CastController() = %T, want non-nil *actorcast.AIController", controller)
	}
	if aiController.OnHitResult == nil {
		t.Fatal("AIController.OnHitResult is nil, want it wired from EffectHandlers.OnHitResult")
	}

	aiController.OnHitResult(actorcast.EffectResult{AttackFailed: 1})
	if calls != 1 {
		t.Fatalf("OnHitResult calls = %d, want 1", calls)
	}
	if got.AttackFailed != 1 {
		t.Fatalf("OnHitResult AttackFailed = %d, want 1", got.AttackFailed)
	}
}

// TestNewLiveHostileLeavesOnHitResultUnsetWithoutCastDefs pins the existing
// nil-castDefs no-skills-to-cast contract: no AIController is installed at
// all, so there is nothing to wire an OnHitResult hook onto.
func TestNewLiveHostileLeavesOnHitResultUnsetWithoutCastDefs(t *testing.T) {
	inst := &npc.Instance{
		ObjectID: 2,
		Template: &npc.Template{
			ID:          9002,
			Type:        "Monster",
			RunSpeed:    100,
			AIParams:    commons.NewStatSet(),
			NoSleepMode: true,
		},
	}

	state := world.New()
	positions := task.NewPositionUpdates(state)

	hostile, _, err := newLiveHostile(inst, 100, blockedHomeGeo{}, positions, zerolog.Nop(), nil, actorcast.EffectHandlers{}, nil, 20, 0, nil, effect.Env{}, npcQueues().NewQueue("npc"))
	if err != nil {
		t.Fatalf("newLiveHostile() error: %v", err)
	}

	if controller := hostile.AI().CastController(); controller != nil {
		t.Fatalf("CastController() = %v, want nil with no castDefs", controller)
	}
}
