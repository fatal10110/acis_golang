package valakas

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// maxPlayersAllowed is how many entrants the Heart lets into the lair.
const maxPlayersAllowed = 200

// slotEntrants is the Heart's count of entrants, kept as its db value.
const slotEntrants = script.SlotIntAI0

// HeartOfVolcano is the Heart of Volcano, the way into Valakas's lair. A
// fresh spawn stores db value 0 as its count of entrants; one restored
// from its row resumes its count. While Valakas lives asleep or waiting
// (its db value 0 or 1), up to 200 characters enter: each is teleported
// to the Heart's TelPos and counted, and the first entry wakes Valakas
// with script event 0. Every minute the Heart forgets its count once
// Valakas is gone, dead or asleep again.
func HeartOfVolcano() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{
		Dir: "ai/boss/valakas", Behavior: true, NPCs: []int32{31385},
		Bind: script.Bindings{script.EventTalked: {31385}},
		Hooks: base.With(script.Hooks{
			OnCreated: func(s *script.Script, e script.Created) {
				n := e.NPC
				if !n.DBLoaded() {
					n.SetInt(slotEntrants, 0)
					n.SetDBValue(0)
				} else {
					n.SetInt(slotEntrants, int32(n.DBValue()))
				}

				s.StartTimerAtFixedRate("2002", n, nil, 60000*time.Millisecond, 60000*time.Millisecond)
			},
			OnFirstTalk: func(*script.Script, script.FirstTalk) string {
				return "heart_of_volcano001.htm"
			},
			OnEvent: heartEvent,
			OnTimer: func(s *script.Script, e script.Timer) string {
				n := e.NPC
				if strings.EqualFold(e.Name, "2002") {
					npcDBValue := n.DBValue()
					if npcDBValue != 0 {
						if c0, ok := s.GlobalMemo().Creature(memoKey).(*script.NPC); ok && !c0.Dead() {
							c0DBValue := c0.DBValue()
							if c0DBValue != 0 {
								return base.Timer(s, e)
							}
						}

						n.SetDBValue(0)
						n.SetInt(slotEntrants, 0)
					}
				}
				return base.Timer(s, e)
			},
		}),
	}
}

// heartEvent runs the entry on "enter", ignoring case; any other event
// answers nothing. A memo key naming no NPC counts as Valakas being gone.
func heartEvent(s *script.Script, e script.Event) string {
	htmltext := ""

	if strings.EqualFold(e.Name, "enter") {
		n, player := e.NPC, e.Player
		if c0, ok := s.GlobalMemo().Creature(memoKey).(*script.NPC); ok && !c0.Dead() {
			dbValue := c0.DBValue()
			if dbValue == 0 {
				s.SendScriptEvent(c0, 0, 0, 0)
			}

			if dbValue == 0 || dbValue == 1 {
				if n.Int(slotEntrants) >= maxPlayersAllowed {
					htmltext = "heart_of_volcano004.htm"
				} else {
					telX := n.IntParam("TelPosX")
					telY := n.IntParam("TelPosY")
					telZ := n.IntParam("TelPosZ")

					player.TeleportTo(script.Loc{X: int(telX), Y: int(telY), Z: int(telZ)}, 0)

					n.SetInt(slotEntrants, n.Int(slotEntrants)+1)
					n.SetDBValue(int(n.Int(slotEntrants)))
				}
			} else if dbValue == 2 || dbValue == 3 {
				htmltext = "heart_of_volcano003.htm"
			}
		} else {
			htmltext = "heart_of_volcano002.htm"
		}
	}
	return htmltext
}
