package summon

import (
	"sync"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestClaimDepartureRefusesDeadPet: a pet already dead cannot be claimed
// for a mount (#5059).
func TestClaimDepartureRefusesDeadPet(t *testing.T) {
	pet, _ := newKillPet(t)
	if !pet.Kill(nil) {
		t.Fatal("Kill() = false on a live pet")
	}
	if pet.ClaimDeparture() {
		t.Fatal("ClaimDeparture() = true on a dead pet")
	}
}

// TestClaimedDepartureCannotDie: a pet claimed for a mount survives a Kill
// and a lethal hit until the claim is cancelled; then it dies as before
// (#5059).
func TestClaimedDepartureCannotDie(t *testing.T) {
	pet, rec := newKillPet(t)
	if !pet.ClaimDeparture() {
		t.Fatal("ClaimDeparture() = false on a live pet")
	}
	hp := pet.HP()
	if pet.Kill(nil) {
		t.Fatal("Kill() = true on a departing pet")
	}
	pet.ReduceHP(hp+100, nil, modelskill.Definition{})
	if pet.Dead() || pet.HP() != hp {
		t.Fatalf("departing pet dead=%v hp=%v, want alive at %v", pet.Dead(), pet.HP(), hp)
	}
	pet.CancelDeparture()
	if !pet.Kill(nil) {
		t.Fatal("Kill() = false once the departure was cancelled")
	}
	assertOneDeath(t, pet, rec)
}

// TestClaimDepartureRacingKill starts ClaimDeparture and Kill together on a
// live pet: exactly one wins, and a claimed pet is never dead. Run under
// -race.
func TestClaimDepartureRacingKill(t *testing.T) {
	for range 200 {
		pet, _ := newKillPet(t)
		start := make(chan struct{})
		var claimed, killed bool
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			claimed = pet.ClaimDeparture()
		}()
		go func() {
			defer wg.Done()
			<-start
			killed = pet.Kill(nil)
		}()
		close(start)
		wg.Wait()
		if claimed == killed {
			t.Fatalf("claimed=%v killed=%v, want exactly one", claimed, killed)
		}
		if claimed && pet.Dead() {
			t.Fatal("claimed pet is dead")
		}
	}
}
