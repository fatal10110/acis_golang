// Package q220 is the quest Testimony of Glory, a second class-change
// testimony for orcs: Vokian sends the traveller after trophies, Chianta has
// the five chiefs' scepters fetched, and Tanapi and Kakai close the errand
// with the scepter of the Revenant of Tantos for the Mark of Glory.
package q220

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q220_TestimonyOfGlory"

// privateLife is how long a chief's private stays before it is deleted.
const privateLife = 200000 * time.Millisecond

// Radar marks of the chiefs.
func vukuChiefDriko() (x, y, z int32)   { return -2150, 124443, -3724 }
func turekChiefBurai() (x, y, z int32)  { return -94294, 110818, -3696 }
func leuntChiefHarak() (x, y, z int32)  { return -55217, 200628, -3724 }
func brekaChiefVoltar() (x, y, z int32) { return 80100, 119991, -2264 }
func enkuChiefKepra() (x, y, z int32)   { return 12805, 189249, -3601 }

// Items.
const (
	vokianOrder1         = 3204
	manashenShard        = 3205
	tyrantTalon          = 3206
	guardianBasiliskFang = 3207
	vokianOrder2         = 3208
	necklaceOfAuthority  = 3209
	chiantaOrder1        = 3210
	scepterOfBreka       = 3211
	scepterOfEnku        = 3212
	scepterOfVuku        = 3213
	scepterOfTurek       = 3214
	scepterOfTunath      = 3215
	chiantaOrder2        = 3216
	chiantaOrder3        = 3217
	tamlinOrcSkull       = 3218
	timakOrcHead         = 3219
	scepterBox           = 3220
	pashikaHead          = 3221
	vultusHead           = 3222
	gloveOfVoltar        = 3223
	enkuOverlordHead     = 3224
	gloveOfKepra         = 3225
	makumBugbearHead     = 3226
	gloveOfBurai         = 3227
	manakiaLetter1       = 3228
	manakiaLetter2       = 3229
	kasmanLetter1        = 3230
	kasmanLetter2        = 3231
	kasmanLetter3        = 3232
	drikoContract        = 3233
	stakatoDroneHusk     = 3234
	tanapiOrder          = 3235
	scepterOfTantos      = 3236
	ritualBox            = 3237
)

// Rewards.
const markOfGlory = 3203

// NPCs.
const (
	kasman  = 30501
	vokian  = 30514
	manakia = 30515
	kakai   = 30565
	tanapi  = 30571
	voltar  = 30615
	kepra   = 30616
	burai   = 30617
	harak   = 30618
	driko   = 30619
	chianta = 30642
)

// Monsters.
const (
	tyrant                = 20192
	marshStakatoDrone     = 20234
	guardianBasilisk      = 20550
	manashenGargoyle      = 20563
	timakOrc              = 20583
	timakOrcArcher        = 20584
	timakOrcSoldier       = 20585
	timakOrcWarrior       = 20586
	timakOrcShaman        = 20587
	timakOrcOverlord      = 20588
	tamlinOrc             = 20601
	tamlinOrcArcher       = 20602
	ragnaOrcOverlord      = 20778
	ragnaOrcSeer          = 20779
	pashikaSonOfVoltar    = 27080
	vultusSonOfVoltar     = 27081
	enkuOrcOverlord       = 27082
	makumBugbearThug      = 27083
	revenantOfTantosChief = 27086
)

// Chat lines of the Ragna orcs and the Revenant of Tantos.
const (
	sayRagnaAttacked    = 22051
	sayRagnaDying       = 22052
	sayRevenantAttacked = 22055
	sayRevenantDying    = 22056
	sayRevenantWeak     = 22057
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Testimony Of Glory",
		QuestID: 220,
		Items: []int32{
			vokianOrder1, manashenShard, tyrantTalon, guardianBasiliskFang, vokianOrder2, necklaceOfAuthority,
			chiantaOrder1, scepterOfBreka, scepterOfEnku, scepterOfVuku, scepterOfTurek, scepterOfTunath,
			chiantaOrder2, chiantaOrder3, tamlinOrcSkull, timakOrcHead, scepterBox, pashikaHead, vultusHead,
			gloveOfVoltar, enkuOverlordHead, gloveOfKepra, makumBugbearHead, gloveOfBurai, manakiaLetter1,
			manakiaLetter2, kasmanLetter1, kasmanLetter2, kasmanLetter3, drikoContract, stakatoDroneHusk,
			tanapiOrder, scepterOfTantos, ritualBox,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {vokian},
			script.EventTalked: {
				kasman, vokian, manakia, kakai, tanapi, voltar, kepra, burai, harak, driko, chianta,
			},
			script.EventAttacked: {ragnaOrcOverlord, ragnaOrcSeer, revenantOfTantosChief},
			script.EventMyDying: {
				tyrant, marshStakatoDrone, guardianBasilisk, manashenGargoyle, timakOrc, timakOrcArcher,
				timakOrcSoldier, timakOrcWarrior, timakOrcShaman, timakOrcOverlord, tamlinOrc, tamlinOrcArcher,
				ragnaOrcOverlord, ragnaOrcSeer, pashikaSonOfVoltar, vultusSonOfVoltar, enkuOrcOverlord,
				makumBugbearThug, revenantOfTantosChief,
			},
		},
		Hooks: script.Hooks{
			OnEvent: onAdvEvent, OnTalk: onTalk, OnAttacked: onAttacked, OnMyDying: onMyDying,
		},
	}
}

// callPrivates gives master one private of each of npcIDs, once: only a
// master with none left gets them.
func callPrivates(s *script.Script, master *script.NPC, npcIDs ...int32) {
	if len(master.Minions()) != 0 {
		return
	}
	for _, npcID := range npcIDs {
		s.Spawner().CreateOnePrivate(master, npcID, privateLife)
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
	case strings.EqualFold(e.Name, "30514-05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, vokianOrder1, 1)
		if secondclass.GiveDimensionalDiamonds37(s, p) {
			htmltext = "30514-05a.htm"
		}
	case strings.EqualFold(e.Name, "30642-03.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, vokianOrder2, 1)
		s.GiveItems(p, chiantaOrder1, 1)
	case strings.EqualFold(e.Name, "30642-07.htm"):
		s.TakeItems(p, chiantaOrder1, 1)
		s.TakeItems(p, kasmanLetter1, 1)
		s.TakeItems(p, manakiaLetter1, 1)
		s.TakeItems(p, manakiaLetter2, 1)
		s.TakeItems(p, scepterOfBreka, 1)
		s.TakeItems(p, scepterOfEnku, 1)
		s.TakeItems(p, scepterOfTunath, 1)
		s.TakeItems(p, scepterOfTurek, 1)
		s.TakeItems(p, scepterOfVuku, 1)
		if p.Level() >= 37 {
			st.SetCond(6)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, chiantaOrder3, 1)
		} else {
			htmltext = "30642-06.htm"
			s.PlaySound(p, script.SoundItemGet)
			s.GiveItems(p, chiantaOrder2, 1)
		}
	case strings.EqualFold(e.Name, "30501-02.htm") && !p.HasItem(scepterOfVuku):
		if p.HasItem(kasmanLetter1) {
			htmltext = "30501-04.htm"
		} else {
			htmltext = "30501-03.htm"
			s.PlaySound(p, script.SoundItemGet)
			s.GiveItems(p, kasmanLetter1, 1)
		}
		p.AddRadarMarker(vukuChiefDriko())
	case strings.EqualFold(e.Name, "30501-05.htm") && !p.HasItem(scepterOfTurek):
		if p.HasItem(kasmanLetter2) {
			htmltext = "30501-07.htm"
		} else {
			htmltext = "30501-06.htm"
			s.PlaySound(p, script.SoundItemGet)
			s.GiveItems(p, kasmanLetter2, 1)
		}
		p.AddRadarMarker(turekChiefBurai())
	case strings.EqualFold(e.Name, "30501-08.htm") && !p.HasItem(scepterOfTunath):
		if p.HasItem(kasmanLetter3) {
			htmltext = "30501-10.htm"
		} else {
			htmltext = "30501-09.htm"
			s.PlaySound(p, script.SoundItemGet)
			s.GiveItems(p, kasmanLetter3, 1)
		}
		p.AddRadarMarker(leuntChiefHarak())
	case strings.EqualFold(e.Name, "30515-02.htm") && !p.HasItem(scepterOfBreka):
		if p.HasItem(manakiaLetter1) {
			htmltext = "30515-04.htm"
		} else {
			htmltext = "30515-03.htm"
			s.PlaySound(p, script.SoundItemGet)
			s.GiveItems(p, manakiaLetter1, 1)
		}
		p.AddRadarMarker(brekaChiefVoltar())
	case strings.EqualFold(e.Name, "30515-05.htm") && !p.HasItem(scepterOfEnku):
		if p.HasItem(manakiaLetter2) {
			htmltext = "30515-07.htm"
		} else {
			htmltext = "30515-06.htm"
			s.PlaySound(p, script.SoundItemGet)
			s.GiveItems(p, manakiaLetter2, 1)
		}
		p.AddRadarMarker(enkuChiefKepra())
	case strings.EqualFold(e.Name, "30615-04.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, manakiaLetter1, 1)
		s.GiveItems(p, gloveOfVoltar, 1)
		callPrivates(s, e.NPC, pashikaSonOfVoltar, vultusSonOfVoltar)
	case strings.EqualFold(e.Name, "30616-05.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, manakiaLetter2, 1)
		s.GiveItems(p, gloveOfKepra, 1)
		callPrivates(s, e.NPC, enkuOrcOverlord, enkuOrcOverlord, enkuOrcOverlord, enkuOrcOverlord)
	case strings.EqualFold(e.Name, "30617-04.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, kasmanLetter2, 1)
		s.GiveItems(p, gloveOfBurai, 1)
		callPrivates(s, e.NPC, makumBugbearThug, makumBugbearThug)
	case strings.EqualFold(e.Name, "30618-03.htm"):
		s.TakeItems(p, kasmanLetter3, 1)
		s.GiveItems(p, scepterOfTunath, 1)
		if p.HasItems(scepterOfBreka, scepterOfEnku, scepterOfVuku, scepterOfTurek) {
			st.SetCond(5)
			s.PlaySound(p, script.SoundMiddle)
		} else {
			s.PlaySound(p, script.SoundItemGet)
		}
	case strings.EqualFold(e.Name, "30619-03.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, kasmanLetter1, 1)
		s.GiveItems(p, drikoContract, 1)
	case strings.EqualFold(e.Name, "30571-03.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, scepterBox, 1)
		s.GiveItems(p, tanapiOrder, 1)
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
		case p.Race() != player.RaceOrc:
			htmltext = "30514-01.htm"
		case p.Level() < 37:
			htmltext = "30514-02.htm"
		case p.ClassLevel() != 1:
			htmltext = "30514-01a.htm"
		default:
			htmltext = "30514-03.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case vokian:
			switch cond {
			case 1:
				htmltext = "30514-06.htm"
			case 2:
				htmltext = "30514-08.htm"
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, guardianBasiliskFang, 10)
				s.TakeItems(p, manashenShard, 10)
				s.TakeItems(p, tyrantTalon, 10)
				s.TakeItems(p, vokianOrder1, 1)
				s.GiveItems(p, necklaceOfAuthority, 1)
				s.GiveItems(p, vokianOrder2, 1)
			case 3:
				htmltext = "30514-09.htm"
			case 8:
				htmltext = "30514-10.htm"
			}
		case chianta:
			switch {
			case cond == 3:
				htmltext = "30642-01.htm"
			case cond == 4:
				htmltext = "30642-04.htm"
			case cond == 5:
				if p.HasItem(chiantaOrder2) {
					if p.Level() >= 37 {
						htmltext = "30642-09.htm"
						st.SetCond(6)
						s.PlaySound(p, script.SoundMiddle)
						s.TakeItems(p, chiantaOrder2, 1)
						s.GiveItems(p, chiantaOrder3, 1)
					} else {
						htmltext = "30642-08.htm"
					}
				} else {
					htmltext = "30642-05.htm"
				}
			case cond == 6:
				htmltext = "30642-10.htm"
			case cond == 7:
				htmltext = "30642-11.htm"
				st.SetCond(8)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, chiantaOrder3, 1)
				s.TakeItems(p, necklaceOfAuthority, 1)
				s.TakeItems(p, tamlinOrcSkull, 20)
				s.TakeItems(p, timakOrcHead, 20)
				s.GiveItems(p, scepterBox, 1)
			case cond == 8:
				htmltext = "30642-12.htm"
			case cond > 8:
				htmltext = "30642-13.htm"
			}
		case kasman:
			if p.HasItem(chiantaOrder1) {
				htmltext = "30501-01.htm"
			} else if cond > 4 {
				htmltext = "30501-11.htm"
			}
		case manakia:
			if p.HasItem(chiantaOrder1) {
				htmltext = "30515-01.htm"
			} else if cond > 4 {
				htmltext = "30515-08.htm"
			}
		case voltar:
			if cond > 3 {
				switch {
				case p.HasItem(manakiaLetter1):
					htmltext = "30615-02.htm"
					p.RemoveRadarMarker(brekaChiefVoltar())
				case p.HasItem(gloveOfVoltar):
					htmltext = "30615-05.htm"
					callPrivates(s, e.NPC, pashikaSonOfVoltar, vultusSonOfVoltar)
				case p.HasItems(pashikaHead, vultusHead):
					htmltext = "30615-06.htm"
					s.TakeItems(p, pashikaHead, 1)
					s.TakeItems(p, vultusHead, 1)
					s.GiveItems(p, scepterOfBreka, 1)
					scepterEarned(s, st, p, scepterOfEnku, scepterOfVuku, scepterOfTurek, scepterOfTunath)
				case p.HasItem(scepterOfBreka):
					htmltext = "30615-07.htm"
				case p.HasItem(chiantaOrder1):
					htmltext = "30615-01.htm"
				case cond < 9:
					htmltext = "30615-08.htm"
				}
			}
		case kepra:
			if cond > 3 {
				switch {
				case p.HasItem(manakiaLetter2):
					htmltext = "30616-02.htm"
					p.RemoveRadarMarker(enkuChiefKepra())
				case p.HasItem(gloveOfKepra):
					htmltext = "30616-05.htm"
					callPrivates(s, e.NPC, enkuOrcOverlord, enkuOrcOverlord, enkuOrcOverlord, enkuOrcOverlord)
				case p.ItemCount(enkuOverlordHead) == 4:
					htmltext = "30616-06.htm"
					s.TakeItems(p, enkuOverlordHead, 4)
					s.GiveItems(p, scepterOfEnku, 1)
					scepterEarned(s, st, p, scepterOfBreka, scepterOfVuku, scepterOfTurek, scepterOfTunath)
				case p.HasItem(scepterOfEnku):
					htmltext = "30616-07.htm"
				case p.HasItem(chiantaOrder1):
					htmltext = "30616-01.htm"
				case cond < 9:
					htmltext = "30616-08.htm"
				}
			}
		case burai:
			if cond > 3 {
				switch {
				case p.HasItem(kasmanLetter2):
					htmltext = "30617-02.htm"
					p.RemoveRadarMarker(turekChiefBurai())
				case p.HasItem(gloveOfBurai):
					htmltext = "30617-04.htm"
					callPrivates(s, e.NPC, makumBugbearThug, makumBugbearThug)
				case p.ItemCount(makumBugbearHead) == 2:
					htmltext = "30617-05.htm"
					s.TakeItems(p, makumBugbearHead, 2)
					s.GiveItems(p, scepterOfTurek, 1)
					scepterEarned(s, st, p, scepterOfBreka, scepterOfVuku, scepterOfEnku, scepterOfTunath)
				case p.HasItem(scepterOfTurek):
					htmltext = "30617-06.htm"
				case p.HasItem(chiantaOrder1):
					htmltext = "30617-01.htm"
				case cond < 8:
					htmltext = "30617-07.htm"
				}
			}
		case harak:
			if cond > 3 {
				switch {
				case p.HasItem(kasmanLetter3):
					htmltext = "30618-02.htm"
					p.RemoveRadarMarker(leuntChiefHarak())
				case p.HasItem(scepterOfTunath):
					htmltext = "30618-04.htm"
				case p.HasItem(chiantaOrder1):
					htmltext = "30618-01.htm"
				case cond < 9:
					htmltext = "30618-05.htm"
				}
			}
		case driko:
			if cond > 3 {
				switch {
				case p.HasItem(kasmanLetter1):
					htmltext = "30619-02.htm"
					p.RemoveRadarMarker(vukuChiefDriko())
				case p.HasItem(drikoContract):
					if p.ItemCount(stakatoDroneHusk) == 30 {
						htmltext = "30619-05.htm"
						s.TakeItems(p, drikoContract, 1)
						s.TakeItems(p, stakatoDroneHusk, 30)
						s.GiveItems(p, scepterOfVuku, 1)
						scepterEarned(s, st, p, scepterOfBreka, scepterOfTurek, scepterOfEnku, scepterOfTunath)
					} else {
						htmltext = "30619-04.htm"
					}
				case p.HasItem(scepterOfVuku):
					htmltext = "30619-06.htm"
				case p.HasItem(chiantaOrder1):
					htmltext = "30619-01.htm"
				case cond < 8:
					htmltext = "30619-07.htm"
				}
			}
		case tanapi:
			switch cond {
			case 8:
				htmltext = "30571-01.htm"
			case 9:
				htmltext = "30571-04.htm"
			case 10:
				htmltext = "30571-05.htm"
				st.SetCond(11)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, scepterOfTantos, 1)
				s.TakeItems(p, tanapiOrder, 1)
				s.GiveItems(p, ritualBox, 1)
			case 11:
				htmltext = "30571-06.htm"
			}
		case kakai:
			switch {
			case cond > 7 && cond < 11:
				htmltext = "30565-01.htm"
			case cond == 11:
				htmltext = "30565-02.htm"
				s.TakeItems(p, ritualBox, 1)
				s.GiveItems(p, markOfGlory, 1)
				s.RewardExpAndSp(p, 91457, 2500)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// scepterEarned moves the quest to cond 5 when the traveller holds the other
// four scepters, and only plays the item sound otherwise.
func scepterEarned(s *script.Script, st *script.QuestState, p *script.Player, others ...int32) {
	for _, id := range others {
		if !p.HasItem(id) {
			s.PlaySound(p, script.SoundItemGet)
			return
		}
	}
	st.SetCond(5)
	s.PlaySound(p, script.SoundMiddle)
}

// onAttacked makes the Ragna orcs and the Revenant of Tantos speak when a
// player on the scepter errand hits them.
func onAttacked(s *script.Script, e script.Attacked) {
	p := script.ActingPlayer(e.Attacker)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	switch e.NPC.NpcID() {
	case ragnaOrcOverlord, ragnaOrcSeer:
		if st.Cond() == 9 && e.NPC.IsScriptValue(0) {
			e.NPC.Say(script.NpcString(sayRagnaAttacked))
			e.NPC.SetScriptValue(1)
		}
	case revenantOfTantosChief:
		if st.Cond() == 9 {
			if e.NPC.IsScriptValue(0) {
				e.NPC.Say(script.NpcString(sayRevenantAttacked))
				e.NPC.SetScriptValue(1)
			} else if e.NPC.IsScriptValue(1) && e.NPC.HPRatio() < 0.33 {
				e.NPC.Say(script.NpcString(sayRevenantWeak))
				e.NPC.SetScriptValue(2)
			}
		}
	}
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	cond := st.Cond()
	npcID := e.NPC.NpcID()
	switch npcID {
	case tyrant:
		if cond == 1 && s.DropItems(p, tyrantTalon, 1, 10, 500000) &&
			p.ItemCount(guardianBasiliskFang)+p.ItemCount(manashenShard) == 20 {
			st.SetCond(2)
		}
	case guardianBasilisk:
		if cond == 1 && s.DropItems(p, guardianBasiliskFang, 1, 10, 500000) &&
			p.ItemCount(tyrantTalon)+p.ItemCount(manashenShard) == 20 {
			st.SetCond(2)
		}
	case manashenGargoyle:
		if cond == 1 && s.DropItems(p, manashenShard, 1, 10, 750000) &&
			p.ItemCount(tyrantTalon)+p.ItemCount(guardianBasiliskFang) == 20 {
			st.SetCond(2)
		}
	case marshStakatoDrone:
		if p.HasItem(drikoContract) {
			s.DropItems(p, stakatoDroneHusk, 1, 30, 750000)
		}
	case pashikaSonOfVoltar:
		sonSlain(s, p, pashikaHead, vultusHead)
	case vultusSonOfVoltar:
		sonSlain(s, p, vultusHead, pashikaHead)
	case enkuOrcOverlord:
		if p.HasItem(gloveOfKepra) && s.DropItemsAlways(p, enkuOverlordHead, 1, 4) {
			s.TakeItems(p, gloveOfKepra, 1)
		}
	case makumBugbearThug:
		if p.HasItem(gloveOfBurai) && s.DropItemsAlways(p, makumBugbearHead, 1, 2) {
			s.TakeItems(p, gloveOfBurai, 1)
		}
	case timakOrc, timakOrcArcher, timakOrcSoldier, timakOrcWarrior, timakOrcShaman, timakOrcOverlord:
		if cond == 6 && s.DropItems(p, timakOrcHead, 1, 20, 500000+(npcID-timakOrc)*100000) &&
			p.ItemCount(tamlinOrcSkull) == 20 {
			st.SetCond(7)
		}
	case tamlinOrc:
		if cond == 6 && s.DropItems(p, tamlinOrcSkull, 1, 20, 500000) && p.ItemCount(timakOrcHead) == 20 {
			st.SetCond(7)
		}
	case tamlinOrcArcher:
		if cond == 6 && s.DropItems(p, tamlinOrcSkull, 1, 20, 600000) && p.ItemCount(timakOrcHead) == 20 {
			st.SetCond(7)
		}
	case ragnaOrcOverlord, ragnaOrcSeer:
		if cond == 9 {
			e.NPC.Say(script.NpcString(sayRagnaDying))
			at := script.LocOf(e.NPC)
			s.Spawner().CreateOnePrivateEx(e.NPC, revenantOfTantosChief, script.Loc{X: at.X, Y: at.Y, Z: at.Z}, privateLife, script.Params{})
		}
	case revenantOfTantosChief:
		if cond == 9 && s.DropItemsAlways(p, scepterOfTantos, 1, 1) {
			st.SetCond(10)
			e.NPC.Say(script.NpcString(sayRevenantDying))
		}
	}
}

// sonSlain gives the head of a slain son of Voltar to a traveller holding
// Voltar's glove and not yet that head; the glove goes once both heads are held.
func sonSlain(s *script.Script, p *script.Player, head, otherHead int32) {
	if !p.HasItem(gloveOfVoltar) || p.HasItem(head) {
		return
	}
	s.GiveItems(p, head, 1)
	if p.HasItem(otherHead) {
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, gloveOfVoltar, 1)
		return
	}
	s.PlaySound(p, script.SoundItemGet)
}
