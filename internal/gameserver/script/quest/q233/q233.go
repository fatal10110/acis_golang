// Package q233 is the quest Test of the War Spirit, a second class-change
// test for orc shamans: Somak sends the traveller to gather the remains of
// four fallen orcs from Orim, Racoy, Pekiron and Manakia, then the heads of
// Tamlin orcs for the vendetta totem, for the Mark of War Spirit.
package q233

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q233_TestOfTheWarSpirit"

// Items.
const (
	vendettaTotem    = 2880
	tamlinOrcHead    = 2881
	warspiritTotem   = 2882
	orimContract     = 2883
	portaEye         = 2884
	excuroScale      = 2885
	mordeoTalon      = 2886
	brakiRemains1    = 2887
	pekironTotem     = 2888
	tonarSkull       = 2889
	tonarRibbone     = 2890
	tonarSpine       = 2891
	tonarArmbone     = 2892
	tonarThighbone   = 2893
	tonarRemains1    = 2894
	manakiaTotem     = 2895
	hermodtSkull     = 2896
	hermodtRibbone   = 2897
	hermodtSpine     = 2898
	hermodtArmbone   = 2899
	hermodtThighbone = 2900
	hermodtRemains1  = 2901
	racoyTotem       = 2902
	vivyanLetter     = 2903
	insectDiagram    = 2904
	kirunaSkull      = 2905
	kirunaRibbone    = 2906
	kirunaSpine      = 2907
	kirunaArmbone    = 2908
	kirunaThighbone  = 2909
	kirunaRemains1   = 2910
	brakiRemains2    = 2911
	tonarRemains2    = 2912
	hermodtRemains2  = 2913
	kirunaRemains2   = 2914
)

// Rewards.
const markOfWarspirit = 2879

// NPCs.
const (
	vivyan            = 30030
	sarien            = 30436
	racoy             = 30507
	somak             = 30510
	manakia           = 30515
	orim              = 30630
	ancestorMartankus = 30649
	pekiron           = 30682
)

// Monsters.
const (
	nobleAnt              = 20089
	nobleAntLeader        = 20090
	medusa                = 20158
	porta                 = 20213
	excuro                = 20214
	mordeo                = 20215
	letoLizardmanShaman   = 20581
	letoLizardmanOverlord = 20582
	tamlinOrc             = 20601
	tamlinOrcArcher       = 20602
	stenoaGorgonQueen     = 27108
)

// The class the test is open to.
const classOrcShaman = 50

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Test of the War Spirit",
		QuestID: 233,
		Items: []int32{
			vendettaTotem, tamlinOrcHead, warspiritTotem, orimContract, portaEye, excuroScale, mordeoTalon,
			brakiRemains1, pekironTotem, tonarSkull, tonarRibbone, tonarSpine, tonarArmbone, tonarThighbone,
			tonarRemains1, manakiaTotem, hermodtSkull, hermodtRibbone, hermodtSpine, hermodtArmbone,
			hermodtThighbone, hermodtRemains1, racoyTotem, vivyanLetter, insectDiagram, kirunaSkull,
			kirunaRibbone, kirunaSpine, kirunaArmbone, kirunaThighbone, kirunaRemains1, brakiRemains2,
			tonarRemains2, hermodtRemains2, kirunaRemains2,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {somak},
			script.EventTalked:     {somak, vivyan, sarien, racoy, manakia, orim, ancestorMartankus, pekiron},
			script.EventMyDying: {
				nobleAnt, nobleAntLeader, medusa, porta, excuro, mordeo, letoLizardmanShaman,
				letoLizardmanOverlord, tamlinOrc, tamlinOrcArcher, stenoaGorgonQueen,
			},
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
	// given is the page that hands the traveller one item.
	given := func(item int32) {
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, item, 1)
	}
	switch {
	// Somak.
	case strings.EqualFold(e.Name, "30510-05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		if secondclass.GiveDimensionalDiamonds39(s, p) {
			htmltext = "30510-05e.htm"
		}
	// Orim.
	case strings.EqualFold(e.Name, "30630-04.htm"):
		given(orimContract)
	// Racoy.
	case strings.EqualFold(e.Name, "30507-02.htm"):
		given(racoyTotem)
	// Vivyan.
	case strings.EqualFold(e.Name, "30030-04.htm"):
		given(vivyanLetter)
	// Pekiron.
	case strings.EqualFold(e.Name, "30682-02.htm"):
		given(pekironTotem)
	// Manakia.
	case strings.EqualFold(e.Name, "30515-02.htm"):
		given(manakiaTotem)
	// Ancestor Martankus.
	case strings.EqualFold(e.Name, "30649-03.htm"):
		s.TakeItems(p, tamlinOrcHead, -1)
		s.TakeItems(p, warspiritTotem, -1)
		s.TakeItems(p, brakiRemains2, -1)
		s.TakeItems(p, hermodtRemains2, -1)
		s.TakeItems(p, kirunaRemains2, -1)
		s.TakeItems(p, tonarRemains2, -1)
		s.GiveItems(p, markOfWarspirit, 1)
		s.RewardExpAndSp(p, 63483, 17500)
		p.SocialAction(3)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
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
		case p.ClassID() == classOrcShaman && p.Level() < 39:
			htmltext = "30510-03.htm"
		case p.ClassID() == classOrcShaman:
			htmltext = "30510-04.htm"
		case p.Race() == player.RaceOrc:
			htmltext = "30510-02.htm"
		default:
			htmltext = "30510-01.htm"
		}
	case questlog.StatusStarted:
		htmltext = talkStarted(s, e.NPC.NpcID(), p, st)
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// middle answers a set of remains just handed in: the quest advances to its
// cond 2 once the other three are held (done), else the traveller only hears
// of the item.
func middle(s *script.Script, p *script.Player, st *script.QuestState, done bool) {
	if done {
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	} else {
		s.PlaySound(p, script.SoundItemGet)
	}
}

// talkStarted is the page npcID gives a started test, with the step it
// takes.
func talkStarted(s *script.Script, npcID int32, p *script.Player, st *script.QuestState) string {
	htmltext := script.NoQuestMsg()
	cond := st.Cond()
	switch npcID {
	case somak:
		switch cond {
		case 1:
			htmltext = "30510-06.htm"
		case 2:
			htmltext = "30510-07.htm"
			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, brakiRemains1, 1)
			s.TakeItems(p, hermodtRemains1, 1)
			s.TakeItems(p, kirunaRemains1, 1)
			s.TakeItems(p, tonarRemains1, 1)
			s.GiveItems(p, vendettaTotem, 1)
		case 3:
			htmltext = "30510-08.htm"
		case 4:
			htmltext = "30510-09.htm"
			st.SetCond(5)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, vendettaTotem, 1)
			s.GiveItems(p, brakiRemains2, 1)
			s.GiveItems(p, hermodtRemains2, 1)
			s.GiveItems(p, kirunaRemains2, 1)
			s.GiveItems(p, tonarRemains2, 1)
			s.GiveItems(p, warspiritTotem, 1)
		case 5:
			htmltext = "30510-10.htm"
		}
	case orim:
		switch {
		case cond != 1 || p.HasItem(brakiRemains1):
			htmltext = "30630-07.htm"
		case !p.HasItem(orimContract):
			htmltext = "30630-01.htm"
		case p.ItemCount(portaEye)+p.ItemCount(excuroScale)+p.ItemCount(mordeoTalon) == 30:
			htmltext = "30630-06.htm"
			s.TakeItems(p, excuroScale, 10)
			s.TakeItems(p, mordeoTalon, 10)
			s.TakeItems(p, portaEye, 10)
			s.TakeItems(p, orimContract, 1)
			s.GiveItems(p, brakiRemains1, 1)
			middle(s, p, st, p.HasItems(hermodtRemains1, kirunaRemains1, tonarRemains1))
		default:
			htmltext = "30630-05.htm"
		}
	case racoy:
		switch {
		case cond != 1 || p.HasItem(kirunaRemains1):
			htmltext = "30507-07.htm"
		case !p.HasItem(racoyTotem):
			htmltext = "30507-01.htm"
		case p.HasItem(vivyanLetter):
			htmltext = "30507-04.htm"
		case p.HasItem(insectDiagram):
			if p.HasItems(kirunaArmbone, kirunaRibbone, kirunaSkull, kirunaSpine, kirunaThighbone) {
				htmltext = "30507-06.htm"
				s.TakeItems(p, insectDiagram, 1)
				s.TakeItems(p, racoyTotem, 1)
				s.TakeItems(p, kirunaArmbone, 1)
				s.TakeItems(p, kirunaRibbone, 1)
				s.TakeItems(p, kirunaSkull, 1)
				s.TakeItems(p, kirunaSpine, 1)
				s.TakeItems(p, kirunaThighbone, 1)
				s.GiveItems(p, kirunaRemains1, 1)
				middle(s, p, st, p.HasItems(brakiRemains1, hermodtRemains1, tonarRemains1))
			} else {
				htmltext = "30507-05.htm"
			}
		default:
			htmltext = "30507-03.htm"
		}
	case vivyan:
		switch {
		case cond != 1 || !p.HasItem(racoyTotem):
			htmltext = "30030-07.htm"
		case p.HasItem(vivyanLetter):
			htmltext = "30030-05.htm"
		case p.HasItem(insectDiagram):
			htmltext = "30030-06.htm"
		default:
			htmltext = "30030-01.htm"
		}
	case sarien:
		if cond == 1 && p.HasItem(racoyTotem) {
			switch {
			case p.HasItem(vivyanLetter):
				htmltext = "30436-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, vivyanLetter, 1)
				s.GiveItems(p, insectDiagram, 1)
			case p.HasItem(insectDiagram):
				htmltext = "30436-02.htm"
			}
		} else {
			htmltext = "30436-03.htm"
		}
	case pekiron:
		switch {
		case cond != 1 || p.HasItem(tonarRemains1):
			htmltext = "30682-05.htm"
		case !p.HasItem(pekironTotem):
			htmltext = "30682-01.htm"
		case p.HasItems(tonarArmbone, tonarRibbone, tonarSkull, tonarSpine, tonarThighbone):
			htmltext = "30682-04.htm"
			s.TakeItems(p, pekironTotem, 1)
			s.TakeItems(p, tonarArmbone, 1)
			s.TakeItems(p, tonarRibbone, 1)
			s.TakeItems(p, tonarSkull, 1)
			s.TakeItems(p, tonarSpine, 1)
			s.TakeItems(p, tonarThighbone, 1)
			s.GiveItems(p, tonarRemains1, 1)
			middle(s, p, st, p.HasItems(brakiRemains1, hermodtRemains1, kirunaRemains1))
		default:
			htmltext = "30682-03.htm"
		}
	case manakia:
		switch {
		case cond != 1 || p.HasItem(hermodtRemains1):
			htmltext = "30515-05.htm"
		case !p.HasItem(manakiaTotem):
			htmltext = "30515-01.htm"
		case p.HasItems(hermodtArmbone, hermodtRibbone, hermodtSkull, hermodtSpine, hermodtThighbone):
			htmltext = "30515-04.htm"
			s.TakeItems(p, manakiaTotem, 1)
			s.TakeItems(p, hermodtArmbone, 1)
			s.TakeItems(p, hermodtRibbone, 1)
			s.TakeItems(p, hermodtSkull, 1)
			s.TakeItems(p, hermodtSpine, 1)
			s.TakeItems(p, hermodtThighbone, 1)
			s.GiveItems(p, hermodtRemains1, 1)
			middle(s, p, st, p.HasItems(brakiRemains1, kirunaRemains1, tonarRemains1))
		default:
			htmltext = "30515-03.htm"
		}
	case ancestorMartankus:
		if cond == 5 {
			htmltext = "30649-01.htm"
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
	switch e.NPC.NpcID() {
	case porta:
		if p.HasItem(orimContract) {
			s.DropItemsAlways(p, portaEye, 1, 10)
		}
	case excuro:
		if p.HasItem(orimContract) {
			s.DropItemsAlways(p, excuroScale, 1, 10)
		}
	case mordeo:
		if p.HasItem(orimContract) {
			s.DropItemsAlways(p, mordeoTalon, 1, 10)
		}
	case nobleAnt, nobleAntLeader:
		if p.HasItem(insectDiagram) {
			switch rnd := s.Rnd(100); {
			case rnd > 70:
				if p.HasItem(kirunaThighbone) {
					s.DropItemsAlways(p, kirunaArmbone, 1, 1)
				} else {
					s.DropItemsAlways(p, kirunaThighbone, 1, 1)
				}
			case rnd > 40:
				if p.HasItem(kirunaSpine) {
					s.DropItemsAlways(p, kirunaRibbone, 1, 1)
				} else {
					s.DropItemsAlways(p, kirunaSpine, 1, 1)
				}
			case rnd > 10:
				s.DropItemsAlways(p, kirunaSkull, 1, 1)
			}
		}
	case letoLizardmanShaman, letoLizardmanOverlord:
		if p.HasItem(pekironTotem) && s.RndBool() {
			switch {
			case !p.HasItem(tonarSkull):
				s.DropItemsAlways(p, tonarSkull, 1, 1)
			case !p.HasItem(tonarRibbone):
				s.DropItemsAlways(p, tonarRibbone, 1, 1)
			case !p.HasItem(tonarSpine):
				s.DropItemsAlways(p, tonarSpine, 1, 1)
			case !p.HasItem(tonarArmbone):
				s.DropItemsAlways(p, tonarArmbone, 1, 1)
			default:
				s.DropItemsAlways(p, tonarThighbone, 1, 1)
			}
		}
	case medusa:
		if p.HasItem(manakiaTotem) && s.RndBool() {
			switch {
			case !p.HasItem(hermodtRibbone):
				s.DropItemsAlways(p, hermodtRibbone, 1, 1)
			case !p.HasItem(hermodtSpine):
				s.DropItemsAlways(p, hermodtSpine, 1, 1)
			case !p.HasItem(hermodtArmbone):
				s.DropItemsAlways(p, hermodtArmbone, 1, 1)
			default:
				s.DropItemsAlways(p, hermodtThighbone, 1, 1)
			}
		}
	case stenoaGorgonQueen:
		if p.HasItem(manakiaTotem) {
			s.DropItemsAlways(p, hermodtSkull, 1, 1)
		}
	case tamlinOrc, tamlinOrcArcher:
		if p.HasItem(vendettaTotem) && s.DropItems(p, tamlinOrcHead, 1, 13, 500000) {
			st.SetCond(4)
		}
	}
}
