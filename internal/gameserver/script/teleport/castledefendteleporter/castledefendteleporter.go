// Package castledefendteleporter is the gatekeepers inside the castles that
// move a castle's own side between its inner points. A player whose clan
// owns the gatekeeper's castle or defends it in its siege gets the page of
// the gatekeeper's links; anyone else, or anyone at a gatekeeper of no
// castle, the refusal. Each link names an index, and the gatekeeper moves
// the player to one point drawn from the three of that index; an index the
// gatekeeper has no points for moves no one.
package castledefendteleporter

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The gatekeepers, three per castle except Aden and Rune, which have five.
const (
	gludio1     = 35092
	gludio2     = 35093
	gludio3     = 35094
	dion1       = 35134
	dion2       = 35135
	dion3       = 35136
	giran1      = 35176
	giran2      = 35177
	giran3      = 35178
	oren1       = 35218
	oren2       = 35219
	oren3       = 35220
	aden1       = 35261
	aden2       = 35262
	aden3       = 35263
	aden4       = 35264
	aden5       = 35265
	innadril1   = 35308
	innadril2   = 35309
	innadril3   = 35310
	goddard1    = 35352
	goddard2    = 35353
	goddard3    = 35354
	rune1       = 35497
	rune2       = 35498
	rune3       = 35499
	rune4       = 35500
	rune5       = 35501
	schuttgart1 = 35544
	schuttgart2 = 35545
	schuttgart3 = 35546
)

// gatekeepers returns every gatekeeper's id.
func gatekeepers() []int32 {
	return []int32{
		gludio1, gludio2, gludio3,
		dion1, dion2, dion3,
		giran1, giran2, giran3,
		oren1, oren2, oren3,
		aden1, aden2, aden3, aden4, aden5,
		innadril1, innadril2, innadril3,
		goddard1, goddard2, goddard3,
		rune1, rune2, rune3, rune4, rune5,
		schuttgart1, schuttgart2, schuttgart3,
	}
}

// New returns the teleporter.
func New() script.Script {
	return script.Script{
		Dir: "teleport",
		Bind: script.Bindings{
			script.EventTalked:    gatekeepers(),
			script.EventFirstTalk: gatekeepers(),
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnFirstTalk: onFirstTalk},
	}
}

// onAdvEvent moves the player to a point drawn from those of the index the
// event names, at no offset; an event naming no number aborts. An index past
// any gatekeeper's last moves no one, as one too large for an int32.
func onAdvEvent(s *script.Script, e script.Event) string {
	index, err := strconv.Atoi(e.Name)
	if err != nil {
		panic(err)
	}
	if locs := gatekeeperOf(e.NPC.NpcID()).pointsAt(index); locs != nil {
		e.Player.TeleportTo(locs[s.Rnd(len(locs))], 0)
	}
	return ""
}

func onFirstTalk(s *script.Script, e script.FirstTalk) string {
	g := gatekeeperOf(e.NPC.NpcID())
	if castle := s.Castle(e.NPC); castle != nil && castle.OnSiegeSide(e.Player, script.SiegeOwner, script.SiegeDefender) {
		return g.page
	}
	return g.refusal
}

// gatekeeper is one gatekeeper's dialog and destinations.
type gatekeeper struct {
	// page is the first talk's page for its castle's side, refusal the one
	// for anyone else.
	page, refusal string
	// points are the destinations of each link index, nil where the
	// gatekeeper has none.
	points [][]script.Loc
}

// pointsAt returns the destinations of the link index i, nil for none.
func (g gatekeeper) pointsAt(i int) []script.Loc {
	if i < 0 || i >= len(g.points) {
		return nil
	}
	return g.points[i]
}

// The first-talk pages.
const (
	firstGroup1        = "first_group_1.htm"
	firstGroup1No      = "first_group_1_no.htm"
	firstGroup2        = "first_group_2.htm"
	firstGroup2No      = "first_group_2_no.htm"
	firstGroup3        = "first_group_3.htm"
	firstGroup3No      = "first_group_3_no.htm"
	secondGroup1       = "second_group_1.htm"
	secondGroup2       = "second_group_2.htm"
	secondGroup3       = "second_group_3.htm"
	secondGroup4       = "second_group_4.htm"
	secondGroup5       = "second_group_5.htm"
	secondGroupRefusal = "second_group_no.htm"
	thirdGroup1        = "third_group_1.htm"
	thirdGroup2        = "third_group_2.htm"
	thirdGroup3        = "third_group_3.htm"
	thirdGroup4        = "third_group_4.htm"
	thirdGroup5        = "third_group_5.htm"
	thirdGroupRefusal  = "third_group_no.htm"
)

// gatekeeperOf returns the gatekeeper npcID, the zero gatekeeper for one
// that is none.
func gatekeeperOf(npcID int32) gatekeeper {
	switch npcID {
	case gludio1:
		return gatekeeper{firstGroup1, firstGroup1No, only(gludioPoints(), 0, 1)}
	case gludio2:
		return gatekeeper{firstGroup2, firstGroup2No, only(gludioPoints(), 1, 2)}
	case gludio3:
		return gatekeeper{firstGroup3, firstGroup3No, only(gludioPoints(), 0, 2)}
	case dion1:
		return gatekeeper{firstGroup1, firstGroup1No, only(dionPoints(), 0, 1)}
	case dion2:
		return gatekeeper{firstGroup2, firstGroup2No, only(dionPoints(), 1, 2)}
	case dion3:
		return gatekeeper{firstGroup3, firstGroup3No, only(dionPoints(), 0, 2)}
	case giran1:
		return gatekeeper{firstGroup1, firstGroup1No, only(giranPoints(), 0, 1)}
	case giran2:
		return gatekeeper{firstGroup2, firstGroup2No, only(giranPoints(), 1, 2)}
	case giran3:
		return gatekeeper{firstGroup3, firstGroup3No, only(giranPoints(), 0, 2)}
	case oren1:
		return gatekeeper{firstGroup1, firstGroup1No, only(orenPoints(), 0, 1)}
	case oren2:
		return gatekeeper{firstGroup2, firstGroup2No, only(orenPoints(), 1, 2)}
	case oren3:
		return gatekeeper{firstGroup3, firstGroup3No, only(orenPoints(), 0, 2)}
	case aden1:
		return gatekeeper{secondGroup1, secondGroupRefusal, only(adenPoints(), 1, 2, 3, 4)}
	case aden2:
		return gatekeeper{secondGroup2, secondGroupRefusal, only(adenPoints(), 0, 2, 3, 4)}
	case aden3:
		return gatekeeper{secondGroup3, secondGroupRefusal, only(adenPoints(), 0, 1, 3, 4)}
	case aden4:
		return gatekeeper{secondGroup4, secondGroupRefusal, only(adenPoints(), 0, 1, 2, 4)}
	case aden5:
		return gatekeeper{secondGroup5, secondGroupRefusal, only(adenPoints(), 0, 1, 2, 3)}
	case innadril1:
		return gatekeeper{firstGroup1, firstGroup1No, only(innadrilPoints(), 0, 1)}
	case innadril2:
		return gatekeeper{firstGroup2, firstGroup2No, only(innadrilPoints(), 1, 2)}
	case innadril3:
		return gatekeeper{firstGroup3, firstGroup3No, only(innadrilPoints(), 0, 2)}
	case goddard1:
		return gatekeeper{firstGroup1, firstGroup1No, only(goddardPoints(), 0, 1)}
	case goddard2:
		return gatekeeper{firstGroup2, firstGroup2No, only(goddardPoints(), 1, 2)}
	case goddard3:
		return gatekeeper{firstGroup3, firstGroup3No, only(goddardPoints(), 0, 2)}
	case rune1:
		return gatekeeper{thirdGroup1, thirdGroupRefusal, only(runePoints(), 1, 2, 3, 4)}
	case rune2:
		return gatekeeper{thirdGroup2, thirdGroupRefusal, only(runePoints(), 0, 2, 3, 4)}
	case rune3:
		return gatekeeper{thirdGroup3, thirdGroupRefusal, only(runePoints(), 0, 1, 3, 4)}
	case rune4:
		return gatekeeper{thirdGroup4, thirdGroupRefusal, only(runePoints(), 0, 1, 2, 4)}
	case rune5:
		return gatekeeper{thirdGroup5, thirdGroupRefusal, only(runePoints(), 0, 1, 2, 3)}
	case schuttgart1:
		return gatekeeper{firstGroup1, firstGroup1No, only(schuttgartPoints(), 0, 1)}
	case schuttgart2:
		return gatekeeper{firstGroup2, firstGroup2No, only(schuttgartPoints(), 1, 2)}
	case schuttgart3:
		return gatekeeper{firstGroup3, firstGroup3No, only(schuttgartPoints(), 0, 2)}
	}
	return gatekeeper{}
}

// only keeps the points of the given indexes and drops the others.
func only(points [][]script.Loc, indexes ...int) [][]script.Loc {
	kept := make([][]script.Loc, len(points))
	for _, i := range indexes {
		kept[i] = points[i]
	}
	return kept
}

// The castles' destinations, by link index.

func gludioPoints() [][]script.Loc {
	return [][]script.Loc{
		{{X: -18123, Y: 109927, Z: -2492}, {X: -18103, Y: 109947, Z: -2492}, {X: -18123, Y: 109947, Z: -2492}},
		{{X: -18635, Y: 108298, Z: -2556}, {X: -18615, Y: 108298, Z: -2556}, {X: -18635, Y: 108298, Z: -2556}},
		{{X: -18988, Y: 112081, Z: -2724}, {X: -18968, Y: 112101, Z: -2724}, {X: -18988, Y: 112101, Z: -2724}},
	}
}

func dionPoints() [][]script.Loc {
	return [][]script.Loc{
		{{X: 22094, Y: 159894, Z: -2686}, {X: 22074, Y: 159874, Z: -2686}, {X: 22094, Y: 159874, Z: -2686}},
		{{X: 22606, Y: 161523, Z: -2750}, {X: 22586, Y: 161523, Z: -2750}, {X: 22606, Y: 161523, Z: -2750}},
		{{X: 22959, Y: 157740, Z: -2918}, {X: 22939, Y: 157720, Z: -2918}, {X: 22959, Y: 157720, Z: -2918}},
	}
}

func giranPoints() [][]script.Loc {
	return [][]script.Loc{
		{{X: 115999, Y: 145031, Z: -2561}, {X: 115979, Y: 145051, Z: -2561}, {X: 115979, Y: 145031, Z: -2561}},
		{{X: 117628, Y: 144519, Z: -2625}, {X: 117628, Y: 144539, Z: -2625}, {X: 117628, Y: 144519, Z: -2625}},
		{{X: 113845, Y: 144166, Z: -2793}, {X: 113825, Y: 144186, Z: -2793}, {X: 113825, Y: 144166, Z: -2793}},
	}
}

func orenPoints() [][]script.Loc {
	return [][]script.Loc{
		{{X: 82077, Y: 37179, Z: -2282}, {X: 82057, Y: 37199, Z: -2282}, {X: 82057, Y: 37179, Z: -2282}},
		{{X: 83706, Y: 36667, Z: -2346}, {X: 83706, Y: 36687, Z: -2346}, {X: 83706, Y: 36667, Z: -2346}},
		{{X: 79923, Y: 36314, Z: -2514}, {X: 79903, Y: 36334, Z: -2514}, {X: 79903, Y: 36314, Z: -2514}},
	}
}

func adenPoints() [][]script.Loc {
	return [][]script.Loc{
		{{X: 147848, Y: 7951, Z: -470}, {X: 147731, Y: 7886, Z: -470}, {X: 147874, Y: 7852, Z: -470}},
		{{X: 146445, Y: 4739, Z: -408}, {X: 146411, Y: 4613, Z: -408}, {X: 146460, Y: 4491, Z: -408}},
		{{X: 148551, Y: 4607, Z: -408}, {X: 148482, Y: 4527, Z: -408}, {X: 148498, Y: 4707, Z: -408}},
		{{X: 147453, Y: 5354, Z: -345}, {X: 147501, Y: 5298, Z: -345}, {X: 147403, Y: 5302, Z: -345}},
		{{X: 147050, Y: 1498, Z: -473}, {X: 146977, Y: 1584, Z: -473}, {X: 147111, Y: 1586, Z: -473}},
	}
}

func innadrilPoints() [][]script.Loc {
	return [][]script.Loc{
		{{X: 116034, Y: 248602, Z: -782}, {X: 116014, Y: 248582, Z: -782}, {X: 116034, Y: 248582, Z: -782}},
		{{X: 116546, Y: 250231, Z: -846}, {X: 116526, Y: 250231, Z: -846}, {X: 116546, Y: 250231, Z: -846}},
		{{X: 116899, Y: 246448, Z: -1014}, {X: 116879, Y: 246428, Z: -1014}, {X: 116899, Y: 246428, Z: -1014}},
	}
}

func goddardPoints() [][]script.Loc {
	return [][]script.Loc{
		{{X: 147461, Y: -46782, Z: -1360}, {X: 147520, Y: -46784, Z: -1360}, {X: 147419, Y: -46791, Z: -1360}},
		{{X: 147449, Y: -50965, Z: -1344}, {X: 147514, Y: -50970, Z: -1344}, {X: 147388, Y: -50960, Z: -1344}},
		{{X: 147475, Y: -45860, Z: -2080}, {X: 147399, Y: -45851, Z: -2080}, {X: 147540, Y: -45841, Z: -2080}},
	}
}

func runePoints() [][]script.Loc {
	return [][]script.Loc{
		{{X: 16094, Y: -48945, Z: -1060}, {X: 16087, Y: -49009, Z: -1060}, {X: 16143, Y: -48958, Z: -1060}},
		{{X: 13076, Y: -51241, Z: -1089}, {X: 12990, Y: -51241, Z: -1089}, {X: 13035, Y: -51331, Z: -1089}},
		{{X: 14071, Y: -49547, Z: 987}, {X: 14006, Y: -49523, Z: 987}, {X: 13911, Y: -49493, Z: 987}},
		{{X: 13158, Y: -49149, Z: -536}, {X: 13288, Y: -49148, Z: -536}, {X: 13417, Y: -49149, Z: -536}},
		{{X: 9762, Y: -49184, Z: 997}, {X: 9897, Y: -49184, Z: 997}, {X: 10013, Y: -49184, Z: 997}},
	}
}

func schuttgartPoints() [][]script.Loc {
	return [][]script.Loc{
		{{X: 77546, Y: -150836, Z: 376}, {X: 77605, Y: -150838, Z: 376}, {X: 77504, Y: -150845, Z: 376}},
		{{X: 77534, Y: -155019, Z: 392}, {X: 77599, Y: -155024, Z: 392}, {X: 77473, Y: -155014, Z: 392}},
		{{X: 77560, Y: -149914, Z: -344}, {X: 77484, Y: -149905, Z: -344}, {X: 77625, Y: -149895, Z: -344}},
	}
}
