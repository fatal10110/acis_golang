// Package monsterracetrack is the town gatekeepers' trip to the Monster
// Race Track and the race manager's trip back. A gatekeeper moves the
// player to the track and remembers itself in the player's memo; the race
// manager moves a player with that memo back to its gatekeeper's town and
// forgets it. A player without the memo is not moved.
package monsterracetrack

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// raceManager is the track's race manager.
const raceManager = 30995

// memo is the player memo holding the gatekeeper the player came from.
const memo = "MonsterRaceTrack_Npc"

// returnLocations are the gatekeepers that send players to the track, each
// with the point the race manager sends their players back to.
func returnLocations() map[int32]script.Loc {
	return map[int32]script.Loc{
		30059: {X: 15670, Y: 142983, Z: -2705},  // TRISHA
		30080: {X: 83400, Y: 147943, Z: -3404},  // CLARISSA
		30177: {X: 82956, Y: 53162, Z: -1495},   // VALENTIA
		30233: {X: 116819, Y: 76994, Z: -2714},  // ESMERALDA
		30256: {X: -12672, Y: 122776, Z: -3116}, // BELLA
		30320: {X: -80826, Y: 149775, Z: -3043}, // RICHLIN
		30848: {X: 146331, Y: 25762, Z: -2018},  // ELISA
		30899: {X: 111409, Y: 219364, Z: -3545}, // FLAUEN
		31210: {X: 12882, Y: 181053, Z: -3560},  // RACE TRACK GK
		31275: {X: 147930, Y: -55281, Z: -2728}, // TATIANA
		31320: {X: 43835, Y: -47749, Z: -792},   // ILYANA
		31964: {X: 87386, Y: -143246, Z: -1293}, // BILIA
	}
}

// New returns the teleporter.
func New() script.Script {
	return script.Script{
		Dir: "teleport",
		Bind: script.Bindings{script.EventTalked: {
			raceManager, 30059, 30080, 30177, 30233, 30256, 30320, 30848, 30899, 31210, 31275, 31320, 31964,
		}},
		Hooks: script.Hooks{OnTalk: onTalk},
	}
}

// onTalk answers nothing. A memo naming a gatekeeper of no return point
// aborts the hook before the memo is forgotten.
func onTalk(_ *script.Script, e script.Talk) string {
	player := e.Player
	if e.NPC.NpcID() == raceManager {
		npcID := memoInteger(player, memo, -1)
		if npcID >= 0 {
			loc, ok := returnLocations()[npcID]
			if !ok {
				panic("monsterracetrack: no return point for gatekeeper " + strconv.Itoa(int(npcID)))
			}
			player.TeleportTo(loc, 0)
			player.UnsetMemo(memo)
		}
	} else if _, ok := returnLocations()[e.NPC.NpcID()]; ok {
		player.TeleportTo(script.Loc{X: 12661, Y: 181687, Z: -3560}, 0)
		player.SetMemo(memo, strconv.Itoa(int(e.NPC.NpcID())))
	}

	return ""
}

// memoInteger reads the player's memo key as a 32-bit number, def when it
// is not set; a value that is not one panics.
func memoInteger(p *script.Player, key string, def int32) int32 {
	v, ok := p.Memo(key)
	if !ok {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		panic(err)
	}
	return int32(n)
}
