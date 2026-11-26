// Package bossrank ranks the three targets a grand boss keeps in its
// script memory: each holds points, raised by the hits and the sightings
// that rank a creature, and the creature itself.
package bossrank

import (
	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Points are the points a hit or a sighting ranks a creature with.
type Points struct {
	stored int32   // what a slot takes, before its random share
	exact  float64 // the unrounded points of a spell's damage
	scaled bool    // whether exact, not stored, is compared
}

// Int are points counted in whole numbers, as a plain hit or a sighting is.
func Int(v int32) Points { return Points{stored: v} }

// Scaled are the points scaled from a spell's damage, compared unrounded
// and stored truncated.
func Scaled(v float64) Points {
	return Points{stored: commons.TruncInt32Sat(v), exact: v, scaled: true}
}

// beats reports whether p beats a standing of cur by more than 1000: in
// whole numbers for a plain hit or a sighting, unrounded for a spell.
func (p Points) beats(cur int32) bool {
	if p.scaled {
		return p.exact+1000 > float64(cur)
	}
	return p.stored+1000 > cur
}

// Targets are the slots of a boss's three targets: their points, and who
// each is. Slot numbers 2 to 4 name them in that order.
type Targets struct {
	Points [3]script.IntSlot
	Who    [3]script.CreatureSlot
}

// Rank weighs c with p among n's three targets. A creature already ranked
// keeps its slot, raised to p (plus a random share) when p beats its
// standing; any other takes the weakest slot, a tie broken at random. It
// returns the slot it gave c, 2 to 4, or 0 when it gave none.
func (t Targets) Rank(s *script.Script, n *script.NPC, c script.Creature, p Points) int {
	q0, q1, q2 := n.Int(t.Points[0]), n.Int(t.Points[1]), n.Int(t.Points[2])
	var i1 int
	switch {
	case script.Same(c, n.Slot(t.Who[0])):
		if p.beats(q0) {
			n.SetInt(t.Points[0], p.stored+int32(s.Rnd(3000)))
		}
	case script.Same(c, n.Slot(t.Who[1])):
		if p.beats(q1) {
			n.SetInt(t.Points[1], p.stored+int32(s.Rnd(3000)))
		}
	case script.Same(c, n.Slot(t.Who[2])):
		if p.beats(q2) {
			n.SetInt(t.Points[2], p.stored+int32(s.Rnd(3000)))
		}
	case q0 > q1:
		i1 = 3
	case q0 == q1:
		i1 = pick(s, 2, 3)
	case q0 < q1:
		i1 = 2
	}
	return t.Settle(s, n, c, i1, p)
}

// Settle gives c slot i1 (2 to 4) with p, plus a random share, unless the
// third slot stands lower: then c takes that one, a tie broken at random.
// It returns the slot it gave c, or 0 when i1 names none. A hit that gave
// its attacker a new slot settles it once more, on the standings it just
// changed: that second pass may move it to another slot, so it can hold
// two.
func (t Targets) Settle(s *script.Script, n *script.NPC, c script.Creature, i1 int, p Points) int {
	q2 := n.Int(t.Points[2])
	switch i1 {
	case 2:
		switch q0 := n.Int(t.Points[0]); {
		case q0 > q2:
			i1 = 4
		case q0 == q2:
			i1 = pick(s, 2, 4)
		case q0 < q2:
			i1 = 2
		}
	case 3:
		switch q1 := n.Int(t.Points[1]); {
		case q1 > q2:
			i1 = 4
		case q1 == q2:
			i1 = pick(s, 3, 4)
		case q1 < q2:
			i1 = 3
		}
	}

	switch i1 {
	case 2, 3, 4:
		n.SetInt(t.Points[i1-2], p.stored+int32(s.Rnd(3000)))
		n.SetSlot(t.Who[i1-2], c)
		return i1
	}
	return 0
}

// pick returns a or b on an even draw.
func pick(s *script.Script, a, b int) int {
	if s.RndBool() {
		return a
	}
	return b
}
