package sailren

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// The statue's script memory.
const (
	slotBusy = script.SlotIntAI1 // 1 while a party is inside the nest
	slotOpen = script.SlotIntAI2 // 1 once Sailren waits in the nest
)

// gazkh is the sacred axe a party leader gives up to enter the nest.
const gazkh = 8784

// StatueOfShilen is Shilen's Stone Statue, the way into Sailren's nest.
// Once Sailren waits there (event 11041) and while no party is inside,
// the leader of a party holding Gazkh, with every member within 1000,
// gives up one Gazkh: every member may enter the nest's zone for 30 s
// and is teleported in, and the statue runs 11053 on the three dinosaur
// makers and 11048 on the dummy's maker, and a minute later 1001 on the
// first dinosaur maker. Events 11043, 11045 and 11050 free the nest,
// 11045 also closing it until Sailren waits again; 11047 marks it busy.
func StatueOfShilen() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{
		Dir: dir, Behavior: true, NPCs: []int32{32109},
		Bind: script.Bindings{script.EventTalked: {32109}},
		Hooks: base.With(script.Hooks{
			OnCreated: func(s *script.Script, e script.Created) {
				n := e.NPC
				n.SetInt(slotBusy, 0)
				n.SetInt(slotOpen, 0)
				n.SetInt(script.SlotIntAI3, 0)
				n.SetInt(script.SlotIntAI4, 0)

				base.Created(s, e)
			},
			OnEvent: statueEvent,
			OnTimer: func(s *script.Script, e script.Timer) string {
				if strings.EqualFold(e.Name, "1001") {
					s.Spawner().SendMakerEvent("rune20_mb2017_01m1", "1001", 0, 0)
				}
				return base.Timer(s, e)
			},
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				n := e.NPC
				switch e.EventID {
				case 11041:
					n.SetInt(slotOpen, 1)
				case 11043:
					n.SetInt(slotBusy, 0)
				case 11045:
					n.SetInt(slotBusy, 0)
					n.SetInt(slotOpen, 0)
				case 11047:
					n.SetInt(slotBusy, 1)
				case 11050:
					n.SetInt(slotBusy, 0)
				}
			},
		}),
	}
}

// statueEvent runs the entry or answers the page the event names, ignoring
// case; any other event answers nothing.
func statueEvent(s *script.Script, e script.Event) string {
	htmltext := ""

	n, player := e.NPC, e.Player
	switch {
	case strings.EqualFold(e.Name, "enter"):
		switch {
		case !player.InParty():
			htmltext = "statue_of_shilen002.htm"
		case n.Int(slotOpen) != 1:
			htmltext = "statue_of_shilen005.htm"
		case n.Int(slotBusy) != 0:
			htmltext = "statue_of_shilen006.htm"
		case !player.IsPartyLeader():
			htmltext = "statue_of_shilen004.htm"
		case !player.HasItem(gazkh):
			htmltext = "statue_of_shilen003.htm"
		default:
			for _, partyMember := range player.PartyMembers() {
				if !script.InRange(1000, player, partyMember, true) {
					return "party_range.htm"
				}
			}

			s.TakeItems(player, gazkh, 1)

			n.SetInt(slotBusy, 1)

			if nest := s.Zone(110011); nest != nil {
				for _, partyMember := range player.PartyMembers() {
					nest.AllowPlayerEntry(partyMember, 30)
					partyMember.TeleportTo(script.Loc{X: 27549, Y: -6638, Z: -2008}, 0)
				}
			}

			s.Spawner().SendMakerEvent("rune20_mb2017_01m1", "11053", 0, 0)

			s.StartTimer("1001", n, nil, 60000*time.Millisecond)

			s.Spawner().SendMakerEvent("rune20_mb2017_02m1", "11053", 0, 0)
			s.Spawner().SendMakerEvent("rune20_mb2017_03m1", "11053", 0, 0)
			s.Spawner().SendMakerEvent("rune16_npc2017_13m1", "11048", 0, 0)
		}
	case strings.EqualFold(e.Name, "battle_prep"):
		htmltext = "statue_of_shilen002a.htm"
	case strings.EqualFold(e.Name, "enter_details"):
		htmltext = "statue_of_shilen003a.htm"
	case strings.EqualFold(e.Name, "learn_more"):
		htmltext = "statue_of_shilen004a.htm"
	}

	return htmltext
}
