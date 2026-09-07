package npc

import (
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/npcstring"
)

const (
	// DefaultMaxGeoPathFailCount is the shipped MaxGeopathFailCount.
	DefaultMaxGeoPathFailCount = 50
	minMaxGeoPathFailCount     = 15
)

// maxGeoPathFailCount is the process-wide overflow threshold from
// geoengine.properties. Zero means "use DefaultMaxGeoPathFailCount".
var maxGeoPathFailCount atomic.Int32

// ClampMaxGeoPathFailCount applies the configured floor of 15.
func ClampMaxGeoPathFailCount(n int) int {
	if n < minMaxGeoPathFailCount {
		return minMaxGeoPathFailCount
	}
	return n
}

// SetMaxGeoPathFailCount records the process-wide overflow threshold.
// Tests may stub a value below the config floor.
func SetMaxGeoPathFailCount(n int) {
	maxGeoPathFailCount.Store(int32(n))
}

// MaxGeoPathFailCount reports the overflow threshold currently in force.
func MaxGeoPathFailCount() int {
	return int(currentMaxGeoPathFailCount())
}

func currentMaxGeoPathFailCount() int32 {
	if n := maxGeoPathFailCount.Load(); n > 0 {
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
	max := currentMaxGeoPathFailCount()
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
