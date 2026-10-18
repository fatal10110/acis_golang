// Package randomteleport is the NPCs that move from place to place every
// half hour: Master Toma, the Merchant and the Blacksmith of Mammon, and
// Rooney, the Blacksmith of Wind. The two of Mammon never stay where they
// were, and the Blacksmith of Mammon shouts as he appears and as he moves.
package randomteleport

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The NPCs that move.
const (
	masterToma             = 30556
	merchantOfMammon       = 31113
	blacksmithOfMammon     = 31126
	blacksmithOfWindRooney = 32049
)

// movePeriod is how often each NPC moves, the first time that long after it
// appears.
const movePeriod = 1800000 * time.Millisecond

// The timers each NPC moves on.
const (
	tomaTimer   = "toma"
	momTimer    = "mom"
	bomTimer    = "bom"
	rooneyTimer = "rooney"
)

// bomShouts are what the Blacksmith of Mammon shouts.
func bomShouts() []int32 { return []int32{1000431, 1000432, 1000433} }

// tomaLocs are where Master Toma moves to.
func tomaLocs() []script.Loc {
	return []script.Loc{
		{X: 151572, Y: -174829, Z: -1781},
		{X: 154132, Y: -220070, Z: -3404},
		{X: 178849, Y: -184342, Z: -342},
	}
}

// momLocs are where the Merchant of Mammon moves to.
func momLocs() []script.Loc {
	return []script.Loc{
		{X: 118336, Y: 132592, Z: -4829}, // Necropolis of Martyrdom
		{X: 111296, Y: 173792, Z: -5440}, // Necropolis of Worship
		{X: 83136, Y: 208992, Z: -5437},  // The Saints Necropolis
		{X: 45020, Y: 123830, Z: -5408},  // The Pilgrims Necropolis
		{X: -21680, Y: 77152, Z: -5171},  // The Patriots Necropolis
		{X: -52208, Y: 78896, Z: -4739},  // Necropolis of Devotion
		{X: -41856, Y: 209904, Z: -5088}, // Necropolis of Sacrifice
		{X: 172384, Y: -17823, Z: -4897}, // The Disciple Necropolis
	}
}

// bomLocs are where the Blacksmith of Mammon moves to.
func bomLocs() []script.Loc {
	return []script.Loc{
		{X: 12629, Y: -248700, Z: -9584},  // Catacomb of Forbidden Path
		{X: -20533, Y: -251012, Z: -8161}, // Catacomb of the Apostate
		{X: -53188, Y: -250502, Z: -7905}, // Catacomb of the Heretic
		{X: 46288, Y: 170096, Z: -4979},   // Catacomb of the Branded
		{X: -19378, Y: 13264, Z: -4899},   // Catacomb of Dark Omens
		{X: 140480, Y: 79472, Z: -5427},   // Catacomb of the Witch
	}
}

// rooneyLocs are where Rooney moves to.
func rooneyLocs() []script.Loc {
	return []script.Loc{
		{X: 175937, Y: -112167, Z: -5550},
		{X: 178896, Y: -112425, Z: -5860},
		{X: 180628, Y: -115992, Z: -6135},
		{X: 183010, Y: -114753, Z: -6135},
		{X: 184496, Y: -116773, Z: -6135},
		{X: 181857, Y: -109491, Z: -5865},
		{X: 178917, Y: -107633, Z: -5853},
		{X: 178804, Y: -110080, Z: -5853},
		{X: 182221, Y: -106806, Z: -6025},
		{X: 186488, Y: -109715, Z: -5915},
		{X: 183847, Y: -119231, Z: -3113},
		{X: 185193, Y: -120342, Z: -3113},
		{X: 188047, Y: -120867, Z: -3113},
		{X: 189734, Y: -120471, Z: -3113},
		{X: 188754, Y: -118940, Z: -3313},
		{X: 190022, Y: -116803, Z: -3313},
		{X: 188443, Y: -115814, Z: -3313},
		{X: 186421, Y: -114614, Z: -3313},
		{X: 185188, Y: -113307, Z: -3313},
		{X: 187378, Y: -112946, Z: -3313},
		{X: 189815, Y: -113425, Z: -3313},
		{X: 189301, Y: -111327, Z: -3313},
		{X: 190289, Y: -109176, Z: -3313},
		{X: 187783, Y: -110478, Z: -3313},
		{X: 185889, Y: -109990, Z: -3313},
		{X: 181881, Y: -109060, Z: -3695},
		{X: 183570, Y: -111344, Z: -3675},
		{X: 182077, Y: -112567, Z: -3695},
		{X: 180127, Y: -112776, Z: -3698},
		{X: 179155, Y: -108629, Z: -3695},
		{X: 176282, Y: -109510, Z: -3698},
		{X: 176071, Y: -113163, Z: -3515},
		{X: 179376, Y: -117056, Z: -3640},
		{X: 179760, Y: -115385, Z: -3640},
		{X: 177950, Y: -119691, Z: -4140},
		{X: 177037, Y: -120820, Z: -4340},
		{X: 181125, Y: -120148, Z: -3702},
		{X: 182212, Y: -117969, Z: -3352},
		{X: 186074, Y: -118154, Z: -3312},
	}
}

// lastPlaces are the indexes, in momLocs and bomLocs, of where the two of
// Mammon last moved to; both start at the first place.
type lastPlaces struct {
	mom, bom *script.IntCell
}

// New returns the script.
func New() script.Script {
	last := lastPlaces{mom: &script.IntCell{}, bom: &script.IntCell{}}
	return script.Script{
		Dir:   "ai/group",
		Bind:  script.Bindings{script.EventCreated: {masterToma, merchantOfMammon, blacksmithOfMammon, blacksmithOfWindRooney}},
		Hooks: script.Hooks{OnTimer: last.onTimer, OnCreated: onCreated},
	}
}

// onTimer moves the NPC to a random place of its list. The two of Mammon
// draw again until the place differs from where they last moved to, and
// the Blacksmith of Mammon then shouts one of his lines.
func (last lastPlaces) onTimer(s *script.Script, e script.Timer) string {
	switch {
	case strings.EqualFold(e.Name, tomaTimer):
		locs := tomaLocs()
		e.NPC.TeleportTo(locs[s.Rnd(len(locs))])
	case strings.EqualFold(e.Name, momTimer):
		e.NPC.TeleportTo(momLocs()[moveFrom(s, last.mom, len(momLocs()))])
	case strings.EqualFold(e.Name, bomTimer):
		e.NPC.TeleportTo(bomLocs()[moveFrom(s, last.bom, len(bomLocs()))])

		shouts := bomShouts()
		e.NPC.Shout(script.NpcString(shouts[s.Rnd(len(shouts))]))
	case strings.EqualFold(e.Name, rooneyTimer):
		locs := rooneyLocs()
		e.NPC.TeleportTo(locs[s.Rnd(len(locs))])
	}
	return ""
}

// moveFrom draws an index below n other than the one last holds, keeps it in
// last and returns it.
func moveFrom(s *script.Script, last *script.IntCell, n int) int {
	i := s.Rnd(n)
	for i == last.Get() {
		i = s.Rnd(n)
	}
	last.Set(i)
	return i
}

// onCreated starts the NPC's half-hourly move; the Blacksmith of Mammon
// first shouts one of his lines.
func onCreated(s *script.Script, e script.Created) {
	switch e.NPC.NpcID() {
	case masterToma:
		s.StartTimerAtFixedRate(tomaTimer, e.NPC, nil, movePeriod, movePeriod)
	case merchantOfMammon:
		s.StartTimerAtFixedRate(momTimer, e.NPC, nil, movePeriod, movePeriod)
	case blacksmithOfMammon:
		shouts := bomShouts()
		e.NPC.Shout(script.NpcString(shouts[s.Rnd(len(shouts))]))
		s.StartTimerAtFixedRate(bomTimer, e.NPC, nil, movePeriod, movePeriod)
	case blacksmithOfWindRooney:
		s.StartTimerAtFixedRate(rooneyTimer, e.NPC, nil, movePeriod, movePeriod)
	}

	var base script.Hooks
	base.Created(s, e)
}
