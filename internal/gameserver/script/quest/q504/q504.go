// Package q504 is the quest Competition for the Bandit Stronghold: while the
// Bandit Stronghold waits for its battle, the leader of a level 4 clan with
// no clan hall brings the messenger thirty Tarlk amulets for the Trophy of
// Alliance.
package q504

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q504_CompetitionForTheBanditStronghold"

// banditStronghold is the siegable clan hall the quest waits on.
const banditStronghold = 35

// messenger is the quest giver.
const messenger = 35437

// Items.
const (
	tarlkAmulet        = 4332
	contestCertificate = 4333
	trophyOfAlliance   = 5009
)

// siegeDateLayout is how the pages print the next siege's date.
const siegeDateLayout = "2006-01-02 15:04:05"

// dropChance returns the amulet drop chance of the Tarlk monster npcID, 0
// for another.
func dropChance(npcID int32) int32 {
	switch npcID {
	case 20570: // Tarlk Bugbear
		return 600000
	case 20571: // Tarlk Bugbear Warrior
		return 700000
	case 20572: // Tarlk Bugbear High Warrior
		return 800000
	case 20573: // Tarlk Basilisk
		return 900000
	case 20574: // Elder Tarlk Basilisk
		return 700000
	}
	return 0
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Competition for the Bandit Stronghold",
		QuestID: 504,
		Bind: script.Bindings{
			script.EventQuestStart: {messenger},
			script.EventTalked:     {messenger},
			script.EventMyDying:    {20570, 20571, 20572, 20573, 20574},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	if strings.EqualFold(e.Name, "35437-02.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
		s.GiveItems(e.Player, contestCertificate, 1)
	}
	return htmltext
}

// onTalk answers with the next siege's date until the hall waits for its
// battle, then gates on the clan before the quest's own steps. The created
// step's date page cannot show, as the first gate already caught it.
func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	p := e.Player
	hall := s.SiegableHall(banditStronghold)
	switch {
	case !hall.WaitingBattle():
		htmltext = strings.ReplaceAll(s.HTMLText("35437-09.htm"), "%nextSiege%", hall.SiegeDate().Format(siegeDateLayout))
	case !p.InClan() || p.ClanLevel() < 4:
		htmltext = "35437-04.htm"
	case !p.IsClanLeader():
		htmltext = "35437-05.htm"
	case p.ClanHallID() > 0:
		htmltext = "35437-10.htm"
	case p.HasAtLeastOneItem(trophyOfAlliance):
		htmltext = "35437-07a.htm"
	default:
		switch st.Status() {
		case questlog.StatusCreated:
			switch {
			case !hall.WaitingBattle():
				htmltext = strings.ReplaceAll(s.HTMLText("35437-03.htm"), "%nextSiege%", hall.SiegeDate().Format(siegeDateLayout))
			case p.HasAtLeastOneItem(contestCertificate):
				s.TakeItems(p, contestCertificate, -1)
				s.TakeItems(p, tarlkAmulet, -1)
				s.TakeItems(p, trophyOfAlliance, -1)
				htmltext = "35437-06.htm"
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			default:
				htmltext = "35437-01.htm"
			}
		case questlog.StatusStarted:
			if p.ItemCount(tarlkAmulet) < 30 {
				htmltext = "35437-07.htm"
			} else {
				htmltext = "35437-08.htm"
				s.TakeItems(p, tarlkAmulet, 30)
				s.TakeItems(p, contestCertificate, -1)
				s.TakeItems(p, trophyOfAlliance, -1)
				s.RewardItems(p, trophyOfAlliance, 1)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case questlog.StatusCompleted:
			htmltext = "35437-07a.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItems(p, tarlkAmulet, 1, 30, dropChance(e.NPC.NpcID()))
}
