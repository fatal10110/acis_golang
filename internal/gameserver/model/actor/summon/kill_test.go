package summon

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// killPetExp is the level-79 wolf's experience: well inside level 79, so one
// death penalty neither empties it nor takes the level down.
const killPetExp = wolfExpLevel79 + 10_000_000

// killPetPenalty is the level-79 wolf's death penalty:
// round((555934039 - 480562077) * (6.5 - 0.07*79) / 100) = round(731108.03).
const killPetPenalty int64 = 731108

// newKillPet returns a live, owned level-79 wolf pet with a recorder
// attached, so its death sequence runs in full: the owner's kill hooks,
// DeathSettled and the death penalty.
func newKillPet(t *testing.T) (*Actor, *event.Recorder) {
	t.Helper()
	growth := wolfTopGrowth()
	row := growth.Levels[79]
	pet := mustPet(t, PetConfig{
		ObjectID: 7,
		NPCID:    12077,
		Owner:    &fakeSummonOwner{id: 1},
		Level:    79,
		Exp:      killPetExp,
		Growth:   growth,
		Stats:    CombatStats{MaxHP: row.MaxHP, MaxMP: row.MaxMP},
	})
	rec := &event.Recorder{}
	pet.Attach(Runtime{Sink: rec})
	return pet, rec
}

// assertOneDeath checks that pet's death sequence ran exactly once: one Die
// broadcast, one DeathSettled, one death penalty off its experience, and the
// pre-death experience kept for a resurrection.
func assertOneDeath(t *testing.T, pet *Actor, rec *event.Recorder) {
	t.Helper()
	if !pet.Dead() || pet.HP() != 0 {
		t.Fatalf("pet dead=%v hp=%v, want dead at 0 HP", pet.Dead(), pet.HP())
	}
	if n := event.Count[event.Died](rec); n != 1 {
		t.Fatalf("Died events = %d, want 1", n)
	}
	if n := event.Count[event.DeathSettled](rec); n != 1 {
		t.Fatalf("DeathSettled events = %d, want 1", n)
	}
	if got := pet.Exp(); got != killPetExp-killPetPenalty {
		t.Fatalf("pet exp = %d, want one penalty off: %d", got, killPetExp-killPetPenalty)
	}
	pet.statusMu.Lock()
	before := pet.expBeforeDeath
	pet.statusMu.Unlock()
	if before != killPetExp {
		t.Fatalf("expBeforeDeath = %d, want the pre-death %d", before, killPetExp)
	}
}

// TestKillOnPetAlreadyKilledByHit: a pet a lethal hit already killed is
// not killed again. Kill reports false, and the death sequence, the death
// penalty and the kept pre-death experience stay those of the one death.
func TestKillOnPetAlreadyKilledByHit(t *testing.T) {
	pet, rec := newKillPet(t)
	pet.ReduceHP(pet.HP()+100, nil, modelskill.Definition{})
	assertOneDeath(t, pet, rec)

	if pet.Kill(nil) {
		t.Fatal("Kill() = true on a dead pet, want false")
	}
	assertOneDeath(t, pet, rec)
}

// TestKillRacingLethalHitKillsOnce starts Kill and a lethal ReduceHP
// together on a live pet. Both mark the pet dead under the same lock, so
// exactly one of them runs the death sequence: one death, one penalty.
// Run under -race.
func TestKillRacingLethalHitKillsOnce(t *testing.T) {
	for range 200 {
		pet, rec := newKillPet(t)
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			pet.Kill(nil)
		}()
		go func() {
			defer wg.Done()
			<-start
			pet.ReduceHP(pet.MaxHPValue()+100, nil, modelskill.Definition{})
		}()
		close(start)
		wg.Wait()
		assertOneDeath(t, pet, rec)
	}
}
