// Package echocrystal is the music sellers who trade a score and adena for
// the echo crystal of its song.
package echocrystal

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const (
	adena = 57
	// cost is the adena an echo crystal costs.
	cost = 200
)

// score is the trade of one score: the crystal it buys and the page
// suffixes of the answer, when bought, when short of adena and when the
// score is missing.
type score struct {
	crystalID                int32
	okMsg, noAdenaMsg, noMsg string
}

// scores are the trades, by score item id.
func scores() map[int32]score {
	return map[int32]score{
		4410: {4411, "01", "02", "03"},
		4409: {4412, "04", "05", "06"},
		4408: {4413, "07", "08", "09"},
		4420: {4414, "10", "11", "12"},
		4421: {4415, "13", "14", "15"},
		4419: {4417, "16", "05", "06"},
		4418: {4416, "17", "05", "06"},
	}
}

// New returns the feature.
func New() script.Script {
	return script.Script{
		Dir:   "feature",
		Bind:  script.Bindings{script.EventTalked: {31042, 31043}},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk},
	}
}

// onAdvEvent answers a score's id: a player holding the score and the cost
// gives up the cost for the crystal; one without the score, or short of the
// cost, is told so. Any other event answers nothing.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := ""
	if script.IsDigit(e.Name) {
		// An id outside the int32 range aborts the event here.
		id, err := strconv.ParseInt(e.Name, 10, 32)
		if err != nil {
			panic(err)
		}
		scoreID := int32(id)
		if sd, ok := scores()[scoreID]; ok {
			p := e.Player
			prefix := strconv.Itoa(int(e.NPC.NpcID())) + "-"
			switch {
			case !p.HasItem(scoreID):
				htmltext = prefix + sd.noMsg + ".htm"
			case p.ItemCount(adena) < cost:
				htmltext = prefix + sd.noAdenaMsg + ".htm"
			default:
				htmltext = prefix + sd.okMsg + ".htm"
				s.TakeItems(p, adena, cost)
				s.GiveItems(p, sd.crystalID, 1)
			}
		}
	}
	return htmltext
}

func onTalk(*script.Script, script.Talk) string {
	return "1.htm"
}
