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
