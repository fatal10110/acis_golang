package player

import "testing"

func TestCharacterToggleAutoSoulShotAppliesItemRules(t *testing.T) {
	c := &Character{}

	if status := c.ToggleAutoSoulShot(1463, true, true, false); status != AutoSoulShotToggled {
		t.Fatalf("ToggleAutoSoulShot regular status = %v, want toggled", status)
	}
	if !c.AutoSoulShotEnabled(1463) {
		t.Fatal("regular soulshot was not enabled")
	}
	if status := c.ToggleAutoSoulShot(1463, false, true, false); status != AutoSoulShotToggled {
		t.Fatalf("ToggleAutoSoulShot disable status = %v, want toggled", status)
	}
	if c.AutoSoulShotEnabled(1463) {
		t.Fatal("regular soulshot remained enabled after disable")
	}
	if status := c.ToggleAutoSoulShot(6535, true, true, false); status != AutoSoulShotNoop {
		t.Fatalf("ToggleAutoSoulShot fishing status = %v, want noop", status)
	}
	if c.AutoSoulShotEnabled(6535) {
		t.Fatal("fishing shot was enabled")
	}
	if status := c.ToggleAutoSoulShot(6645, true, true, false); status != AutoSoulShotNeedsSummon {
		t.Fatalf("ToggleAutoSoulShot summon status = %v, want needs summon", status)
	}
	if c.AutoSoulShotEnabled(6645) {
		t.Fatal("summon shot was enabled without a summon")
	}
	if status := c.ToggleAutoSoulShot(1463, true, false, true); status != AutoSoulShotNoop {
		t.Fatalf("ToggleAutoSoulShot missing item status = %v, want noop", status)
	}
}
