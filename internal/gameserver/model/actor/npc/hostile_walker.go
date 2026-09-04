package npc

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/npcstring"
)

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
func (h *Hostile) AddGeoPathFailCount() {
	h.geoPathFailCount.Add(1)
}

// SocialAction broadcasts a social-animation packet, driven by a
// walkerRoutes.xml node's socialId (aCis NpcAI.onEvtArrived).
func (h *Hostile) SocialAction(id int) {
	if h.frames == nil {
		return
	}
	_ = h.broadcastFrame(func() wire.Frame { return h.frames.SocialAction(h.ObjectID(), int32(id)) })
}

// SayNPCString broadcasts a walkerRoutes.xml node's fstring chat line
// (aCis Npc.broadcastNpcSay(NpcStringId), resolved via NpcStringId.getMessage()).
// An unmapped id is a no-op: the reference has no such gap, but staying
// silent beats fabricating text the client would show as this NPC's line.
func (h *Hostile) SayNPCString(id int) {
	text, ok := npcstring.Text(int32(id))
	if !ok || h.frames == nil {
		return
	}
	npcID := h.Instance.Template.TemplateID
	_ = h.broadcastFrame(func() wire.Frame { return h.frames.NpcSay(h.ObjectID(), npcID, text) })
}
