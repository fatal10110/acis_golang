package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// HitObserver watches the hits a hostile NPC registers.
type HitObserver interface {
	// Hit sees one live hit by attacker, on the attacker's queue, before
	// the hit adds hate or takes HP.
	Hit(attacker attackable.Combatant)
}

// BroadcastOnScreen shows text in the middle of its observers' screens for
// durationMs milliseconds.
func (h *Hostile) BroadcastOnScreen(durationMs int32, text string) {
	h.emit(event.OnScreenMessage{Text: text, DurationMs: durationMs})
}
