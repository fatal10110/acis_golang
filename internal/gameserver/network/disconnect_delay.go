package network

import (
	"context"
	"time"
)

const (
	// disconnectDelay is how long a player whose connection was lost stays
	// in the world before it detaches.
	disconnectDelay = 100 * time.Millisecond
	// disconnectCombatDelay replaces disconnectDelay for a player in attack
	// stance or in the middle of a class change: it stays attackable,
	// visible and in the fight that long.
	disconnectCombatDelay = 15 * time.Second
)

// detachDelay is how long leaving stays in the world after its session
// ended. In combat means in attack stance, the flag the stance tracker keeps. Only a lost connection (dropped by the client, or gone idle) waits;
// a logout, a restart, and every end the server decides (a kick, an account
// takeover, a handler panic, a malformed packet, a flood) detach at once.
func detachDelay(leaving *livePlayer, lost bool) time.Duration {
	if !lost {
		return 0
	}
	if leaving.InCombat() || leaving.ClassChangeLocked() {
		return disconnectCombatDelay
	}
	return disconnectDelay
}

// awaitDetachDelay keeps leaving in the world for d, timed on its own queue's
// clock. An eviction of session (a kick, an account takeover or a duplicate
// selection of the character) or the server stopping cuts the wait short.
func awaitDetachDelay(ctx context.Context, session *Session, leaving *livePlayer, d time.Duration) {
	if d <= 0 {
		return
	}
	due := make(chan struct{})
	timer := leaving.after(d, func() { close(due) })
	defer timer.Stop()
	select {
	case <-due:
	case <-session.evicted:
	case <-ctx.Done():
	}
}
