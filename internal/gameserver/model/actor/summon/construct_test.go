package summon

import "testing"

func mustServitor(t testing.TB, cfg ServitorConfig) *Actor {
	t.Helper()
	a, err := NewServitor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func mustPet(t testing.TB, cfg PetConfig) *Actor {
	t.Helper()
	a, err := NewPet(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

type peaceZoneQueryStub bool

func (q peaceZoneQueryStub) EffectRangeInPeaceZone(_, _, _, _, _, _ int) bool { return bool(q) }

func TestSummonInPeaceZoneQueriesCurrentZone(t *testing.T) {
	pet := mustPet(t, PetConfig{ObjectID: 1})
	pet.SetZones(peaceZoneQueryStub(true))
	if !pet.InPeaceZone() {
		t.Fatal("InPeaceZone() = false for a summon inside a peace zone")
	}
}
