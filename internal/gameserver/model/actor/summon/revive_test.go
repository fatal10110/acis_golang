package summon

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// reviveFixture is a dead summon in the world beside its owner, with a
// pending corpse decay, whose queue runs only when the test runs loop.
type reviveFixture struct {
	summon *Actor
	state  *world.State
	rec    *event.Recorder
	loop   *sim.Inline
}

func newDeadSummon(t *testing.T, pet bool) reviveFixture {
	t.Helper()
	owner := &fakeSummonOwner{id: 1}
	var a *Actor
	if pet {
		a = mustPet(t, PetConfig{ObjectID: 7, Owner: owner})
	} else {
		a = mustServitor(t, ServitorConfig{ObjectID: 7, Owner: owner})
	}
	loop := sim.NewInline(time.Unix(0, 0))
	a.SetQueue(loop.NewQueue("owner"))
	rec := &event.Recorder{}
	a.Attach(Runtime{Sink: rec})
	state := world.New()
	SpawnBesideOwner(state, a, owner, location.Location{})
	a.RestoreDead()
	a.SetCorpseDeadline(time.Unix(0, 0).Add(a.DecayDelay()))
	if !a.Dead() || !a.HasCorpse() {
		t.Fatal("fixture summon is not a dead corpse with a pending decay")
	}
	return reviveFixture{summon: a, state: state, rec: rec, loop: loop}
}

func (f reviveFixture) assertStaysDead(t *testing.T) {
	t.Helper()
	if f.summon.Revive() {
		t.Fatal("Revive() = true on a claimed corpse, want false")
	}
	if f.summon.ReviveRestoringExp(100) {
		t.Fatal("ReviveRestoringExp(100) = true on a claimed corpse, want false")
	}
	if !f.summon.Dead() {
		t.Fatal("claimed pet corpse stood back up")
	}
	if n := event.Count[event.Revived](f.rec); n != 0 {
		t.Fatalf("Revived events = %d, want 0", n)
	}
}

// TestPetClaimedCorpseRefusesRevive: once its decay has claimed a dead
// pet's corpse, and before the corpse has left the world, no revive stands
// it up, so the owner never loses the collar of a living pet.
func TestPetClaimedCorpseRefusesRevive(t *testing.T) {
	f := newDeadSummon(t, true)
	if !f.summon.claimCorpse() {
		t.Fatal("claimCorpse() = false on a dead pet")
	}
	if f.summon.claimCorpse() {
		t.Fatal("claimCorpse() = true twice, want one claim")
	}
	f.assertStaysDead(t)
}

// TestPetDecayedCorpseRefusesRevive: a pet its decay removed stays dead.
func TestPetDecayedCorpseRefusesRevive(t *testing.T) {
	f := newDeadSummon(t, true)
	if !f.summon.Decay(f.state, nil) {
		t.Fatal("Decay() = false for a dead pet's corpse")
	}
	if n := event.Count[event.PetCorpseDecayed](f.rec); n != 1 {
		t.Fatalf("PetCorpseDecayed events = %d, want 1", n)
	}
	f.assertStaysDead(t)
}

// TestPetRevivedCorpseIsNotClaimed: a revived pet's late decay leaves it be.
func TestPetRevivedCorpseIsNotClaimed(t *testing.T) {
	f := newDeadSummon(t, true)
	if !f.summon.Revive() {
		t.Fatal("Revive() = false for a dead pet")
	}
	if f.summon.Decay(f.state, nil) {
		t.Fatal("Decay() = true for a revived pet")
	}
	if n := event.Count[event.PetCorpseDecayed](f.rec); n != 0 {
		t.Fatalf("PetCorpseDecayed events = %d, want 0", n)
	}
}

// TestResurrectOutrightRunsOnSummonQueueAndCancelsDecayFirst: a non-player
// caster's resurrection runs on the summon's own queue, dropping the
// corpse's decay before the revive, so the revived servitor is not removed
// at its old deadline.
func TestResurrectOutrightRunsOnSummonQueueAndCancelsDecayFirst(t *testing.T) {
	for _, pet := range []bool{false, true} {
		f := newDeadSummon(t, pet)
		f.summon.ResurrectOutright(50)
		if !f.summon.Dead() || len(f.rec.Events()) != 0 {
			t.Fatalf("pet=%v: revive ran on the caller's goroutine, want it on the summon's queue", pet)
		}
		f.loop.Run()
		if f.summon.Dead() {
			t.Fatalf("pet=%v: summon still dead after its queue ran", pet)
		}
		cancel, revived := -1, -1
		for i, e := range f.rec.Events() {
			switch e.(type) {
			case event.DecayCanceled:
				if cancel < 0 {
					cancel = i
				}
			case event.Revived:
				revived = i
			}
		}
		if cancel < 0 || revived < 0 || cancel > revived {
			t.Fatalf("pet=%v: DecayCanceled at %d, Revived at %d; want the cancel before the revive", pet, cancel, revived)
		}
	}
}

// TestResurrectOutrightKeepsLeftBehindCorpseDecay: a corpse its owner left
// behind cannot be revived, so it keeps the decay that removes it.
func TestResurrectOutrightKeepsLeftBehindCorpseDecay(t *testing.T) {
	for _, pet := range []bool{false, true} {
		f := newDeadSummon(t, pet)
		f.summon.LeaveWithOwner()
		if !f.summon.OwnerLeft() {
			t.Fatalf("pet=%v: fixture corpse not left behind", pet)
		}
		f.summon.ResurrectOutright(100)
		f.loop.Run()
		if !f.summon.Dead() {
			t.Fatalf("pet=%v: left-behind corpse revived", pet)
		}
		if n := event.Count[event.DecayCanceled](f.rec); n != 0 {
			t.Fatalf("pet=%v: DecayCanceled events = %d, want 0", pet, n)
		}
	}
}
