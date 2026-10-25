// Package q225 is the quest Test of the Searcher, a second class-change
// test for rogues, elven scouts, assassins and scavengers: Luther sends the
// traveller to Alex, who has Leirynn hunt the Delu lizardmen and their
// chief Kalkis, then follows the strange map through Borys, Tyra and Jax to
// the torn map pieces, the tree and its strong wooden chest of gold bars,
// for the Mark of Searcher.
package q225

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q225_TestOfTheSearcher"

// Items.
const (
	lutherLetter   = 2784
	alexWarrant    = 2785
	leirynnOrder1  = 2786
	deluTotem      = 2787
	leirynnOrder2  = 2788
	chiefKalkiFang = 2789
	leirynnReport  = 2790
	strangeMap     = 2791
	lambertMap     = 2792
	alexLetter     = 2793
	alexOrder      = 2794
	wineCatalog    = 2795
	tyraContract   = 2796
	redSporeDust   = 2797
	malrukianWine  = 2798
	oldOrder       = 2799
	jaxDiary       = 2800
	tornMapPiece1  = 2801
	tornMapPiece2  = 2802
	soltMap        = 2803
	makelMap       = 2804
	combinedMap    = 2805
	rustedKey      = 2806
	goldBar        = 2807
	alexRecommend  = 2808
)

// Rewards.
const markOfSearcher = 2809

// NPCs.
const (
	alex              = 30291
	tyra              = 30420
	tree              = 30627
	strongWoodenChest = 30628
	luther            = 30690
	leirynn           = 30728
	borys             = 30729
	jax               = 30730
)

// Monsters.
const (
	hangmanTree         = 20144
	roadScavenger       = 20551
	giantFungus         = 20555
	deluLizardmanShaman = 20781
	deluChiefKalkis     = 27093
	neerBodyguard       = 27092
)

// The classes the test is open to.
const (
	classRogue      = 7
	classElvenScout = 22
	classAssassin   = 35
	classScavenger  = 54
)

// quest holds the strong wooden chest the tree spawned, shared by every
// player: one at a time, until it decays.
type quest struct{ chest *script.NPCCell }

// New returns the quest.
func New() script.Script {
	q := quest{chest: new(script.NPCCell)}
	return script.Script{
		Title:   "Test of the Searcher",
		QuestID: 225,
		Items: []int32{
			lutherLetter, alexWarrant, leirynnOrder1, deluTotem, leirynnOrder2, chiefKalkiFang, leirynnReport,
			strangeMap, lambertMap, alexLetter, alexOrder, wineCatalog, tyraContract, redSporeDust, malrukianWine,
			oldOrder, jaxDiary, tornMapPiece1, tornMapPiece2, soltMap, makelMap, combinedMap, rustedKey, goldBar,
			alexRecommend,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {luther},
			script.EventTalked:     {alex, tyra, tree, strongWoodenChest, luther, leirynn, borys, jax},
			script.EventAttacked:   {deluLizardmanShaman},
			script.EventDecayed:    {strongWoodenChest},
			script.EventMyDying: {
				hangmanTree, roadScavenger, giantFungus, deluLizardmanShaman, deluChiefKalkis, neerBodyguard,
			},
		},
		Hooks: script.Hooks{
			OnEvent: q.onAdvEvent, OnTalk: onTalk, OnAttacked: onAttacked, OnDecayed: q.onDecayed,
			OnMyDying: onMyDying,
		},
	}
}

func (q quest) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	// Luther.
	case strings.EqualFold(e.Name, "30690-05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, lutherLetter, 1)
		if secondclass.GiveDimensionalDiamonds39(s, p) {
			htmltext = "30690-05a.htm"
		}
	// Alex.
	case strings.EqualFold(e.Name, "30291-07.htm"):
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, leirynnReport, 1)
		s.TakeItems(p, strangeMap, 1)
		s.GiveItems(p, alexLetter, 1)
		s.GiveItems(p, alexOrder, 1)
		s.GiveItems(p, lambertMap, 1)
	// Tyra.
	case strings.EqualFold(e.Name, "30420-01a.htm"):
		st.SetCond(10)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, wineCatalog, 1)
		s.GiveItems(p, tyraContract, 1)
	// Jax.
	case strings.EqualFold(e.Name, "30730-01d.htm"):
		st.SetCond(14)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, oldOrder, 1)
		s.GiveItems(p, jaxDiary, 1)
	// Tree.
	case strings.EqualFold(e.Name, "30627-01a.htm"):
		if q.chest.Empty() {
			if st.Cond() == 16 {
				st.SetCond(17)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, rustedKey, 1)
			}
			chest := s.Spawner().AddSpawn(strongWoodenChest, script.Loc{X: 10098, Y: 157287, Z: -2406, Heading: 300000}, false, 0)
			// Another player's tree may have spawned one meanwhile: one chest stands.
			if chest != nil && !q.chest.SetIfEmpty(chest) {
				chest.DeleteMe()
			}
		}
	// Strong wooden chest.
	case strings.EqualFold(e.Name, "30628-01a.htm"):
		if !p.HasItem(rustedKey) {
			htmltext = "30628-02.htm"
		} else {
			st.SetCond(18)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, rustedKey, -1)
			s.GiveItems(p, goldBar, 20)
			if chest := q.chest.Get(); chest != nil {
				chest.DeleteMe()
			}
		}
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
		switch class := p.ClassID(); {
		case class != classRogue && class != classElvenScout && class != classAssassin && class != classScavenger:
			htmltext = "30690-01.htm"
		case p.Level() < 39:
			htmltext = "30690-02.htm"
		case class == classScavenger:
			htmltext = "30690-04.htm"
		default:
			htmltext = "30690-03.htm"
		}
	case questlog.StatusStarted:
		htmltext = talkStarted(s, e.NPC.NpcID(), p, st)
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// talkStarted is the page npcID gives a started test, with the step it
// takes.
func talkStarted(s *script.Script, npcID int32, p *script.Player, st *script.QuestState) string {
	htmltext := script.NoQuestMsg()
	cond := st.Cond()
	switch npcID {
	case luther:
		switch {
		case cond == 1:
			htmltext = "30690-06.htm"
		case cond > 1 && cond < 19:
			htmltext = "30690-07.htm"
		case cond == 19:
			htmltext = "30690-08.htm"
			s.TakeItems(p, alexRecommend, 1)
			s.GiveItems(p, markOfSearcher, 1)
			s.RewardExpAndSp(p, 37831, 18750)
			p.SocialAction(3)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		}
	case alex:
		switch {
		case cond == 1:
			htmltext = "30291-01.htm"
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, lutherLetter, 1)
			s.GiveItems(p, alexWarrant, 1)
		case cond == 2:
			htmltext = "30291-02.htm"
		case cond > 2 && cond < 7:
			htmltext = "30291-03.htm"
		case cond == 7:
			htmltext = "30291-04.htm"
		case cond > 7 && cond < 13:
			htmltext = "30291-08.htm"
		case cond > 12 && cond < 16:
			htmltext = "30291-09.htm"
		case cond > 15 && cond < 18:
			htmltext = "30291-10.htm"
		case cond == 18:
			htmltext = "30291-11.htm"
			st.SetCond(19)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, alexOrder, 1)
			s.TakeItems(p, combinedMap, 1)
			s.TakeItems(p, goldBar, -1)
			s.GiveItems(p, alexRecommend, 1)
		case cond == 19:
			htmltext = "30291-12.htm"
		}
	case leirynn:
		switch {
		case cond == 2:
			htmltext = "30728-01.htm"
			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, alexWarrant, 1)
			s.GiveItems(p, leirynnOrder1, 1)
		case cond == 3:
			htmltext = "30728-02.htm"
		case cond == 4:
			htmltext = "30728-03.htm"
			st.SetCond(5)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, deluTotem, -1)
			s.TakeItems(p, leirynnOrder1, 1)
			s.GiveItems(p, leirynnOrder2, 1)
		case cond == 5:
			htmltext = "30728-04.htm"
		case cond == 6:
			htmltext = "30728-05.htm"
			st.SetCond(7)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, chiefKalkiFang, 1)
			s.TakeItems(p, leirynnOrder2, 1)
			s.GiveItems(p, leirynnReport, 1)
		case cond == 7:
			htmltext = "30728-06.htm"
		case cond > 7:
			htmltext = "30728-07.htm"
		}
	case borys:
		switch {
		case cond == 8:
			htmltext = "30729-01.htm"
			st.SetCond(9)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, alexLetter, 1)
			s.GiveItems(p, wineCatalog, 1)
		case cond > 8 && cond < 12:
			htmltext = "30729-02.htm"
		case cond == 12:
			htmltext = "30729-03.htm"
			st.SetCond(13)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, malrukianWine, 1)
			s.TakeItems(p, wineCatalog, 1)
			s.GiveItems(p, oldOrder, 1)
		case cond == 13:
			htmltext = "30729-04.htm"
		case cond > 13:
			htmltext = "30729-05.htm"
		}
	case tyra:
		switch {
		case cond == 9:
			htmltext = "30420-01.htm"
		case cond == 10:
			htmltext = "30420-02.htm"
		case cond == 11:
			htmltext = "30420-03.htm"
			st.SetCond(12)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, redSporeDust, -1)
			s.TakeItems(p, tyraContract, 1)
			s.GiveItems(p, malrukianWine, 1)
		case cond > 11:
			htmltext = "30420-04.htm"
		}
	case jax:
		switch {
		case cond == 13:
			htmltext = "30730-01.htm"
		case cond == 14:
			htmltext = "30730-02.htm"
		case cond == 15:
			htmltext = "30730-03.htm"
			st.SetCond(16)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, lambertMap, 1)
			s.TakeItems(p, makelMap, 1)
			s.TakeItems(p, jaxDiary, 1)
			s.TakeItems(p, soltMap, 1)
			s.GiveItems(p, combinedMap, 1)
		case cond > 15:
			htmltext = "30730-04.htm"
		}
	case tree:
		if cond == 16 || cond == 17 {
			htmltext = "30627-01.htm"
		}
	case strongWoodenChest:
		if cond == 17 {
			htmltext = "30628-01.htm"
		}
	}
	return htmltext
}

// onAttacked calls one Neer bodyguard, for 200 seconds, the first time a
// Delu lizardman shaman is hit by a player holding Leirynn's first order.
func onAttacked(s *script.Script, e script.Attacked) {
	p := script.ActingPlayer(e.Attacker)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	if p.HasItem(leirynnOrder1) && !e.NPC.IsScriptValue(1) {
		e.NPC.SetScriptValue(1)
		s.Spawner().AddSpawn(neerBodyguard, e.NPC, false, 200000*time.Millisecond)
	}
}

// onDecayed lets the tree spawn a chest again once its chest has left.
func (q quest) onDecayed(_ *script.Script, e script.Decayed) {
	q.chest.ClearIf(e.NPC)
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	switch e.NPC.NpcID() {
	case deluLizardmanShaman:
		st := s.CheckPlayerCondition(p, e.NPC, 3)
		if st == nil {
			return
		}
		if s.DropItemsAlways(p, deluTotem, 1, 10) {
			st.SetCond(4)
		}
	case deluChiefKalkis:
		st := s.CheckPlayerCondition(p, e.NPC, 5)
		if st == nil {
			return
		}
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, chiefKalkiFang, 1)
		s.GiveItems(p, strangeMap, 1)
	case giantFungus:
		st := s.CheckPlayerCondition(p, e.NPC, 10)
		if st == nil {
			return
		}
		if s.DropItemsAlways(p, redSporeDust, 1, 10) {
			st.SetCond(11)
		}
	case roadScavenger:
		st := s.CheckPlayerCondition(p, e.NPC, 14)
		if st == nil {
			return
		}
		if !p.HasItem(soltMap) && s.DropItems(p, tornMapPiece1, 1, 4, 500000) {
			s.TakeItems(p, tornMapPiece1, -1)
			s.GiveItems(p, soltMap, 1)
			if p.HasItem(makelMap) {
				st.SetCond(15)
			}
		}
	case hangmanTree:
		st := s.CheckPlayerCondition(p, e.NPC, 14)
		if st == nil {
			return
		}
		if !p.HasItem(makelMap) && s.DropItems(p, tornMapPiece2, 1, 4, 500000) {
			s.TakeItems(p, tornMapPiece2, -1)
			s.GiveItems(p, makelMap, 1)
			if p.HasItem(soltMap) {
				st.SetCond(15)
			}
		}
	}
}
