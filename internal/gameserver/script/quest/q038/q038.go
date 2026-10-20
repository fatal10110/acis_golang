// Package q038 is the quest Dragon Fangs: Luis sends a player after feather
// ornaments for a tooth of totem, which Iris and Rohmer trade through two
// letters before Iris pays for fifty dragon teeth with a random piece of gear.
package q038

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q038_DragonFangs"

// Items.
const (
	featherOrnament = 7173
	toothOfTotem    = 7174
	toothOfDragon   = 7175
	letterOfIris    = 7176
	letterOfRohmer  = 7177
)

// NPCs.
const (
	luis   = 30386
	iris   = 30034
	rohmer = 30344
)

// reward is one of Iris's rewards: an item and an adena sum.
type reward struct{ item, adena int32 }

// rewards returns Iris's rewards, one of which is picked at random.
func rewards() []reward {
	return []reward{
		{45, 5200},
		{627, 1500},
		{1123, 3200},
		{605, 3200},
	}
}

// drop is what a monster drops: at cond, the item, up to needed, at chance.
type drop struct{ cond, item, needed, chance int32 }

// droplist returns the monsters' drops.
func droplist() map[int32]drop {
	return map[int32]drop{
		21100: {1, featherOrnament, 100, 1000000},
		20357: {1, featherOrnament, 100, 1000000},
		21101: {6, toothOfDragon, 50, 500000},
		20356: {6, toothOfDragon, 50, 500000},
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Dragon Fangs",
		QuestID: 38,
		Items:   []int32{featherOrnament, toothOfTotem, toothOfDragon, letterOfIris, letterOfRohmer},
		Bind: script.Bindings{
			script.EventQuestStart: {luis},
			script.EventTalked:     {luis, iris, rohmer},
			script.EventMyDying:    {20356, 20357, 21100, 21101},
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
	case strings.EqualFold(e.Name, "30386-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30386-04.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, featherOrnament, 100)
		s.GiveItems(p, toothOfTotem, 1)
	case strings.EqualFold(e.Name, "30034-02a.htm"):
		if p.HasItem(toothOfTotem) {
			htmltext = "30034-02.htm"
			st.SetCond(4)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, toothOfTotem, 1)
			s.GiveItems(p, letterOfIris, 1)
		}
	case strings.EqualFold(e.Name, "30344-02a.htm"):
		if p.HasItem(letterOfIris) {
			htmltext = "30344-02.htm"
			st.SetCond(5)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, letterOfIris, 1)
			s.GiveItems(p, letterOfRohmer, 1)
		}
	case strings.EqualFold(e.Name, "30034-04a.htm"):
		if p.HasItem(letterOfRohmer) {
			htmltext = "30034-04.htm"
			st.SetCond(6)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, letterOfRohmer, 1)
		}
	case strings.EqualFold(e.Name, "30034-06a.htm"):
		if p.ItemCount(toothOfDragon) >= 50 {
			list := rewards()
			r := list[s.Rnd(len(list))]
			htmltext = "30034-06.htm"
			s.TakeItems(p, toothOfDragon, 50)
			s.GiveItems(p, r.item, 1)
			s.RewardItems(p, 57, r.adena)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
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
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 19 {
			htmltext = "30386-01a.htm"
		} else {
			htmltext = "30386-01.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch e.NPC.NpcID() {
		case luis:
			if cond == 1 {
				htmltext = "30386-02a.htm"
			} else if cond == 2 {
				htmltext = "30386-03.htm"
			} else if cond > 2 {
				htmltext = "30386-03a.htm"
			}
		case iris:
			switch cond {
			case 3:
				htmltext = "30034-01.htm"
			case 4:
				htmltext = "30034-02b.htm"
			case 5:
				htmltext = "30034-03.htm"
			case 6:
				htmltext = "30034-05a.htm"
			case 7:
				htmltext = "30034-05.htm"
			}
		case rohmer:
			if cond == 4 {
				htmltext = "30344-01.htm"
			} else if cond > 4 {
				htmltext = "30344-03.htm"
			}
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
	d := droplist()[e.NPC.NpcID()]
	if st.Cond() == d.cond && s.DropItems(p, d.item, 1, d.needed, d.chance) {
		st.SetCond(d.cond + 1)
	}
}
