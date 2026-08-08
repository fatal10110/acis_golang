package summon

import "testing"

func TestActorPAtkSpdHungryHalvesBase(t *testing.T) {
	hungry := NewPet(PetConfig{ObjectID: 1, Level: 10, MaxMeal: 100, Fed: 10, HungryLimit: 0.3, Roll: zeroSummonRoll})
	fed := NewPet(PetConfig{ObjectID: 2, Level: 10, MaxMeal: 100, Fed: 100, HungryLimit: 0.3, Roll: zeroSummonRoll})

	hungryValue := hungry.PAtkSpd(300)
	fedValue := fed.PAtkSpd(300)
	if hungryValue*2 != fedValue {
		t.Fatalf("PAtkSpd(300) hungry=%v fed=%v, want hungry == fed/2 (base halved while under-fed)", hungryValue, fedValue)
	}
}

func TestActorMAtkSpdHungryHalvesBase(t *testing.T) {
	hungry := NewPet(PetConfig{ObjectID: 1, Level: 10, MaxMeal: 100, Fed: 10, HungryLimit: 0.3, Roll: zeroSummonRoll})
	fed := NewPet(PetConfig{ObjectID: 2, Level: 10, MaxMeal: 100, Fed: 100, HungryLimit: 0.3, Roll: zeroSummonRoll})

	hungryValue := hungry.MAtkSpd()
	fedValue := fed.MAtkSpd()
	if hungryValue*2 != fedValue {
		t.Fatalf("MAtkSpd() hungry=%v fed=%v, want hungry == fed/2 (base halved while under-fed)", hungryValue, fedValue)
	}
}

func TestActorCriticalRateCapsAt500(t *testing.T) {
	pet := NewPet(PetConfig{ObjectID: 1, Level: 10, Roll: zeroSummonRoll})
	if got := pet.CriticalRate(600); got != 500 {
		t.Fatalf("CriticalRate(600) = %v, want capped at 500", got)
	}
}

func TestActorMAtkSpdServitorNeverHalved(t *testing.T) {
	// A servitor has no feeding state at all (isPet is false), so its
	// magic attack speed must equal a well-fed pet's, never a hungry one's.
	servitor := NewServitor(ServitorConfig{ObjectID: 1, Level: 10, Roll: zeroSummonRoll})
	fed := NewPet(PetConfig{ObjectID: 2, Level: 10, MaxMeal: 100, Fed: 100, HungryLimit: 0.3, Roll: zeroSummonRoll})
	if servitor.MAtkSpd() != fed.MAtkSpd() {
		t.Fatalf("MAtkSpd() servitor=%v, want unhalved (matching a well-fed pet) %v", servitor.MAtkSpd(), fed.MAtkSpd())
	}
}
