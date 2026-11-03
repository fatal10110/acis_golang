// Package q405 is the quest Path to a Cleric: a human mystic earns the Mark
// of Fate by gathering the books of three scholars, delivering Lionel's
// book to Gallint and bringing back the covenant.
package q405

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q405_PathToACleric"

// Quest items.
const (
	letterOfOrder1       = 1191
	letterOfOrder2       = 1192
	lionelBook           = 1193
	bookOfVivyan         = 1194
	bookOfSimplon        = 1195
	bookOfPraga          = 1196
	certificateOfGallint = 1197
	pendantOfMother      = 1198
	necklaceOfMother     = 1199
	lionelCovenant       = 1200
)

// Npcs.
const (
	gallint = 30017
	zigaunt = 30022
	vivyan  = 30030
	praga   = 30333
	simplon = 30253
	lionel  = 30408
)

// Monsters.
const (
	ruinZombieLeader = 20029
	ruinZombie       = 20026
)

// The reward.
const markOfFate = 1201

// The classes the quest checks.
const (
	classHumanMystic = 10
	classCleric      = 15
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Path to a Cleric",
		QuestID: 405,
		Items: []int32{
			letterOfOrder1, bookOfSimplon, bookOfPraga, bookOfVivyan, necklaceOfMother, pendantOfMother, letterOfOrder2,
			lionelBook, certificateOfGallint, lionelCovenant,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {zigaunt},
			script.EventTalked:     {zigaunt, simplon, praga, vivyan, lionel, gallint},
			script.EventMyDying:    {ruinZombieLeader, ruinZombie},
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
	p := e.Player
	if strings.EqualFold(e.Name, "30022-05.htm") {
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, letterOfOrder1, 1)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case p.ClassID() != classHumanMystic:
			if p.ClassID() == classCleric {
				htmltext = "30022-02a.htm"
			} else {
				htmltext = "30022-02.htm"
			}
		case p.Level() < 19:
			htmltext = "30022-03.htm"
		case p.HasItem(markOfFate):
			htmltext = "30022-04.htm"
		default:
			htmltext = "30022-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case zigaunt:
			switch {
			case cond == 1:
				htmltext = "30022-06.htm"
			case cond == 2:
				htmltext = "30022-08.htm"
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, bookOfPraga, 1)
				s.TakeItems(p, bookOfVivyan, 1)
				s.TakeItems(p, bookOfSimplon, 3)
				s.TakeItems(p, letterOfOrder1, 1)
				s.GiveItems(p, letterOfOrder2, 1)
			case cond > 2 && cond < 6:
				htmltext = "30022-07.htm"
			case cond == 6:
				htmltext = "30022-09.htm"
				s.TakeItems(p, letterOfOrder2, 1)
				s.TakeItems(p, lionelCovenant, 1)
				s.GiveItems(p, markOfFate, 1)
				s.RewardExpAndSp(p, 3200, 5610)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case simplon:
			if cond == 1 && !p.HasItem(bookOfSimplon) {
				htmltext = "30253-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, bookOfSimplon, 3)
			} else if cond > 1 || p.HasItem(bookOfSimplon) {
				htmltext = "30253-02.htm"
			}
		case praga:
			if cond == 1 {
				if !p.HasItem(bookOfPraga) && !p.HasItem(necklaceOfMother) && p.HasItem(bookOfSimplon) {
					htmltext = "30333-01.htm"
					s.PlaySound(p, script.SoundItemGet)
					s.GiveItems(p, necklaceOfMother, 1)
				} else if !p.HasItem(pendantOfMother) {
					htmltext = "30333-02.htm"
				} else if p.HasItem(pendantOfMother) {
					htmltext = "30333-03.htm"
					s.TakeItems(p, necklaceOfMother, 1)
					s.TakeItems(p, pendantOfMother, 1)
					s.GiveItems(p, bookOfPraga, 1)

					if p.HasItem(bookOfVivyan) {
						st.SetCond(2)
						s.PlaySound(p, script.SoundMiddle)
					} else {
						s.PlaySound(p, script.SoundItemGet)
					}
				}
			} else if cond > 1 || p.HasItem(bookOfPraga) {
				htmltext = "30333-04.htm"
			}
		case vivyan:
			if cond == 1 && !p.HasItem(bookOfVivyan) && p.HasItem(bookOfSimplon) {
				htmltext = "30030-01.htm"
				s.GiveItems(p, bookOfVivyan, 1)

				if p.HasItem(bookOfPraga) {
					st.SetCond(2)
					s.PlaySound(p, script.SoundMiddle)
				} else {
					s.PlaySound(p, script.SoundItemGet)
				}
			} else if cond > 1 || p.HasItem(bookOfVivyan) {
				htmltext = "30030-02.htm"
			}
		case lionel:
			switch {
			case cond < 3:
				htmltext = "30408-02.htm"
			case cond == 3:
				htmltext = "30408-01.htm"
				st.SetCond(4)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, lionelBook, 1)
			case cond == 4:
				htmltext = "30408-03.htm"
			case cond == 5:
				htmltext = "30408-04.htm"
				st.SetCond(6)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, certificateOfGallint, 1)
				s.GiveItems(p, lionelCovenant, 1)
			case cond == 6:
				htmltext = "30408-05.htm"
			}
		case gallint:
			if cond == 4 {
				htmltext = "30017-01.htm"
				st.SetCond(5)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, lionelBook, 1)
				s.GiveItems(p, certificateOfGallint, 1)
			} else if cond > 4 {
				htmltext = "30017-02.htm"
			}
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerCondition(p, e.NPC, 1) == nil {
		return
	}
	if p.HasItem(necklaceOfMother) && !p.HasItem(pendantOfMother) {
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, pendantOfMother, 1)
	}
}
