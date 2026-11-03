// Package q343 is the quest Under the Shadow of the Ivory Tower: Cema buys
// nebulite orbs, Marsha and Trumpin gamble for them and Icarus trades
// ectoplasm liqueur for rewards.
package q343

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q343_UnderTheShadowOfTheIvoryTower"

// NPCs.
const (
	cema    = 30834
	icarus  = 30835
	marsha  = 30934
	trumpin = 30935
)

// Item.
const nebuliteOrb = 4364

// Rewards.
const (
	towerShield               = 103
	necklaceOfMagic           = 118
	sageBlood                 = 316
	squareShield              = 630
	scrollOfEscape            = 736
	ringOfAges                = 885
	necklaceOfMermaid         = 917
	scrollEnchantWeaponCGrade = 951
	scrollEnchantWeaponDGrade = 955
	spiritshotDGrade          = 2510
	spiritshotCGrade          = 2511
	ectoplasmLiqueur          = 4365
)

// drop is a monster's nebulite orb drop chance and its chance, in percent,
// to lower the memoStateEx count.
type drop struct {
	chance int32
	lower  int32
}

// drops returns the drop of each monster, by NPC id.
func drops() map[int32]drop {
	return map[int32]drop{
		20563: {630000, 12}, // Manashen Gargoyle
		20564: {630000, 12}, // Enchanted Monster Eye
		20565: {650000, 12}, // Enchanted Stone Golem
		20566: {680000, 13}, // Enchanted Iron Golem
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Under the Shadow of the Ivory Tower",
		QuestID: 343,
		Bind: script.Bindings{
			script.EventQuestStart: {cema},
			script.EventTalked:     {cema, icarus, marsha, trumpin},
			script.EventMyDying:    {20563, 20564, 20565, 20566},
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
	case strings.EqualFold(e.Name, "30834-05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		st.Set("memoState", itoa(1))
		st.Set("memoStateEx", itoa(0))
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30834-08.htm"):
		if p.HasItem(nebuliteOrb) {
			count := p.ItemCount(nebuliteOrb) * 120
			s.TakeItems(p, nebuliteOrb, -1)
			s.GiveItems(p, 57, count)
		} else {
			htmltext = "30834-08a.htm"
		}
	case strings.EqualFold(e.Name, "30834-11.htm"):
		s.PlaySound(p, script.SoundGiveUp)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30835-02.htm"):
		if p.HasItem(ectoplasmLiqueur) {
			htmltext = "30835-03.htm"
			s.TakeItems(p, ectoplasmLiqueur, 1)
			icarusReward(s, p)
		}
	case strings.EqualFold(e.Name, "30934-05.htm"):
		if st.GetInt("memoState") == 1 {
			memoStateEx := st.GetInt("memoStateEx")
			if memoStateEx >= 1 && memoStateEx < 25 {
				if p.ItemCount(nebuliteOrb) < 10 {
					htmltext = "30934-06.htm"
				} else {
					st.Set("memoState", itoa(2))
					s.TakeItems(p, nebuliteOrb, 10)
					htmltext = "30934-07.htm"
				}
			}
		}
	case strings.EqualFold(e.Name, "30934-08a.htm"):
		if st.GetInt("memoState") == 2 {
			htmltext = marshaDeal(s, st, htmltext)
		}
	case strings.EqualFold(e.Name, "30934-11a.htm"):
		if st.GetInt("memoState") == 2 {
			switch st.GetInt("param1") {
			case 0:
				s.GiveItems(p, nebuliteOrb, 10)
				st.Set("param1", itoa(4))
			case 1:
				htmltext = "30934-11b.htm"
			case 2:
				s.GiveItems(p, nebuliteOrb, 20)
				st.Set("param1", itoa(4))
				htmltext = "30934-11c.htm"
			}
			st.Set("memoState", itoa(1))
		}
	case strings.EqualFold(e.Name, "30934-12a.htm"):
		if st.GetInt("memoState") == 2 {
			switch st.GetInt("param1") {
			case 0:
				s.GiveItems(p, nebuliteOrb, 20)
				st.Set("param1", itoa(4))
			case 1:
				s.GiveItems(p, nebuliteOrb, 10)
				st.Set("param1", itoa(4))
				htmltext = "30934-12b.htm"
			case 2:
				htmltext = "30934-12c.htm"
			}
			st.Set("memoState", itoa(1))
		}
	case strings.EqualFold(e.Name, "30934-13a.htm"):
		if st.GetInt("memoState") == 2 {
			switch st.GetInt("param1") {
			case 1:
				s.GiveItems(p, nebuliteOrb, 20)
				st.Set("param1", itoa(4))
				htmltext = "30934-13b.htm"
			case 2:
				s.GiveItems(p, nebuliteOrb, 10)
				st.Set("param1", itoa(4))
				htmltext = "30934-13c.htm"
			}
			st.Set("memoState", itoa(1))
		}
	case strings.EqualFold(e.Name, "30935-03.htm"):
		if p.ItemCount(nebuliteOrb) >= 10 {
			st.Set("param2", itoa(int32(s.Rnd(2))))
			htmltext = "30935-04.htm"
		}
	case strings.EqualFold(e.Name, "30935-05.htm"):
		if p.ItemCount(nebuliteOrb) >= 10 {
			switch st.GetInt("param2") {
			case 0:
				htmltext = trumpinWin(s, p, st, htmltext, htmltext, "30935-05a.htm", "30935-05b.htm", "30935-05c.htm", "30935-05d.htm")
			case 1:
				s.TakeItems(p, nebuliteOrb, 10)
				st.Set("param1", itoa(0))
				st.Set("param2", itoa(2))
				htmltext = "30935-06.htm"
			}
		} else {
			htmltext = "30935-03.htm"
		}
	case strings.EqualFold(e.Name, "30935-07.htm"):
		if p.ItemCount(nebuliteOrb) >= 10 {
			switch st.GetInt("param2") {
			case 0:
				s.TakeItems(p, nebuliteOrb, 10)
				st.Set("param1", itoa(0))
				st.Set("param2", itoa(2))
			case 1:
				htmltext = trumpinWin(s, p, st, htmltext, "30935-08.htm", "30935-08a.htm", "30935-08b.htm", "30935-08c.htm", "30935-08d.htm")
			}
		} else {
			htmltext = "30935-03.htm"
		}
	case strings.EqualFold(e.Name, "30935-09.htm"):
		switch st.GetInt("param1") {
		case 1:
			st.Set("param1", itoa(0))
			st.Set("param2", itoa(2))
			s.GiveItems(p, nebuliteOrb, 10)
		case 2:
			st.Set("param1", itoa(0))
			st.Set("param2", itoa(2))
			s.GiveItems(p, nebuliteOrb, 30)
			htmltext = "30935-09a.htm"
		case 3:
			st.Set("param1", itoa(0))
			st.Set("param2", itoa(2))
			s.GiveItems(p, nebuliteOrb, 70)
			htmltext = "30935-09b.htm"
		case 4:
			st.Set("param1", itoa(0))
			st.Set("param2", itoa(2))
			s.GiveItems(p, nebuliteOrb, 150)
			htmltext = "30935-09c.htm"
		}
	case strings.EqualFold(e.Name, "30935-10.htm"):
		st.Set("param1", itoa(0))
		st.Set("param2", itoa(2))
	}
	return htmltext
}

// icarusReward gives the reward Icarus draws for one ectoplasm liqueur.
func icarusReward(s *script.Script, p *script.Player) {
	chance := s.Rnd(1000)
	switch {
	case chance < 120:
		s.GiveItems(p, scrollEnchantWeaponDGrade, 1)
	case chance < 170:
		s.GiveItems(p, scrollEnchantWeaponCGrade, 1)
	case chance < 330:
		s.GiveItems(p, spiritshotCGrade, int32(s.Rnd(200)+401))
	case chance < 560:
		s.GiveItems(p, spiritshotDGrade, int32(s.Rnd(200)+401))
	case chance < 562:
		s.GiveItems(p, sageBlood, 1)
	case chance < 579:
		s.GiveItems(p, squareShield, 1)
	case chance < 580:
		s.GiveItems(p, necklaceOfMagic, 1)
	case chance < 582:
		s.GiveItems(p, ringOfAges, 1)
	case chance < 583:
		s.GiveItems(p, towerShield, 1)
	case chance < 585:
		s.GiveItems(p, necklaceOfMermaid, 1)
	default:
		s.GiveItems(p, scrollOfEscape, 1)
	}
}

// marshaDeal draws Marsha's hidden card and her hint about it, and returns
// the page that tells the hint (htmltext when she says nothing).
func marshaDeal(s *script.Script, st *script.QuestState, htmltext string) string {
	i0 := s.Rnd(100)
	i1 := s.Rnd(3)
	switch {
	case i0 < 20:
		switch i1 {
		case 0:
			st.Set("param1", itoa(0))
		case 1:
			st.Set("param1", itoa(1))
			htmltext = "30934-08b.htm"
		case 2:
			st.Set("param1", itoa(2))
			htmltext = "30934-08c.htm"
		}
		st.Set("memoStateEx", itoa(st.GetInt("memoStateEx")+4))
	case i0 < 50:
		switch i1 {
		case 0:
			htmltext = "30934-09a.htm"
			st.Set("param1", itoa(pick(s.RndBool(), 0, 1)))
		case 1:
			htmltext = "30934-09b.htm"
			st.Set("param1", itoa(pick(s.RndBool(), 1, 2)))
		case 2:
			htmltext = "30934-09c.htm"
			st.Set("param1", itoa(pick(s.RndBool(), 2, 0)))
		}
		st.Set("memoStateEx", itoa(st.GetInt("memoStateEx")+4))
	default:
		htmltext = "30934-10.htm"
		st.Set("param1", itoa(int32(i1)))
	}
	return htmltext
}

// trumpinWin moves a won round of Trumpin's game one step up and returns its
// page: param1 0 to 3 climb (pages first to fourth), and the fifth win pays
// 310 orbs and starts over. Any other param1 changes nothing and keeps
// htmltext.
func trumpinWin(s *script.Script, p *script.Player, st *script.QuestState, htmltext, first, second, third, fourth, fifth string) string {
	switch st.GetInt("param1") {
	case 0:
		st.Set("param1", itoa(1))
		st.Set("param2", itoa(2))
		htmltext = first
	case 1:
		st.Set("param1", itoa(2))
		st.Set("param2", itoa(2))
		htmltext = second
	case 2:
		st.Set("param1", itoa(3))
		st.Set("param2", itoa(2))
		htmltext = third
	case 3:
		st.Set("param1", itoa(4))
		st.Set("param2", itoa(2))
		htmltext = fourth
	case 4:
		st.Set("param1", itoa(0))
		st.Set("param2", itoa(2))
		s.GiveItems(p, nebuliteOrb, 310)
		htmltext = fifth
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
		case script.ClassTypeOf(p.ClassID()) != player.ClassTypeMystic || p.Race() == player.RaceOrc:
			htmltext = "30834-01.htm"
		case p.Level() < 40:
			htmltext = "30834-02.htm"
		default:
			htmltext = "30834-03.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case cema:
			if !p.HasItem(nebuliteOrb) {
				htmltext = "30834-06.htm"
			} else {
				htmltext = "30834-07.htm"
			}
		case icarus:
			htmltext = "30835-01.htm"
		case marsha:
			if st.GetInt("memoStateEx") == 0 {
				st.Set("memoStateEx", itoa(1))
				htmltext = "30934-03.htm"
			} else {
				st.Set("memoState", itoa(1))
				htmltext = "30934-04.htm"
			}
		case trumpin:
			st.Set("param1", itoa(0))
			htmltext = "30935-01.htm"
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
	data, ok := drops()[e.NPC.NpcID()]
	if !ok {
		return
	}
	s.DropItems(p, nebuliteOrb, 1, -1, data.chance)
	memoStateEx := st.GetInt("memoStateEx")
	if memoStateEx > 1 && int32(s.Rnd(100)) <= data.lower {
		st.Set("memoStateEx", itoa(memoStateEx-1))
	}
}

// pick returns a when cond holds, else b.
func pick(cond bool, a, b int32) int32 {
	if cond {
		return a
	}
	return b
}

func itoa(n int32) string { return strconv.Itoa(int(n)) }
