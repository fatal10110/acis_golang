package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// HostileSinks returns the event-sink factory for hostile NPCs spawned into
// state. stance tracks their attack stances; a nil stance leaves NPCs out
// of it.
func HostileSinks(state *world.State, stance AttackStanceTracker) func(*npc.Hostile) event.Sink {
	return func(h *npc.Hostile) event.Sink { return &hostileSink{world: state, stance: stance, h: h} }
}
