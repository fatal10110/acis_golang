// Package q503 is the quest Pursuit Of Clan Ambition (Sir Gustav Athebaldt):
// the leader of a level 4 clan runs a chain of errands for the clan's sake,
// and the clan's members help along the way. Every progress check follows
// the leader's quest state, wherever the member stands.
package q503

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q503_PursuitOfClanAmbition"

// NPCs.
const (
	kusto          = 30512
	martien        = 30645
	witchAthrea    = 30758
	witchKalis     = 30759
	sirGustav      = 30760
	corpseOfFritz  = 30761
	corpseOfLutz   = 30762
	corpseOfKurtz  = 30763
	balthazar      = 30764
	imperialCoffer = 30765
	witchCleo      = 30766
	sirEricRodemai = 30868
)

// Monsters.
const (
	thunderWyrm         = 20243
	thunderWyrmHold     = 20282
	blitzWyrm           = 27178
	drake               = 20137
	drakeHold           = 20285
	graveGuard          = 20668
	graveKeymaster      = 27179
	lesserGiantSoldier  = 20654
	lesserGiantScout    = 20656
	imperialGravekeeper = 27181
)

// Quest items.
const (
	recipeTitanPowerstone = 3838
	mistDrakeEgg          = 3839
	blitzWyrmEgg          = 3840
	drakeEgg              = 3841
	thunderWyrmEgg        = 3842
	broochOfTheMagpie     = 3843
	nebuliteCrystals      = 3844
	brokenTitanPowerstone = 3845
	titanPowerstone       = 3846
	imperialKey           = 3847
	gustav1stLetter       = 3866
	gustav2ndLetter       = 3867
	gustav3rdLetter       = 3868
	scepterOfJudgment     = 3869
	blackAnvilCoin        = 3871
	sealOfAspiration      = 3870
)

// The chat lines of Cleo, the witches she calls and the coffer.
const (
	sayCleo   = 50338
	sayCoffer = 50339
	sayAthrea = 50340
	sayKalis  = 50341
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Pursuit Of Clan Ambition",
		QuestID: 503,
		Items: []int32{
			mistDrakeEgg, blitzWyrmEgg, drakeEgg, thunderWyrmEgg, broochOfTheMagpie, nebuliteCrystals,
			titanPowerstone, imperialKey, gustav1stLetter, gustav2ndLetter, gustav3rdLetter, scepterOfJudgment,
			blackAnvilCoin,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {
				sirGustav, martien, corpseOfFritz, corpseOfLutz, corpseOfKurtz, kusto, balthazar, sirEricRodemai,
				witchCleo, witchAthrea, witchKalis, imperialCoffer,
			},
			script.EventTalked: {
				sirGustav, martien, corpseOfFritz, corpseOfLutz, corpseOfKurtz, kusto, balthazar, sirEricRodemai,
				witchCleo, witchAthrea, witchKalis, imperialCoffer,
			},
			script.EventMyDying: {
				blitzWyrm, drake, drakeHold, graveGuard, graveKeymaster, lesserGiantScout, lesserGiantSoldier,
				thunderWyrm, thunderWyrmHold, imperialGravekeeper,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTimer: onTimer, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// finish pays the Seal of Aspiration for the Scepter of Judgment and ends the
// quest; false when the player has no scepter.
func finish(s *script.Script, p *script.Player, st *script.QuestState) bool {
	if !p.HasItem(scepterOfJudgment) {
		return false
	}
	s.TakeItems(p, scepterOfJudgment, -1)
	s.GiveItems(p, sealOfAspiration, 1)
	s.RewardExpAndSp(p, 0, 250000)
	s.PlaySound(p, script.SoundFinish)
	st.Exit(true)
	return true
}

// callBlitzWyrms stirs two Blitz Wyrms out of a corpse, which is taken away
// soon after.
func callBlitzWyrms(s *script.Script, corpse *script.NPC) {
	s.Spawner().CreateOnePrivate(corpse, blitzWyrm, 180000*time.Millisecond)
	s.Spawner().CreateOnePrivate(corpse, blitzWyrm, 180000*time.Millisecond)
	s.StartTimer("despawn", corpse, nil, 10000*time.Millisecond)
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	p := e.Player
	st := s.QuestState(p, questName)
	if st == nil {
		return htmltext
	}
	switch {
	// Sir Gustav Athebaldt
	case strings.EqualFold(e.Name, "30760-08.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.GiveItems(p, gustav1stLetter, 1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30760-12.htm"):
		st.SetCond(4)
		s.GiveItems(p, gustav2ndLetter, 1)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30760-16.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, gustav3rdLetter, 1)
	case strings.EqualFold(e.Name, "30760-20.htm"):
		if !finish(s, p, st) {
			return ""
		}
	case strings.EqualFold(e.Name, "30760-22.htm"):
		if st.Cond() == 11 {
			// The page can be asked for again; only the first ask moves on.
			st.SetCond(12)
			s.PlaySound(p, script.SoundMiddle)
		}
	case strings.EqualFold(e.Name, "30760-23.htm"):
		if !finish(s, p, st) {
			return ""
		}
	// Martien: the three dwarf corpses each hold their reward once.
	case strings.EqualFold(e.Name, "30645-03.htm"):
		st.Set("30761", strconv.FormatBool(true))
		st.Set("30762", strconv.FormatBool(true))
		st.Set("30763", strconv.FormatBool(true))

		st.SetCond(2)
		s.TakeItems(p, gustav1stLetter, -1)
		s.PlaySound(p, script.SoundMiddle)
	// Corpse of Fritz
	case strings.EqualFold(e.Name, "30761-02.htm"):
		flag := strconv.Itoa(int(e.NPC.NpcID()))
		if _, ok := st.Get(flag); ok {
			st.Unset(flag)
			s.GiveItems(p, blitzWyrmEgg, 3)
		} else {
			htmltext = "30761-03.htm"
		}
		callBlitzWyrms(s, e.NPC)
	// Corpse of Lutz
	case strings.EqualFold(e.Name, "30762-02.htm"):
		flag := strconv.Itoa(int(e.NPC.NpcID()))
		if _, ok := st.Get(flag); ok {
			st.Unset(flag)
			s.GiveItems(p, blitzWyrmEgg, 3)
			s.GiveItems(p, mistDrakeEgg, 4)
		} else {
			htmltext = "30761-03.htm"
		}
		callBlitzWyrms(s, e.NPC)
	// Corpse of Kurtz
	case strings.EqualFold(e.Name, "30763-02.htm"):
		st.Unset(strconv.Itoa(int(e.NPC.NpcID())))
		s.GiveItems(p, broochOfTheMagpie, 1)
		s.GiveItems(p, mistDrakeEgg, 6)

		e.NPC.DeleteMe()
	// Kusto
	case strings.EqualFold(e.Name, "30512-03.htm"):
		if p.HasItem(broochOfTheMagpie) {
			s.TakeItems(p, broochOfTheMagpie, -1)
			s.GiveItems(p, blackAnvilCoin, 1)
		}
	// Balthazar
	case strings.EqualFold(e.Name, "30764-03.htm"):
		st.SetCond(5)
		s.TakeItems(p, gustav2ndLetter, -1)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30764-06.htm"):
		st.SetCond(5)
		s.TakeItems(p, gustav2ndLetter, -1)
		s.TakeItems(p, blackAnvilCoin, -1)
		s.GiveItems(p, recipeTitanPowerstone, 1)
		s.PlaySound(p, script.SoundMiddle)
	// Sir Eric Rodemai
	case strings.EqualFold(e.Name, "30868-04.htm"):
		st.SetCond(8)
		s.TakeItems(p, gustav3rdLetter, -1)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30868-10.htm"):
		st.SetCond(11)
		s.PlaySound(p, script.SoundMiddle)
	// Witch Cleo
	case strings.EqualFold(e.Name, "30766-04.htm"):
		e.NPC.Say(script.NpcString(sayCleo))

		witch := s.Spawner().CreateOnePrivateEx(e.NPC, witchAthrea, script.Loc{X: 160688, Y: 21296, Z: -3714, Heading: 32768}, 5000*time.Millisecond, script.Params{})
		witch.Say(script.NpcString(sayAthrea))

		witch = s.Spawner().CreateOnePrivateEx(e.NPC, witchKalis, script.Loc{X: 160690, Y: 21176, Z: -3712, Heading: 32768}, 5000*time.Millisecond, script.Params{})
		witch.Say(script.NpcString(sayKalis))

		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30766-08.htm"):
		if !finish(s, p, st) {
			return ""
		}
	// Imperial Coffer
	case strings.EqualFold(e.Name, "30765-04.htm"):
		if p.ItemCount(imperialKey) >= 6 {
			st.Unset("killed")
			s.TakeItems(p, imperialKey, -1)
			s.GiveItems(p, scepterOfJudgment, 1)
		} else {
			htmltext = "30765-05a.htm"
		}
	}
	return htmltext
}

// onTimer takes the corpse away once the Blitz Wyrms have come out of it.
func onTimer(_ *script.Script, e script.Timer) string {
	e.NPC.DeleteMe()
	return ""
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	p := e.Player
	lst := s.ClanLeaderQuestState(p, nil)
	if lst == nil {
		if e.NPC.NpcID() == sirGustav && !p.IsClanLeader() {
			htmltext = "30760-04t.htm"
		}
		return htmltext
	}

	switch lst.Status() {
	case questlog.StatusCreated:
		if e.NPC.NpcID() == sirGustav {
			if p.IsClanLeader() {
				level := p.ClanLevel()
				switch {
				case level < 4:
					htmltext = "30760-01.htm"
				case level == 4:
					if p.HasItem(sealOfAspiration) {
						htmltext = "30760-03.htm"
					} else {
						htmltext = "30760-04.htm"
					}
				default:
					htmltext = "30760-02.htm"
				}
			} else {
				htmltext = "30760-04t.htm"
			}
		}
	case questlog.StatusStarted:
		cond := lst.Cond()
		leader := p.IsClanLeader()
		switch e.NPC.NpcID() {
		case sirGustav:
			if leader {
				switch {
				case cond == 1:
					htmltext = "30760-09.htm"
				case cond == 2:
					htmltext = "30760-10.htm"
				case cond == 3:
					htmltext = "30760-11.htm"
				case cond == 4:
					htmltext = "30760-13.htm"
				case cond == 5:
					htmltext = "30760-14.htm"
				case cond == 6:
					htmltext = "30760-15.htm"
				case cond == 7:
					htmltext = "30760-17.htm"
				case cond == 8 || cond == 9:
					htmltext = "30760-18.htm"
				case cond == 10:
					if p.HasItem(scepterOfJudgment) {
						htmltext = "30760-19.htm"
					} else {
						htmltext = "30760-18.htm"
					}
				case cond == 11:
					htmltext = "30760-19.htm"
				case cond == 12:
					htmltext = "30760-24.htm"
				}
			} else {
				switch cond {
				case 3:
					htmltext = "30760-11t.htm"
				case 6:
					htmltext = "30760-15t.htm"
				case 11:
					htmltext = "30760-19t.htm"
				case 12:
					htmltext = "30760-24t.htm"
				}
			}
		case martien:
			if leader {
				switch cond {
				case 1:
					htmltext = "30645-02.htm"
				case 2:
					if p.ItemCount(mistDrakeEgg) > 9 && p.ItemCount(blitzWyrmEgg) > 9 && p.ItemCount(drakeEgg) > 9 && p.ItemCount(thunderWyrmEgg) > 9 {
						htmltext = "30645-05.htm"
						s.TakeItems(p, mistDrakeEgg, -1)
						s.TakeItems(p, blitzWyrmEgg, -1)
						s.TakeItems(p, drakeEgg, -1)
						s.TakeItems(p, thunderWyrmEgg, -1)
						lst.SetCond(3)
						s.PlaySound(p, script.SoundMiddle)
					} else {
						htmltext = "30645-04.htm"
					}
				case 3:
					htmltext = "30645-07.htm"
				default:
					htmltext = "30645-08.htm"
				}
			} else if cond == 1 {
				htmltext = "30645-01.htm"
			}
		case corpseOfFritz:
			if leader && cond == 2 {
				htmltext = "30761-01.htm"
			}
		case corpseOfLutz:
			if leader && cond == 2 {
				htmltext = "30762-01.htm"
			}
		case corpseOfKurtz:
			if leader && cond == 2 {
				if _, ok := lst.Get(strconv.Itoa(int(e.NPC.NpcID()))); ok {
					htmltext = "30763-01.htm"
				} else {
					htmltext = "30763-03.htm"
				}
			}
		case kusto:
			if leader {
				if cond <= 4 {
					switch {
					case !p.HasItem(broochOfTheMagpie) && !p.HasItem(blackAnvilCoin):
						htmltext = "30512-01.htm"
					case p.HasItem(broochOfTheMagpie):
						htmltext = "30512-02.htm"
					case p.HasItem(blackAnvilCoin) && !p.HasItem(broochOfTheMagpie):
						htmltext = "30512-04.htm"
					}
				}
			} else {
				htmltext = "30512-01a.htm"
			}
		case balthazar:
			if leader {
				switch {
				case cond == 4:
					if p.HasItem(blackAnvilCoin) {
						htmltext = "30764-04.htm"
					} else {
						htmltext = "30764-02.htm"
					}
				case cond == 5:
					if p.ItemCount(nebuliteCrystals) > 9 && p.ItemCount(titanPowerstone) > 9 {
						htmltext = "30764-08.htm"
						lst.SetCond(6)
						s.TakeItems(p, nebuliteCrystals, -1)
						s.TakeItems(p, titanPowerstone, -1)
						s.PlaySound(p, script.SoundMiddle)
					} else {
						htmltext = "30764-07.htm"
					}
				case cond >= 6:
					htmltext = "30764-09.htm"
				}
			} else if cond == 4 {
				htmltext = "30764-01.htm"
			}
		case sirEricRodemai:
			if leader {
				switch {
				case cond == 7:
					htmltext = "30868-02.htm"
				case cond == 8:
					htmltext = "30868-05.htm"
				case cond == 9:
					htmltext = "30868-06.htm"
					lst.SetCond(10)
					s.PlaySound(p, script.SoundMiddle)
				case cond == 10:
					if p.HasItem(scepterOfJudgment) {
						htmltext = "30868-09.htm"
					} else {
						htmltext = "30868-08.htm"
					}
				case cond >= 11:
					htmltext = "30868-11.htm"
				}
			} else {
				switch cond {
				case 7:
					htmltext = "30868-01.htm"
				case 9:
					htmltext = "30868-07.htm"
				}
			}
		case witchCleo:
			if leader {
				switch cond {
				case 8:
					htmltext = "30766-02.htm"
				case 9:
					htmltext = "30766-05.htm"
				case 10, 11:
					htmltext = "30766-06.htm"
				case 12:
					htmltext = "30766-07.htm"
				}
			} else {
				htmltext = "30766-01.htm"
			}
		case witchAthrea:
			if cond == 9 {
				htmltext = "30758-01.htm"
			}
		case witchKalis:
			if cond == 9 {
				htmltext = "30759-01.htm"
			}
		case imperialCoffer:
			if cond == 10 {
				if leader {
					if p.HasItem(scepterOfJudgment) {
						htmltext = "30765-05.htm"
					} else {
						htmltext = "30765-03.htm"
					}
				} else {
					htmltext = "30765-01.htm"
				}
			}
		}
	}
	return htmltext
}

// onMyDying pays every drop to the killer's clan leader, when the leader
// stands near; the Grave Guards of the last step call their Keymaster after a
// count of kills.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	lst := s.ClanLeaderQuestState(p, e.NPC)
	if lst == nil {
		return
	}
	leader := lst.Player()

	switch e.NPC.NpcID() {
	case thunderWyrm, thunderWyrmHold:
		if lst.Cond() == 2 {
			s.DropItems(leader, thunderWyrmEgg, 1, 10, 500000)
		}
	case drake, drakeHold:
		if lst.Cond() == 2 {
			s.DropItems(leader, mistDrakeEgg, 1, 10, 100000)
			s.DropItems(leader, drakeEgg, 1, 10, 500000)
		}
	case blitzWyrm:
		if lst.Cond() == 2 {
			s.DropItemsAlways(leader, blitzWyrmEgg, 1, 10)
		}
	case lesserGiantScout, lesserGiantSoldier:
		if lst.Cond() == 5 {
			chance := s.Rnd(100)
			switch {
			case chance < 10:
				s.DropItemsAlways(leader, titanPowerstone, 1, 10)
			case chance < 30:
				s.DropItemsAlways(leader, nebuliteCrystals, 1, 10)
			case chance < 80:
				s.DropItemsAlways(leader, brokenTitanPowerstone, 1, 0)
			}
		}
	case graveGuard:
		if lst.Cond() == 10 && !leader.HasItem(scepterOfJudgment) {
			// Kills 1-4 never call the Keymaster, 5-9 call it half the time, the
			// 10th always does.
			killed := lst.GetInt("killed")
			killed++
			if (s.Rnd(100) < 50 && killed > 4) || killed > 9 {
				killed = 0
				s.Spawner().AddSpawn(graveKeymaster, e.NPC, false, 0)
			}
			lst.Set("killed", strconv.Itoa(int(killed)))
		}
	case graveKeymaster:
		if lst.Cond() == 10 && !leader.HasItem(scepterOfJudgment) {
			s.DropItemsAlways(leader, imperialKey, 1, 6)
		}
	case imperialGravekeeper:
		if lst.Cond() == 10 && !leader.HasItem(scepterOfJudgment) {
			coffer := s.Spawner().AddSpawn(imperialCoffer, e.NPC, false, 180000*time.Millisecond)
			coffer.Say(script.NpcString(sayCoffer))
		}
	}
}
