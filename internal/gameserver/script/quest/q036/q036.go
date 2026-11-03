// Package q036 is the quest Make a Sewing Kit: for a player whose formal wear
// needs a sewing kit, Ferris wants reinforced steel from iron golems, then
// artisan's frames and oriharukon for the kit.
package q036

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q036_MakeASewingKit"

// Items.
const (
	reinforcedSteel = 7163
	artisansFrame   = 1891
	oriharukon      = 1893
)

// Reward.
const sewingKit = 7078

// ferris is the quest giver.
const ferris = 30847

// ironGolem drops the reinforced steel.
const ironGolem = 20566

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Make a Sewing Kit",
		QuestID: 36,
		Items:   []int32{reinforcedSteel},
		Bind: script.Bindings{
			script.EventQuestStart: {ferris},
			script.EventTalked:     {ferris},
			script.EventMyDying:    {ironGolem},
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
	case strings.EqualFold(e.Name, "30847-1.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30847-3.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, reinforcedSteel, 5)
	case strings.EqualFold(e.Name, "30847-5.htm"):
		if hasMaterials(p) {
			s.TakeItems(p, artisansFrame, 10)
			s.TakeItems(p, oriharukon, 10)
			s.GiveItems(p, sewingKit, 1)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		} else {
			htmltext = "30847-4a.htm"
		}
	}
	return htmltext
}

// hasMaterials reports whether the player holds the oriharukon and artisan's
// frames the kit takes.
func hasMaterials(p *script.Player) bool {
	return p.ItemCount(oriharukon) >= 10 && p.ItemCount(artisansFrame) >= 10
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
		if p.Level() >= 60 {
			if fwear := s.QuestState(p, "Q037_MakeFormalWear"); fwear != nil && fwear.Cond() == 6 {
				htmltext = "30847-0.htm"
			} else {
				htmltext = "30847-0a.htm"
			}
		} else {
			htmltext = "30847-0b.htm"
		}
	case questlog.StatusStarted:
		switch st.Cond() {
		case 1:
			htmltext = "30847-1a.htm"
		case 2:
			htmltext = "30847-2.htm"
		case 3:
			if hasMaterials(p) {
				htmltext = "30847-4.htm"
			} else {
				htmltext = "30847-4a.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerCondition(p, e.NPC, 1)
	if st == nil {
		return
	}
	if s.DropItems(p, reinforcedSteel, 1, 5, 500000) {
		st.SetCond(2)
	}
}
