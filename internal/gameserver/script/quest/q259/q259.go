// Package q259 is the quest Rancher's Plea: Edmond pays for giant spider
// skins, and Marius trades ten of them for a healing potion or arrows.
package q259

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q259_RanchersPlea"

// NPCs.
const (
	edmond = 30497
	marius = 30405
)

// Monsters.
const (
	giantSpider = 20103
	talonSpider = 20106
	bladeSpider = 20108
)

// Item.
const giantSpiderSkin = 1495

// Rewards.
const (
	adena         = 57
	healingPotion = 1061
	woodenArrow   = 17
)

// incorrectCount is the page Marius answers a trade the player holds too
// few skins for.
const incorrectCount = "<html><body>Incorrect item count</body></html>"

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Rancher's Plea",
		QuestID: 259,
		Items:   []int32{giantSpiderSkin},
		Bind: script.Bindings{
			script.EventQuestStart: {edmond},
			script.EventTalked:     {edmond, marius},
			script.EventMyDying:    {giantSpider, talonSpider, bladeSpider},
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
	case strings.EqualFold(e.Name, "30497-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30497-06.htm"):
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30405-04.htm"):
		if p.ItemCount(giantSpiderSkin) >= 10 {
			s.TakeItems(p, giantSpiderSkin, 10)
			s.RewardItems(p, healingPotion, 1)
		} else {
			htmltext = incorrectCount
		}
	case strings.EqualFold(e.Name, "30405-05.htm"):
		if p.ItemCount(giantSpiderSkin) >= 10 {
			s.TakeItems(p, giantSpiderSkin, 10)
			s.RewardItems(p, woodenArrow, 50)
		} else {
			htmltext = incorrectCount
		}
	case strings.EqualFold(e.Name, "30405-07.htm"):
		if p.ItemCount(giantSpiderSkin) >= 10 {
			htmltext = "30405-06.htm"
		}
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
		if p.Level() < 15 {
			htmltext = "30497-01.htm"
		} else {
			htmltext = "30497-02.htm"
		}
	case questlog.StatusStarted:
		count := p.ItemCount(giantSpiderSkin)
		switch e.NPC.NpcID() {
		case edmond:
			if count == 0 {
				htmltext = "30497-04.htm"
			} else {
				htmltext = "30497-05.htm"
				s.TakeItems(p, giantSpiderSkin, -1)
				bonus := int32(0)
				if count >= 10 {
					bonus = 250
				}
				s.RewardItems(p, adena, bonus+count*25)
			}
		case marius:
			if count < 10 {
				htmltext = "30405-01.htm"
			} else {
				htmltext = "30405-02.htm"
			}
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	s.DropItemsAlways(p, giantSpiderSkin, 1, 0)
}
