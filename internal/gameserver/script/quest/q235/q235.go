// Package q235 is the quest Mimir's Elixir, one of the two quests a
// subclass asks for: Ladd and Joan send a level 75 holder of the Star of
// Destiny after the elixir's ingredients, mixed at the Alchemist's Mixing
// Urn, and Ladd trades the elixir for a Scroll of Enchant Weapon (A grade).
package q235

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q235_MimirsElixir"

// Items.
const (
	starOfDestiny       = 5011
	pureSilver          = 6320
	trueGold            = 6321
	sageStone           = 6322
	bloodFire           = 6318
	mimirElixir         = 6319
	magisterMixingStone = 5905
)

// scrollEnchantWeaponA is the reward.
const scrollEnchantWeaponA = 729

// NPCs.
const (
	joan      = 30718
	ladd      = 30721
	mixingUrn = 31149
)

// Monsters.
const (
	chimeraPiece   = 20965
	bloodyGuardian = 21090
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Mimir's Elixir",
		QuestID: 235,
		Items:   []int32{pureSilver, trueGold, sageStone, bloodFire, magisterMixingStone, mimirElixir},
		Bind: script.Bindings{
			script.EventQuestStart: {ladd},
			script.EventTalked:     {ladd, joan, mixingUrn},
			script.EventMyDying:    {chimeraPiece, bloodyGuardian},
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
	case strings.EqualFold(e.Name, "30721-06.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30721-12.htm") && p.HasItem(trueGold):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, magisterMixingStone, 1)
	case strings.EqualFold(e.Name, "30721-16.htm") && p.HasItem(mimirElixir):
		p.BroadcastMagicSkillUse(p, p, 4339, 1, 1, 1)
		s.TakeItems(p, magisterMixingStone, -1)
		s.TakeItems(p, mimirElixir, -1)
		s.TakeItems(p, starOfDestiny, -1)
		s.GiveItems(p, scrollEnchantWeaponA, 1)
		p.SocialAction(3)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	case strings.EqualFold(e.Name, "30718-03.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
	// The urn: each step of the mixing needs every ingredient so far.
	case strings.EqualFold(e.Name, "31149-02.htm"):
		if !p.HasItem(magisterMixingStone) {
			htmltext = "31149-havent.htm"
		}
	case strings.EqualFold(e.Name, "31149-03.htm"):
		if !p.HasItems(magisterMixingStone, pureSilver) {
			htmltext = "31149-havent.htm"
		}
	case strings.EqualFold(e.Name, "31149-05.htm"):
		if !p.HasItems(magisterMixingStone, pureSilver, trueGold) {
			htmltext = "31149-havent.htm"
		}
	case strings.EqualFold(e.Name, "31149-07.htm"):
		if !p.HasItems(magisterMixingStone, pureSilver, trueGold, bloodFire) {
			htmltext = "31149-havent.htm"
		}
	case strings.EqualFold(e.Name, "31149-success.htm"):
		if p.HasItems(magisterMixingStone, pureSilver, trueGold, bloodFire) {
			st.SetCond(8)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, pureSilver, -1)
			s.TakeItems(p, trueGold, -1)
			s.TakeItems(p, bloodFire, -1)
			s.GiveItems(p, mimirElixir, 1)
		} else {
			htmltext = "31149-havent.htm"
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
		switch {
		case p.Level() < 75:
			htmltext = "30721-01b.htm"
		case !p.HasItem(starOfDestiny):
			htmltext = "30721-01a.htm"
		default:
			htmltext = "30721-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case ladd:
			switch {
			case cond == 1:
				if p.HasItem(pureSilver) {
					htmltext = "30721-08.htm"
					st.SetCond(2)
					s.PlaySound(p, script.SoundMiddle)
				} else {
					htmltext = "30721-07.htm"
				}
			case cond < 5:
				htmltext = "30721-10.htm"
			case cond == 5 && p.HasItem(trueGold):
				htmltext = "30721-11.htm"
			case cond == 6 || cond == 7:
				htmltext = "30721-13.htm"
			case cond == 8 && p.HasItem(mimirElixir):
				htmltext = "30721-14.htm"
			}
		case joan:
			switch {
			case cond == 2:
				htmltext = "30718-01.htm"
			case cond == 3:
				htmltext = "30718-04.htm"
			case cond == 4 && p.HasItem(sageStone):
				htmltext = "30718-05.htm"
				st.SetCond(5)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, sageStone, -1)
				s.GiveItems(p, trueGold, 1)
			case cond > 4:
				htmltext = "30718-06.htm"
			}
		// The urn answers its first page; the links do the mixing.
		case mixingUrn:
			htmltext = "31149-01.htm"
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
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
	case chimeraPiece:
		if st.Cond() == 3 && s.DropItems(p, sageStone, 1, 1, 200000) {
			st.SetCond(4)
		}
	case bloodyGuardian:
		if st.Cond() == 6 && s.DropItems(p, bloodFire, 1, 1, 200000) {
			st.SetCond(7)
		}
	}
}
