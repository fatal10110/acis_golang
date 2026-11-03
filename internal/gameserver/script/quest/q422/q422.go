// Package q422 is the quest Repent Your Sins: the Black Judge sends a
// player who has killed players after monsters' parts by its level, has
// Pushkin forge the penitent's manacles, and lowers its PK count each time
// the Sin Eater the manacles summon has grown a level.
package q422

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q422_RepentYourSins"

// Items.
const (
	ratmanScavengerSkull     = 4326
	turekWarHoundTail        = 4327
	tyrantKingpinHeart       = 4328
	trisalimTarantulaVenomSc = 4329

	qitemPenitentManacles = 4330
	manualOfManacles      = 4331
	penitentManacles      = 4425
	leftPenitentManacles  = 4426

	silverNugget     = 1873
	adamantineNugget = 1877
	blacksmithFrame  = 1892
	cokes            = 1879
	steel            = 1880
)

// NPCs.
const (
	blackJudge = 30981
	katari     = 30668
	piotur     = 30597
	casian     = 30612
	joan       = 30718
	pushkin    = 30300
)

// Monsters.
const (
	scavengerWererat  = 20039
	turekWarHound     = 20494
	tyrantKingpin     = 20193
	trisalimTarantula = 20561
)

// sinEater is the NPC id of the pet the manacles summon.
const sinEater = 12564

// varLevel is the quest variable holding the player's level when it last
// took the manacles.
const varLevel = "level"

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Repent Your Sins",
		QuestID: 422,
		Items: []int32{
			ratmanScavengerSkull, turekWarHoundTail, tyrantKingpinHeart, trisalimTarantulaVenomSc,
			manualOfManacles, penitentManacles, qitemPenitentManacles,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {blackJudge},
			script.EventTalked:     {blackJudge, katari, piotur, casian, joan, pushkin},
			script.EventMyDying:    {scavengerWererat, turekWarHound, tyrantKingpin, trisalimTarantula},
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
	case strings.EqualFold(e.Name, "Start"):
		st.SetCond(1)
		switch level := p.Level(); {
		case level <= 20:
			htmltext = "30981-03.htm"
			st.SetCond(2)
		case level >= 20 && level <= 30:
			htmltext = "30981-04.htm"
			st.SetCond(3)
		case level >= 30 && level <= 40:
			htmltext = "30981-05.htm"
			st.SetCond(4)
		default:
			htmltext = "30981-06.htm"
			st.SetCond(5)
		}
		st.SetStatus(questlog.StatusStarted)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30981-11.htm"):
		if !p.HasItem(penitentManacles) {
			switch st.Cond() {
			// The forged manacles brought back: the Black Judge gives the
			// pet item.
			case 15:
				st.SetCond(16)
				st.Set(varLevel, strconv.Itoa(int(p.Level())))
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, qitemPenitentManacles, -1)
				s.GiveItems(p, penitentManacles, 1)
			// Back with the leftover manacles of a previous round.
			case 16:
				st.Set(varLevel, strconv.Itoa(int(p.Level())))
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, leftPenitentManacles, -1)
				s.GiveItems(p, penitentManacles, 1)
			}
		}
	case strings.EqualFold(e.Name, "30981-19.htm"):
		if p.HasItem(leftPenitentManacles) {
			st.SetStatus(questlog.StatusStarted)
			st.SetCond(16)
			s.PlaySound(p, script.SoundAccept)
		}
	case strings.EqualFold(e.Name, "Pk"):
		summon := p.ActiveSummon()
		switch {
		// The Sin Eater is out: a warning.
		case summon != nil && script.SummonNpcID(summon) == sinEater:
			htmltext = "30981-16.htm"
		// The Sin Eater has grown past the registered level: 1 to 10 PKs
		// fewer.
		case sinEaterLevel(p) > st.GetInt(varLevel):
			s.TakeItems(p, penitentManacles, 1)
			s.GiveItems(p, leftPenitentManacles, 1)
			removePkAmount := int32(s.Rnd(10)) + 1
			if p.PkKills() <= removePkAmount {
				// No PK left: the quest ends.
				htmltext = "30981-15.htm"
				s.PlaySound(p, script.SoundFinish)
				st.Exit(true)
				p.SetPkKills(0)
				p.SendUserInfo()
			} else {
				htmltext = "30981-14.htm"
				st.Set(varLevel, strconv.Itoa(int(p.Level())))
				s.PlaySound(p, script.SoundMiddle)
				p.SetPkKills(p.PkKills() - removePkAmount)
				p.SendUserInfo()
			}
		}
	case strings.EqualFold(e.Name, "Quit"):
		htmltext = "30981-20.htm"
		s.TakeItems(p, ratmanScavengerSkull, -1)
		s.TakeItems(p, turekWarHoundTail, -1)
		s.TakeItems(p, tyrantKingpinHeart, -1)
		s.TakeItems(p, trisalimTarantulaVenomSc, -1)
		s.TakeItems(p, manualOfManacles, -1)
		s.TakeItems(p, penitentManacles, -1)
		s.TakeItems(p, qitemPenitentManacles, -1)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.AlreadyCompletedMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case p.PkKills() < 1:
			htmltext = "30981-01.htm"
		case p.HasItem(leftPenitentManacles):
			htmltext = "30981-18.htm"
		default:
			htmltext = "30981-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case blackJudge:
			switch {
			case cond <= 9:
				htmltext = "30981-07.htm"
			case cond > 9 && cond < 14:
				htmltext = "30981-08.htm"
				st.SetCond(14)
				s.PlaySound(p, script.SoundMiddle)
				s.GiveItems(p, manualOfManacles, 1)
			case cond == 14:
				htmltext = "30981-09.htm"
			case cond == 15:
				htmltext = "30981-10.htm"
			case cond == 16:
				switch {
				case !p.HasItem(penitentManacles):
					htmltext = "30981-18.htm"
				case sinEaterLevel(p) > st.GetInt(varLevel):
					htmltext = "30981-13.htm"
				default:
					htmltext = "30981-12.htm"
				}
			}
		case katari:
			htmltext = talkCollector(s, p, st, cond, collector{
				given: 2, collecting: 6, done: 10, item: ratmanScavengerSkull, needed: 10,
				pages: [4]string{"30668-01.htm", "30668-02.htm", "30668-03.htm", "30668-04.htm"},
			}, htmltext)
		case piotur:
			htmltext = talkCollector(s, p, st, cond, collector{
				given: 3, collecting: 7, done: 11, item: turekWarHoundTail, needed: 10,
				pages: [4]string{"30597-01.htm", "30597-02.htm", "30597-03.htm", "30597-04.htm"},
			}, htmltext)
		case casian:
			htmltext = talkCollector(s, p, st, cond, collector{
				given: 4, collecting: 8, done: 12, item: tyrantKingpinHeart, needed: 1,
				pages: [4]string{"30612-01.htm", "30612-02.htm", "30612-03.htm", "30612-04.htm"},
			}, htmltext)
		case joan:
			htmltext = talkCollector(s, p, st, cond, collector{
				given: 5, collecting: 9, done: 13, item: trisalimTarantulaVenomSc, needed: 3,
				pages: [4]string{"30718-01.htm", "30718-02.htm", "30718-03.htm", "30718-04.htm"},
			}, htmltext)
		case pushkin:
			switch {
			case cond == 14 && p.ItemCount(manualOfManacles) == 1:
				if p.ItemCount(silverNugget) < 10 || p.ItemCount(steel) < 5 || p.ItemCount(adamantineNugget) < 2 ||
					p.ItemCount(cokes) < 10 || p.ItemCount(blacksmithFrame) < 1 {
					htmltext = "30300-02.htm"
				} else {
					htmltext = "30300-01.htm"
					st.SetCond(15)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, manualOfManacles, 1)
					s.TakeItems(p, silverNugget, 10)
					s.TakeItems(p, adamantineNugget, 2)
					s.TakeItems(p, cokes, 10)
					s.TakeItems(p, steel, 5)
					s.TakeItems(p, blacksmithFrame, 1)
					s.GiveItems(p, qitemPenitentManacles, 1)
				}
			case p.HasAtLeastOneItem(qitemPenitentManacles, penitentManacles, leftPenitentManacles):
				htmltext = "30300-03.htm"
			}
		}
	}
	return htmltext
}

// collector is one of the four NPCs the Black Judge sends a player to by
// its level: at cond given it names the monster and moves the quest to
// collecting, where needed units of item move it to done.
type collector struct {
	given, collecting, done int32
	item, needed            int32
	// pages are the answers: the errand, still collecting, collected,
	// already done.
	pages [4]string
}

// talkCollector is c's answer to a started quest at cond; htmltext is the
// answer when c has nothing to say.
func talkCollector(s *script.Script, p *script.Player, st *script.QuestState, cond int32, c collector, htmltext string) string {
	switch cond {
	case c.given:
		st.SetCond(c.collecting)
		s.PlaySound(p, script.SoundMiddle)
		return c.pages[0]
	case c.collecting:
		if p.ItemCount(c.item) < c.needed {
			return c.pages[1]
		}
		st.SetCond(c.done)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, c.item, -1)
		return c.pages[2]
	case c.done:
		return c.pages[3]
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
	case scavengerWererat:
		if st.Cond() == 6 {
			s.DropItemsAlways(p, ratmanScavengerSkull, 1, 10)
		}
	case turekWarHound:
		if st.Cond() == 7 {
			s.DropItemsAlways(p, turekWarHoundTail, 1, 10)
		}
	case tyrantKingpin:
		if st.Cond() == 8 {
			s.DropItemsAlways(p, tyrantKingpinHeart, 1, 1)
		}
	case trisalimTarantula:
		if st.Cond() == 9 {
			s.DropItemsAlways(p, trisalimTarantulaVenomSc, 1, 3)
		}
	}
}

// sinEaterLevel is the level of the Sin Eater the player's manacles
// summon: their enchant level. A player without them aborts the
// invocation there.
func sinEaterLevel(p *script.Player) int32 { return p.ItemEnchantLevel(penitentManacles) }
