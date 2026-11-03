// Package q327 is the quest Recover the Farmland: Piotur and Leikan pay for
// the Turek orcs' dogtags and medallions, Asha restores the relic pieces the
// orcs drop, Iris buys them for experience and Nestle trades the restored
// relics for supplies.
package q327

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q327_RecoverTheFarmland"

// Items.
const (
	leikanLetter        = 5012
	turekDogtag         = 1846
	turekMedallion      = 1847
	clayUrnFragment     = 1848
	brassTrinketPiece   = 1849
	bronzeMirrorPiece   = 1850
	jadeNecklaceBead    = 1851
	ancientClayUrn      = 1852
	ancientBrassTiara   = 1853
	ancientBronzeMirror = 1854
	ancientJadeNecklace = 1855
)

// Rewards.
const (
	adena                = 57
	soulshotD            = 1463
	spiritshotD          = 2510
	healingPotion        = 1061
	hastePotion          = 734
	potionOfAlacrity     = 735
	scrollOfEscape       = 736
	scrollOfResurrection = 737
)

// NPCs.
const (
	leikan = 30382
	piotur = 30597
	iris   = 30034
	asha   = 30313
	nestle = 30314
)

// Monsters.
const (
	turekOrcWarlord    = 20495
	turekOrcArcher     = 20496
	turekOrcSkirmisher = 20497
	turekOrcSupplier   = 20498
	turekOrcFootman    = 20499
	turekOrcSentinel   = 20500
	turekOrcShaman     = 20501
)

// drop is the dogtag or medallion a Turek orc always drops, and the chance
// of a relic piece, out of a million.
type drop struct {
	npcID, chance, itemID int32
}

// droplist returns the drops of each Turek orc.
func droplist() []drop {
	return []drop{
		{turekOrcArcher, 140000, turekDogtag},
		{turekOrcSkirmisher, 70000, turekDogtag},
		{turekOrcSupplier, 120000, turekDogtag},
		{turekOrcFootman, 100000, turekDogtag},
		{turekOrcSentinel, 80000, turekDogtag},
		{turekOrcShaman, 90000, turekMedallion},
		{turekOrcWarlord, 180000, turekMedallion},
	}
}

// expReward returns the experience Iris gives for each restored relic.
func expReward() map[int32]int32 {
	return map[int32]int32{
		ancientClayUrn:      2766,
		ancientBrassTiara:   3227,
		ancientBronzeMirror: 3227,
		ancientJadeNecklace: 3919,
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Recover the Farmland",
		QuestID: 327,
		Items:   []int32{leikanLetter},
		Bind: script.Bindings{
			script.EventQuestStart: {leikan, piotur},
			script.EventTalked:     {leikan, piotur, iris, asha, nestle},
			script.EventMyDying:    {turekOrcWarlord, turekOrcArcher, turekOrcSkirmisher, turekOrcSupplier, turekOrcFootman, turekOrcSentinel, turekOrcShaman},
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
	is := func(name string) bool { return strings.EqualFold(e.Name, name) }
	switch {
	// Piotur
	case is("30597-03.htm") && st.Cond() < 1:
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case is("30597-06.htm"):
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	// Leikan
	case is("30382-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(2)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, leikanLetter, 1)
	// Asha
	case is("30313-02.htm"):
		htmltext = restore(s, p, htmltext, clayUrnFragment, 6, 5, "30313-03.htm", ancientClayUrn)
	case is("30313-04.htm"):
		htmltext = restore(s, p, htmltext, brassTrinketPiece, 7, 6, "30313-05.htm", ancientBrassTiara)
	case is("30313-06.htm"):
		htmltext = restore(s, p, htmltext, bronzeMirrorPiece, 7, 6, "30313-07.htm", ancientBronzeMirror)
	case is("30313-08.htm"):
		htmltext = restore(s, p, htmltext, jadeNecklaceBead, 8, 7, "30313-09.htm", ancientJadeNecklace)
	// Iris
	case is("30034-03.htm"):
		htmltext = sellPieces(s, p, htmltext, clayUrnFragment, 307)
	case is("30034-04.htm"):
		htmltext = sellPieces(s, p, htmltext, brassTrinketPiece, 368)
	case is("30034-05.htm"):
		htmltext = sellPieces(s, p, htmltext, bronzeMirrorPiece, 368)
	case is("30034-06.htm"):
		htmltext = sellPieces(s, p, htmltext, jadeNecklaceBead, 430)
	case is("30034-07.htm"):
		isRewarded := false
		for i := int32(1852); i < 1856; i++ {
			n := p.ItemCount(i)
			if n > 0 {
				s.TakeItems(p, i, n)
				s.RewardExpAndSp(p, int64(n*expReward()[i]), 0)
				isRewarded = true
			}
		}
		if !isRewarded {
			htmltext = "30034-02.htm"
		} else {
			s.PlaySound(p, script.SoundItemGet)
		}
	// Nestle
	case is("30314-03.htm"):
		if !p.HasItem(ancientClayUrn) {
			htmltext = "30314-07.htm"
		} else {
			s.TakeItems(p, ancientClayUrn, 1)
			s.RewardItems(p, soulshotD, 70+int32(s.Rnd(41)))
		}
	case is("30314-04.htm"):
		if !p.HasItem(ancientBrassTiara) {
			htmltext = "30314-07.htm"
		} else {
			s.TakeItems(p, ancientBrassTiara, 1)
			rnd := s.Rnd(100)
			switch {
			case rnd < 40:
				s.RewardItems(p, healingPotion, 1)
			case rnd < 84:
				s.RewardItems(p, hastePotion, 1)
			default:
				s.RewardItems(p, potionOfAlacrity, 1)
			}
		}
	case is("30314-05.htm"):
		if !p.HasItem(ancientBronzeMirror) {
			htmltext = "30314-07.htm"
		} else {
			s.TakeItems(p, ancientBronzeMirror, 1)
			scroll := int32(scrollOfResurrection)
			if s.Rnd(100) < 59 {
				scroll = scrollOfEscape
			}
			s.RewardItems(p, scroll, 1)
		}
	case is("30314-06.htm"):
		if !p.HasItem(ancientJadeNecklace) {
			htmltext = "30314-07.htm"
		} else {
			s.TakeItems(p, ancientJadeNecklace, 1)
			s.RewardItems(p, spiritshotD, 50+int32(s.Rnd(41)))
		}
	}
	return htmltext
}

// restore is Asha's restoration of five relic pieces: it succeeds when a
// draw of [0, bound) is below wins, giving the restored relic. Without
// five pieces nothing happens and the page stays htmltext.
func restore(s *script.Script, p *script.Player, htmltext string, piece int32, bound, wins int, success string, relic int32) string {
	if p.ItemCount(piece) < 5 {
		return htmltext
	}
	s.TakeItems(p, piece, 5)
	if s.Rnd(bound) < wins {
		s.RewardItems(p, relic, 1)
		return success
	}
	return "30313-10.htm"
}

// sellPieces is Iris's purchase of every piece of a kind, at exp
// experience each.
func sellPieces(s *script.Script, p *script.Player, htmltext string, piece int32, exp int64) string {
	n := p.ItemCount(piece)
	if n == 0 {
		return "30034-02.htm"
	}
	s.PlaySound(p, script.SoundItemGet)
	s.TakeItems(p, piece, n)
	s.RewardExpAndSp(p, int64(n)*exp, 0)
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	npcID := e.NPC.NpcID()
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 25 {
			htmltext = strconv.Itoa(int(npcID)) + "-01.htm"
		} else {
			htmltext = strconv.Itoa(int(npcID)) + "-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch npcID {
		case piotur:
			if !p.HasItem(leikanLetter) {
				if p.HasAtLeastOneItem(turekDogtag, turekMedallion) {
					htmltext = "30597-05.htm"
					if cond < 4 {
						st.SetCond(4)
						s.PlaySound(p, script.SoundMiddle)
					}
					dogtag := p.ItemCount(turekDogtag)
					medallion := p.ItemCount(turekMedallion)
					s.TakeItems(p, turekDogtag, -1)
					s.TakeItems(p, turekMedallion, -1)
					bonus := int32(0)
					if dogtag+medallion >= 10 {
						bonus = 619
					}
					s.RewardItems(p, adena, dogtag*40+medallion*50+bonus)
				} else {
					htmltext = "30597-04.htm"
				}
			} else {
				htmltext = "30597-03a.htm"
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, leikanLetter, 1)
			}
		case leikan:
			switch cond {
			case 2:
				htmltext = "30382-04.htm"
			case 3, 4:
				htmltext = "30382-05.htm"
				st.SetCond(5)
				s.PlaySound(p, script.SoundMiddle)
			case 5:
				htmltext = "30382-05.htm"
			}
		default:
			htmltext = strconv.Itoa(int(npcID)) + "-01.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	for _, d := range droplist() {
		if d.npcID == e.NPC.NpcID() {
			s.DropItemsAlways(p, d.itemID, 1, -1)
			s.DropItems(p, rndBetween(s, 1848, 1851), 1, 0, d.chance)
			break
		}
	}
}

// rndBetween returns a random number of [lo, hi].
func rndBetween(s *script.Script, lo, hi int32) int32 {
	return lo + int32(s.Rnd(int(hi-lo+1)))
}
