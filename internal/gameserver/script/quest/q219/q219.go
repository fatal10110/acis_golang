// Package q219 is the quest Testimony of Fate, a second class-change
// testimony for dark elves: Kaira sends the traveller through a chain of
// errands (a funeral jar, a herbalist's list, the Alder spirit, a manuscript,
// the garnet and the treant seed) to Thifiell for the Mark of Fate.
package q219

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q219_TestimonyOfFate"

// NPCs.
const (
	kaira        = 30476
	metheus      = 30614
	ixia         = 30463
	alderSpirit  = 30613
	roa          = 30114
	norman       = 30210
	thifiell     = 30358
	arkenia      = 30419
	bloodyPixy   = 31845
	blightTreant = 31850
)

// Items.
const (
	kairaLetter           = 3173
	metheusFuneralJar     = 3174
	kasandraRemains       = 3175
	herbalismTextbook     = 3176
	ixiaList              = 3177
	medusaIchor           = 3178
	marshSpiderFluids     = 3179
	deadSeekerDung        = 3180
	tyrantBlood           = 3181
	nightshadeRoot        = 3182
	belladonna            = 3183
	alderSkull1           = 3184
	alderSkull2           = 3185
	alderReceipt          = 3186
	revelationsManuscript = 3187
	kairaRecommendation   = 3189
	kairaInstructions     = 3188
	palusCharm            = 3190
	thifiellLetter        = 3191
	arkeniaNote           = 3192
	pixyGarnet            = 3193
	grandisSkull          = 3194
	karulBugbearSkull     = 3195
	brekaOverlordSkull    = 3196
	letoOverlordSkull     = 3197
	redFairyDust          = 3198
	blightTreantSeed      = 3199
	blackWillowLeaf       = 3200
	blightTreantSap       = 3201
	arkeniaLetter         = 3202
)

// Rewards.
const markOfFate = 3172

// Monsters.
const (
	hangmanTree           = 20144
	marshStakato          = 20157
	medusa                = 20158
	tyrant                = 20192
	tyrantKingpin         = 20193
	deadSeeker            = 20202
	marshStakatoWorker    = 20230
	marshStakatoSoldier   = 20232
	marshSpider           = 20233
	marshStakatoDrone     = 20234
	brekaOrcOverlord      = 20270
	grandis               = 20554
	letoLizardmanOverlord = 20582
	karulBugbear          = 20600
	blackWillowLurker     = 27079
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Testimony of Fate",
		QuestID: 219,
		Items: []int32{
			kairaLetter, metheusFuneralJar, kasandraRemains, herbalismTextbook, ixiaList, medusaIchor,
			marshSpiderFluids, deadSeekerDung, tyrantBlood, nightshadeRoot, belladonna, alderSkull1, alderSkull2,
			alderReceipt, revelationsManuscript, kairaRecommendation, kairaInstructions, palusCharm, thifiellLetter,
			arkeniaNote, pixyGarnet, grandisSkull, karulBugbearSkull, brekaOverlordSkull, letoOverlordSkull,
			redFairyDust, blightTreantSeed, blackWillowLeaf, blightTreantSap, arkeniaLetter,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {kaira},
			script.EventTalked: {
				kaira, metheus, ixia, alderSpirit, roa, norman, thifiell, arkenia, bloodyPixy, blightTreant,
			},
			script.EventMyDying: {
				hangmanTree, marshStakato, medusa, tyrant, tyrantKingpin, deadSeeker, marshStakatoWorker,
				marshStakatoSoldier, marshSpider, marshStakatoDrone, brekaOrcOverlord, grandis,
				letoLizardmanOverlord, karulBugbear, blackWillowLurker,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// dropChance is the per-kill chance of the cond 6 ingredient a monster drops.
func dropChance(npcID int32) int32 {
	switch npcID {
	case deadSeeker, tyrant, medusa, marshStakatoSoldier, marshSpider:
		return 500000
	case tyrantKingpin, marshStakatoDrone:
		return 600000
	case marshStakato:
		return 400000
	case marshStakatoWorker:
		return 300000
	}
	return 0
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30476-05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, kairaLetter, 1)
		if secondclass.GiveDimensionalDiamonds37(s, p) {
			htmltext = "30476-05a.htm"
		}
	case strings.EqualFold(e.Name, "30114-04.htm"):
		st.SetCond(12)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, alderSkull2, 1)
		s.GiveItems(p, alderReceipt, 1)
	case strings.EqualFold(e.Name, "30476-12.htm"):
		s.PlaySound(p, script.SoundMiddle)
		if p.Level() < 38 {
			htmltext = "30476-13.htm"
			st.SetCond(14)
			s.GiveItems(p, kairaInstructions, 1)
		} else {
			st.SetCond(15)
			s.TakeItems(p, revelationsManuscript, 1)
			s.GiveItems(p, kairaRecommendation, 1)
		}
	case strings.EqualFold(e.Name, "30419-02.htm"):
		st.SetCond(17)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, thifiellLetter, 1)
		s.GiveItems(p, arkeniaNote, 1)
	case strings.EqualFold(e.Name, "31845-02.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, pixyGarnet, 1)
	case strings.EqualFold(e.Name, "31850-02.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, blightTreantSeed, 1)
	case strings.EqualFold(e.Name, "30419-05.htm"):
		st.SetCond(18)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, arkeniaNote, 1)
		s.TakeItems(p, blightTreantSap, 1)
		s.TakeItems(p, redFairyDust, 1)
		s.GiveItems(p, arkeniaLetter, 1)
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
		case p.Race() != player.RaceDarkElf:
			htmltext = "30476-02.htm"
		case p.Level() < 37 || p.ClassLevel() != 1:
			htmltext = "30476-01.htm"
		default:
			htmltext = "30476-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case kaira:
			switch {
			case cond == 1:
				htmltext = "30476-06.htm"
			case cond == 2 || cond == 3:
				htmltext = "30476-07.htm"
			case cond > 3 && cond < 9:
				htmltext = "30476-08.htm"
			case cond == 9:
				htmltext = "30476-09.htm"
				st.SetCond(10)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, alderSkull1, 1)
				s.Spawner().AddSpawn(alderSpirit, p, false, 0)
			case cond > 9 && cond < 13:
				htmltext = "30476-10.htm"
			case cond == 13:
				htmltext = "30476-11.htm"
			case cond == 14:
				if p.Level() < 38 {
					htmltext = "30476-14.htm"
				} else {
					htmltext = "30476-12.htm"
					st.SetCond(15)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, kairaInstructions, 1)
					s.TakeItems(p, revelationsManuscript, 1)
					s.GiveItems(p, kairaRecommendation, 1)
				}
			case cond == 15:
				htmltext = "30476-16.htm"
			case cond > 15:
				htmltext = "30476-17.htm"
			}
		case metheus:
			switch {
			case cond == 1:
				htmltext = "30614-01.htm"
				st.SetCond(2)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, kairaLetter, 1)
				s.GiveItems(p, metheusFuneralJar, 1)
			case cond == 2:
				htmltext = "30614-02.htm"
			case cond == 3:
				htmltext = "30614-03.htm"
				st.SetCond(4)
				st.SetCond(5)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, kasandraRemains, 1)
				s.GiveItems(p, herbalismTextbook, 1)
			case cond > 3 && cond < 8:
				htmltext = "30614-04.htm"
			case cond == 8:
				htmltext = "30614-05.htm"
				st.SetCond(9)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, belladonna, 1)
				s.GiveItems(p, alderSkull1, 1)
			case cond > 8:
				htmltext = "30614-06.htm"
			}
		case ixia:
			switch {
			case cond == 5:
				htmltext = "30463-01.htm"
				st.SetCond(6)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, herbalismTextbook, 1)
				s.GiveItems(p, ixiaList, 1)
			case cond == 6:
				htmltext = "30463-02.htm"
			case cond == 7:
				htmltext = "30463-03.htm"
				st.SetCond(8)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, ixiaList, 1)
				s.TakeItems(p, deadSeekerDung, -1)
				s.TakeItems(p, marshSpiderFluids, -1)
				s.TakeItems(p, medusaIchor, -1)
				s.TakeItems(p, nightshadeRoot, -1)
				s.TakeItems(p, tyrantBlood, -1)
				s.GiveItems(p, belladonna, 1)
			case cond == 8:
				htmltext = "30463-04.htm"
			case cond > 8:
				htmltext = "30463-05.htm"
			}
		case alderSpirit:
			if cond == 10 {
				htmltext = "30613-01.htm"
				st.SetCond(11)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, alderSkull2, 1)
				e.NPC.DeleteMe()
			}
		case roa:
			switch {
			case cond == 11:
				htmltext = "30114-01.htm"
			case cond == 12:
				htmltext = "30114-05.htm"
			case cond > 12:
				htmltext = "30114-06.htm"
			}
		case norman:
			switch {
			case cond == 12:
				htmltext = "30210-01.htm"
				st.SetCond(13)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, alderReceipt, 1)
				s.GiveItems(p, revelationsManuscript, 1)
			case cond > 12:
				htmltext = "30210-02.htm"
			}
		case thifiell:
			switch cond {
			case 15:
				htmltext = "30358-01.htm"
				st.SetCond(16)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, kairaRecommendation, 1)
				s.GiveItems(p, palusCharm, 1)
				s.GiveItems(p, thifiellLetter, 1)
			case 16:
				htmltext = "30358-02.htm"
			case 17:
				htmltext = "30358-03.htm"
			case 18:
				htmltext = "30358-04.htm"
				s.TakeItems(p, palusCharm, 1)
				s.TakeItems(p, arkeniaLetter, 1)
				s.GiveItems(p, markOfFate, 1)
				s.RewardExpAndSp(p, 68183, 1750)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		case arkenia:
			switch cond {
			case 16:
				htmltext = "30419-01.htm"
			case 17:
				if p.HasItem(blightTreantSap) && p.HasItem(redFairyDust) {
					htmltext = "30419-04.htm"
				} else {
					htmltext = "30419-03.htm"
				}
			case 18:
				htmltext = "30419-06.htm"
			}
		case bloodyPixy:
			switch cond {
			case 17:
				switch {
				case p.HasItem(pixyGarnet):
					if p.ItemCount(grandisSkull) >= 10 && p.ItemCount(karulBugbearSkull) >= 10 &&
						p.ItemCount(brekaOverlordSkull) >= 10 && p.ItemCount(letoOverlordSkull) >= 10 {
						htmltext = "31845-04.htm"
						s.PlaySound(p, script.SoundItemGet)
						s.TakeItems(p, brekaOverlordSkull, -1)
						s.TakeItems(p, grandisSkull, -1)
						s.TakeItems(p, karulBugbearSkull, -1)
						s.TakeItems(p, letoOverlordSkull, -1)
						s.TakeItems(p, pixyGarnet, 1)
						s.GiveItems(p, redFairyDust, 1)
					} else {
						htmltext = "31845-03.htm"
					}
				case p.HasItem(redFairyDust):
					htmltext = "31845-05.htm"
				default:
					htmltext = "31845-01.htm"
				}
			case 18:
				htmltext = "31845-05.htm"
			}
		case blightTreant:
			switch cond {
			case 17:
				switch {
				case p.HasItem(blightTreantSeed):
					if p.HasItem(blackWillowLeaf) {
						htmltext = "31850-04.htm"
						s.PlaySound(p, script.SoundItemGet)
						s.TakeItems(p, blackWillowLeaf, 1)
						s.TakeItems(p, blightTreantSeed, 1)
						s.GiveItems(p, blightTreantSap, 1)
					} else {
						htmltext = "31850-03.htm"
					}
				case p.HasItem(blightTreantSap):
					htmltext = "31850-05.htm"
				default:
					htmltext = "31850-01.htm"
				}
			case 18:
				htmltext = "31850-05.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// allHeld reports whether the traveller holds ten of every ingredient listed.
func allHeld(p *script.Player, items ...int32) bool {
	for _, item := range items {
		if p.ItemCount(item) < 10 {
			return false
		}
	}
	return true
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	npcID := e.NPC.NpcID()
	switch npcID {
	case hangmanTree:
		if st.Cond() == 2 {
			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, metheusFuneralJar, 1)
			s.GiveItems(p, kasandraRemains, 1)
		}
	case deadSeeker:
		if st.Cond() == 6 && s.DropItems(p, deadSeekerDung, 1, 10, dropChance(npcID)) &&
			allHeld(p, tyrantBlood, medusaIchor, nightshadeRoot, marshSpiderFluids) {
			st.SetCond(7)
		}
	case tyrant, tyrantKingpin:
		if st.Cond() == 6 && s.DropItems(p, tyrantBlood, 1, 10, dropChance(npcID)) &&
			allHeld(p, deadSeekerDung, medusaIchor, nightshadeRoot, marshSpiderFluids) {
			st.SetCond(7)
		}
	case medusa:
		if st.Cond() == 6 && s.DropItems(p, medusaIchor, 1, 10, dropChance(npcID)) &&
			allHeld(p, deadSeekerDung, tyrantBlood, nightshadeRoot, marshSpiderFluids) {
			st.SetCond(7)
		}
	case marshStakato, marshStakatoWorker, marshStakatoSoldier, marshStakatoDrone:
		if st.Cond() == 6 && s.DropItems(p, nightshadeRoot, 1, 10, dropChance(npcID)) &&
			allHeld(p, deadSeekerDung, tyrantBlood, medusaIchor, marshSpiderFluids) {
			st.SetCond(7)
		}
	case marshSpider:
		if st.Cond() == 6 && s.DropItems(p, marshSpiderFluids, 1, 10, dropChance(npcID)) &&
			allHeld(p, deadSeekerDung, tyrantBlood, medusaIchor, nightshadeRoot) {
			st.SetCond(7)
		}
	case grandis:
		if p.HasItem(pixyGarnet) {
			s.DropItemsAlways(p, grandisSkull, 1, 10)
		}
	case letoLizardmanOverlord:
		if p.HasItem(pixyGarnet) {
			s.DropItemsAlways(p, letoOverlordSkull, 1, 10)
		}
	case brekaOrcOverlord:
		if p.HasItem(pixyGarnet) {
			s.DropItemsAlways(p, brekaOverlordSkull, 1, 10)
		}
	case karulBugbear:
		if p.HasItem(pixyGarnet) {
			s.DropItemsAlways(p, karulBugbearSkull, 1, 10)
		}
	case blackWillowLurker:
		if p.HasItem(blightTreantSeed) {
			s.DropItemsAlways(p, blackWillowLeaf, 1, 1)
		}
	}
}
