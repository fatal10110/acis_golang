package target

import (
	"math"
	"slices"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestRegistryRegistersRepresentativeHandlers(t *testing.T) {
	registry := NewRegistry(knownList{})

	for _, typ := range []modelskill.Target{
		modelskill.TargetSelf,
		modelskill.TargetOne,
		modelskill.TargetArea,
		modelskill.TargetFrontArea,
		modelskill.TargetAura,
		modelskill.TargetFrontAura,
		modelskill.TargetBehindAura,
		modelskill.TargetSummon,
		modelskill.TargetAreaSummon,
		modelskill.TargetOwnerPet,
	} {
		if _, ok := registry.Handler(typ); !ok {
			t.Fatalf("Handler(%s) missing", typ)
		}
	}

	if _, ok := registry.Handler(modelskill.TargetParty); ok {
		t.Fatal("Handler(PARTY) registered before party targeting is ported")
	}
}

func TestSelfAndOneHandlers(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable}
	target := &targetActor{id: 2, category: CategoryAttackable}
	registry := NewRegistry(knownList{})
	skill := &modelskill.Definition{Radius: 100}

	self := mustHandler(t, registry, modelskill.TargetSelf)
	if got := self.FinalTarget(caster, target, skill); got != caster {
		t.Fatalf("self final target = %v, want caster", got)
	}
	if got := ids(self.Targets(caster, target, skill)); !slices.Equal(got, []int32{1}) {
		t.Fatalf("self targets = %v, want [1]", got)
	}

	one := mustHandler(t, registry, modelskill.TargetOne)
	if got := one.FinalTarget(caster, target, skill); got != target {
		t.Fatalf("one final target = %v, want aimed target", got)
	}
	if got := ids(one.Targets(caster, target, skill)); !slices.Equal(got, []int32{2}) {
		t.Fatalf("one targets = %v, want [2]", got)
	}
	if one.CanCast(caster, nil, skill, false) {
		t.Fatal("one CanCast with nil target = true, want false")
	}
}

func TestAreaTargetsAnchorOnAimedTarget(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable, x: 0, y: 0, attackableWithoutForce: true}
	aimed := &targetActor{id: 2, category: CategoryAttackable, x: 100, y: 0, attackableWithoutForce: true}
	near := &targetActor{id: 3, category: CategoryAttackable, x: 150, y: 0, attackableWithoutForce: true}
	dead := &targetActor{id: 4, category: CategoryAttackable, x: 120, y: 0, dead: true, attackableWithoutForce: true}
	blocked := &targetActor{id: 5, category: CategoryAttackable, x: 130, y: 0, attackableWithoutForce: true}
	far := &targetActor{id: 6, category: CategoryAttackable, x: 260, y: 0, attackableWithoutForce: true}
	aimed.see = map[int32]bool{5: false}

	registry := NewRegistry(knownList{caster, aimed, near, dead, blocked, far})
	area := mustHandler(t, registry, modelskill.TargetArea)

	got := ids(area.Targets(caster, aimed, &modelskill.Definition{Radius: 100}))
	if want := []int32{2, 3}; !slices.Equal(got, want) {
		t.Fatalf("area targets = %v, want %v", got, want)
	}

	if final := area.FinalTarget(caster, caster, &modelskill.Definition{}); final != nil {
		t.Fatalf("area final target on self = %v, want nil", final)
	}
}

func TestAuraTargetsFilterBySightAndAttackability(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable}
	attackable := &targetActor{id: 2, category: CategoryAttackable, x: 80, attackableWithoutForce: true}
	playable := &targetActor{id: 3, category: CategoryPlayable, x: 90, attackableWithoutForce: true}
	passive := &targetActor{id: 4, category: CategoryAttackable, x: 70}
	dead := &targetActor{id: 5, category: CategoryAttackable, x: 60, dead: true, attackableWithoutForce: true}
	blocked := &targetActor{id: 6, category: CategoryAttackable, x: 50, attackableWithoutForce: true}
	caster.see = map[int32]bool{6: false}

	registry := NewRegistry(knownList{caster, attackable, playable, passive, dead, blocked})
	aura := mustHandler(t, registry, modelskill.TargetAura)

	got := ids(aura.Targets(caster, nil, &modelskill.Definition{Radius: 100}))
	if want := []int32{2, 3}; !slices.Equal(got, want) {
		t.Fatalf("aura targets = %v, want %v", got, want)
	}
}

func TestFrontAndBehindAurasUseCasterHeading(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable, heading: 0}
	front := &targetActor{id: 2, category: CategoryAttackable, x: 80, attackableWithoutForce: true}
	behind := &targetActor{id: 3, category: CategoryAttackable, x: -80, attackableWithoutForce: true}
	side := &targetActor{id: 4, category: CategoryAttackable, y: 80, attackableWithoutForce: true}

	registry := NewRegistry(knownList{caster, front, behind, side})

	gotFront := ids(mustHandler(t, registry, modelskill.TargetFrontAura).Targets(caster, nil, &modelskill.Definition{Radius: 100}))
	if want := []int32{2}; !slices.Equal(gotFront, want) {
		t.Fatalf("front aura targets = %v, want %v", gotFront, want)
	}

	gotBehind := ids(mustHandler(t, registry, modelskill.TargetBehindAura).Targets(caster, nil, &modelskill.Definition{Radius: 100}))
	if want := []int32{3}; !slices.Equal(gotBehind, want) {
		t.Fatalf("behind aura targets = %v, want %v", gotBehind, want)
	}
}

func TestFrontAreaKeepsAimedTargetAndFiltersSplashByCasterHeading(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable, heading: 0}
	aimed := &targetActor{id: 2, category: CategoryAttackable, x: 100, attackableWithoutForce: true}
	front := &targetActor{id: 3, category: CategoryAttackable, x: 130, attackableWithoutForce: true}
	behind := &targetActor{id: 4, category: CategoryAttackable, x: -10, attackableWithoutForce: true}

	registry := NewRegistry(knownList{caster, aimed, front, behind})
	frontArea := mustHandler(t, registry, modelskill.TargetFrontArea)

	got := ids(frontArea.Targets(caster, aimed, &modelskill.Definition{Radius: 100}))
	if want := []int32{2, 3}; !slices.Equal(got, want) {
		t.Fatalf("front area targets = %v, want %v", got, want)
	}
}

func TestSummonTargetsCasterSummon(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable}
	summon := &targetActor{id: 2, category: CategoryPlayable}
	caster.summon = summon

	handler := mustHandler(t, NewRegistry(knownList{caster, summon}), modelskill.TargetSummon)

	if got := handler.FinalTarget(caster, nil, &modelskill.Definition{}); got != summon {
		t.Fatalf("summon final target = %v, want summon", got)
	}
	if got := ids(handler.Targets(caster, nil, &modelskill.Definition{})); !slices.Equal(got, []int32{2}) {
		t.Fatalf("summon targets = %v, want [2]", got)
	}
	if !handler.CanCast(caster, nil, &modelskill.Definition{}, false) {
		t.Fatal("summon CanCast with live summon = false, want true")
	}

	summon.dead = true
	if handler.CanCast(caster, nil, &modelskill.Definition{}, false) {
		t.Fatal("summon CanCast with dead summon = true, want false")
	}
	caster.summon = nil
	if got := handler.FinalTarget(caster, nil, &modelskill.Definition{}); got != nil {
		t.Fatalf("summon final target without summon = %v, want nil", got)
	}
}

func TestOwnerPetTargetsSummonOwner(t *testing.T) {
	owner := &targetActor{id: 1, category: CategoryPlayable}
	summon := &targetActor{id: 2, category: CategoryPlayable, owner: owner}
	other := &targetActor{id: 3, category: CategoryPlayable}

	handler := mustHandler(t, NewRegistry(knownList{owner, summon, other}), modelskill.TargetOwnerPet)

	if got := handler.FinalTarget(summon, nil, &modelskill.Definition{}); got != owner {
		t.Fatalf("owner pet final target = %v, want owner", got)
	}
	if got := ids(handler.Targets(summon, nil, &modelskill.Definition{})); !slices.Equal(got, []int32{1}) {
		t.Fatalf("owner pet targets = %v, want [1]", got)
	}
	if !handler.CanCast(summon, owner, &modelskill.Definition{}, false) {
		t.Fatal("owner pet CanCast on owner = false, want true")
	}
	if handler.CanCast(summon, other, &modelskill.Definition{}, false) {
		t.Fatal("owner pet CanCast on another target = true, want false")
	}

	owner.dead = true
	if handler.CanCast(summon, owner, &modelskill.Definition{}, false) {
		t.Fatal("owner pet CanCast on dead owner = true, want false")
	}
}

func TestAreaSummonUsesSummonAsAnchor(t *testing.T) {
	caster := &targetActor{id: 1, category: CategoryPlayable, x: 0, attackableWithoutForce: true}
	summon := &targetActor{id: 2, category: CategoryPlayable, x: 100}
	near := &targetActor{id: 3, category: CategoryAttackable, x: 130, attackableWithoutForce: true}
	dead := &targetActor{id: 4, category: CategoryAttackable, x: 120, dead: true, attackableWithoutForce: true}
	blocked := &targetActor{id: 5, category: CategoryAttackable, x: 110, attackableWithoutForce: true}
	passive := &targetActor{id: 6, category: CategoryAttackable, x: 115}
	far := &targetActor{id: 7, category: CategoryAttackable, x: 300, attackableWithoutForce: true}
	caster.summon = summon
	summon.see = map[int32]bool{5: false}

	handler := mustHandler(t, NewRegistry(knownList{caster, summon, near, dead, blocked, passive, far}), modelskill.TargetAreaSummon)

	if got := handler.FinalTarget(caster, nil, &modelskill.Definition{}); got != summon {
		t.Fatalf("area summon final target = %v, want summon", got)
	}
	got := ids(handler.Targets(caster, summon, &modelskill.Definition{Radius: 50}))
	if want := []int32{3}; !slices.Equal(got, want) {
		t.Fatalf("area summon targets = %v, want %v", got, want)
	}
}

func mustHandler(t *testing.T, r *Registry, target modelskill.Target) Handler {
	t.Helper()
	h, ok := r.Handler(target)
	if !ok {
		t.Fatalf("Handler(%s) missing", target)
	}
	return h
}

func ids(creatures []Creature) []int32 {
	out := make([]int32, 0, len(creatures))
	for _, creature := range creatures {
		out = append(out, creature.ObjectID())
	}
	return out
}

type targetActor struct {
	id       int32
	x, y, z  int
	heading  int
	dead     bool
	category Category

	see                    map[int32]bool
	attackableBy           bool
	attackableWithoutForce bool
	summon                 Creature
	owner                  Creature
}

func (a *targetActor) ObjectID() int32 { return a.id }

func (a *targetActor) Position() (int, int, int) { return a.x, a.y, a.z }

func (a *targetActor) Heading() int { return a.heading }

func (a *targetActor) Dead() bool { return a.dead }

func (a *targetActor) Category() Category { return a.category }

func (a *targetActor) CanSeeTarget(target Creature) bool {
	if a.see == nil {
		return true
	}
	visible, ok := a.see[target.ObjectID()]
	return !ok || visible
}

func (a *targetActor) AttackableBy(Creature) bool { return a.attackableBy }

func (a *targetActor) AttackableWithoutForceBy(Creature) bool { return a.attackableWithoutForce }

func (a *targetActor) Summon() (Creature, bool) { return a.summon, a.summon != nil }

func (a *targetActor) Owner() (Creature, bool) { return a.owner, a.owner != nil }

type knownList []*targetActor

func (k knownList) ForEachKnownCreatureInRadius(anchor Creature, radius int, fn func(Creature)) {
	ax, ay, az := anchor.Position()
	for _, actor := range k {
		if actor.ObjectID() == anchor.ObjectID() {
			continue
		}
		if radius != -1 {
			dx := float64(actor.x - ax)
			dy := float64(actor.y - ay)
			dz := float64(actor.z - az)
			if math.Sqrt(dx*dx+dy*dy+dz*dz) > float64(radius) {
				continue
			}
		}
		fn(actor)
	}
}
