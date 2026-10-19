// Package q108 is the quest Jumble, Tumble, Diamond Fuss: a dwarf runs
// Gouph's contract through Reep, Torocco, Maron, Brunon, Murdoc and Airy,
// gathering gems from the goblin brigands and a star diamond from the
// blade bats, for the Silversmith Hammer.
package q108

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q108_JumbleTumbleDiamondFuss"

// NPCs.
const (
	gouph   = 30523
	reep    = 30516
	murdoc  = 30521
	airy    = 30522
	brunon  = 30526
	maron   = 30529
	torocco = 30555
)

// Items.
const (
	gouphContract  = 1559
	reepContract   = 1560
	elvenWine      = 1561
	brunonDice     = 1562
	brunonContract = 1563
	aquamarine     = 1564
	chrysoberyl    = 1565
	gemBox         = 1566
	coalPiece      = 1567
	brunonLetter   = 1568
	berryTart      = 1569
	batDiagram     = 1570
	starDiamond    = 1571
)

// Monsters.
const (
	goblinBrigandLeader     = 20323
	goblinBrigandLieutenant = 20324
	bladeBat                = 20480
)

// Rewards.
const (
	lesserHealingPot  = 1060
	silversmithHammer = 1511
	echoBattle        = 4412
	echoLove          = 4413
	echoSolitude      = 4414
	echoFeast         = 4415
	echoCelebration   = 4416
)

// leaderDroplist is the goblin brigand leader's gems.
func leaderDroplist() []script.DropInfo {
	return []script.DropInfo{
		{ItemID: aquamarine, Count: 1, Needed: 10, Chance: 800000},
		{ItemID: chrysoberyl, Count: 1, Needed: 10, Chance: 800000},
	}
}

// lieutenantDroplist is the goblin brigand lieutenant's gems.
func lieutenantDroplist() []script.DropInfo {
	return []script.DropInfo{
		{ItemID: aquamarine, Count: 1, Needed: 10, Chance: 600000},
		{ItemID: chrysoberyl, Count: 1, Needed: 10, Chance: 600000},
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Jumble, Tumble, Diamond Fuss",
		QuestID: 108,
		Items: []int32{
			gouphContract, reepContract, elvenWine, brunonDice, brunonContract, aquamarine, chrysoberyl,
			gemBox, coalPiece, brunonLetter, berryTart, batDiagram, starDiamond,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {gouph},
			script.EventTalked:     {gouph, reep, murdoc, airy, brunon, maron, torocco},
			script.EventMyDying:    {goblinBrigandLeader, goblinBrigandLieutenant, bladeBat},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	st := s.QuestState(e.Player, questName)
	htmltext := e.Name
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30523-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, gouphContract, 1)
	case strings.EqualFold(e.Name, "30555-02.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, reepContract, 1)
		s.GiveItems(p, elvenWine, 1)
	case strings.EqualFold(e.Name, "30526-02.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, brunonDice, 1)
		s.GiveItems(p, brunonContract, 1)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case p.Race() != player.RaceDwarf:
			htmltext = "30523-00.htm"
		case p.Level() < 10:
			htmltext = "30523-01.htm"
		default:
			htmltext = "30523-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case gouph:
			htmltext = talkGouph(s, p, st, cond, htmltext)
		case reep:
			switch {
			case cond == 1:
				htmltext = "30516-01.htm"
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, gouphContract, 1)
				s.GiveItems(p, reepContract, 1)
			case cond > 1:
				htmltext = "30516-02.htm"
			}
		case torocco:
			switch {
			case cond == 2:
				htmltext = "30555-01.htm"
			case cond == 3:
				htmltext = "30555-03.htm"
			case cond == 7:
				htmltext = "30555-04.htm"
			case cond > 7:
				htmltext = "30555-05.htm"
			}
		case maron:
			switch {
			case cond == 3:
				htmltext = "30529-01.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, elvenWine, 1)
				s.GiveItems(p, brunonDice, 1)
			case cond == 4:
				htmltext = "30529-02.htm"
			case cond > 4:
				htmltext = "30529-03.htm"
			}
		case brunon:
			htmltext = talkBrunon(s, p, st, cond, htmltext)
		case murdoc:
			switch {
			case cond == 9:
				htmltext = "30521-01.htm"
				st.SetCond(10)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, brunonLetter, 1)
				s.GiveItems(p, berryTart, 1)
			case cond == 10:
				htmltext = "30521-02.htm"
			case cond > 10:
				htmltext = "30521-03.htm"
			}
		case airy:
			switch cond {
			case 10:
				htmltext = "30522-01.htm"
				st.SetCond(11)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, berryTart, 1)
				s.GiveItems(p, batDiagram, 1)
			case 11:
				if s.RndBool() {
					htmltext = "30522-02.htm"
				} else {
					htmltext = "30522-04.htm"
				}
			case 12:
				htmltext = "30522-03.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// talkGouph is Gouph's answer on a started quest of condition cond;
// htmltext is the answer when no condition matches.
func talkGouph(s *script.Script, p *script.Player, st *script.QuestState, cond int32, htmltext string) string {
	switch {
	case cond == 1:
		htmltext = "30523-04.htm"
	case cond > 1 && cond < 7:
		htmltext = "30523-05.htm"
	case cond == 7:
		htmltext = "30523-06.htm"
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, gemBox, 1)
		s.GiveItems(p, coalPiece, 1)
	case cond > 7 && cond < 12:
		htmltext = "30523-07.htm"
	case cond == 12:
		htmltext = "30523-08.htm"
		s.TakeItems(p, starDiamond, -1)
		s.GiveItems(p, silversmithHammer, 1)

		s.RewardNewbieShots(p, 7000, 3000)
		s.RewardItems(p, lesserHealingPot, 100)
		s.RewardItems(p, echoBattle, 10)
		s.RewardItems(p, echoLove, 10)
		s.RewardItems(p, echoSolitude, 10)
		s.RewardItems(p, echoFeast, 10)
		s.RewardItems(p, echoCelebration, 10)

		p.SocialAction(3)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	}
	return htmltext
}

// talkBrunon is Brunon's answer on a started quest of condition cond;
// htmltext is the answer when no condition matches.
func talkBrunon(s *script.Script, p *script.Player, st *script.QuestState, cond int32, htmltext string) string {
	switch {
	case cond == 4:
		htmltext = "30526-01.htm"
	case cond == 5:
		htmltext = "30526-03.htm"
	case cond == 6:
		htmltext = "30526-04.htm"
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, brunonContract, 1)
		s.TakeItems(p, aquamarine, -1)
		s.TakeItems(p, chrysoberyl, -1)
		s.GiveItems(p, gemBox, 1)
	case cond == 7:
		htmltext = "30526-05.htm"
	case cond == 8:
		htmltext = "30526-06.htm"
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, coalPiece, 1)
		s.GiveItems(p, brunonLetter, 1)
	case cond == 9:
		htmltext = "30526-07.htm"
	case cond > 9:
		htmltext = "30526-08.htm"
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	switch e.NPC.NpcID() {
	case goblinBrigandLeader:
		if st.Cond() == 5 && s.DropMultipleItems(p, leaderDroplist(), script.DropDivmod) {
			st.SetCond(6)
		}
	case goblinBrigandLieutenant:
		if st.Cond() == 5 && s.DropMultipleItems(p, lieutenantDroplist(), script.DropDivmod) {
			st.SetCond(6)
		}
	case bladeBat:
		if st.Cond() == 11 && s.DropItems(p, starDiamond, 1, 1, 200000) {
			s.TakeItems(p, batDiagram, 1)
			st.SetCond(12)
		}
	}
}
