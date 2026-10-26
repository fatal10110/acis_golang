// Package q348 is the quest An Arrogant Search: the player gathers a titan's
// powerstone for Hanellin, fetches the three saints' relics from the holy arks
// the ark guardians watch, then carries white fabric to the Platinum Tribe to
// have it blooded for the alchemists.
package q348

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q348_AnArrogantSearch"

// Items.
const (
	titanPowerstone      = 4287
	hanellinFirstLetter  = 4288
	hanellinSecondLetter = 4289
	hanellinThirdLetter  = 4290
	firstKeyOfArk        = 4291
	secondKeyOfArk       = 4292
	thirdKeyOfArk        = 4293
	whiteFabric          = 4294
	bloodedFabric        = 4295
	bookOfSaint          = 4397
	bloodOfSaint         = 4398
	boughOfSaint         = 4399

	antidote      = 1831
	healingPotion = 1061
)

// NPCs.
const (
	hanellin          = 30864
	claudiaAthebaldt  = 31001
	martien           = 30645
	harne             = 30144
	arkGuardianCorpse = 30980
	holyArk1          = 30977
	holyArk2          = 30978
	holyArk3          = 30979
	gustavAthebaldt   = 30760
	hardin            = 30832
	iasonHeine        = 30969
)

// Monsters.
const (
	lesserGiantMage       = 20657
	lesserGiantElder      = 20658
	platinumTribeShaman   = 20828
	platinumTribeOverlord = 20829
	guardianAngel         = 20859
	sealAngel             = 20860
	angelKiller           = 27184
	arkGuardianElberoth   = 27182
	arkGuardianShadowFang = 27183
)

// What the quest monsters say.
const (
	sayAngelKillerDefeated = 34830
	sayAngelKillerSpawn    = 34831
	sayElberothDies        = 34832
	sayElberothHit         = 34833
	sayShadowFangDies      = 34835
	sayShadowFangHit       = 34836
	sayElberothSpawn       = 34837
	sayShadowFangSpawn     = 34838
	sayAngelKillerLeaves   = 34839
)

// The spawns last ten minutes.
const despawn = 600000 * time.Millisecond

// marker is a spot the radar shows.
type marker struct{ x, y, z int32 }

var (
	holyArk1Loc       = marker{-418, 44174, -3568}
	holyArk2Loc       = marker{181472, 7158, -2725}
	holyArk3Loc       = marker{50693, 158674, 376}
	guardianCorpseLoc = marker{-2908, 44128, -2712}
)

// quest holds the guardians the arks called, shared by every player: one of
// each at a time, until it decays.
type quest struct{ elberoth, shadowFang, angelKiller *script.NPCCell }

// New returns the quest.
func New() script.Script {
	q := quest{elberoth: new(script.NPCCell), shadowFang: new(script.NPCCell), angelKiller: new(script.NPCCell)}
	return script.Script{
		Title:   "An Arrogant Search",
		QuestID: 348,
		Items: []int32{
			titanPowerstone, hanellinFirstLetter, hanellinSecondLetter, hanellinThirdLetter,
			firstKeyOfArk, secondKeyOfArk, thirdKeyOfArk, bookOfSaint, bloodOfSaint, boughOfSaint, whiteFabric,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {hanellin},
			script.EventTalked: {
				hanellin, claudiaAthebaldt, martien, harne, holyArk1, holyArk2, holyArk3, arkGuardianCorpse,
				gustavAthebaldt, hardin, iasonHeine,
			},
			script.EventAttacked: {
				arkGuardianElberoth, arkGuardianShadowFang, angelKiller, platinumTribeShaman, platinumTribeOverlord,
			},
			script.EventCreated: {arkGuardianElberoth, arkGuardianShadowFang, angelKiller},
			script.EventDecayed: {arkGuardianElberoth, arkGuardianShadowFang, angelKiller},
			script.EventMyDying: {
				lesserGiantMage, lesserGiantElder, arkGuardianElberoth, arkGuardianShadowFang,
				platinumTribeShaman, platinumTribeOverlord, guardianAngel, sealAngel,
			},
		},
		Hooks: script.Hooks{
			OnEvent: onAdvEvent, OnTalk: q.onTalk, OnAttacked: onAttacked, OnCreated: onCreated,
			OnDecayed: q.onDecayed, OnMyDying: onMyDying,
		},
	}
}

// rndBetween returns a random number of [lo, hi].
func rndBetween(s *script.Script, lo, hi int) int {
	return lo + s.Rnd(hi-lo+1)
}

func setInt(st *script.QuestState, key string, n int32) {
	st.Set(key, strconv.Itoa(int(n)))
}

// mark shows m on p's radar.
func mark(p *script.Player, m marker) { p.AddRadarMarker(m.x, m.y, m.z) }

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30864-05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		st.SetCond(2)
		setInt(st, "points", 0)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30864-09.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, titanPowerstone, 1)
	case strings.EqualFold(e.Name, "30864-17.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, hanellinFirstLetter, 1)
		s.GiveItems(p, hanellinSecondLetter, 1)
		s.GiveItems(p, hanellinThirdLetter, 1)
	case strings.EqualFold(e.Name, "30864-36.htm"):
		st.SetCond(24)
		s.PlaySound(p, script.SoundMiddle)
		s.RewardItems(p, 57, int32(rndBetween(s, 1, 2)*12000))
	case strings.EqualFold(e.Name, "30864-37.htm"):
		st.SetCond(25)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30864-51.htm"):
		st.SetCond(26)
		s.PlaySound(p, script.SoundMiddle)
		if p.HasItem(bloodedFabric) {
			s.GiveItems(p, whiteFabric, 9)
		} else {
			s.GiveItems(p, whiteFabric, 10)
		}
	case strings.EqualFold(e.Name, "30864-58.htm"):
		st.SetCond(27)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30864-57.htm"):
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30864-56.htm"):
		st.SetCond(29)
		st.Unset("gustav")
		st.Unset("hardin")
		st.Unset("iason")
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, whiteFabric, 10)
	}
	return htmltext
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
		switch {
		case p.HasItem(bloodedFabric):
			htmltext = "30864-00.htm"
		case p.Level() < 60:
			htmltext = "30864-01.htm"
		default:
			htmltext = "30864-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case hanellin:
			htmltext = onHanellin(s, p, st, cond, htmltext)
		case gustavAthebaldt:
			if cond == 27 {
				switch {
				case p.ItemCount(bloodedFabric) >= 3 && st.GetInt("gustav") == 0:
					setInt(st, "gustav", 1)
					htmltext = "30760-01.htm"
					s.TakeItems(p, bloodedFabric, 3)
				case st.GetInt("gustav") == 1:
					htmltext = "30760-02.htm"
				default:
					htmltext = "30760-03.htm"
					setInt(st, "usedonbaium", 1)
				}
			}
		case hardin:
			if cond == 27 {
				switch {
				case p.HasItem(bloodedFabric) && st.GetInt("hardin") == 0:
					setInt(st, "hardin", 1)
					htmltext = "30832-01.htm"
					s.TakeItems(p, bloodedFabric, 1)
				case st.GetInt("hardin") == 1:
					htmltext = "30832-02.htm"
				default:
					htmltext = "30832-03.htm"
					setInt(st, "usedonbaium", 1)
				}
			}
		case iasonHeine:
			if cond == 27 {
				switch {
				case p.ItemCount(bloodedFabric) >= 6 && st.GetInt("iason") == 0:
					setInt(st, "iason", 1)
					htmltext = "30969-01.htm"
					s.TakeItems(p, bloodedFabric, 6)
				case st.GetInt("iason") == 1:
					htmltext = "30969-02.htm"
				default:
					htmltext = "30969-03.htm"
					setInt(st, "usedonbaium", 1)
				}
			}
		case harne:
			if cond >= 5 && cond <= 22 {
				switch {
				case p.HasItem(bloodOfSaint):
					htmltext = "30144-05.htm"
				case p.HasItem(hanellinFirstLetter):
					htmltext = "30144-01.htm"
					st.SetCond(17)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, hanellinFirstLetter, 1)
					mark(p, guardianCorpseLoc)
				case !p.HasItem(firstKeyOfArk):
					htmltext = "30144-03.htm"
					mark(p, guardianCorpseLoc)
				default:
					htmltext = "30144-04.htm"
				}
			}
		case claudiaAthebaldt:
			if cond >= 5 && cond <= 22 {
				switch {
				case p.HasItem(bookOfSaint):
					htmltext = "31001-05.htm"
				case p.HasItem(hanellinSecondLetter):
					htmltext = "31001-01.htm"
					st.SetCond(9)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, hanellinSecondLetter, 1)
					mark(p, holyArk2Loc)
				case !p.HasItem(secondKeyOfArk):
					htmltext = "31001-03.htm"
					mark(p, holyArk2Loc)
				default:
					htmltext = "31001-04.htm"
				}
			}
		case martien:
			if cond >= 5 && cond <= 22 {
				switch {
				case p.HasItem(boughOfSaint):
					htmltext = "30645-05.htm"
				case p.HasItem(hanellinThirdLetter):
					htmltext = "30645-01.htm"
					st.SetCond(13)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, hanellinThirdLetter, 1)
					mark(p, holyArk3Loc)
				case !p.HasItem(thirdKeyOfArk):
					htmltext = "30645-03.htm"
					mark(p, holyArk3Loc)
				default:
					htmltext = "30645-04.htm"
				}
			}
		case arkGuardianCorpse:
			if !p.HasItem(hanellinFirstLetter) && cond >= 5 && cond <= 22 {
				switch {
				case p.HasItem(firstKeyOfArk) || p.HasItem(bloodOfSaint):
					htmltext = "30980-03.htm"
				case st.GetInt("angelkiller") == 0:
					htmltext = "30980-01.htm"
					if q.angelKiller.Empty() {
						q.spawn(s, q.angelKiller, angelKiller, e.NPC)
					}
					if st.Cond() != 18 {
						st.SetCond(18)
						s.PlaySound(p, script.SoundMiddle)
					}
				default:
					htmltext = "30980-02.htm"
					s.GiveItems(p, firstKeyOfArk, 1)
					s.PlaySound(p, script.SoundItemGet)
					mark(p, holyArk1Loc)
					st.Unset("angelkiller")
				}
			}
		case holyArk1:
			if !p.HasItem(hanellinFirstLetter) && cond >= 5 && cond <= 22 {
				switch {
				case p.HasItem(bloodOfSaint):
					htmltext = "30977-03.htm"
				case p.HasItem(firstKeyOfArk):
					htmltext = "30977-02.htm"
					st.SetCond(20)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, firstKeyOfArk, 1)
					s.GiveItems(p, bloodOfSaint, 1)
					if p.HasItems(bookOfSaint, boughOfSaint) {
						st.SetCond(21)
					}
				default:
					htmltext = "30977-04.htm"
				}
			}
		case holyArk2:
			if !p.HasItem(hanellinSecondLetter) && cond >= 5 && cond <= 22 {
				switch {
				case p.HasItem(bookOfSaint):
					htmltext = "30978-03.htm"
				case !p.HasItem(secondKeyOfArk):
					htmltext = "30978-01.htm"
					if q.elberoth.Empty() {
						q.spawn(s, q.elberoth, arkGuardianElberoth, e.NPC)
					}
				default:
					htmltext = "30978-02.htm"
					st.SetCond(12)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, secondKeyOfArk, 1)
					s.GiveItems(p, bookOfSaint, 1)
					if p.HasItems(bloodOfSaint, boughOfSaint) {
						st.SetCond(21)
					}
				}
			}
		case holyArk3:
			if !p.HasItem(hanellinThirdLetter) && cond >= 5 && cond <= 22 {
				switch {
				case p.HasItem(boughOfSaint):
					htmltext = "30979-03.htm"
				case !p.HasItem(thirdKeyOfArk):
					htmltext = "30979-01.htm"
					if q.shadowFang.Empty() {
						q.spawn(s, q.shadowFang, arkGuardianShadowFang, e.NPC)
					}
				default:
					htmltext = "30979-02.htm"
					st.SetCond(16)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, thirdKeyOfArk, 1)
					s.GiveItems(p, boughOfSaint, 1)
					if p.HasItems(bloodOfSaint, bookOfSaint) {
						st.SetCond(21)
					}
				}
			}
		}
	}
	return htmltext
}

// spawn calls the guardian id at the ark, and keeps it in cell. Another
// player's ark may have called one meanwhile: one guardian stands.
func (q quest) spawn(s *script.Script, cell *script.NPCCell, id int32, at *script.NPC) {
	guardian := s.Spawner().AddSpawn(id, at, true, despawn)
	if guardian != nil && !cell.SetIfEmpty(guardian) {
		guardian.DeleteMe()
	}
}

// onHanellin is the answer of Hanellin to a started quest at cond.
func onHanellin(s *script.Script, p *script.Player, st *script.QuestState, cond int32, htmltext string) string {
	switch {
	case cond == 1:
		htmltext = "30864-02.htm"
	case cond == 2:
		if !p.HasItem(titanPowerstone) {
			htmltext = "30864-06.htm"
		} else {
			htmltext = "30864-07.htm"
		}
	case cond == 4:
		htmltext = "30864-09.htm"
	case cond > 4 && cond < 21:
		if p.HasAtLeastOneItem(bookOfSaint, bloodOfSaint, boughOfSaint) {
			htmltext = "30864-28.htm"
		} else {
			htmltext = "30864-24.htm"
		}
	case cond == 21:
		htmltext = "30864-29.htm"
		st.SetCond(22)
		s.TakeItems(p, bookOfSaint, 1)
		s.TakeItems(p, bloodOfSaint, 1)
		s.TakeItems(p, boughOfSaint, 1)
		s.PlaySound(p, script.SoundMiddle)
	case cond == 22:
		switch {
		case p.HasItem(whiteFabric):
			htmltext = "30864-31.htm"
		case p.ItemCount(antidote) < 5 || !p.HasItem(healingPotion):
			htmltext = "30864-30.htm"
		default:
			htmltext = "30864-31.htm"
			s.TakeItems(p, antidote, 5)
			s.TakeItems(p, healingPotion, 1)
			s.GiveItems(p, whiteFabric, 1)
			s.PlaySound(p, script.SoundItemGet)
		}
	case cond == 24:
		htmltext = "30864-38.htm"
	case cond == 25:
		switch {
		case p.HasItem(whiteFabric):
			htmltext = "30864-39.htm"
		case p.HasItem(bloodedFabric):
			htmltext = "30864-49.htm"
		// Use the only fabric on Baium, drop the quest.
		default:
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		}
	case cond == 26:
		count := p.ItemCount(bloodedFabric)
		switch {
		case count+p.ItemCount(whiteFabric) < 10:
			htmltext = "30864-54.htm"
			s.TakeItems(p, bloodedFabric, -1)
			s.RewardItems(p, 57, 1000*count+4000)
			st.Exit(true)
		case count < 10:
			htmltext = "30864-52.htm"
		default:
			htmltext = "30864-53.htm"
		}
	case cond == 27:
		switch {
		case st.GetInt("gustav")+st.GetInt("hardin")+st.GetInt("iason") == 3:
			htmltext = "30864-60.htm"
			st.SetCond(28)
			s.RewardItems(p, 57, 49000)
			s.PlaySound(p, script.SoundMiddle)
		case p.HasItem(bloodedFabric) && st.GetInt("usedonbaium") != 1:
			htmltext = "30864-59.htm"
		default:
			htmltext = "30864-61.htm"
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		}
	case cond == 28:
		htmltext = "30864-55.htm"
	case cond == 29:
		count := p.ItemCount(bloodedFabric)
		switch {
		case count+p.ItemCount(whiteFabric) < 10:
			htmltext = "30864-54.htm"
			s.TakeItems(p, bloodedFabric, -1)
			s.RewardItems(p, 57, 5000*count)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		case count < 10:
			htmltext = "30864-52.htm"
		default:
			htmltext = "30864-53.htm"
		}
	}
	return htmltext
}

// bloodFabric adds points to the player's tally of Platinum Tribe blows and
// kills, and turns a white fabric to a blooded one once it tops the stage's
// mark.
func bloodFabric(s *script.Script, p *script.Player, st *script.QuestState, npcID, shaman, overlord int32) {
	cond := st.Cond()
	if (cond != 24 && cond != 25) || !p.HasItem(whiteFabric) {
		return
	}
	add := overlord
	if npcID == platinumTribeShaman {
		add = shaman
	}
	points := st.GetInt("points") + add
	limit := int32(100000)
	if cond == 24 {
		limit = 80000
	}
	if points <= limit {
		setInt(st, "points", points)
		return
	}
	setInt(st, "points", 0)
	s.TakeItems(p, whiteFabric, 1)
	s.GiveItems(p, bloodedFabric, 1)
	if cond != 24 {
		s.PlaySound(p, script.SoundItemGet)
		return
	}
	s.PlaySound(p, script.SoundFinish)
	st.Exit(true)
}

func onAttacked(s *script.Script, e script.Attacked) {
	p := script.ActingPlayer(e.Attacker)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	switch npcID := e.NPC.NpcID(); npcID {
	case arkGuardianElberoth:
		if e.NPC.ScriptValue() == 0 {
			e.NPC.Say(script.NpcString(sayElberothHit))
			e.NPC.SetScriptValue(1)
		}
	case arkGuardianShadowFang:
		if e.NPC.ScriptValue() == 0 {
			e.NPC.Say(script.NpcString(sayShadowFangHit))
			e.NPC.SetScriptValue(1)
		}
	case angelKiller:
		if st.GetInt("angelkiller") == 1 || p.HasAtLeastOneItem(firstKeyOfArk, bloodOfSaint) {
			e.NPC.StopAttack()
			e.NPC.Say(script.NpcString(sayAngelKillerLeaves))
			e.NPC.DeleteMe()
		} else if e.NPC.HPRatio() < 0.3 {
			e.NPC.StopAttack()
			e.NPC.Say(script.NpcString(sayAngelKillerDefeated))
			e.NPC.DeleteMe()
			st.SetCond(19)
			setInt(st, "angelkiller", 1)
			s.PlaySound(p, script.SoundMiddle)
			mark(p, guardianCorpseLoc)
		}
	case platinumTribeOverlord, platinumTribeShaman:
		bloodFabric(s, p, st, npcID, 60, 70)
	}
}

// onCreated has each guardian announce itself.
func onCreated(_ *script.Script, e script.Created) {
	switch e.NPC.NpcID() {
	case arkGuardianElberoth:
		e.NPC.Say(script.NpcString(sayElberothSpawn))
	case arkGuardianShadowFang:
		e.NPC.Say(script.NpcString(sayShadowFangSpawn))
	case angelKiller:
		e.NPC.Say(script.NpcString(sayAngelKillerSpawn))
	}
}

// onDecayed lets the ark call its guardian again once it has left.
func (q quest) onDecayed(_ *script.Script, e script.Decayed) {
	q.elberoth.ClearIf(e.NPC)
	q.shadowFang.ClearIf(e.NPC)
	q.angelKiller.ClearIf(e.NPC)
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	cond := st.Cond()
	switch npcID := e.NPC.NpcID(); npcID {
	case lesserGiantElder, lesserGiantMage:
		if cond == 2 {
			s.DropItems(p, titanPowerstone, 1, 1, 100000)
		}
	case arkGuardianElberoth:
		if cond >= 5 && cond <= 22 && !p.HasItem(secondKeyOfArk) {
			st.SetCond(11)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, secondKeyOfArk, 1)
			e.NPC.Say(script.NpcString(sayElberothDies))
		}
	case arkGuardianShadowFang:
		if cond >= 5 && cond <= 22 && !p.HasItem(thirdKeyOfArk) {
			st.SetCond(15)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, thirdKeyOfArk, 1)
			e.NPC.Say(script.NpcString(sayShadowFangDies))
		}
	case platinumTribeOverlord, platinumTribeShaman:
		bloodFabric(s, p, st, npcID, 600, 700)
	case sealAngel, guardianAngel:
		if (cond == 26 || cond == 29) && s.Rnd(4) < 1 && p.HasItem(whiteFabric) {
			s.PlaySound(p, script.SoundItemGet)
			s.TakeItems(p, whiteFabric, 1)
			s.GiveItems(p, bloodedFabric, 1)
		}
	}
}
