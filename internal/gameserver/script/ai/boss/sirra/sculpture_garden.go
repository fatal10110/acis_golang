package sirra

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// slotSeen is the last player the garden sculpture saw.
const slotSeen = script.SlotCreature0

// SculptureGarden is the sculpture in Ice Queen Freya's garden. The first
// one created while global memo 7 names no creature stores its own object
// id there, so the garden's script events reach it. It remembers the last
// player it saw. Script event 10027 sends every player in its spawn
// territory to 115792,-125760,-3373, up to 200 away; 11038 makes it look
// around, then cast Resist Cold (4479, level 1) on each member of the
// remembered player's party standing in its spawn territory.
func SculptureGarden() script.Script {
	base := defaultnpc.New().Hooks
	resistCold := skill.Ref{ID: 4479, Level: 1}
	return script.Script{
		Dir: dir, Behavior: true, NPCs: []int32{32030},
		Hooks: base.With(script.Hooks{
			OnCreated: func(s *script.Script, e script.Created) {
				c0 := s.GlobalMemo().Creature("7")
				if c0 == nil {
					s.GlobalMemo().Set("7", int(e.NPC.ObjectID()))
				}

				base.Created(s, e)
			},
			OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
				if p, ok := e.Creature.(*script.Player); ok {
					e.NPC.SetSlot(slotSeen, p)
				}

				base.SeeCreature(s, e)
			},
			OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
				n := e.NPC
				switch e.EventID {
				case 10027:
					s.TeleportPlayersInTerritory(n, script.Loc{X: 115792, Y: -125760, Z: -3373}, 200)
				case 11038:
					n.LookNeighbor()

					if seen, ok := n.Slot(slotSeen).(*script.Player); ok {
						for _, member := range seen.PartyMembers() {
							if n.InSpawnTerritory(member) {
								n.CallSkill(member, resistCold)
							}
						}
					}
				}
			},
		}),
	}
}
