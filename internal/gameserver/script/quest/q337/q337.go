// Package q337 is the quest Audience with the Land Dragon: the player proves
// worthy to the watchmen of the land dragon by collecting four marks, then
// by taking an abyssal jewel's fragment and the fangs of its guardians, and
// by a third fragment for the portal stone.
package q337

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q337_AudienceWithTheLandDragon"

// NPCs.
const (
	gabrielle = 30753
	orven     = 30857
	kendra    = 30851
	chakiris  = 30705
	kaiena    = 30720
	moke      = 30498
	helton    = 30678
	gilmore   = 30754
	theodric  = 30755
)

// Monsters.
const (
	bloodQueen              = 18001
	sacrificeOfTheSacrified = 27171
	haritLizardmanShaman    = 20644
	haritLizardmanMatriarch = 20645
	haritLizardmanZealot    = 27172
	kranrot                 = 20650
	hamrut                  = 20649
	marshDrake              = 20680
	marshStalker            = 20679
	abyssalJewel1           = 27165
	abyssalJewel2           = 27166
	abyssalJewel3           = 27167
	jewelGuardianMara       = 27168
	jewelGuardianMusfel     = 27169
	jewelGuardianPython     = 27170
	caveMaiden1             = 20134
	caveMaiden2             = 20287
	caveKeeper1             = 20246
	caveKeeper2             = 20277
)

// Quest items.
const (
	featherOfGabrielle         = 3852
	markOfWatchman             = 3864
	remainsOfSacrified         = 3857
	totemOfLandDragon          = 3858
	kranrotSkin                = 3855
	hamrutLeg                  = 3856
	marshDrakeTalons           = 3854
	marshStalkerHorn           = 3853
	firstFragmentOfAbyssJewel  = 3859
	secondFragmentOfAbyssJewel = 3860
	thirdFragmentOfAbyssJewel  = 3861
	maraFang                   = 3862
	musfelFang                 = 3863
	heraldOfSlayer             = 3890
)

// Items.
const portalStone = 3865

// The spawns last a minute.
const despawn = 60000 * time.Millisecond

// dropOnKill is a monster that drops item for a player at cond holding the
// quest variable "drop<variable>" at 1.
type dropOnKill struct {
	monster, cond, variable, item int32
}

func dropsOnKill() []dropOnKill {
	return []dropOnKill{
		{sacrificeOfTheSacrified, 1, 1, remainsOfSacrified},
		{haritLizardmanZealot, 1, 2, totemOfLandDragon},
		{kranrot, 1, 3, kranrotSkin},
		{hamrut, 1, 3, hamrutLeg},
		{marshDrake, 1, 4, marshDrakeTalons},
		{marshStalker, 1, 4, marshStalkerHorn},
		{jewelGuardianMara, 2, 5, maraFang},
		{jewelGuardianMusfel, 2, 6, musfelFang},
	}
}

// dropOnAttack is a jewel that gives item for a player at cond holding the
// quest variable "drop<variable>" at 1, and calls count guardians when hit.
type dropOnAttack struct {
	jewel, cond, variable, item, count, guardian int32
}

func dropsOnAttack() []dropOnAttack {
	return []dropOnAttack{
		{abyssalJewel1, 2, 5, firstFragmentOfAbyssJewel, 20, jewelGuardianMara},
		{abyssalJewel2, 2, 6, secondFragmentOfAbyssJewel, 20, jewelGuardianMusfel},
		{abyssalJewel3, 4, 7, thirdFragmentOfAbyssJewel, 3, jewelGuardianPython},
	}
}

// quest holds, for each jewel, whether it has regained its health since it
// last called its guardians, shared by every player.
type quest struct{ jewel1, jewel2, jewel3 *script.BoolCell }

// New returns the quest.
func New() script.Script {
	q := quest{jewel1: new(script.BoolCell), jewel2: new(script.BoolCell), jewel3: new(script.BoolCell)}
	return script.Script{
		Title:   "Audience with the Land Dragon",
		QuestID: 337,
		Items: []int32{
			featherOfGabrielle, markOfWatchman, remainsOfSacrified, totemOfLandDragon,
			kranrotSkin, hamrutLeg, marshDrakeTalons, marshStalkerHorn,
			firstFragmentOfAbyssJewel, maraFang, secondFragmentOfAbyssJewel, musfelFang,
			heraldOfSlayer, thirdFragmentOfAbyssJewel,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {gabrielle},
			script.EventTalked:     {gabrielle, orven, kendra, chakiris, kaiena, moke, helton, gilmore, theodric},
			script.EventAttacked:   {abyssalJewel1, abyssalJewel2, abyssalJewel3},
			script.EventMyDying: {
				bloodQueen, sacrificeOfTheSacrified, haritLizardmanShaman, haritLizardmanMatriarch, haritLizardmanZealot,
				kranrot, hamrut, marshDrake, marshStalker, jewelGuardianMara, jewelGuardianMusfel, caveMaiden1, caveMaiden2,
				caveKeeper1, caveKeeper2, jewelGuardianPython,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnAttacked: q.onAttacked, OnMyDying: onMyDying},
	}
}

// flag returns the cell of jewel.
func (q quest) flag(jewel int32) *script.BoolCell {
	switch jewel {
	case abyssalJewel3:
		return q.jewel3
	case abyssalJewel2:
		return q.jewel2
	}
	return q.jewel1
}

// dropKey is the quest variable of the stage n.
func dropKey(n int32) string { return "drop" + strconv.Itoa(int(n)) }

// rndBetween returns a random number of [lo, hi].
func rndBetween(s *script.Script, lo, hi int) int {
	return lo + s.Rnd(hi-lo+1)
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
	// Gabrielle
	case strings.EqualFold(e.Name, "30753-05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		setInt(st, "drop1", 1)
		setInt(st, "drop2", 1)
		setInt(st, "drop3", 1)
		setInt(st, "drop4", 1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, featherOfGabrielle, 1)
	case strings.EqualFold(e.Name, "30753-09.htm"):
		if p.ItemCount(markOfWatchman) >= 4 {
			st.SetCond(2)
			setInt(st, "drop5", 2)
			setInt(st, "drop6", 2)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, markOfWatchman, 4)
		} else {
			htmltext = ""
		}
	// Theodric
	case strings.EqualFold(e.Name, "30755-05.htm"):
		if p.HasItem(thirdFragmentOfAbyssJewel) {
			s.TakeItems(p, thirdFragmentOfAbyssJewel, 1)
			s.TakeItems(p, heraldOfSlayer, 1)
			s.GiveItems(p, portalStone, 1)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(true)
		} else {
			htmltext = ""
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
		if p.Level() < 50 {
			htmltext = "30753-02.htm"
		} else {
			htmltext = "30753-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case gabrielle:
			switch cond {
			case 1:
				if p.ItemCount(markOfWatchman) < 4 {
					htmltext = "30753-06.htm"
				} else {
					htmltext = "30753-08.htm"
				}
			case 2:
				if p.ItemCount(markOfWatchman) < 2 {
					htmltext = "30753-10.htm"
				} else {
					htmltext = "30753-11.htm"
					st.SetCond(3)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, featherOfGabrielle, 1)
					s.TakeItems(p, markOfWatchman, 1)
					s.GiveItems(p, heraldOfSlayer, 1)
				}
			case 3:
				htmltext = "30753-12.htm"
			case 4:
				htmltext = "30753-13.htm"
			}
		case orven:
			if cond == 1 {
				htmltext = watchman(s, p, st, "30857-01.htm", "30857-02.htm", "30857-03.htm", "30857-04.htm", 1, p.HasItem(remainsOfSacrified), remainsOfSacrified)
			}
		case kendra:
			if cond == 1 {
				htmltext = watchman(s, p, st, "30851-01.htm", "30851-02.htm", "30851-03.htm", "30851-04.htm", 2, p.HasItem(totemOfLandDragon), totemOfLandDragon)
			}
		case chakiris:
			if cond == 1 {
				htmltext = watchman(s, p, st, "30705-01.htm", "30705-02.htm", "30705-03.htm", "30705-04.htm", 3, p.HasItems(kranrotSkin, hamrutLeg), kranrotSkin, hamrutLeg)
			}
		case kaiena:
			if cond == 1 {
				htmltext = watchman(s, p, st, "30720-01.htm", "30720-02.htm", "30720-03.htm", "30720-04.htm", 4, p.HasItems(marshDrakeTalons, marshStalkerHorn), marshDrakeTalons, marshStalkerHorn)
			}
		case moke:
			if cond == 2 {
				htmltext = jewelWatchman(s, p, st, "30498-01.htm", "30498-02.htm", "30498-03.htm", "30498-04.htm", "30498-05.htm", 5, p.HasItems(firstFragmentOfAbyssJewel, maraFang), firstFragmentOfAbyssJewel, maraFang)
			}
		case helton:
			if cond == 2 {
				htmltext = jewelWatchman(s, p, st, "30678-01.htm", "30678-02.htm", "30678-03.htm", "30678-04.htm", "30678-05.htm", 6, p.HasItems(secondFragmentOfAbyssJewel, musfelFang), secondFragmentOfAbyssJewel, musfelFang)
			}
		case gilmore:
			switch cond {
			case 1, 2:
				htmltext = "30754-01.htm"
			case 3:
				htmltext = "30754-02.htm"
				st.SetCond(4)
				setInt(st, "drop7", 1)
				s.PlaySound(p, script.SoundMiddle)
			case 4:
				if !p.HasItem(thirdFragmentOfAbyssJewel) {
					htmltext = "30754-04.htm"
				} else {
					htmltext = "30754-05.htm"
				}
			}
		case theodric:
			switch cond {
			case 1, 2:
				htmltext = "30755-01.htm"
			case 3:
				htmltext = "30755-02.htm"
			case 4:
				if !p.HasItem(thirdFragmentOfAbyssJewel) {
					htmltext = "30755-03.htm"
				} else {
					htmltext = "30755-04.htm"
				}
			}
		}
	}
	return htmltext
}

// watchman answers a watchman of the first stage, whose request is the quest
// variable drop<variable>: while it stands, the watchman takes held, one
// each, for a mark (asked while the request is open and nothing is held,
// done once it is handed in); otherwise he tells how the marks stand
// (short of four, or all four).
func watchman(s *script.Script, p *script.Player, st *script.QuestState, asked, done, short, enough string, variable int32, holds bool, items ...int32) string {
	if st.GetInt(dropKey(variable)) == 1 {
		if !holds {
			return asked
		}
		st.Unset(dropKey(variable))
		s.PlaySound(p, script.SoundMiddle)
		for _, item := range items {
			s.TakeItems(p, item, 1)
		}
		s.GiveItems(p, markOfWatchman, 1)
		return done
	}
	if p.ItemCount(markOfWatchman) < 4 {
		return short
	}
	return enough
}

// jewelWatchman answers a watchman of the second stage, whose request is the
// quest variable drop<variable>: 2 until he is first told, 1 while the
// fragment and the fang are asked for, absent once they are handed in.
func jewelWatchman(s *script.Script, p *script.Player, st *script.QuestState, told, asked, done, short, enough string, variable int32, holds bool, items ...int32) string {
	switch st.GetInt(dropKey(variable)) {
	case 2:
		setInt(st, dropKey(variable), 1)
		return told
	case 1:
		if !holds {
			return asked
		}
		st.Unset(dropKey(variable))
		s.PlaySound(p, script.SoundMiddle)
		for _, item := range items {
			s.TakeItems(p, item, 1)
		}
		s.GiveItems(p, markOfWatchman, 1)
		return done
	case 0:
		if p.ItemCount(markOfWatchman) < 2 {
			return short
		}
		return enough
	}
	return script.NoQuestMsg()
}

func (q quest) onAttacked(s *script.Script, e script.Attacked) {
	p := script.ActingPlayer(e.Attacker)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	npcID := e.NPC.NpcID()
	for _, d := range dropsOnAttack() {
		if d.jewel != npcID {
			continue
		}
		if d.cond != st.Cond() {
			break
		}
		hpRatio := (e.NPC.HP() + float64(e.Damage)) / float64(e.NPC.MaxHP())
		switch {
		// Reward the jewel fragment.
		case hpRatio < 0.33:
			if s.Rnd(100) < 33 && st.GetInt(dropKey(d.variable)) == 1 {
				if !p.HasItem(d.item) {
					s.GiveItems(p, d.item, 1)
					s.PlaySound(p, script.SoundItemGet)
				}
			}
		// Call the guardians, once until the jewel regains its health.
		case hpRatio < 0.66:
			if s.Rnd(100) < 33 && st.GetInt(dropKey(d.variable)) == 1 {
				flag := q.flag(npcID)
				if flag.Get() {
					at := script.LocOf(e.NPC)
					for range d.count {
						x := at.X + rndBetween(s, -150, 150)
						y := at.Y + rndBetween(s, -150, 150)
						mob := s.Spawner().AddSpawn(d.guardian, script.Loc{X: x, Y: y, Z: at.Z, Heading: at.Heading}, true, despawn)
						if mob == nil {
							continue
						}
						mob.AddAttackDesire(e.Attacker, 200)
					}
					flag.Set(false)
				}
			}
		// Allow the guardians again once the jewel is back to 90% of its health.
		case hpRatio > 0.9:
			q.flag(npcID).Set(true)
		}
		break
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
	// spawn calls a monster of id within spread of the dying one.
	spawn := func(id int32, spread int) {
		at := script.LocOf(e.NPC)
		x := at.X + rndBetween(s, -spread, spread)
		y := at.Y + rndBetween(s, -spread, spread)
		s.Spawner().AddSpawn(id, script.Loc{X: x, Y: y, Z: at.Z, Heading: at.Heading}, true, despawn)
	}
	switch npcID {
	case sacrificeOfTheSacrified, haritLizardmanZealot, kranrot, hamrut, marshDrake, marshStalker, jewelGuardianMara, jewelGuardianMusfel:
		for _, d := range dropsOnKill() {
			if d.monster != npcID {
				continue
			}
			if d.cond == cond && st.GetInt(dropKey(d.variable)) == 1 {
				if !p.HasItem(d.item) {
					s.GiveItems(p, d.item, 1)
					s.PlaySound(p, script.SoundItemGet)
				}
			}
			break
		}
	// Orven's request.
	case bloodQueen:
		if cond == 1 && st.GetInt("drop1") == 1 && !p.HasItem(remainsOfSacrified) {
			for range 8 {
				spawn(sacrificeOfTheSacrified, 100)
			}
		}
	// Kendra's request.
	case haritLizardmanShaman, haritLizardmanMatriarch:
		if cond == 1 && s.Rnd(5) == 0 && st.GetInt("drop2") == 1 && !p.HasItem(totemOfLandDragon) {
			for range 3 {
				spawn(haritLizardmanZealot, 50)
			}
		}
	// Gilmore's request.
	case caveMaiden1, caveMaiden2, caveKeeper1, caveKeeper2:
		if cond == 4 && s.Rnd(5) == 0 && !p.HasItem(thirdFragmentOfAbyssJewel) {
			spawn(abyssalJewel3, 50)
		}
	}
}
