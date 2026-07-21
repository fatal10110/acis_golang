package player

import (
	"sync"
	"testing"
	"time"
)

func TestCharacterUpdateShortBuffBroadcastsAndClearsAfterDuration(t *testing.T) {
	c := &Character{ID: 1}

	var mu sync.Mutex
	var updates []ShortBuffUpdate
	done := make(chan struct{}, 2)
	c.SetShortBuffBroadcaster(func(u ShortBuffUpdate) {
		mu.Lock()
		updates = append(updates, u)
		mu.Unlock()
		done <- struct{}{}
	})

	c.UpdateShortBuff(2031, 1, 1) // 1 second, short enough for a fast test
	<-done

	if got := c.ShortBuffTaskSkillID(); got != 2031 {
		t.Fatalf("ShortBuffTaskSkillID() = %d, want 2031", got)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the scheduled clear broadcast")
	}

	if got := c.ShortBuffTaskSkillID(); got != 0 {
		t.Fatalf("ShortBuffTaskSkillID() after clear = %d, want 0", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(updates) != 2 {
		t.Fatalf("broadcast calls = %d, want 2 (start + clear)", len(updates))
	}
	if updates[0] != (ShortBuffUpdate{SkillID: 2031, Level: 1, DurationSeconds: 1}) {
		t.Fatalf("start update = %+v, want {2031 1 1}", updates[0])
	}
	if updates[1] != (ShortBuffUpdate{}) {
		t.Fatalf("clear update = %+v, want zero value", updates[1])
	}
}

func TestCharacterUpdateShortBuffCancelsPreviousTimer(t *testing.T) {
	c := &Character{ID: 1}

	var mu sync.Mutex
	var updates []ShortBuffUpdate
	c.SetShortBuffBroadcaster(func(u ShortBuffUpdate) {
		mu.Lock()
		defer mu.Unlock()
		updates = append(updates, u)
	})

	c.UpdateShortBuff(2031, 1, 100) // long duration; must not fire before the restart below
	c.UpdateShortBuff(2037, 1, 100)

	if got := c.ShortBuffTaskSkillID(); got != 2037 {
		t.Fatalf("ShortBuffTaskSkillID() = %d, want 2037 (the restarted buff)", got)
	}

	// Give any (incorrectly still-running) first timer a moment it
	// shouldn't need, then check no clear fired for skill 2031.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(updates) != 2 {
		t.Fatalf("broadcast calls = %d, want 2 (two starts, no premature clear)", len(updates))
	}
}
