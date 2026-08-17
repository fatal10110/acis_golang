package cast

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// permissiveGeo is a test-only move.Geo that permits every move, needed
// only because creature.NewLive requires a non-nil Geo.
type permissiveGeo struct{}

func (permissiveGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool { return true }
func (permissiveGeo) Height(x, y, z int) int16                { return int16(z) }
func (permissiveGeo) FindPath(origin, target location.Location) ([]location.Location, bool) {
	return nil, false
}
func (permissiveGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}

func TestPlayerActorResourcesAndInventory(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 57, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	ch := &player.Character{ID: 1}
	ch.SetResourceValues(player.Resources{MaxHP: 12, CurrentHP: 12, MaxMP: 7, CurrentMP: 7})
	inv := itemcontainer.NewPlayerInventory(ch.ID, templates)
	inv.AddNew(57, 5, 100)
	ch.AttachRuntime(&player.Template{}, inv)

	actor := PlayerActor{Character: ch}
	actor.ReduceMP(9)
	actor.ReduceHP(20)

	resources := ch.ResourceValues()
	if resources.CurrentMP != 0 || resources.CurrentHP != 0 {
		t.Fatalf("resources = hp %.0f mp %.0f, want both clamped to 0", resources.CurrentHP, resources.CurrentMP)
	}
	if got := actor.ItemCount(57); got != 5 {
		t.Fatalf("ItemCount() = %d, want 5", got)
	}
	if !actor.ConsumeItem(57, 3) {
		t.Fatalf("ConsumeItem() = false, want true")
	}
	if got := actor.ItemCount(57); got != 2 {
		t.Fatalf("ItemCount() after consume = %d, want 2", got)
	}
}

// TestPlayerActorMPCostAppliesDanceSurcharge covers Java's
// CreatureStatus.getMpConsume: a dance/song skill's MP cost grows by
// def.NextDanceCost for each dance/song already active on the caster, and
// a non-dance skill never picks up the surcharge.
func TestPlayerActorMPCostAppliesDanceSurcharge(t *testing.T) {
	ch := &player.Character{ID: 1}
	live, err := creature.NewLive(location.Location{}, 100, permissiveGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	ch.Live = live
	actor := PlayerActor{Character: ch}

	dance := modelskill.Definition{Dance: true, MPConsume: 10, NextDanceCost: 4}
	if got := actor.MPCost(dance); got != 10 {
		t.Fatalf("MPCost() with no active dances = %d, want 10", got)
	}

	ch.EffectList().Add(&effect.Effect{Skill: effect.Skill{ID: 1, Dance: true, Toggle: true}, Type: effect.TypeBuff})
	ch.EffectList().Add(&effect.Effect{Skill: effect.Skill{ID: 2, Dance: true, Toggle: true}, Type: effect.TypeBuff})

	if got := actor.MPCost(dance); got != 18 {
		t.Fatalf("MPCost() with 2 active dances = %d, want 18 (10 + 2*4)", got)
	}

	nonDance := modelskill.Definition{MPConsume: 10, NextDanceCost: 4}
	if got := actor.MPCost(nonDance); got != 10 {
		t.Fatalf("MPCost() for non-dance skill = %d, want 10, unaffected by active dances", got)
	}
}

func TestPlayerActorMPCostAppliesSkillMPConsumeRates(t *testing.T) {
	ch := &player.Character{ID: 1}
	live, err := creature.NewLive(location.Location{}, 100, permissiveGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	ch.Live = live
	owner := effect.ModOwnerSkill(modelskill.Ref{ID: 1, Level: 1})
	ch.AddStatFuncs([]effect.Mod{
		{Stat: stat.MagicalMpConsumeRate, Op: effect.OpMul, Value: 0.9, Owner: owner},
		{Stat: stat.PhysicalMpConsumeRate, Op: effect.OpMul, Value: 3, Owner: owner},
		{Stat: stat.DanceMpConsumeRate, Op: effect.OpMul, Value: 2, Owner: owner},
	})
	actor := PlayerActor{Character: ch}

	for _, tt := range []struct {
		name string
		def  modelskill.Definition
		init int
		cost int
	}{
		{name: "magic", def: modelskill.Definition{Magic: true, MPInitialConsume: 11, MPConsume: 21}, init: 9, cost: 18},
		{name: "physical", def: modelskill.Definition{MPInitialConsume: 11, MPConsume: 21}, init: 33, cost: 63},
		{name: "dance takes precedence", def: modelskill.Definition{Dance: true, Magic: true, MPInitialConsume: 11, MPConsume: 21}, init: 22, cost: 42},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := actor.MPInitialCost(tt.def); got != tt.init {
				t.Fatalf("MPInitialCost() = %d, want %d", got, tt.init)
			}
			if got := actor.MPCost(tt.def); got != tt.cost {
				t.Fatalf("MPCost() = %d, want %d", got, tt.cost)
			}
		})
	}
}

// TestPlayerActorAllSkillsDisabledReflectsCrowdControl covers Java's
// Creature.isAllSkillsDisabled(): a live crowd-control state (here, Stun)
// blocks casting through the same allSkillsDisabler seam Controller.CanCast
// and Controller.Stop both probe, and EnableAllSkills stays a no-op since
// this port doesn't model the raw Duel-defeat lock.
func TestPlayerActorAllSkillsDisabledReflectsCrowdControl(t *testing.T) {
	ch := &player.Character{ID: 1}
	live, err := creature.NewLive(location.Location{}, 100, permissiveGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	ch.Live = live
	actor := PlayerActor{Character: ch}

	if actor.AllSkillsDisabled() {
		t.Fatal("AllSkillsDisabled() = true before any lock is active")
	}

	e := &effect.Effect{Skill: effect.Skill{ID: 1}, Type: effect.TypeBuff, Flag: effect.FlagStunned}
	ch.EffectList().Add(e)
	if !actor.AllSkillsDisabled() {
		t.Fatal("AllSkillsDisabled() = false while stunned, want true")
	}

	actor.EnableAllSkills()
	if !actor.AllSkillsDisabled() {
		t.Fatal("AllSkillsDisabled() = false after EnableAllSkills, want still true: it only clears the unmodeled raw Duel lock, not crowd control")
	}

	ch.EffectList().Remove(e)
	if actor.AllSkillsDisabled() {
		t.Fatal("AllSkillsDisabled() = true after the stun effect was removed")
	}
}

func TestPlayerActorSkillReuseDelegatesToCharacter(t *testing.T) {
	ch := &player.Character{}
	actor := PlayerActor{Character: ch}
	ref := modelskill.Ref{ID: 10, Level: 2}
	key := int32(10*256 + 2)

	actor.AddSkillReuse(ref, key, time.Minute)

	if !actor.SkillDisabled(key) {
		t.Fatalf("SkillDisabled() = false, want true")
	}
}

func TestPlayerActorResourceAccessIsRaceFree(t *testing.T) {
	ch := &player.Character{
		ID: 1,
	}
	ch.SetResourceValues(player.Resources{MaxHP: 100000, CurrentHP: 100000, MaxMP: 100000, CurrentMP: 100000})
	ch.AttachRuntime(&player.Template{}, nil)
	actor := PlayerActor{Character: ch}

	const iterations = 1000
	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			ch.TakeDamage(1, nil)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			actor.ReduceHP(1)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			actor.ReduceMP(1)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = actor.HP()
			_ = actor.MP()
			_ = ch.CurrentHP()
			_ = ch.CurrentMP()
		}
	}()

	wg.Wait()

	if got := ch.CurrentHP(); got <= 0 {
		t.Fatalf("CurrentHP() = %d, want still alive", got)
	}
	if got := ch.CurrentMP(); got <= 0 {
		t.Fatalf("CurrentMP() = %d, want MP remaining", got)
	}
}
