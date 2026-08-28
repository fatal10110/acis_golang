package npc

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// GeoPathFailCount reports how many consecutive route-walk moves this NPC
// failed to path toward, for task.Walker's teleport-to-start recovery.
func (h *Hostile) GeoPathFailCount() int {
	return int(h.geoPathFailCount.Load())
}

// ResetGeoPathFailCount clears the route-walk path-failure streak.
func (h *Hostile) ResetGeoPathFailCount() {
	h.geoPathFailCount.Store(0)
}

// AddGeoPathFailCount records one more failed route-walk path attempt.
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

// SayNPCString would broadcast a walkerRoutes.xml node's fstring chat line
// (aCis Npc.broadcastNpcSay(NpcStringId)). It is a no-op: the reference
// resolves the id through NpcStringId.java, a ~12,300-line id-to-text table
// that isn't ported yet (issue #2028) — broadcasting without it would show
// players fabricated text, which is worse than staying silent.
func (h *Hostile) SayNPCString(id int) {}
