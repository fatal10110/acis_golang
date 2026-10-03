package player

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
)

// partyBars tracks which segment of the CP, HP and MP gauges of the
// character's party-window row its party last saw.
type partyBars struct {
	calibrate  sync.Once
	cp, hp, mp creature.HPBar
}

// PartyWindowStale reports whether a vitals change must refresh the
// character's row in its party's windows. The CP and HP gauges are checked,
// and their segments advanced, on every call, in a party or not; the MP
// gauge only for a party member whose CP and HP both stayed in their
// segments.
//
// The gauges are sized once, from the level-1 base maxima of the class the
// character first plays: segments finer than a point, so nearly every whole
// change refreshes, while a full gauge that stays full does not.
func (c *Character) PartyWindowStale(inParty bool) bool {
	_, partyRow := c.VitalsGaugesStale(inParty)
	return partyRow
}

// VitalsGaugesStale is PartyWindowStale, also reporting whether the CP or
// HP gauge left its segment, which refreshes the character in its duel
// opponents' window.
func (c *Character) VitalsGaugesStale(inParty bool) (cpOrHP, partyRow bool) {
	b := &c.partyBars
	b.calibrate.Do(func() {
		tmpl := c.template()
		if tmpl == nil || len(tmpl.HPTable) == 0 || len(tmpl.MPTable) == 0 || len(tmpl.CPTable) == 0 {
			return
		}
		b.cp.Calibrate(tmpl.CPTable[0])
		b.hp.Calibrate(tmpl.HPTable[0])
		b.mp.Calibrate(tmpl.MPTable[0])
	})
	res := c.ResourceValues()
	_, cp := b.cp.Report(func() float64 { return res.CurrentCP }, res.MaxCP)
	_, hp := b.hp.Report(func() float64 { return res.CurrentHP }, res.MaxHP)
	if !inParty || cp || hp {
		return cp || hp, inParty
	}
	_, mp := b.mp.Report(func() float64 { return res.CurrentMP }, res.MaxMP)
	return false, mp
}
