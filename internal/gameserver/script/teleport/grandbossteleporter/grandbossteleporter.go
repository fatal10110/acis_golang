// Package grandbossteleporter is the teleporters of the grand boss lairs:
// the cubes that appear after a boss falls and send the players out, Klein
// who sends a player with a floating stone into the Lair of Valakas, and
// the gatekeepers of the fire dragon who open the doors to the Heart of
// Volcano. Klein's page follows how many players he sent in since Valakas
// last died.
package grandbossteleporter

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// baiumZoneID is the zone of Baium's lair.
const baiumZoneID = 110002

// floatingStone is what Klein takes for the way into the Lair of Valakas.
const floatingStone = 7267

// valakas is the boss whose death resets Klein's count.
const valakas = 29028

func baiumOut() []script.Loc {
	return []script.Loc{
		{X: 108784, Y: 16000, Z: -4928},
		{X: 113824, Y: 10448, Z: -5164},
		{X: 115488, Y: 22096, Z: -5168},
	}
}

func benomOut() []script.Loc {
	return []script.Loc{
		{X: 11913, Y: -48851, Z: -1088},
		{X: 11918, Y: -49447, Z: -1088},
	}
}

func coreOut() []script.Loc {
	return []script.Loc{
		{X: 17252, Y: 114121, Z: -3439},
		{X: 17253, Y: 114232, Z: -3439},
	}
}

func frintezzaOut() script.Loc { return script.Loc{X: 150037, Y: -57720, Z: -2976} }

func sailrenOut() []script.Loc {
	return []script.Loc{
		{X: 10610, Y: -24035, Z: -3676},
		{X: 10703, Y: -24041, Z: -3673},
		{X: 10769, Y: -24107, Z: -3672},
	}
}

func valakasIn() script.Loc { return script.Loc{X: 183813, Y: -115157, Z: -3303} }

func valakasOut() script.Loc { return script.Loc{X: 150037, Y: -57720, Z: -2976} }

// teleporter holds how many players Klein sent into the Lair of Valakas
// since Valakas last died.
type teleporter struct{ valakasPlayersCount *script.IntCell }

// New returns the teleporter.
func New() script.Script {
	t := teleporter{valakasPlayersCount: new(script.IntCell)}
	return script.Script{
		Dir: "teleport",
		Bind: script.Bindings{
			script.EventFirstTalk: {29055, 29061, 31842},
			script.EventTalked:    {29055, 29061, 31540, 31842, 31859, 31384, 31686, 31687, 31759, 32107},
			script.EventCreated:   {29055, 29061, 31759, 31842, 31859, 32107},
			script.EventMyDying:   {valakas},
		},
		Hooks: script.Hooks{
			OnCreated:   onCreated,
			OnEvent:     t.onAdvEvent,
			OnFirstTalk: onFirstTalk,
			OnTalk:      t.onTalk,
			OnMyDying:   t.onMyDying,
		},
	}
}

// onCreated has a cube leave the world after twenty minutes for Benom's,
// fifteen for any other.
func onCreated(s *script.Script, e script.Created) {
	if e.NPC.NpcID() == 29055 {
		s.Spawner().ScheduleDespawn(e.NPC, 1200000*time.Millisecond)
	} else {
		s.Spawner().ScheduleDespawn(e.NPC, 900000*time.Millisecond)
	}
}

// onAdvEvent runs the exit or entry the event names, ignoring case; any
// other event answers nothing.
func (t teleporter) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := ""

	player := e.Player
	if strings.EqualFold(e.Name, "benom_exit") {
		locs := benomOut()
		player.TeleportTo(locs[s.Rnd(len(locs))], 100)
	} else if strings.EqualFold(e.Name, "frintezza_exit") {
		player.TeleportTo(frintezzaOut(), 500)
	} else if strings.EqualFold(e.Name, "31540") {
		if player.HasItem(floatingStone) {
			s.TakeItems(player, floatingStone, 1)
			player.TeleportTo(valakasIn(), 0)

			t.valakasPlayersCount.Add(1)
		} else {
			htmltext = "31540-06.htm"
		}
	}

	return htmltext
}

// onFirstTalk answers the cube's own page.
func onFirstTalk(_ *script.Script, e script.FirstTalk) string {
	return strconv.Itoa(int(e.NPC.NpcID())) + "-01.htm"
}

// onTalk sends the player out of the lair, answers Klein's page or opens
// the gatekeeper's door. A door or zone the world does not have aborts
// the hook.
func (t teleporter) onTalk(s *script.Script, e script.Talk) string {
	htmltext := ""

	player := e.Player
	switch e.NPC.NpcID() {
	case 31842:
		if s.Zone(baiumZoneID).Contains(e.NPC) {
			locs := baiumOut()
			player.TeleportTo(locs[s.Rnd(len(locs))], 100)
		} else {
			locs := coreOut()
			player.TeleportTo(locs[s.Rnd(len(locs))], 0)
		}

	case 31540:
		count := t.valakasPlayersCount.Get()
		if count < 50 {
			htmltext = "31540-01.htm"
		} else if count < 100 {
			htmltext = "31540-02.htm"
		} else if count < 150 {
			htmltext = "31540-03.htm"
		} else if count < 200 {
			htmltext = "31540-04.htm"
		} else {
			htmltext = "31540-05.htm"
		}

	case 31859:
		x := 79800 + s.Rnd(600)
		y := 151200 + s.Rnd(1100)
		player.TeleportTo(script.Loc{X: x, Y: y, Z: -3534}, 0)

	case 31384:
		s.DoorByID(24210004).Open()

	case 31686:
		s.DoorByID(24210006).Open()

	case 31687:
		s.DoorByID(24210005).Open()

	case 31759:
		player.TeleportTo(valakasOut(), 250)

	case 32107:
		locs := sailrenOut()
		player.TeleportTo(locs[s.Rnd(len(locs))], 100)
	}

	return htmltext
}

// onMyDying forgets the players Klein sent in.
func (t teleporter) onMyDying(*script.Script, script.MyDying) {
	t.valakasPlayersCount.Set(0)
}
