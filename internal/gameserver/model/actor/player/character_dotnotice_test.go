package player

import "testing"

func TestNotifyEffectRemovedDueLackHPAndMP(t *testing.T) {
	c, err := NewCharacter(1, humanFighterTemplate(), "acct", "dot", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() error: %v", err)
	}

	hpNotices, mpNotices, relaxNotices := 0, 0, 0
	c.SetLackHPNotifier(func() { hpNotices++ })
	c.SetLackMPNotifier(func() { mpNotices++ })
	c.SetRelaxHPFullNotifier(func() { relaxNotices++ })

	c.NotifyEffectRemovedDueLackHP(nil)
	c.NotifyEffectRemovedDueLackMP(nil)
	c.NotifyRelaxDeactivatedHPFull(nil)
	if hpNotices != 1 {
		t.Fatalf("hp notices = %d, want 1", hpNotices)
	}
	if mpNotices != 1 {
		t.Fatalf("mp notices = %d, want 1", mpNotices)
	}
	if relaxNotices != 1 {
		t.Fatalf("relax notices = %d, want 1", relaxNotices)
	}

	// Unwiring the hook (as lifecycle detach does) must not panic.
	c.SetLackHPNotifier(nil)
	c.SetLackMPNotifier(nil)
	c.SetRelaxHPFullNotifier(nil)
	c.NotifyEffectRemovedDueLackHP(nil)
	c.NotifyEffectRemovedDueLackMP(nil)
}
