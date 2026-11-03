package sirra

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// dir is the script directory of the Sirra and Freya behaviors.
const dir = "ai/boss/sirra"

// The deacon's pages and the two ice barriers it opens and closes.
const (
	szName    = "freya_deacon_q0656"
	doorName1 = "ice_barrier_001"
	doorName2 = "ice_barrier_002"
	fnHi      = "freya_deacon001.htm"
	fnHi2     = "freya_deacon002.htm"
)

// The deacon's script memory.
const (
	slotLeader  = script.SlotIntAI1    // object id of the leader of the party let in
	slotBusy    = script.SlotIntAI2    // 1 while a party is let in
	slotCount   = script.SlotIntAI4    // members of that party as it paid
	slotEntrant = script.SlotCreature0 // the leader who paid
)

// The Silver Hemocyte each member pays and the item the leader receives.
const (
	silverHemocyte = 8057
	entryReward    = 8379
)

// msgInventoryLessThan80Percent refuses an overweight leader.
const msgInventoryLessThan80Percent = 1118

// FreyaDeacon is the steward who lets a party into Ice Queen Freya's
// garden. The leader of a party whose members each hold 10 Silver
// Hemocytes pays for them all and receives three of item 8379; two
// minutes later the deacon closes the two ice barriers and, 5 s after,
// lets every member of the party still at its side within 1000 into the
// three garden zones for 2100 s and teleports them in, running 11040 on
// the boss maker with the leader. A party that grew meanwhile is turned
// away and the barriers open again. Script events 10005 and 10026 end the
// visit: the barriers open and the deacon is free for the next party.
func FreyaDeacon() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{
		Dir: dir, Behavior: true, NPCs: []int32{32029},
		Bind: script.Bindings{script.EventTalked: {32029}},
		Hooks: base.With(script.Hooks{
			OnCreated: func(_ *script.Script, e script.Created) {
				n := e.NPC
				n.SetInt(script.SlotIntAI1, 0)
				n.SetInt(script.SlotIntAI2, 0)
				n.SetInt(script.SlotIntAI3, 0)
				n.SetInt(script.SlotIntAI4, 0)
				n.SetSlot(slotEntrant, nil)
			},
			OnFirstTalk: func(_ *script.Script, e script.FirstTalk) string {
				htmltext := ""

				n := e.NPC
				leader := e.Player.PartyLeader()
				if n.Int(slotBusy) == 1 {
					if leader != nil {
						if n.Int(slotLeader) == leader.ObjectID() {
							htmltext = fnHi2
						} else {
							htmltext = szName + "_01.htm"
						}
					} else {
						htmltext = szName + "_01.htm"
					}
				} else if n.Int(slotBusy) == 0 {
					htmltext = fnHi
				}

				return htmltext
			},
			OnEvent:       deaconEvent,
			OnScriptEvent: deaconScriptEvent,
			OnTimer: func(s *script.Script, e script.Timer) string {
				n := e.NPC
				switch {
				case strings.EqualFold(e.Name, "2002"):
					entrant := entrantOf(n)
					if entrant == nil {
						return ""
					}

					if entrant.InParty() {
						members := entrant.PartyMembers()
						if int(n.Int(slotCount)) >= len(members) {
							for _, partyMember := range members {
								if script.InRadius3D(partyMember, n, 1000) {
									allowPlayerEntry(s, partyMember)
									partyMember.TeleportTo(script.Loc{X: 113533, Y: -126159, Z: -3488}, 0)
								}
							}

							s.Spawner().SendMakerEvent("schuttgart13_mb2314_05m1", "11040", int(entrant.ObjectID()), 0)
						} else {
							n.Shout(script.NpcString(1121007))

							n.SetInt(slotBusy, 0)
							n.SetInt(slotLeader, 0)
							n.SetSlot(slotEntrant, nil)

							openBarriers(s)
						}
					}
				case strings.EqualFold(e.Name, "1005"):
					if c0 := s.GlobalMemo().Creature("7"); c0 != nil {
						s.SendScriptEvent(c0.(*script.NPC), 10027, 0, 0)
					}

					closeBarriers(s)

					s.StartTimer("1006", n, nil, 5000*time.Millisecond)
					s.StartTimer("2002", n, nil, 5000*time.Millisecond)

					n.SetInt(script.SlotIntAI3, 0)
				case strings.EqualFold(e.Name, "1006"):
					entrant := entrantOf(n)
					if entrant == nil {
						return ""
					}

					entrant.ShowPartyScreenMessage(10000, script.NpcString(1121001))
				}

				return base.Timer(s, e)
			},
		}),
	}
}

// deaconEvent runs the entry, or answers the page a link_ event names.
func deaconEvent(s *script.Script, e script.Event) string {
	htmltext := ""

	n, player := e.NPC, e.Player
	switch {
	case strings.HasPrefix(e.Name, "link_"):
		htmltext = szName + strings.ReplaceAll(e.Name, "link", "") + ".htm"
	case strings.EqualFold(e.Name, "enter"):
		if n.Int(slotBusy) != 0 {
			return szName + "_01.htm"
		}

		leader := player.PartyLeader()
		switch {
		case leader == nil:
			htmltext = szName + "_06.htm"
		case n.Int(slotLeader) == 0 || leader.ObjectID() == n.Int(slotLeader):
			if !script.Same(leader, player) {
				return szName + "_02.htm"
			}

			members := player.PartyMembers()
			for _, partyMember := range members {
				if partyMember.ItemCount(silverHemocyte) < 10 {
					html, _ := s.Page("./data/html/script/" + s.Dir + "/" + s.Name + "/" + szName + "_03.htm")
					html = strings.ReplaceAll(html, "%name%", partyMember.Name())
					partyMember.ShowHTML(n.ObjectID(), html)

					return ""
				}
			}

			if player.Overweight() {
				player.SystemMessage(msgInventoryLessThan80Percent)
				return ""
			}

			for _, partyMember := range members {
				s.TakeItems(partyMember, silverHemocyte, 10)
			}

			player.ShowPartyScreenMessage(100000, script.NpcString(1121000))

			s.GiveItems(player, entryReward, 3)

			htmltext = szName + "_05.htm"
			n.SetInt(slotCount, int32(len(members)))

			n.Shout(script.NpcString(1121005))

			s.StartTimer("1005", n, player, 120000*time.Millisecond)

			n.SetInt(slotBusy, 1)
			n.SetInt(slotLeader, leader.ObjectID())
			n.SetSlot(slotEntrant, player)
		}
	}

	return htmltext
}

// deaconScriptEvent ends a visit on 10005 (the boss maker's) and 10026,
// and clears the deacon's third mark on 11037.
func deaconScriptEvent(s *script.Script, e script.ScriptEvent) {
	n := e.NPC
	switch e.EventID {
	case 10005:
		s.Spawner().SendMakerEvent("schuttgart13_npc2314_3m1", "10025", 0, 0)

		if entrant := entrantOf(n); entrant != nil {
			entrant.ShowPartyScreenMessage(10000, script.NpcString(1121003))
		}

		s.BroadcastScriptEvent(n, 11039, 0, 8000)

		openBarriers(s)

		n.SetInt(slotBusy, 0)
		n.SetInt(slotLeader, 0)
		n.SetSlot(slotEntrant, nil)
	case 10026:
		if entrant := entrantOf(n); entrant != nil {
			entrant.ShowPartyScreenMessage(10000, script.NpcString(1121004))
		}

		s.BroadcastScriptEvent(n, 11039, 0, 8000)

		openBarriers(s)

		n.SetInt(script.SlotIntAI3, 1)
		n.SetInt(slotBusy, 0)
		n.SetInt(slotLeader, 0)
		n.SetSlot(slotEntrant, nil)
	case 11037:
		n.SetInt(script.SlotIntAI3, 0)
	}
}

// entrantOf returns the leader who paid for the party let in, nil for none.
func entrantOf(n *script.NPC) *script.Player {
	p, _ := n.Slot(slotEntrant).(*script.Player)
	return p
}

// openBarriers opens the two ice barriers.
func openBarriers(s *script.Script) {
	s.Door(doorName1).Open()
	s.Door(doorName2).Open()
}

// closeBarriers closes the two ice barriers.
func closeBarriers(s *script.Script) {
	s.Door(doorName1).Close()
	s.Door(doorName2).Close()
}

// allowPlayerEntry lets p into the three garden zones for 2100 s.
func allowPlayerEntry(s *script.Script, p *script.Player) {
	if sirra1 := s.Zone(110013); sirra1 != nil {
		sirra1.AllowPlayerEntry(p, 2100)
	}

	if sirra2 := s.Zone(110014); sirra2 != nil {
		sirra2.AllowPlayerEntry(p, 2100)
	}

	if sirra3 := s.Zone(110015); sirra3 != nil {
		sirra3.AllowPlayerEntry(p, 2100)
	}
}
