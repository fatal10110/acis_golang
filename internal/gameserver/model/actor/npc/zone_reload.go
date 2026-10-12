package npc

import "github.com/fatal10110/acis_golang/internal/gameserver/model/zone"

// reload moves the NPC off held, the zones of a swapped-out index that
// hold it, and onto those its index now holds around it.
func (m *zoneMember) reload(held []zone.Kind) {
	if m != nil && m.ix != nil {
		m.Reload(m.ix, m, held)
	}
}

// ReloadZones moves the NPC off held, the zones of a reload's swapped-out
// index that hold it, and onto the reloaded zones around it.
func (h *Hostile) ReloadZones(held []zone.Kind) { h.zones.reload(held) }

// ReloadZones moves the NPC off held, the zones of a reload's swapped-out
// index that hold it, and onto the reloaded zones around it.
func (f *Folk) ReloadZones(held []zone.Kind) { f.zones.reload(held) }

// ReloadZones moves the point off held, the zones of a reload's swapped-out
// index that hold it, and onto the reloaded zones around it.
func (ep *EffectPoint) ReloadZones(held []zone.Kind) { ep.zones.reload(held) }

// ReloadZones moves the decoration off held, the zones of a reload's
// swapped-out index that hold it, and onto the reloaded zones around it.
func (d *Decoration) ReloadZones(held []zone.Kind) { d.zones.reload(held) }
