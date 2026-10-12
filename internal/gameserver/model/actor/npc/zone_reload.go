package npc

// reload moves the NPC onto the zones its index now holds around it.
func (m *zoneMember) reload() {
	if m != nil && m.ix != nil {
		m.Reload(m.ix, m)
	}
}

// ReloadZones moves the NPC off the zones a reload swapped out and onto the
// reloaded zones around it.
func (h *Hostile) ReloadZones() { h.zones.reload() }

// ReloadZones moves the NPC off the zones a reload swapped out and onto the
// reloaded zones around it.
func (f *Folk) ReloadZones() { f.zones.reload() }

// ReloadZones moves the point off the zones a reload swapped out and onto
// the reloaded zones around it.
func (ep *EffectPoint) ReloadZones() { ep.zones.reload() }

// ReloadZones moves the decoration off the zones a reload swapped out and
// onto the reloaded zones around it.
func (d *Decoration) ReloadZones() { d.zones.reload() }
