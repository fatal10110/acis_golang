package npc

// hpBarSize is the number of pixels in the client's target health bar.
const hpBarSize = 352.0

// hpBar tracks which health-bar segment an NPC's current HP was last
// reported in, so a watcher is only sent HP changes that move the bar.
type hpBar struct {
	interval, incCheck, decCheck float64
}

func newHPBar(maxHP float64) hpBar {
	interval := maxHP / hpBarSize
	return hpBar{interval: interval, incCheck: maxHP, decCheck: maxHP - interval}
}

// need reports whether hp must be sent, moving the tracked segment to hp's
// when it is. Near-dead HP and a bar shorter than one pixel per point
// always report.
func (b *hpBar) need(hp, maxHP float64) bool {
	if hp <= 1.0 || maxHP < hpBarSize {
		return true
	}
	if hp > b.decCheck && hp < b.incCheck {
		return false
	}
	if hp == maxHP {
		b.incCheck = hp + 1
		b.decCheck = hp - b.interval
	} else {
		b.decCheck = b.interval * float64(int(hp/b.interval))
		b.incCheck = b.decCheck + b.interval
	}
	return true
}

// HPStatusUpdate returns this NPC's current HP and whether the players
// targeting it must be sent it. Callers invoke it only when at least one
// player is targeting this NPC: an unwatched NPC's bar state stays where it
// was last reported.
func (h *Hostile) HPStatusUpdate() (int, bool) {
	h.hpBarMu.Lock()
	defer h.hpBarMu.Unlock()
	hp := h.health.Current()
	return int(hp), h.hpBar.need(hp, float64(h.MaxHP()))
}
