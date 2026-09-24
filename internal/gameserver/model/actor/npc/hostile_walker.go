package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/npcstring"
)

const (
	// DefaultMaxGeoPathFailCount is the shipped MaxGeopathFailCount.
	DefaultMaxGeoPathFailCount = 50
	minMaxGeoPathFailCount     = 15
)

// ClampMaxGeoPathFailCount applies the configured floor of 15.
func ClampMaxGeoPathFailCount(n int) int {
	if n < minMaxGeoPathFailCount {
		return minMaxGeoPathFailCount
	}
	return n
}

// SetMaxGeoPathFailCount sets h's pathfinding-fail overflow threshold from
// geoengine.properties; zero keeps DefaultMaxGeoPathFailCount. Tests may
// stub a value below the config floor.
func (h *Hostile) SetMaxGeoPathFailCount(n int) {
	h.maxGeoPathFailCount.Store(int32(n))
}

func (h *Hostile) currentMaxGeoPathFailCount() int32 {
	if n := h.maxGeoPathFailCount.Load(); n > 0 {
		return n
	}
	return DefaultMaxGeoPathFailCount
}

// GeoPathFailCount reports how many consecutive pathfinding moves this NPC
// failed to resolve, for walker teleport-to-start and SiegeGuard
// return-home recovery.
func (h *Hostile) GeoPathFailCount() int {
	return int(h.geoPathFailCount.Load())
}

// ResetGeoPathFailCount clears the pathfinding-failure streak.
func (h *Hostile) ResetGeoPathFailCount() {
	h.geoPathFailCount.Store(0)
}

// AddGeoPathFailCount records one more failed pathfinding attempt.
// Sequence at the cap: MAX, then MAX+1, then the next fail zeros without
// incrementing so script AI thresholds can drop for a cycle.
func (h *Hostile) AddGeoPathFailCount() {
	max := h.currentMaxGeoPathFailCount()
	for {
		cur := h.geoPathFailCount.Load()
		if cur > max {
			if h.geoPathFailCount.CompareAndSwap(cur, 0) {
				x, y, z := h.Position()
				h.log.Warn().
					Str("npc", h.CharacterName()).
					Int("x", x).
					Int("y", y).
					Int("z", z).
					Int("heading", h.Heading()).
					Msg("geopath fail overflow")
				return
			}
			continue
		}
		if h.geoPathFailCount.CompareAndSwap(cur, cur+1) {
			return
		}
	}
}

// SocialAction broadcasts a social-animation packet, driven by a
// walkerRoutes.xml node's socialId (aCis NpcAI.onEvtArrived).
func (h *Hostile) SocialAction(id int) {
	h.emit(event.SocialAction{ID: int32(id)})
}

// SayNPCString broadcasts a walkerRoutes.xml node's fstring chat line
// (aCis Npc.broadcastNpcSay(NpcStringId), resolved via NpcStringId.getMessage()).
// An unmapped id is a no-op: the reference has no such gap, but staying
// silent beats fabricating text the client would show as this NPC's line.
func (h *Hostile) SayNPCString(id int) {
	text, ok := npcstring.Text(int32(id))
	if !ok {
		return
	}
	h.emit(event.NpcSay{NpcID: h.Instance.Template.TemplateID, Text: text})
}
