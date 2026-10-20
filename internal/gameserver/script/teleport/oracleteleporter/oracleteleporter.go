// Package oracleteleporter is the Seven Signs oracle teleports. The town
// priests send a player into the festival's necropolis, remembering
// where it came from; the ziggurat gatekeepers send a player with a
// dimension fragment, for adena by level, to the Dimensional Rift; the
// temple priests send it back or into the festival.
package oracleteleporter

import (
	"slices"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// memo is the player memo holding the NPC the player came from.
const memo = "OracleTeleport_Npc"

// msgTooManyQuests is the system message of a player at its quest limit.
const msgTooManyQuests = 401

// The ziggurat gatekeepers' items.
const (
	adena             = 57
	dimensionFragment = 7079
)

// townDawn are the Dawn priests of the towns.
func townDawn() []int32 {
	return []int32{
		31078,
		31079,
		31080,
		31081,
		31082,
		31083,
		31084,
		31692,
		31694,
		31997,
		31168,
	}
}

// townDusk are the Dusk priests of the towns.
func townDusk() []int32 {
	return []int32{
		31085,
		31086,
		31087,
		31088,
		31089,
		31090,
		31091,
		31693,
		31695,
		31998,
		31169,
	}
}

// templePriest are the priests of the oracles' temples.
func templePriest() []int32 {
	return []int32{
		31127,
		31128,
		31129,
		31130,
		31131,
		31137,
		31138,
		31139,
		31140,
		31141,
	}
}

// returnLocations are the NPCs a player can come from, each with the
// point the return sends it back to.
func returnLocations() map[int32]script.Loc {
	return map[int32]script.Loc{
		// Priest of Dusk/Dawn
		31078: {X: -80555, Y: 150337, Z: -3040},
		31079: {X: -13953, Y: 121404, Z: -2984},
		31080: {X: 16354, Y: 142820, Z: -2696},
		31081: {X: 83369, Y: 149253, Z: -3400},
		31082: {X: 111386, Y: 220858, Z: -3544},
		31083: {X: 83106, Y: 53965, Z: -1488},
		31084: {X: 146983, Y: 26595, Z: -2200},
		31085: {X: -82368, Y: 151568, Z: -3120},
		31086: {X: -14748, Y: 123995, Z: -3112},
		31087: {X: 18482, Y: 144576, Z: -3056},
		31088: {X: 81623, Y: 148556, Z: -3464},
		31089: {X: 112486, Y: 220123, Z: -3592},
		31090: {X: 82819, Y: 54607, Z: -1520},
		31091: {X: 147570, Y: 28877, Z: -2264},

		// Gatekeeper Ziggurat
		31095: {X: -41561, Y: 209225, Z: -5087},
		31096: {X: 45242, Y: 124466, Z: -5413},
		31097: {X: 110711, Y: 174010, Z: -5439},
		31098: {X: -22341, Y: 77375, Z: -5173},
		31099: {X: -52889, Y: 79098, Z: -4741},
		31100: {X: 117760, Y: 132794, Z: -4831},
		31101: {X: 171792, Y: -17609, Z: -4901},
		31102: {X: 82564, Y: 209207, Z: -5439},
		31103: {X: -41565, Y: 210048, Z: -5085},
		31104: {X: 45278, Y: 123608, Z: -5411},
		31105: {X: 111510, Y: 174013, Z: -5437},
		31106: {X: -21489, Y: 77372, Z: -5171},
		31107: {X: -52016, Y: 79103, Z: -4739},
		31108: {X: 118557, Y: 132804, Z: -4829},
		31109: {X: 172570, Y: -17605, Z: -4899},
		31110: {X: 83347, Y: 209215, Z: -5437},

		// Gatekeeper Ziggurat
		31114: {X: 42495, Y: 143944, Z: -5381},
		31115: {X: 45666, Y: 170300, Z: -4981},
		31116: {X: 77138, Y: 78389, Z: -5125},
		31117: {X: 139903, Y: 79674, Z: -5429},
		31118: {X: -20021, Y: 13499, Z: -4901},
		31119: {X: 113418, Y: 84535, Z: -6541},
		31120: {X: -52940, Y: -250272, Z: -7907},
		31121: {X: 46499, Y: 170301, Z: -4979},
		31122: {X: -20280, Y: -250785, Z: -8163},
		31123: {X: 140673, Y: 79680, Z: -5437},
		31124: {X: -19182, Y: 13503, Z: -4899},
		31125: {X: 12837, Y: -248483, Z: -9579},

		// Priest of Dusk/Dawn
		31168: {X: 115136, Y: 74717, Z: -2608},
		31169: {X: 116642, Y: 77510, Z: -2688},

		// Priest of Dusk/Dawn
		31692: {X: 148256, Y: -55454, Z: -2779},
		31693: {X: 149888, Y: -56574, Z: -2979},
		31694: {X: 45664, Y: -50318, Z: -800},
		31695: {X: 44528, Y: -48370, Z: -800},

		// Priest of Dusk/Dawn
		31997: {X: 86795, Y: -143078, Z: -1341},
		31998: {X: 85129, Y: -142103, Z: -1542},
	}
}

// dawnFestival is the Dawn side's necropolis of the festival.
func dawnFestival() script.Loc { return script.Loc{X: -80157, Y: 111344, Z: -4901} }

// duskFestival is the Dusk side's necropolis of the festival.
func duskFestival() script.Loc { return script.Loc{X: -81261, Y: 86531, Z: -5157} }

// riftEntrance is where the Dimensional Rift is entered.
func riftEntrance() script.Loc { return script.Loc{X: -114755, Y: -179466, Z: -6752} }

// riftRoom is the Dimensional Rift's waiting room.
func riftRoom() script.Loc { return script.Loc{X: -114796, Y: -179334, Z: -6752} }

// New returns the teleporter.
func New() script.Script {
	talked := templePriest()
	for id := range returnLocations() {
		talked = append(talked, id)
	}
	slices.Sort(talked)
	return script.Script{
		Dir:   "teleport",
		Bind:  script.Bindings{script.EventTalked: talked},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

// onAdvEvent runs the oracle link the event names, ignoring case; any
// other event answers nothing. A memo naming an NPC of no return point
// aborts the return.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := ""

	player := e.Player
	switch {
	case strings.EqualFold(e.Name, "Return"):
		id := memoInteger(player, memo, -1)
		if id >= 0 {
			loc, ok := returnLocations()[id]
			if !ok {
				panic("oracleteleporter: no return point for NPC " + strconv.Itoa(int(id)))
			}
			player.TeleportTo(loc, 0)

			if slices.Contains(templePriest(), e.NPC.NpcID()) {
				player.SetIn7sDungeon(false)
			}
		}

	case strings.EqualFold(e.Name, "Festival"):
		id := memoInteger(player, memo, -1)
		if slices.Contains(townDawn(), id) {
			player.TeleportTo(dawnFestival(), 0)
			player.SetIn7sDungeon(true)
		} else if slices.Contains(townDusk(), id) {
			player.TeleportTo(duskFestival(), 0)
			player.SetIn7sDungeon(true)
		} else {
			htmltext = "oracle1.htm"
		}

	case strings.EqualFold(e.Name, "5.htm"):
		id := memoInteger(player, memo, -1)
		if id >= 0 {
			htmltext = "5a.htm"
		}

		player.SetMemo(memo, strconv.Itoa(int(e.NPC.NpcID())))
		player.TeleportTo(riftEntrance(), 0)

	case strings.EqualFold(e.Name, "zigurratDimensional"):
		level := player.Level()
		if level >= 20 && level < 30 {
			s.TakeItems(player, adena, 2000)
		} else if level >= 30 && level < 40 {
			s.TakeItems(player, adena, 4500)
		} else if level >= 40 && level < 50 {
			s.TakeItems(player, adena, 8000)
		} else if level >= 50 && level < 60 {
			s.TakeItems(player, adena, 12500)
		} else if level >= 60 && level < 70 {
			s.TakeItems(player, adena, 18000)
		} else if level >= 70 {
			s.TakeItems(player, adena, 24500)
		}

		htmltext = "ziggurat_rift.htm"

		player.SetMemo(memo, strconv.Itoa(int(e.NPC.NpcID())))
		player.TeleportTo(riftEntrance(), 0)

	case strings.EqualFold(e.Name, "Dimensional"):
		htmltext = "oracle.htm"

		player.TeleportTo(riftRoom(), 0)
	}
	return htmltext
}

// onTalk sends a town priest's player into its side's necropolis, and
// answers a ziggurat gatekeeper's player its page: refused below level 20,
// at the quest limit, without a fragment or short of the adena of its
// level.
func onTalk(_ *script.Script, e script.Talk) string {
	htmltext := ""

	player := e.Player
	npcID := e.NPC.NpcID()

	if slices.Contains(townDawn(), npcID) {
		player.SetMemo(memo, strconv.Itoa(int(npcID)))
		player.TeleportTo(dawnFestival(), 0)
		player.SetIn7sDungeon(true)
	} else if slices.Contains(townDusk(), npcID) {
		player.SetMemo(memo, strconv.Itoa(int(npcID)))
		player.TeleportTo(duskFestival(), 0)
		player.SetIn7sDungeon(true)
	} else if (npcID >= 31095 && npcID <= 31111) || (npcID >= 31114 && npcID <= 31126) {
		level := player.Level()
		if level < 20 {
			htmltext = "ziggurat_lowlevel.htm"
		} else if player.StartedQuests() >= 25 {
			player.SystemMessage(msgTooManyQuests)
		} else if !player.HasItem(dimensionFragment) {
			htmltext = "ziggurat_nofrag.htm"
		} else if level < 30 && player.Adena() < 2000 {
			htmltext = "ziggurat_noadena.htm"
		} else if level < 40 && player.Adena() < 4500 {
			htmltext = "ziggurat_noadena.htm"
		} else if level < 50 && player.Adena() < 8000 {
			htmltext = "ziggurat_noadena.htm"
		} else if level < 60 && player.Adena() < 12500 {
			htmltext = "ziggurat_noadena.htm"
		} else if level < 70 && player.Adena() < 18000 {
			htmltext = "ziggurat_noadena.htm"
		} else if player.Adena() < 24500 {
			htmltext = "ziggurat_noadena.htm"
		} else {
			htmltext = "ziggurat.htm"
		}
	}

	return htmltext
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
