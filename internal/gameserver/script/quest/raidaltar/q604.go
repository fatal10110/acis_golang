package raidaltar

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const q604Name = "Q604_DaimonTheWhiteEyed_Part2"

// daimonTheWhiteEyed is the raid the altar calls.
const daimonTheWhiteEyed = 25290

// NPCs.
const (
	eyeOfArgos  = 31683
	daimonAltar = 31541
)

// Items.
const (
	unfinishedSummonCrystal = 7192
	summonCrystal           = 7193
	essenceOfDaimon         = 7194
)

// q604Rewards are the items the essence is traded for, one at random.
func q604Rewards() []int32 { return []int32{4595, 4596, 4597, 4598, 4599, 4600} }

// q604 is the quest; the raid mechanics are the site's.
type q604 struct{ site }

// Q604 is the quest Daimon The White-Eyed, Part 2: the Eye of Argos
// turns the unfinished summon crystal into a summon crystal, the altar of
// Daimon spends it on calling Daimon the White-Eyed, and his essence is
// traded back for a reward.
func Q604() script.Script {
	q := q604{newSite(daimonTheWhiteEyed, script.Loc{X: 186320, Y: -43904, Z: -3175}, 60403, 60404, essenceOfDaimon)}
	return script.Script{
		Title:   "Daimon The White-Eyed - Part 2",
		QuestID: 604,
		Items:   []int32{summonCrystal, essenceOfDaimon},
		Bind: script.Bindings{
			script.EventQuestStart: {eyeOfArgos},
			script.EventTalked:     {eyeOfArgos, daimonAltar},
			script.EventDecayed:    {daimonTheWhiteEyed},
			script.EventMyDying:    {daimonTheWhiteEyed},
		},
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: q.onTalk, OnDecayed: q.onDecayed, OnMyDying: q.onMyDying},
	}
}

func (q q604) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, q604Name)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	// Eye of Argos.
	case strings.EqualFold(e.Name, "31683-03.htm"):
		if p.HasItem(unfinishedSummonCrystal) {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
			s.TakeItems(p, unfinishedSummonCrystal, 1)
			s.GiveItems(p, summonCrystal, 1)
		} else {
			htmltext = "31683-04.htm"
		}
	case strings.EqualFold(e.Name, "31683-08.htm"):
		if p.HasItem(essenceOfDaimon) {
			s.TakeItems(p, essenceOfDaimon, 1)
			rs := q604Rewards()
			s.RewardItems(p, rs[s.Rnd(len(rs))], 5)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		} else {
			htmltext = "31683-09.htm"
		}
	// Daimon's altar.
	case strings.EqualFold(e.Name, "31541-02.htm"):
		switch {
		case !p.HasItem(summonCrystal):
			htmltext = "31541-03.htm"
		case !q.raid.Empty():
			htmltext = "31541-04.htm"
		default:
			if !q.call(s, e.NPC) {
				htmltext = "31541-04.htm"
				break
			}
			st.SetCond(2)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, summonCrystal, 1)
		}
	}
	return htmltext
}

func (q q604) onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, q604Name)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 73 {
			htmltext = "31683-02.htm"
			st.Exit(true)
		} else {
			htmltext = "31683-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case eyeOfArgos:
			switch cond {
			case 1:
				htmltext = "31683-05.htm"
			case 2:
				htmltext = "31683-06.htm"
			default:
				htmltext = "31683-07.htm"
			}
		case daimonAltar:
			switch cond {
			case 1:
				htmltext = "31541-01.htm"
			case 2:
				htmltext = "31541-05.htm"
			}
		}
	}
	return htmltext
}
