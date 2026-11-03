// Package q229 is the quest Test of Witchcraft, a second class-change test for
// human wizards, knights and Palus knights: Orim sends the traveller for the
// six Aklantoth gems, then to the Drevanul Prince Zeruel for the Purgatory
// key and the Zeruel bind crystal, for the Mark of Witchcraft.
package q229

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q229_TestOfWitchcraft"

// Items.
const (
	orimDiagram        = 3308
	alexandriaBook     = 3309
	ikerList           = 3310
	direWyrmFang       = 3311
	letoLizardmanCharm = 3312
	enGolemHeartstone  = 3313
	laraMemo           = 3314
	nestleMemo         = 3315
	leopoldJournal     = 3316
	aklantothGem1      = 3317
	aklantothGem2      = 3318
	aklantothGem3      = 3319
	aklantothGem4      = 3320
	aklantothGem5      = 3321
	aklantothGem6      = 3322
	brimstone1         = 3323
	orimInstructions   = 3324
	orimLetter1        = 3325
	orimLetter2        = 3326
	sirVasperLetter    = 3327
	vadinCrucifix      = 3328
	tamlinOrcAmulet    = 3329
	vadinSanctions     = 3330
	ikerAmulet         = 3331
	soultrapCrystal    = 3332
	purgatoryKey       = 3333
	zeruelBindCrystal  = 3334
	brimstone2         = 3335
	swordOfBinding     = 3029
)

// Rewards.
const markOfWitchcraft = 3307

// NPCs.
const (
	lara           = 30063
	alexandria     = 30098
	iker           = 30110
	vadin          = 30188
	nestle         = 30314
	sirKlausVasper = 30417
	leopold        = 30435
	kaira          = 30476
	orim           = 30630
	roderik        = 30631
	endrigo        = 30632
	evert          = 30633
)

// Monsters.
const (
	direWyrm              = 20557
	enchantedStoneGolem   = 20565
	letoLizardman         = 20577
	letoLizardmanArcher   = 20578
	letoLizardmanSoldier  = 20579
	letoLizardmanWarrior  = 20580
	letoLizardmanShaman   = 20581
	letoLizardmanOverlord = 20582
	tamlinOrc             = 20601
	tamlinOrcArcher       = 20602
	namelessRevenant      = 27099
	skeletalMercenary     = 27100
	drevanulPrinceZeruel  = 27101
)

// The classes the test is open to.
const (
	classKnight      = 4
	classHumanWizard = 11
	classPalusKnight = 32
)

// The NpcStrings the Nameless revenant, the Skeletal mercenary and the
// Zeruel speak.
const (
	alertedChat       = 22933
	orimZeruelChat    = 22934
	evertZeruelHit    = 22935
	evertZeruelKilled = 22936
)

// quest holds the Zeruel each of the two errands called, each shared by every
// player: one at a time, until it decays.
type quest struct {
	zeruelOrim, zeruelEvert *script.NPCCell
}

// New returns the quest.
func New() script.Script {
	q := quest{zeruelOrim: new(script.NPCCell), zeruelEvert: new(script.NPCCell)}
	return script.Script{
		Title:   "Test Of Witchcraft",
		QuestID: 229,
		Items: []int32{
			orimDiagram, alexandriaBook, ikerList, direWyrmFang, letoLizardmanCharm, enGolemHeartstone, laraMemo,
			nestleMemo, leopoldJournal, aklantothGem1, aklantothGem2, aklantothGem3, aklantothGem4, aklantothGem5,
			aklantothGem6, brimstone1, orimInstructions, orimLetter1, orimLetter2, sirVasperLetter, vadinCrucifix,
			tamlinOrcAmulet, vadinSanctions, ikerAmulet, soultrapCrystal, purgatoryKey, zeruelBindCrystal,
			brimstone2, swordOfBinding,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {orim},
			script.EventTalked: {
				lara, alexandria, iker, vadin, nestle, sirKlausVasper, leopold, kaira, orim, roderik, endrigo, evert,
			},
			script.EventAttacked: {namelessRevenant, skeletalMercenary, drevanulPrinceZeruel},
			script.EventDecayed:  {drevanulPrinceZeruel},
			script.EventMyDying: {
				direWyrm, enchantedStoneGolem, letoLizardman, letoLizardmanArcher, letoLizardmanSoldier,
				letoLizardmanWarrior, letoLizardmanShaman, letoLizardmanOverlord, tamlinOrc, tamlinOrcArcher,
				namelessRevenant, skeletalMercenary, drevanulPrinceZeruel,
			},
		},
		Hooks: script.Hooks{
			OnEvent: q.onAdvEvent, OnTimer: q.onTimer, OnTalk: q.onTalk, OnAttacked: q.onAttacked,
			OnDecayed: q.onDecayed, OnMyDying: q.onMyDying,
		},
	}
}

// call places the Zeruel at at for lifetime when cell holds none; one another
// player's dialog placed meanwhile wins, and the extra one leaves.
func call(s *script.Script, cell *script.NPCCell, at script.Loc, lifetime time.Duration) {
	if !cell.Empty() {
		return
	}
	if n := s.Spawner().AddSpawn(drevanulPrinceZeruel, at, false, lifetime); n != nil && !cell.SetIfEmpty(n) {
		n.DeleteMe()
	}
}

// orimSpot and evertSpot are where the Zeruels of the two errands stand.
func orimSpot() script.Loc  { return script.Loc{X: 70381, Y: 109638, Z: -3726} }
func evertSpot() script.Loc { return script.Loc{X: 13395, Y: 169807, Z: -3708} }

const (
	orimLifetime  = 120000 * time.Millisecond
	evertLifetime = 300000 * time.Millisecond
)

func (q quest) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	// Orim.
	case strings.EqualFold(e.Name, "30630-08.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, orimDiagram, 1)
		if secondclass.GiveDimensionalDiamonds39(s, p) {
			htmltext = "30630-08a.htm"
		}
	case strings.EqualFold(e.Name, "30630-14.htm"):
		st.SetCond(4)
		st.Unset("gem456")
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, aklantothGem1, 1)
		s.TakeItems(p, aklantothGem2, 1)
		s.TakeItems(p, aklantothGem3, 1)
		s.TakeItems(p, aklantothGem4, 1)
		s.TakeItems(p, aklantothGem5, 1)
		s.TakeItems(p, aklantothGem6, 1)
		s.TakeItems(p, alexandriaBook, 1)
		s.GiveItems(p, brimstone1, 1)
		call(s, q.zeruelOrim, orimSpot(), orimLifetime)
	case strings.EqualFold(e.Name, "30630-16.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, brimstone1, 1)
		s.GiveItems(p, orimInstructions, 1)
		s.GiveItems(p, orimLetter1, 1)
		s.GiveItems(p, orimLetter2, 1)
	case strings.EqualFold(e.Name, "30630-22.htm"):
		s.TakeItems(p, ikerAmulet, 1)
		s.TakeItems(p, orimInstructions, 1)
		s.TakeItems(p, purgatoryKey, 1)
		s.TakeItems(p, swordOfBinding, 1)
		s.TakeItems(p, zeruelBindCrystal, 1)
		s.GiveItems(p, markOfWitchcraft, 1)
		s.RewardExpAndSp(p, 139796, 40000)
		p.SocialAction(3)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	// Alexandria.
	case strings.EqualFold(e.Name, "30098-03.htm"):
		st.SetCond(2)
		st.Set("gem456", "1")
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, orimDiagram, 1)
		s.GiveItems(p, alexandriaBook, 1)
	// Iker.
	case strings.EqualFold(e.Name, "30110-03.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, ikerList, 1)
	case strings.EqualFold(e.Name, "30110-08.htm"):
		s.TakeItems(p, orimLetter2, 1)
		s.GiveItems(p, ikerAmulet, 1)
		s.GiveItems(p, soultrapCrystal, 1)
		if p.HasItem(swordOfBinding) {
			st.SetCond(7)
			s.PlaySound(p, script.SoundMiddle)
		} else {
			s.PlaySound(p, script.SoundItemGet)
		}
	// Kaira.
	case strings.EqualFold(e.Name, "30476-02.htm"):
		s.GiveItems(p, aklantothGem2, 1)
		if p.HasItems(aklantothGem1, aklantothGem3) && st.GetInt("gem456") == 6 {
			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
		} else {
			s.PlaySound(p, script.SoundItemGet)
		}
	// Lara.
	case strings.EqualFold(e.Name, "30063-02.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, laraMemo, 1)
	// Nestle.
	case strings.EqualFold(e.Name, "30314-02.htm"):
		st.Set("gem456", "2")
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, nestleMemo, 1)
	// Leopold.
	case strings.EqualFold(e.Name, "30435-02.htm"):
		st.Set("gem456", "3")
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, nestleMemo, 1)
		s.GiveItems(p, leopoldJournal, 1)
	// Sir Klaus Vasper.
	case strings.EqualFold(e.Name, "30417-03.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, orimLetter1, 1)
		s.GiveItems(p, sirVasperLetter, 1)
	// Evert.
	case strings.EqualFold(e.Name, "30633-02.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, brimstone2, 1)
		call(s, q.zeruelEvert, evertSpot(), evertLifetime)
	}
	return htmltext
}

// onTimer makes the Zeruel Orim called leave, once a hit has told it to.
func (q quest) onTimer(_ *script.Script, e script.Timer) string {
	if strings.EqualFold(e.Name, "zeruel_despawn") {
		if z := q.zeruelOrim.Get(); z != nil && script.Same(e.NPC, z) {
			e.NPC.StopAttack()
			e.NPC.DeleteMe()
		}
	}
	return ""
}

func (q quest) onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		class := p.ClassID()
		switch {
		case class != classKnight && class != classHumanWizard && class != classPalusKnight:
			htmltext = "30630-01.htm"
		case p.Level() < 39:
			htmltext = "30630-02.htm"
		case class == classHumanWizard:
			htmltext = "30630-03.htm"
		default:
			htmltext = "30630-05.htm"
		}
	case questlog.StatusStarted:
		htmltext = q.talkStarted(s, e.NPC.NpcID(), p, st)
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// talkStarted is the page npcID gives a started test, with the step it
// takes.
func (q quest) talkStarted(s *script.Script, npcID int32, p *script.Player, st *script.QuestState) string {
	htmltext := script.NoQuestMsg()
	cond := st.Cond()
	gem456 := st.GetInt("gem456")
	switch npcID {
	case orim:
		switch cond {
		case 1:
			htmltext = "30630-09.htm"
		case 2:
			htmltext = "30630-10.htm"
		case 3:
			htmltext = "30630-11.htm"
		case 4:
			htmltext = "30630-14.htm"
			call(s, q.zeruelOrim, orimSpot(), orimLifetime)
		case 5:
			htmltext = "30630-15.htm"
		case 6:
			htmltext = "30630-17.htm"
		case 7:
			htmltext = "30630-18.htm"
			st.SetCond(8)
			s.PlaySound(p, script.SoundMiddle)
		case 8, 9:
			htmltext = "30630-18.htm"
		case 10:
			htmltext = "30630-19.htm"
		}
	case alexandria:
		switch cond {
		case 1:
			htmltext = "30098-01.htm"
		case 2:
			htmltext = "30098-04.htm"
		default:
			htmltext = "30098-05.htm"
		}
	case kaira:
		switch {
		case p.HasItem(aklantothGem2):
			htmltext = "30476-03.htm"
		case cond == 2:
			htmltext = "30476-01.htm"
		case cond > 3:
			htmltext = "30476-04.htm"
		}
	case iker:
		switch {
		case p.HasItem(aklantothGem1):
			htmltext = "30110-06.htm"
		case p.HasItem(ikerList):
			if p.ItemCount(direWyrmFang)+p.ItemCount(letoLizardmanCharm)+p.ItemCount(enGolemHeartstone) < 60 {
				htmltext = "30110-04.htm"
			} else {
				htmltext = "30110-05.htm"
				s.TakeItems(p, ikerList, 1)
				s.TakeItems(p, direWyrmFang, -1)
				s.TakeItems(p, enGolemHeartstone, -1)
				s.TakeItems(p, letoLizardmanCharm, -1)
				s.GiveItems(p, aklantothGem1, 1)
				if p.HasItems(aklantothGem2, aklantothGem3) && gem456 == 6 {
					st.SetCond(3)
					s.PlaySound(p, script.SoundMiddle)
				} else {
					s.PlaySound(p, script.SoundItemGet)
				}
			}
		case cond == 2:
			htmltext = "30110-01.htm"
		case cond == 6 && !p.HasItem(soultrapCrystal):
			htmltext = "30110-07.htm"
		case cond >= 6 && cond < 10:
			htmltext = "30110-09.htm"
		case cond == 10:
			htmltext = "30110-10.htm"
		}
	case lara:
		switch {
		case p.HasItem(aklantothGem3):
			htmltext = "30063-04.htm"
		case p.HasItem(laraMemo):
			htmltext = "30063-03.htm"
		case cond == 2:
			htmltext = "30063-01.htm"
		case cond > 2:
			htmltext = "30063-05.htm"
		}
	case roderik, endrigo:
		if p.HasAtLeastOneItem(laraMemo, aklantothGem3) {
			htmltext = strconv.Itoa(int(npcID)) + "-01.htm"
		}
	case nestle:
		switch {
		case gem456 == 1:
			htmltext = "30314-01.htm"
		case gem456 == 2:
			htmltext = "30314-03.htm"
		case gem456 > 2:
			htmltext = "30314-04.htm"
		}
	case leopold:
		switch {
		case gem456 == 2:
			htmltext = "30435-01.htm"
		case gem456 > 2 && gem456 < 6:
			htmltext = "30435-03.htm"
		case gem456 == 6:
			htmltext = "30435-04.htm"
		case cond > 3:
			htmltext = "30435-05.htm"
		}
	case sirKlausVasper:
		switch {
		case p.HasAtLeastOneItem(sirVasperLetter, vadinCrucifix):
			htmltext = "30417-04.htm"
		case p.HasItem(vadinSanctions):
			htmltext = "30417-05.htm"
			s.TakeItems(p, vadinSanctions, 1)
			s.GiveItems(p, swordOfBinding, 1)
			if p.HasItem(soultrapCrystal) {
				st.SetCond(7)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				s.PlaySound(p, script.SoundItemGet)
			}
		case cond == 6:
			htmltext = "30417-01.htm"
		case cond > 6:
			htmltext = "30417-06.htm"
		}
	case vadin:
		switch {
		case p.HasItem(sirVasperLetter):
			htmltext = "30188-01.htm"
			s.PlaySound(p, script.SoundItemGet)
			s.TakeItems(p, sirVasperLetter, 1)
			s.GiveItems(p, vadinCrucifix, 1)
		case p.HasItem(vadinCrucifix):
			if p.ItemCount(tamlinOrcAmulet) < 20 {
				htmltext = "30188-02.htm"
			} else {
				htmltext = "30188-03.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, tamlinOrcAmulet, -1)
				s.TakeItems(p, vadinCrucifix, -1)
				s.GiveItems(p, vadinSanctions, 1)
			}
		case p.HasItem(vadinSanctions):
			htmltext = "30188-04.htm"
		case cond > 6:
			htmltext = "30188-05.htm"
		}
	case evert:
		switch cond {
		case 7, 8:
			htmltext = "30633-01.htm"
		case 9:
			htmltext = "30633-02.htm"
			call(s, q.zeruelEvert, evertSpot(), evertLifetime)
		case 10:
			htmltext = "30633-03.htm"
		}
	}
	return htmltext
}

// onAttacked marks a Nameless revenant or a Skeletal mercenary hit by a
// player on its errand, lets the Zeruel Orim called make the test move on and
// leave, and records the player who hits Evert's Zeruel with the Sword of
// Binding.
func (q quest) onAttacked(s *script.Script, e script.Attacked) {
	p := script.ActingPlayer(e.Attacker)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	switch e.NPC.NpcID() {
	case namelessRevenant:
		if p.HasItem(laraMemo) && !e.NPC.IsScriptValue(1) {
			e.NPC.SetScriptValue(1)
			e.NPC.Say(script.NpcString(alertedChat))
		}
	case skeletalMercenary:
		if gem456 := st.GetInt("gem456"); gem456 > 2 && gem456 < 6 && !e.NPC.IsScriptValue(1) {
			e.NPC.SetScriptValue(1)
			e.NPC.Say(script.NpcString(alertedChat))
		}
	case drevanulPrinceZeruel:
		if z := q.zeruelOrim.Get(); z != nil && script.Same(e.NPC, z) && st.Cond() == 4 && e.NPC.IsScriptValue(0) {
			st.SetCond(5)
			s.PlaySound(p, script.SoundMiddle)
			e.NPC.SetScriptValue(1)
			e.NPC.Say(script.NpcString(orimZeruelChat))
			s.StartTimer("zeruel_despawn", e.NPC, nil, 1000*time.Millisecond)
		} else if z := q.zeruelEvert.Get(); z != nil && script.Same(e.NPC, z) && st.Cond() == 9 && e.NPC.IsScriptValue(0) {
			if p.RightHandItemID() == swordOfBinding {
				e.NPC.SetScriptValue(p.ObjectID())
				e.NPC.Say(script.NpcString(evertZeruelHit))
			}
		}
	}
}

// onDecayed lets the dialogs call a Zeruel again once the one that stood has
// left.
func (q quest) onDecayed(_ *script.Script, e script.Decayed) {
	if !q.zeruelOrim.ClearIf(e.NPC) {
		q.zeruelEvert.ClearIf(e.NPC)
	}
}

func (q quest) onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	switch e.NPC.NpcID() {
	case direWyrm:
		if p.HasItem(ikerList) {
			s.DropItemsAlways(p, direWyrmFang, 1, 20)
		}
	case enchantedStoneGolem:
		if p.HasItem(ikerList) {
			s.DropItemsAlways(p, enGolemHeartstone, 1, 20)
		}
	case letoLizardman, letoLizardmanArcher:
		if p.HasItem(ikerList) {
			s.DropItems(p, letoLizardmanCharm, 1, 20, 500000)
		}
	case letoLizardmanSoldier, letoLizardmanWarrior:
		if p.HasItem(ikerList) {
			s.DropItems(p, letoLizardmanCharm, 1, 20, 600000)
		}
	case letoLizardmanShaman, letoLizardmanOverlord:
		if p.HasItem(ikerList) {
			s.DropItems(p, letoLizardmanCharm, 1, 20, 700000)
		}
	case namelessRevenant:
		if p.HasItem(laraMemo) {
			s.TakeItems(p, laraMemo, 1)
			s.GiveItems(p, aklantothGem3, 1)
			if p.HasItems(aklantothGem1, aklantothGem2) && st.GetInt("gem456") == 6 {
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				s.PlaySound(p, script.SoundItemGet)
			}
		}
	case skeletalMercenary:
		switch st.GetInt("gem456") {
		case 3:
			st.Set("gem456", "4")
			s.PlaySound(p, script.SoundItemGet)
			s.GiveItems(p, aklantothGem4, 1)
		case 4:
			st.Set("gem456", "5")
			s.PlaySound(p, script.SoundItemGet)
			s.GiveItems(p, aklantothGem5, 1)
		case 5:
			st.Set("gem456", "6")
			s.TakeItems(p, leopoldJournal, 1)
			s.GiveItems(p, aklantothGem6, 1)
			if p.HasItems(aklantothGem1, aklantothGem2, aklantothGem3) {
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				s.PlaySound(p, script.SoundItemGet)
			}
		}
	case tamlinOrc, tamlinOrcArcher:
		if p.HasItem(vadinCrucifix) {
			s.DropItems(p, tamlinOrcAmulet, 1, 20, 500000)
		}
	case drevanulPrinceZeruel:
		if z := q.zeruelEvert.Get(); z != nil && script.Same(e.NPC, z) && st.Cond() == 9 && e.NPC.ScriptValue() == p.ObjectID() {
			st.SetCond(10)
			s.PlaySound(p, script.SoundItemGet)
			s.TakeItems(p, brimstone2, 1)
			s.TakeItems(p, soultrapCrystal, 1)
			s.GiveItems(p, purgatoryKey, 1)
			s.GiveItems(p, zeruelBindCrystal, 1)
			e.NPC.Say(script.NpcString(evertZeruelKilled))
		}
	}
}
