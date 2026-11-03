// Package q417 is the quest Path to Become a Scavenger: a dwarven fighter
// runs errands for Pippi, Mion and Toma, and a spoiled honey bear and
// tarantula beads, to earn the Ring of Raven.
package q417

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q417_PathToBecomeAScavenger"

// Quest items.
const (
	ringOfRaven        = 1642
	pippiLetter        = 1643
	rautTeleportScroll = 1644
	succubusUndies     = 1645
	mionLetter         = 1646
	bronkIngot         = 1647
	shariAxe           = 1648
	zimenfPotion       = 1649
	bronkPay           = 1650
	shariPay           = 1651
	zimenfPay          = 1652
	bearPicture        = 1653
	tarantulaPicture   = 1654
	honeyJar           = 1655
	bead               = 1656
	beadParcel1        = 1657
	beadParcel2        = 8543
)

// Npcs.
const (
	raut    = 30316
	shari   = 30517
	mion    = 30519
	pippi   = 30524
	bronk   = 30525
	zimenf  = 30538
	toma    = 30556
	torai   = 30557
	yasheni = 31958
)

// Monsters.
const (
	hunterTarantula  = 20403
	plunderTarantula = 20508
	hunterBear       = 20777
	honeyBear        = 27058
)

// The classes the quest checks.
const (
	classDwarvenFighter = 53
	classScavenger      = 54
)

// honeyBearLife is how long a spawned honey bear stays.
const honeyBearLife = 300000 * time.Millisecond

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Path To Become A Scavenger",
		QuestID: 417,
		Items: []int32{
			pippiLetter, rautTeleportScroll, succubusUndies, mionLetter, bronkIngot, shariAxe, zimenfPotion,
			bronkPay, shariPay, zimenfPay, bearPicture, tarantulaPicture, honeyJar, bead, beadParcel1, beadParcel2,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {pippi},
			script.EventTalked:     {raut, shari, mion, pippi, bronk, zimenf, toma, torai, yasheni},
			script.EventMyDying:    {hunterTarantula, plunderTarantula, hunterBear, honeyBear},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func setInt(st *script.QuestState, key string, n int32) {
	st.Set(key, strconv.Itoa(int(n)))
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	// Pippi
	case strings.EqualFold(e.Name, "30524-05.htm"):
		switch {
		case p.ClassID() != classDwarvenFighter:
			if p.ClassID() == classScavenger {
				htmltext = "30524-02a.htm"
			} else {
				htmltext = "30524-08.htm"
			}
		case p.Level() < 19:
			htmltext = "30524-02.htm"
		case p.HasItem(ringOfRaven):
			htmltext = "30524-04.htm"
		default:
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(1)
			s.PlaySound(p, script.SoundAccept)
			s.GiveItems(p, pippiLetter, 1)
		}
	// Mion
	case strings.EqualFold(e.Name, "30519_1"):
		random := int32(s.Rnd(3))
		htmltext = "30519-0" + strconv.Itoa(int(random)+2) + ".htm"
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, pippiLetter, -1)
		s.GiveItems(p, zimenfPotion-random, 1)
	case strings.EqualFold(e.Name, "30519_2"):
		random := int32(s.Rnd(3))
		htmltext = "30519-0" + strconv.Itoa(int(random)+2) + ".htm"
		s.TakeItems(p, bronkPay, -1)
		s.TakeItems(p, shariPay, -1)
		s.TakeItems(p, zimenfPay, -1)
		s.GiveItems(p, zimenfPotion-random, 1)
	case strings.EqualFold(e.Name, "30519-07.htm"):
		setInt(st, "id", st.GetInt("id")+1)
	case strings.EqualFold(e.Name, "30519-09.htm"):
		id := st.GetInt("id")
		switch {
		case id/10 < 2:
			htmltext = "30519-07.htm"
			setInt(st, "id", id+1)
		case id/10 == 2:
			setInt(st, "id", id+1)
		case id/10 >= 3:
			htmltext = "30519-10.htm"
			st.SetCond(4)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, shariAxe, -1)
			s.TakeItems(p, zimenfPotion, -1)
			s.TakeItems(p, bronkIngot, -1)
			s.GiveItems(p, mionLetter, 1)
		}
	case strings.EqualFold(e.Name, "30519-11.htm") && s.RndBool():
		htmltext = "30519-06.htm"
	// Toma
	case strings.EqualFold(e.Name, "30556-05b.htm"):
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, bead, -1)
		s.TakeItems(p, tarantulaPicture, 1)
		s.GiveItems(p, beadParcel1, 1)
	case strings.EqualFold(e.Name, "30556-06b.htm"):
		st.SetCond(12)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, bead, -1)
		s.TakeItems(p, tarantulaPicture, 1)
		s.GiveItems(p, beadParcel2, 1)
	// Raut
	case strings.EqualFold(e.Name, "30316-02.htm"), strings.EqualFold(e.Name, "30316-03.htm"):
		st.SetCond(10)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, beadParcel1, 1)
		s.GiveItems(p, rautTeleportScroll, 1)
	// Torai
	case strings.EqualFold(e.Name, "30557-03.htm"):
		st.SetCond(11)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, rautTeleportScroll, 1)
		s.GiveItems(p, succubusUndies, 1)
	// Yasheni
	case strings.EqualFold(e.Name, "31958-02.htm"):
		s.TakeItems(p, beadParcel2, 1)
		s.GiveItems(p, ringOfRaven, 1)
		s.RewardExpAndSp(p, 3200, 7080)
		p.SocialAction(3)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
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
		htmltext = "30524-01.htm"
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case pippi:
			switch {
			case cond == 1:
				htmltext = "30524-06.htm"
			case cond > 1:
				htmltext = "30524-07.htm"
			}
		case mion:
			switch {
			case p.HasItem(pippiLetter):
				htmltext = "30519-01.htm"
			case p.HasAtLeastOneItem(bronkIngot, shariAxe, zimenfPotion):
				if st.GetInt("id")/10 == 0 {
					htmltext = "30519-05.htm"
				} else {
					htmltext = "30519-08.htm"
				}
			case p.HasAtLeastOneItem(bronkPay, shariPay, zimenfPay):
				if st.GetInt("id") < 50 {
					htmltext = "30519-12.htm"
				} else {
					htmltext = "30519-15.htm"
					st.SetCond(4)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, bronkPay, -1)
					s.TakeItems(p, shariPay, -1)
					s.TakeItems(p, zimenfPay, -1)
					s.GiveItems(p, mionLetter, 1)
				}
			case cond == 4:
				htmltext = "30519-13.htm"
			case cond > 4:
				htmltext = "30519-14.htm"
			}
		case shari:
			switch {
			case p.HasItem(shariAxe):
				id := st.GetInt("id")
				if id < 20 {
					htmltext = "30517-01.htm"
				} else {
					htmltext = "30517-02.htm"
					st.SetCond(3)
					s.PlaySound(p, script.SoundMiddle)
				}
				setInt(st, "id", id+10)
				s.TakeItems(p, shariAxe, 1)
				s.GiveItems(p, shariPay, 1)
			case p.HasItem(shariPay):
				htmltext = "30517-03.htm"
			}
		case bronk:
			switch {
			case p.HasItem(bronkIngot):
				id := st.GetInt("id")
				if id < 20 {
					htmltext = "30525-01.htm"
				} else {
					htmltext = "30525-02.htm"
					st.SetCond(3)
					s.PlaySound(p, script.SoundMiddle)
				}
				setInt(st, "id", id+10)
				s.TakeItems(p, bronkIngot, 1)
				s.GiveItems(p, bronkPay, 1)
			case p.HasItem(bronkPay):
				htmltext = "30525-03.htm"
			}
		case zimenf:
			switch {
			case p.HasItem(zimenfPotion):
				id := st.GetInt("id")
				if id < 20 {
					htmltext = "30538-01.htm"
				} else {
					htmltext = "30538-02.htm"
					st.SetCond(3)
					s.PlaySound(p, script.SoundMiddle)
				}
				setInt(st, "id", id+10)
				s.TakeItems(p, zimenfPotion, 1)
				s.GiveItems(p, zimenfPay, 1)
			case p.HasItem(zimenfPay):
				htmltext = "30538-03.htm"
			}
		case toma:
			switch {
			case cond == 4:
				htmltext = "30556-01.htm"
				st.SetCond(5)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, mionLetter, 1)
				s.GiveItems(p, bearPicture, 1)
			case cond == 5:
				htmltext = "30556-02.htm"
			case cond == 6:
				htmltext = "30556-03.htm"
				st.SetCond(7)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, honeyJar, -1)
				s.TakeItems(p, bearPicture, 1)
				s.GiveItems(p, tarantulaPicture, 1)
			case cond == 7:
				htmltext = "30556-04.htm"
			case cond == 8:
				htmltext = "30556-05a.htm"
			case cond == 9:
				htmltext = "30556-06a.htm"
			case cond == 10 || cond == 11:
				htmltext = "30556-07.htm"
			case cond == 12:
				htmltext = "30556-06c.htm"
			}
		case raut:
			switch {
			case cond == 9:
				htmltext = "30316-01.htm"
			case cond == 10:
				htmltext = "30316-04.htm"
			case cond == 11:
				htmltext = "30316-05.htm"
				s.TakeItems(p, succubusUndies, 1)
				s.GiveItems(p, ringOfRaven, 1)
				s.RewardExpAndSp(p, 3200, 7080)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
			}
		case torai:
			if cond == 10 {
				htmltext = "30557-01.htm"
			}
		case yasheni:
			if cond == 12 {
				htmltext = "31958-01.htm"
			}
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
	case hunterBear:
		if st.Cond() == 5 {
			step := st.GetInt("step")
			if step > 20 {
				if (step-20)*10 >= int32(s.Rnd(100)) {
					s.Spawner().AddSpawn(honeyBear, e.NPC, false, honeyBearLife)
					st.Unset("step")
				} else {
					setInt(st, "step", step+1)
				}
			} else {
				setInt(st, "step", step+1)
			}
		}
	case honeyBear:
		if st.Cond() == 5 && e.NPC.ActualSpoiler(p) && s.DropItemsAlways(p, honeyJar, 1, 5) {
			st.SetCond(6)
		}
	case hunterTarantula, plunderTarantula:
		chance := int32(600000)
		if e.NPC.NpcID() == hunterTarantula {
			chance = 333333
		}
		if st.Cond() == 7 && e.NPC.ActualSpoiler(p) && s.DropItems(p, bead, 1, 20, chance) {
			st.SetCond(8)
		}
	}
}
