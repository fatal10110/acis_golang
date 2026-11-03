// Package wildbeastreserve is the Wild Beast Reserve's hall script: its
// flag war, and the registration its Kel Mahum messenger takes. A clan
// registers by handing in the Trainer License the taming quest gives; the
// first five clans fight, each picking the warrior it defends.
package wildbeastreserve

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/siegablehall/flagwar"
)

// questName is the quest whose license registers a clan.
const questName = "Q655_AGrandPlanForTamingWildBeasts"

// The items a registration takes.
const (
	crystalOfPurity = 8084
	trainerLicense  = 8293
)

// beastFarm is the Wild Beast Reserve's hall id.
const beastFarm int32 = 63

// msgInventoryLessThan80Percent refuses a registration to an overweight
// player.
const msgInventoryLessThan80Percent = 1118

// war returns the Wild Beast Reserve's flag war.
func war() *flagwar.War {
	coords := [7]script.Loc{
		0: {X: 56963, Y: -92211, Z: -1303, Heading: 60611},
		1: {X: 58090, Y: -91641, Z: -1303, Heading: 47274},
		2: {X: 58908, Y: -92556, Z: -1303, Heading: 34450},
		3: {X: 58336, Y: -93600, Z: -1303, Heading: 21100},
		4: {X: 57152, Y: -93360, Z: -1303, Heading: 8400},
		5: {X: 59116, Y: -93251, Z: -1302, Heading: 31000},
		6: {X: 56432, Y: -92864, Z: -1303, Heading: 64000},
	}
	return flagwar.New(flagwar.Config{
		HallID:    beastFarm,
		MakerName: "rune07_azit2115_11m1",

		RoyalFlag: 35606, FlagRed: 35607, FlagYellow: 35608, FlagGreen: 35609, FlagBlue: 35610, FlagPurple: 35611,

		Ally1: 35618, Ally2: 35619, Ally3: 35620, Ally4: 35621, Ally5: 35622,

		Teleport1: 35612,
		Messenger: 35627,

		OuterDoors: []int32{21150003, 21150004},
		InnerDoors: []int32{21150001, 21150002},
		FlagCoords: coords[:],
		Center:     script.Loc{X: 57762, Y: -92696, Z: -1359, Heading: 0},
	})
}

// flagHTML is the messenger's page of flag.
func flagHTML(flag int32) string {
	switch flag {
	case 35607:
		return "farm_kel_mahum_messenger_4a.htm"
	case 35608:
		return "farm_kel_mahum_messenger_4b.htm"
	case 35609:
		return "farm_kel_mahum_messenger_4c.htm"
	case 35610:
		return "farm_kel_mahum_messenger_4d.htm"
	case 35611:
		return "farm_kel_mahum_messenger_4e.htm"
	}
	return ""
}

// allyHTML is the messenger's page of the warrior ally.
func allyHTML(ally int32) string {
	switch ally {
	case 35618:
		return "farm_kel_mahum_messenger_17.htm"
	case 35619:
		return "farm_kel_mahum_messenger_18.htm"
	case 35620:
		return "farm_kel_mahum_messenger_19.htm"
	case 35621:
		return "farm_kel_mahum_messenger_20.htm"
	case 35622:
		return "farm_kel_mahum_messenger_23.htm"
	}
	return ""
}

// New is WildBeastReserve: the Wild Beast Reserve's flag war, whose
// messenger names the hall's owner and takes the registrations.
func New() script.Script {
	w := war()
	base := w.Hooks()
	return script.Script{Dir: "siegablehall", Bind: w.Bind(), BindHall: w.BindHall, Hooks: base.With(script.Hooks{
		OnFirstTalk: func(s *script.Script, e script.FirstTalk) string {
			var html string
			if e.NPC.NpcID() == w.Messenger {
				h := w.Hall(s)
				if !h.Unowned() {
					name, leader := "no owner", "no owner"
					if clan := s.Clan(h.OwnerID()); clan != nil {
						name, leader = clan.Name(), clan.LeaderName()
					}
					html = s.HTMLText("farm_messenger001.htm")
					html = strings.ReplaceAll(html, "%my_pledge_name%", name)
					html = strings.ReplaceAll(html, "%my_owner_name%", leader)
				} else {
					html = "farm_messenger002.htm"
				}
			}
			if html == "" {
				return base.FirstTalk(s, e)
			}
			return html
		},
		OnEvent: func(s *script.Script, e script.Event) string {
			return onEvent(s, w, e)
		},
	})}
}

// onEvent answers the messenger's links.
func onEvent(s *script.Script, w *flagwar.War, e script.Event) string {
	event, p := e.Name, e.Player
	html := event
	h := w.Hall(s)
	clan := s.Clan(p.ClanID())
	switch {
	case strings.EqualFold(event, "register"):
		if p.Overweight() {
			p.SystemMessage(msgInventoryLessThan80Percent)
			return ""
		}
		if h.WaitingBattle() {
			html = "farm_kel_mahum_messenger_1.htm"
		} else {
			html = datedPage(s, h, "farm_messenger_q0655_11.htm")
		}
	case strings.EqualFold(event, "view_participants"):
		html = registeredPledgeList(s, w, h)
	case strings.EqualFold(event, "register_clan"):
		switch {
		case !h.WaitingBattle():
			html = datedPage(s, h, "farm_messenger_q0655_11.htm")
		case clan != nil && h.OwnerID() == p.ClanID():
			html = "farm_kel_mahum_messenger_22.htm"
		default:
			html = clanRegisterStatus(s, w, h, p)
		}
	case strings.EqualFold(event, "register_clan_member"):
		switch {
		case !h.WaitingBattle():
			html = datedPage(s, h, "farm_messenger_q0655_11.htm")
		case !p.IsClanLeader():
			html = registerClanMember(w, h, p)
		default:
			html = "farm_kel_mahum_messenger_5.htm"
		}
	case strings.EqualFold(event, "select_clan_npc") || strings.EqualFold(event, "view_clan_npc"):
		if !h.WaitingBattle() {
			html = datedPage(s, h, "farm_messenger_q0655_11.htm")
		} else {
			html = npcType(s, w, h, p)
		}
	case strings.HasPrefix(event, "select_npc_"):
		// A link with no number answers with its own text.
		if n, err := strconv.ParseInt(event[len("select_npc_"):], 10, 32); err == nil {
			html = setNpcType(w, h, p, 35617+int32(n))
		}
	case strings.EqualFold(event, "reselect_npc"):
		html = s.HTMLText("farm_kel_mahum_messenger_6.htm")
	case strings.HasPrefix(event, "info_npc_"):
		if n, err := strconv.ParseInt(event[len("info_npc_"):], 10, 32); err == nil {
			return s.HTMLText(fmt.Sprintf("farm_kel_mahum_messenger_%d.htm", 10+int32(n)))
		}
	}
	return html
}

// datedPage is the page file with the hall's next siege date in place of
// %next_siege%.
func datedPage(s *script.Script, h *script.Hall, file string) string {
	return strings.ReplaceAll(s.HTMLText(file), "%next_siege%", h.SiegeDate().Format("2006-01-02 15:04:05"))
}

// maxAttackers is how many clans attack the hall at most: 5, and one more
// while it has no owner.
func maxAttackers(h *script.Hall) int {
	n := 5
	if h.OwnerID() == 0 {
		n++
	}
	return n
}

// checkAndRegisterClan registers the player's clan, taking its quest items
// and ending its quest, unless it is registered already, and answers with
// its flag's page.
func checkAndRegisterClan(s *script.Script, w *flagwar.War, h *script.Hall, p *script.Player) string {
	clan := s.Clan(p.ClanID())
	if !h.WaitingBattle() || clan == nil || len(h.Attackers()) >= maxAttackers(h) {
		return "farm_kel_mahum_messenger_3.htm"
	}
	cd, ok := w.Clan(clan.ID())
	if !ok {
		s.TakeItems(p, crystalOfPurity, -1)
		s.TakeItems(p, trainerLicense, 1)
		if st := s.QuestState(p, questName); st != nil {
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		}
		if cd, ok = w.RegisterClan(s, h, clan.ID()); !ok {
			panic(fmt.Sprintf("wildbeastreserve: clan %d owns another hall and holds no registration", clan.ID()))
		}
	}
	return flagHTML(cd.Flag)
}

// clanRegisterStatus answers a registration: a registered clan with its
// flag's page, else the refusals, the license paid, then the registration.
func clanRegisterStatus(s *script.Script, w *flagwar.War, h *script.Hall, p *script.Player) string {
	clan := s.Clan(p.ClanID())
	if cd, ok := w.Clan(p.ClanID()); clan != nil && ok {
		return flagHTML(cd.Flag)
	}
	if len(h.Attackers()) >= maxAttackers(h) {
		return "farm_kel_mahum_messenger_21.htm"
	}
	if clan != nil && h.OwnerID() == p.ClanID() {
		return "farm_kel_mahum_messenger_22.htm"
	}
	if !s.DestroyItems(p, trainerLicense, 1, true) {
		return "farm_kel_mahum_messenger_27.htm"
	}
	return checkAndRegisterClan(s, w, h, p)
}

// npcType answers with the warrior the player's clan picked, or what it
// may do about it.
func npcType(s *script.Script, w *flagwar.War, h *script.Hall, p *script.Player) string {
	clan := s.Clan(p.ClanID())
	if clan != nil && h.OwnerID() == clan.ID() {
		return "farm_kel_mahum_messenger_25.htm"
	}
	cd, ok := w.Clan(p.ClanID())
	switch {
	case clan == nil || !ok:
		return "farm_kel_mahum_messenger_7.htm"
	case cd.NPC != 0:
		return allyHTML(cd.NPC)
	case p.IsClanLeader():
		return "farm_kel_mahum_messenger_6.htm"
	}
	return "farm_kel_mahum_messenger_10.htm"
}

// setNpcType makes ally the warrior of the leader's clan, and stores it.
func setNpcType(w *flagwar.War, h *script.Hall, p *script.Player, ally int32) string {
	if _, ok := w.Clan(p.ClanID()); !p.IsClanLeader() || !ok {
		return "farm_kel_mahum_messenger_7.htm"
	}
	if ally >= 35618 && ally <= 35622 {
		w.SetNPC(h, p.ClanID(), ally)
		return "farm_kel_mahum_messenger_9.htm"
	}
	return ""
}

// registeredPledgeList is the list of the registered challengers and how
// many members each registered.
func registeredPledgeList(s *script.Script, w *flagwar.War, h *script.Hall) string {
	html := s.HTMLText("farm_messenger003.htm")
	i := 0
	for _, e := range w.Clans() {
		attacker := s.Clan(e.Key)
		if attacker == nil || attacker.ID() == h.OwnerID() {
			continue
		}
		html = strings.ReplaceAll(html, "%clan"+strconv.Itoa(i)+"%", attacker.Name())
		html = strings.ReplaceAll(html, "%clanMem"+strconv.Itoa(i)+"%", strconv.Itoa(e.Value.Players.Len()))
		i++
	}
	if w.Registered() < 5 {
		for c := range 5 {
			html = strings.ReplaceAll(html, "%clan"+strconv.Itoa(c)+"%", "**unregistered**")
			html = strings.ReplaceAll(html, "%clanMem"+strconv.Itoa(c)+"%", "")
		}
	}
	return html
}

// registerClanMember registers the player for its clan, up to 18 members,
// while the siege takes registrations.
func registerClanMember(w *flagwar.War, h *script.Hall, p *script.Player) string {
	cd, ok := w.Clan(p.ClanID())
	if !ok || !h.Registering() {
		return "farm_kel_mahum_messenger_7.htm"
	}
	if cd.Players.Len() >= 18 {
		return "farm_kel_mahum_messenger_8.htm"
	}
	w.AddMember(h, p.ClanID(), p.ObjectID())
	return "farm_kel_mahum_messenger_9.htm"
}
