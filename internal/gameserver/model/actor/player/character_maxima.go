package player

// SettleMaxima applies a change of c's HP, MP or CP maximum to the current
// value it bounds, as a read of the changed maximum does (noteMaxima), on
// c's queue. A current value above its new maximum drops to it. Once
// the client has been shown c's maxima (AdoptMaxima), each changed maximum
// whose current value differs from it is reported with its own status
// report, HP first; an HP report raises the low-HP tutorial event after it,
// as any HP write does. A dead character keeps its values and sends no
// report, but its HP maximum's change still owes the low-HP event. The
// regeneration task is then settled against the new maxima.
func (c *Character) SettleMaxima() {
	res := c.ResourceValues()
	c.vitalsMu.Lock()
	shown, dead := c.maximaShown, c.dead.Load()
	reports, hpNoted := 0, false
	if res.MaxHP != c.shownMaxHP {
		c.shownMaxHP = res.MaxHP
		if shown && c.curHP != res.MaxHP {
			hpNoted = true
			if dead {
				c.hpWritten.Store(true)
			} else {
				c.writeHPLocked(min(c.curHP, res.MaxHP))
				reports++
			}
		}
	}
	if res.MaxMP != c.shownMaxMP {
		c.shownMaxMP = res.MaxMP
		if shown && !dead && c.curMP != res.MaxMP {
			reports++
		}
	}
	if res.MaxCP != c.shownMaxCP {
		c.shownMaxCP = res.MaxCP
		if shown && !dead && c.curCP != res.MaxCP {
			reports++
		}
	}
	if !dead {
		if c.curHP > res.MaxHP {
			c.writeHPLocked(res.MaxHP)
		}
		c.curMP = min(c.curMP, res.MaxMP)
		c.curCP = min(c.curCP, res.MaxCP)
	}
	c.vitalsMu.Unlock()
	for range reports {
		c.BroadcastStatus()
	}
	switch {
	case reports > 0:
	case hpNoted:
		c.SettleRegen()
	default:
		// No HP write of its own to report: an HP write whose caller has
		// yet to report it keeps its low-HP event for that report.
		c.settleRegenTask()
	}
}

// AdoptMaxima records c's maxima as the ones its client is shown, as the
// EnterWorld burst's UserInfo shows them, dropping each current value above
// its maximum to it without a report. From then on SettleMaxima reports
// each change. It runs on c's queue.
func (c *Character) AdoptMaxima() {
	res := c.ResourceValues()
	c.vitalsMu.Lock()
	c.shownMaxHP, c.shownMaxMP, c.shownMaxCP = res.MaxHP, res.MaxMP, res.MaxCP
	c.maximaShown = true
	if !c.dead.Load() {
		if c.curHP > res.MaxHP {
			c.writeHPLocked(res.MaxHP)
		}
		c.curMP = min(c.curMP, res.MaxMP)
		c.curCP = min(c.curCP, res.MaxCP)
	}
	c.vitalsMu.Unlock()
	c.settleRegenTask()
}
