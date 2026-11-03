// Package q218 is the quest Testimony of Life, a second class-change
// testimony for elves: Cardien sends the traveller to Hierarch Asterios,
// Thalia, Pushkin, Arkenia, Adonius and Isael for the Water of Life that
// earns the Mark of Life.
package q218

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q218_TestimonyOfLife"

// NPCs.
const (
	asterios = 30154
	pushkin  = 30300
	thalia   = 30371
	adonius  = 30375
	arkenia  = 30419
	cardien  = 30460
	isael    = 30655
)

// Items.
const (
	talinsSpear           = 3026
	cardienLetter         = 3141
	camomileCharm         = 3142
	hierarchLetter        = 3143
	moonflowerCharm       = 3144
	grailDiagram          = 3145
	thaliaLetter1         = 3146
	thaliaLetter2         = 3147
	thaliaInstructions    = 3148
	pushkinList           = 3149
	pureMithrilCup        = 3150
	arkeniaContract       = 3151
	arkeniaInstructions   = 3152
	adoniusList           = 3153
	andarielScriptureCopy = 3154
	stardust              = 3155
	isaelInstructions     = 3156
	isaelLetter           = 3157
	grailOfPurity         = 3158
	tearsOfUnicorn        = 3159
	waterOfLife           = 3160
	pureMithrilOre        = 3161
	antSoldierAcid        = 3162
	wyrmTalon             = 3163
	spiderIchor           = 3164
	harpyDown             = 3165
)

// talinsPieces returns the six pieces of Talin's spear, in the order the
// lizardmen give them.
func talinsPieces() []int32 {
	return []int32{
		3166,
		3167,
		3168,
		3169,
		3170,
		3171,
	}
}

// Rewards.
const markOfLife = 3140

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Testimony of Life",
		QuestID: 218,
		Items: []int32{
			talinsSpear, cardienLetter, camomileCharm, hierarchLetter, moonflowerCharm, grailDiagram, thaliaLetter1,
			thaliaLetter2, thaliaInstructions, pushkinList, pureMithrilCup, arkeniaContract, arkeniaInstructions,
			adoniusList, andarielScriptureCopy, stardust, isaelInstructions, isaelLetter, grailOfPurity,
			tearsOfUnicorn, waterOfLife, pureMithrilOre, antSoldierAcid, wyrmTalon, spiderIchor, harpyDown,
			3166, 3167, 3168, 3169, 3170, 3171,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {cardien},
			script.EventTalked:     {asterios, pushkin, thalia, adonius, arkenia, cardien, isael},
			script.EventMyDying:    {20145, 20176, 20233, 27077, 20550, 20581, 20582, 20082, 20084, 20086, 20087, 20088},
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
	switch {
	case strings.EqualFold(e.Name, "30460-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, cardienLetter, 1)
		if secondclass.GiveDimensionalDiamonds37(s, p) {
			htmltext = "30460-04a.htm"
		}
	case strings.EqualFold(e.Name, "30154-07.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, cardienLetter, 1)
		s.GiveItems(p, hierarchLetter, 1)
		s.GiveItems(p, moonflowerCharm, 1)
	case strings.EqualFold(e.Name, "30371-03.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, hierarchLetter, 1)
		s.GiveItems(p, grailDiagram, 1)
	case strings.EqualFold(e.Name, "30371-11.htm"):
		s.TakeItems(p, stardust, 1)
		s.PlaySound(p, script.SoundMiddle)
		if p.Level() < 38 {
			htmltext = "30371-10.htm"
			st.SetCond(13)
			s.GiveItems(p, thaliaInstructions, 1)
		} else {
			st.SetCond(14)
			s.GiveItems(p, thaliaLetter2, 1)
		}
	case strings.EqualFold(e.Name, "30300-06.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, grailDiagram, 1)
		s.GiveItems(p, pushkinList, 1)
	case strings.EqualFold(e.Name, "30300-10.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, pushkinList, 1)
		s.TakeItems(p, antSoldierAcid, -1)
		s.TakeItems(p, pureMithrilOre, -1)
		s.TakeItems(p, wyrmTalon, -1)
		s.GiveItems(p, pureMithrilCup, 1)
	case strings.EqualFold(e.Name, "30419-04.htm"):
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, thaliaLetter1, 1)
		s.GiveItems(p, arkeniaContract, 1)
		s.GiveItems(p, arkeniaInstructions, 1)
	case strings.EqualFold(e.Name, "30375-02.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, arkeniaInstructions, 1)
		s.GiveItems(p, adoniusList, 1)
	case strings.EqualFold(e.Name, "30655-02.htm"):
		st.SetCond(15)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, thaliaLetter2, 1)
		s.GiveItems(p, isaelInstructions, 1)
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
		case p.Race() != player.RaceElf:
			htmltext = "30460-01.htm"
		case p.Level() < 37 || p.ClassLevel() != 1:
			htmltext = "30460-02.htm"
		default:
			htmltext = "30460-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case asterios:
			switch cond {
			case 1:
				htmltext = "30154-01.htm"
			case 2:
				htmltext = "30154-08.htm"
			case 20:
				htmltext = "30154-09.htm"
				st.SetCond(21)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, moonflowerCharm, 1)
				s.TakeItems(p, waterOfLife, 1)
				s.GiveItems(p, camomileCharm, 1)
			case 21:
				htmltext = "30154-10.htm"
			}
		case pushkin:
			switch {
			case cond == 3:
				htmltext = "30300-01.htm"
			case cond == 4:
				htmltext = "30300-07.htm"
			case cond == 5:
				htmltext = "30300-08.htm"
			case cond == 6:
				htmltext = "30300-11.htm"
			case cond > 6:
				htmltext = "30300-12.htm"
			}
		case thalia:
			switch {
			case cond == 2:
				htmltext = "30371-01.htm"
			case cond == 3:
				htmltext = "30371-04.htm"
			case cond > 3 && cond < 6:
				htmltext = "30371-05.htm"
			case cond == 6:
				htmltext = "30371-06.htm"
				st.SetCond(7)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, pureMithrilCup, 1)
				s.GiveItems(p, thaliaLetter1, 1)
			case cond == 7:
				htmltext = "30371-07.htm"
			case cond > 7 && cond < 12:
				htmltext = "30371-08.htm"
			case cond == 12:
				htmltext = "30371-09.htm"
			case cond == 13:
				if p.Level() < 38 {
					htmltext = "30371-12.htm"
				} else {
					htmltext = "30371-13.htm"
					st.SetCond(14)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, thaliaInstructions, 1)
					s.GiveItems(p, thaliaLetter2, 1)
				}
			case cond == 14:
				htmltext = "30371-14.htm"
			case cond > 14 && cond < 17:
				htmltext = "30371-15.htm"
			case cond == 17:
				htmltext = "30371-16.htm"
				st.SetCond(18)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, isaelLetter, 1)
				s.GiveItems(p, grailOfPurity, 1)
			case cond == 18:
				htmltext = "30371-17.htm"
			case cond == 19:
				htmltext = "30371-18.htm"
				st.SetCond(20)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, tearsOfUnicorn, 1)
				s.GiveItems(p, waterOfLife, 1)
			case cond > 19:
				htmltext = "30371-19.htm"
			}
		case adonius:
			switch {
			case cond == 8:
				htmltext = "30375-01.htm"
			case cond == 9:
				htmltext = "30375-03.htm"
			case cond == 10:
				htmltext = "30375-04.htm"
				st.SetCond(11)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, adoniusList, 1)
				s.TakeItems(p, harpyDown, -1)
				s.TakeItems(p, spiderIchor, -1)
				s.GiveItems(p, andarielScriptureCopy, 1)
			case cond == 11:
				htmltext = "30375-05.htm"
			case cond > 11:
				htmltext = "30375-06.htm"
			}
		case arkenia:
			switch {
			case cond == 7:
				htmltext = "30419-01.htm"
			case cond > 7 && cond < 11:
				htmltext = "30419-05.htm"
			case cond == 11:
				htmltext = "30419-06.htm"
				st.SetCond(12)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, andarielScriptureCopy, 1)
				s.TakeItems(p, arkeniaContract, 1)
				s.GiveItems(p, stardust, 1)
			case cond == 12:
				htmltext = "30419-07.htm"
			case cond > 12:
				htmltext = "30419-08.htm"
			}
		case cardien:
			switch {
			case cond == 1:
				htmltext = "30460-05.htm"
			case cond > 1 && cond < 21:
				htmltext = "30460-06.htm"
			case cond == 21:
				htmltext = "30460-07.htm"
				s.TakeItems(p, camomileCharm, 1)
				s.GiveItems(p, markOfLife, 1)
				s.RewardExpAndSp(p, 104591, 11250)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case isael:
			switch {
			case cond == 14:
				htmltext = "30655-01.htm"
			case cond == 15:
				htmltext = "30655-03.htm"
			case cond == 16:
				if p.HasItems(talinsPieces()...) {
					htmltext = "30655-04.htm"
					st.SetCond(17)
					s.PlaySound(p, script.SoundMiddle)
					for _, itemID := range talinsPieces() {
						s.TakeItems(p, itemID, 1)
					}
					s.TakeItems(p, isaelInstructions, 1)
					s.GiveItems(p, isaelLetter, 1)
					s.GiveItems(p, talinsSpear, 1)
				} else {
					htmltext = "30655-03.htm"
				}
			case cond == 17:
				htmltext = "30655-05.htm"
			case cond > 17:
				htmltext = "30655-06.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
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
	case 20550:
		if st.Cond() == 4 && s.DropItems(p, pureMithrilOre, 1, 10, 500000) && p.ItemCount(wyrmTalon) >= 20 && p.ItemCount(antSoldierAcid) >= 20 {
			st.SetCond(5)
		}
	case 20176:
		if st.Cond() == 4 && s.DropItems(p, wyrmTalon, 1, 20, 500000) && p.ItemCount(pureMithrilOre) >= 10 && p.ItemCount(antSoldierAcid) >= 20 {
			st.SetCond(5)
		}
	case 20082, 20084, 20086, 20087, 20088:
		if st.Cond() == 4 && s.DropItems(p, antSoldierAcid, 1, 20, 800000) && p.ItemCount(pureMithrilOre) >= 10 && p.ItemCount(wyrmTalon) >= 20 {
			st.SetCond(5)
		}
	case 20233:
		if st.Cond() == 9 && s.DropItems(p, spiderIchor, 1, 20, 500000) && p.ItemCount(harpyDown) >= 20 {
			st.SetCond(10)
		}
	case 20145:
		if st.Cond() == 9 && s.DropItems(p, harpyDown, 1, 20, 500000) && p.ItemCount(spiderIchor) >= 20 {
			st.SetCond(10)
		}
	case 27077:
		if st.Cond() == 18 && p.RightHandItemID() == talinsSpear {
			st.SetCond(19)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, grailOfPurity, 1)
			s.TakeItems(p, talinsSpear, 1)
			s.GiveItems(p, tearsOfUnicorn, 1)
		}
	case 20581, 20582:
		if st.Cond() == 15 && s.RndBool() {
			for _, itemID := range talinsPieces() {
				if !p.HasItem(itemID) {
					s.GiveItems(p, itemID, 1)
					if p.HasItems(talinsPieces()...) {
						st.SetCond(16)
						s.PlaySound(p, script.SoundMiddle)
					} else {
						s.PlaySound(p, script.SoundItemGet)
					}
					return
				}
			}
		}
	}
}
