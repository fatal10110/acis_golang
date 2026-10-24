// Package banditstronghold is the Bandit Stronghold's hall script: its flag
// war, and the registration its Oel Mahum messenger takes. A clan leader
// of a level 4 clan registers by handing in the Trophy of Alliance the
// Bandit Stronghold quest gives, or 200,000 adena; the first five clans
// fight, each picking the warrior it defends.
package banditstronghold

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/siegablehall/flagwar"
)

// questName is the quest whose trophy registers a clan.
const questName = "Q504_CompetitionForTheBanditStronghold"

// The items a registration takes.
const (
	tarlkAmulet        = 4332
	contestCertificate = 4333
	trophyOfAlliance   = 5009
)

// msgInventoryLessThan80Percent refuses a registration to an overweight
// player.
const msgInventoryLessThan80Percent = 1118

// war returns the Bandit Stronghold's flag war.
func war() *flagwar.War {
	coords := [7]script.Loc{
		0: {X: 83699, Y: -17468, Z: -1774, Heading: 19048},
		1: {X: 82053, Y: -17060, Z: -1784, Heading: 5432},
		2: {X: 82142, Y: -15528, Z: -1799, Heading: 58792},
		3: {X: 83544, Y: -15266, Z: -1770, Heading: 44976},
		4: {X: 84609, Y: -16041, Z: -1769, Heading: 35816},
		5: {X: 81981, Y: -15708, Z: -1858, Heading: 60392},
		6: {X: 84375, Y: -17060, Z: -1860, Heading: 27712},
	}
	return flagwar.New(flagwar.Config{
		HallID:    clanhall.BanditStronghold,
		MakerName: "oren15_azit_teleporter01",

		RoyalFlag: 35422, FlagRed: 35423, FlagYellow: 35424, FlagGreen: 35425, FlagBlue: 35426, FlagPurple: 35427,

		Ally1: 35428, Ally2: 35429, Ally3: 35430, Ally4: 35431, Ally5: 35432,

		Teleport1: 35560,
		Messenger: 35437,

		OuterDoors: []int32{22170001, 22170002},
		InnerDoors: []int32{22170003, 22170004},
		FlagCoords: coords[:],
		Center:     script.Loc{X: 82882, Y: -16280, Z: -1894, Heading: 0},
	})
}

// flagHTML is the messenger's page of flag.
func flagHTML(flag int32) string {
	switch flag {
	case 35423:
		return "agit_oel_mahum_messeger_4a.htm"
	case 35424:
		return "agit_oel_mahum_messeger_4b.htm"
	case 35425:
		return "agit_oel_mahum_messeger_4c.htm"
	case 35426:
		return "agit_oel_mahum_messeger_4d.htm"
	case 35427:
		return "agit_oel_mahum_messeger_4e.htm"
	}
	return ""
}

// allyHTML is the messenger's page of the warrior ally.
func allyHTML(ally int32) string {
	switch ally {
	case 35428:
		return "agit_oel_mahum_messeger_17.htm"
	case 35429:
		return "agit_oel_mahum_messeger_18.htm"
	case 35430:
		return "agit_oel_mahum_messeger_19.htm"
	case 35431:
		return "agit_oel_mahum_messeger_20.htm"
	case 35432:
		return "agit_oel_mahum_messeger_23.htm"
	}
	return ""
}

// New is BanditStronghold: the Bandit Stronghold's flag war, whose
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
					html = s.HTMLText("azit_messenger001.htm")
					html = strings.ReplaceAll(html, "%my_pledge_name%", name)
					html = strings.ReplaceAll(html, "%my_owner_name%", leader)
				} else {
					html = "azit_messenger002.htm"
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
			html = "agit_oel_mahum_messeger_1.htm"
		} else {
			html = datedPage(s, h, "azit_messenger_q0504_09.htm")
		}
	case strings.EqualFold(event, "view_participants"):
		html = registeredPledgeList(s, w, h)
	case strings.EqualFold(event, "register_clan"):
		switch {
		case !h.WaitingBattle():
			html = datedPage(s, h, "azit_messenger_q0504_03.htm")
		case clan == nil || h.OwnerID() != p.ClanID():
			html = clanRegisterStatus(s, w, h, p, false)
		default:
			html = "agit_oel_mahum_messeger_22.htm"
		}
	case strings.EqualFold(event, "register_clan_member"):
		switch {
		case !h.WaitingBattle():
			html = datedPage(s, h, "azit_messenger_q0504_03.htm")
		case !p.IsClanLeader():
			html = registerClanMember(w, h, p)
		default:
			html = "agit_oel_mahum_messeger_5.htm"
		}
	case strings.EqualFold(event, "register_with_adena"):
		if !h.WaitingBattle() {
			html = datedPage(s, h, "azit_messenger_q0504_03.htm")
		} else {
			html = clanRegisterStatus(s, w, h, p, true)
		}
	case strings.EqualFold(event, "select_clan_npc") || strings.EqualFold(event, "view_clan_npc"):
		if !h.WaitingBattle() {
			html = datedPage(s, h, "azit_messenger_q0504_03.htm")
		} else {
			html = npcType(s, w, h, p)
		}
	case strings.HasPrefix(event, "select_npc_"):
		// A link with no number answers with its own text.
		if n, err := strconv.ParseInt(event[len("select_npc_"):], 10, 32); err == nil {
			html = setNpcType(w, h, p, 35427+int32(n))
		}
	case strings.EqualFold(event, "reselect_npc"):
		html = s.HTMLText("agit_oel_mahum_messeger_6.htm")
	case strings.HasPrefix(event, "info_npc_"):
		if n, err := strconv.ParseInt(event[len("info_npc_"):], 10, 32); err == nil {
			return s.HTMLText(fmt.Sprintf("agit_oel_mahum_messeger_%d.htm", 10+int32(n)))
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
		return "agit_oel_mahum_messeger_3.htm"
	}
	cd, ok := w.Clan(clan.ID())
	if !ok {
		s.TakeItems(p, tarlkAmulet, 30)
		s.TakeItems(p, contestCertificate, -1)
		s.TakeItems(p, trophyOfAlliance, -1)
		if st := s.QuestState(p, questName); st != nil {
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		}
		if cd, ok = w.RegisterClan(s, h, clan.ID()); !ok {
			panic(fmt.Sprintf("banditstronghold: clan %d owns another hall and holds no registration", clan.ID()))
		}
	}
	return flagHTML(cd.Flag)
}

// clanRegisterStatus answers a registration: a registered clan with its
// flag's page, else the refusals, the trophy or the adena paid, then the
// registration.
func clanRegisterStatus(s *script.Script, w *flagwar.War, h *script.Hall, p *script.Player, withAdena bool) string {
	clan := s.Clan(p.ClanID())
	if cd, ok := w.Clan(p.ClanID()); clan != nil && ok {
		return flagHTML(cd.Flag)
	}
	if len(h.Attackers()) >= maxAttackers(h) {
		return "agit_oel_mahum_messeger_21.htm"
	}
	if clan != nil && h.OwnerID() == p.ClanID() {
		return "agit_oel_mahum_messeger_22.htm"
	}
	if !withAdena {
		if !s.DestroyItems(p, trophyOfAlliance, 1, true) {
			return "agit_oel_mahum_messeger_24.htm"
		}
	} else {
		switch {
		case clan == nil || clan.HasHall():
			return "azit_messenger_q0504_10.htm"
		case !p.IsClanLeader():
			return "azit_messenger_q0504_05.htm"
		case p.ClanLevel() < 4:
			return "azit_messenger_q0504_04.htm"
		case !p.ReduceAdena(200000):
			return "agit_oel_mahum_messeger_26.htm"
		}
	}
	return checkAndRegisterClan(s, w, h, p)
}

// npcType answers with the warrior the player's clan picked, or what it
// may do about it.
func npcType(s *script.Script, w *flagwar.War, h *script.Hall, p *script.Player) string {
	clan := s.Clan(p.ClanID())
	if clan != nil && h.OwnerID() == clan.ID() {
		return "agit_oel_mahum_messeger_25.htm"
	}
	cd, ok := w.Clan(p.ClanID())
	switch {
	case clan == nil || !ok:
		return "agit_oel_mahum_messeger_7.htm"
	case cd.NPC != 0:
		return allyHTML(cd.NPC)
	case p.IsClanLeader():
		return "agit_oel_mahum_messeger_6.htm"
	}
	return "agit_oel_mahum_messeger_10.htm"
}

// setNpcType makes ally the warrior of the leader's clan, and stores it.
func setNpcType(w *flagwar.War, h *script.Hall, p *script.Player, ally int32) string {
	if _, ok := w.Clan(p.ClanID()); !p.IsClanLeader() || !ok {
		return "agit_oel_mahum_messeger_7.htm"
	}
	if ally >= 35428 && ally <= 35432 {
		w.SetNPC(h, p.ClanID(), ally)
		return "agit_oel_mahum_messeger_9.htm"
	}
	return ""
}

// registeredPledgeList is the list of the registered challengers and how
// many members each registered.
func registeredPledgeList(s *script.Script, w *flagwar.War, h *script.Hall) string {
	html := s.HTMLText("azit_messenger003.htm")
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
		return "agit_oel_mahum_messeger_7.htm"
	}
	if cd.Players.Len() >= 18 {
		return "agit_oel_mahum_messeger_8.htm"
	}
	w.AddMember(h, p.ClanID(), p.ObjectID())
	return "agit_oel_mahum_messeger_9.htm"
}
