package antharas

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/bossmemo"
)

// heartID is the Heart of Warding; portalStone the item an entrant gives
// up to it.
const (
	heartID     = 13001
	portalStone = 3865
)

// The Heart's script memory.
const (
	slotWaiting  = script.SlotIntAI0 // 1 while the wait before the wake-up runs
	slotEntrants = script.SlotIntAI1 // entrants since the wait started
)

// HeartOfWarding is the Heart of Warding, the way into Antharas's lair.
// While the three Antharas the global memo names live, none of them
// awake past 1, a character holding a Portal Stone gives one up and is
// teleported into the lair. The first entrant starts the boss wait; once
// it ends the Heart wakes the small Antharas for up to 90 entrants, the
// normal one for up to 135, the large one beyond, and counts afresh.
func HeartOfWarding() script.Script {
	var base script.Hooks
	return script.Script{
		Dir: "ai/boss/antharas",
		Bind: script.Bindings{
			script.EventFirstTalk: {heartID},
			script.EventTalked:    {heartID},
			script.EventCreated:   {heartID},
		},
		Hooks: script.Hooks{
			OnCreated: func(s *script.Script, e script.Created) {
				n := e.NPC
				n.SetInt(slotWaiting, 0)
				n.SetInt(slotEntrants, 0)

				base.Created(s, e)
			},
			OnFirstTalk: func(*script.Script, script.FirstTalk) string {
				return "13001-01.htm"
			},
			OnTimer: func(s *script.Script, e script.Timer) string {
				n := e.NPC
				if strings.EqualFold(e.Name, "3001") {
					switch {
					case n.Int(slotEntrants) <= 90:
						if c, ok := n.Slot(script.SlotCreature0).(*script.NPC); ok {
							s.SendScriptEvent(c, 0, 0, 0)
						}
					case n.Int(slotEntrants) <= 135:
						if c, ok := n.Slot(script.SlotCreature1).(*script.NPC); ok {
							s.SendScriptEvent(c, 0, 0, 0)
						}
					default:
						if c, ok := n.Slot(script.SlotCreature2).(*script.NPC); ok {
							s.SendScriptEvent(c, 0, 0, 0)
						}
					}

					n.SetInt(slotWaiting, 0)
					n.SetInt(slotEntrants, 0)
				}
				return base.Timer(s, e)
			},
			OnEvent: heartEvent,
		},
	}
}

// heartEvent runs the entry on "enter", ignoring case; any other event
// answers nothing.
func heartEvent(s *script.Script, e script.Event) string {
	htmltext := ""

	if strings.EqualFold(e.Name, "enter") {
		n, player := e.NPC, e.Player
		// The Heart reads the keys the lair's Antharas spawns store
		// themselves under (owner-approved divergence, #4682).
		i0 := s.GlobalMemo().Int(bossmemo.AntharasSmall, -1)
		i1 := s.GlobalMemo().Int(bossmemo.AntharasNormal, -1)
		i2 := s.GlobalMemo().Int(bossmemo.AntharasLarge, -1)

		if i0 != -1 && i1 != -1 && i2 != -1 {
			c0 := antharasOf(s, bossmemo.AntharasSmall)
			c1 := antharasOf(s, bossmemo.AntharasNormal)
			c2 := antharasOf(s, bossmemo.AntharasLarge)

			if c0 != nil && c1 != nil && c2 != nil && !c0.Dead() && !c1.Dead() && !c2.Dead() {
				c0Value := c0.DBValue()
				c1Value := c1.DBValue()
				c2Value := c2.DBValue()

				switch {
				case (c0Value == 0 || c0Value == 1) && (c1Value == 0 || c1Value == 1) && (c2Value == 0 || c2Value == 1):
					if player.HasItem(portalStone) {
						s.TakeItems(player, portalStone, 1)

						i3 := 179700 + s.Rnd(700)
						i4 := 113800 + s.Rnd(2100)

						player.TeleportTo(script.Loc{X: i3, Y: i4, Z: -7709}, 0)

						n.SetInt(slotEntrants, n.Int(slotEntrants)+1)

						if n.Int(slotWaiting) == 0 {
							s.StartTimer("3001", n, player, s.BossWaitTimes().Antharas)

							n.SetSlot(script.SlotCreature0, c0)
							n.SetSlot(script.SlotCreature1, c1)
							n.SetSlot(script.SlotCreature2, c2)

							n.SetInt(slotWaiting, 1)
						}
					} else {
						htmltext = "13001-04.htm"
					}
				case c0Value == 2 || c0Value == 3 || c1Value == 2 || c1Value == 3 || c2Value == 2 || c2Value == 3:
					htmltext = "13001-03.htm"
				}
			} else {
				htmltext = "13001-02.htm"
			}
		} else {
			htmltext = "13001-02.htm"
		}
	}
	return htmltext
}

// antharasOf returns the NPC whose object id the global memo key holds,
// nil when the key names no NPC in the world (owner-approved: the entry
// treats it as a missing Antharas, #4682).
func antharasOf(s *script.Script, key string) *script.NPC {
	c, _ := s.GlobalMemo().Creature(key).(*script.NPC)
	return c
}
