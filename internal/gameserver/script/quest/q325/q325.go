// Package q325 is the quest Grim Collector: with Samed's anatomy diagram the
// player gathers zombie and skeleton parts, Varsak assembles complete
// skeletons from them, and Samed buys it all.
package q325

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q325_GrimCollector"

// Items.
const (
	anatomyDiagram   = 1349
	zombieHead       = 1350
	zombieHeart      = 1351
	zombieLiver      = 1352
	skull            = 1353
	ribBone          = 1354
	spine            = 1355
	armBone          = 1356
	thighBone        = 1357
	completeSkeleton = 1358
)

// NPCs.
const (
	curtis = 30336
	varsak = 30342
	samed  = 30434
)

// part is one entry of a monster's drop list: the part, and the bound the
// kill's hundred-sided roll must stay under to give it.
type part struct {
	itemID, chance int32
}

// droplist returns each monster's parts, by NPC id, in the order the roll
// tries them.
func droplist() map[int32][]part {
	return map[int32][]part{
		20026: {{zombieHead, 30}, {zombieHeart, 50}, {zombieLiver, 75}},
		20029: {{zombieHead, 30}, {zombieHeart, 52}, {zombieLiver, 75}},
		20035: {{skull, 5}, {ribBone, 15}, {spine, 29}, {thighBone, 79}},
		20042: {{skull, 6}, {ribBone, 19}, {armBone, 69}, {thighBone, 86}},
		20045: {{skull, 9}, {spine, 59}, {armBone, 77}, {thighBone, 97}},
		20051: {{skull, 9}, {ribBone, 59}, {spine, 79}, {armBone, 100}},
		20457: {{zombieHead, 40}, {zombieHeart, 60}, {zombieLiver, 80}},
		20458: {{zombieHead, 40}, {zombieHeart, 70}, {zombieLiver, 100}},
		20514: {{skull, 6}, {ribBone, 21}, {spine, 30}, {armBone, 31}, {thighBone, 64}},
		20515: {{skull, 5}, {ribBone, 20}, {spine, 31}, {armBone, 33}, {thighBone, 69}},
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Grim Collector",
		QuestID: 325,
		Items:   []int32{zombieHead, zombieHeart, zombieLiver, skull, ribBone, spine, armBone, thighBone, completeSkeleton, anatomyDiagram},
		Bind: script.Bindings{
			script.EventQuestStart: {curtis},
			script.EventTalked:     {curtis, varsak, samed},
			script.EventMyDying:    {20026, 20029, 20035, 20042, 20045, 20051, 20457, 20458, 20514, 20515},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// numberOfPieces is how many parts and complete skeletons p holds.
func numberOfPieces(p *script.Player) int32 {
	return p.ItemCount(zombieHead) + p.ItemCount(spine) + p.ItemCount(armBone) + p.ItemCount(zombieHeart) + p.ItemCount(zombieLiver) + p.ItemCount(skull) + p.ItemCount(ribBone) + p.ItemCount(thighBone) + p.ItemCount(completeSkeleton)
}

// payback buys every part and complete skeleton p holds, with a bonus for
// more than ten pieces and another for holding a complete skeleton.
func payback(s *script.Script, p *script.Player) {
	count := numberOfPieces(p)
	if count <= 0 {
		return
	}
	reward := 30*p.ItemCount(zombieHead) + 20*p.ItemCount(zombieHeart) + 20*p.ItemCount(zombieLiver) + 100*p.ItemCount(skull) + 40*p.ItemCount(ribBone) + 14*p.ItemCount(spine) + 14*p.ItemCount(armBone) + 14*p.ItemCount(thighBone) + 341*p.ItemCount(completeSkeleton)
	if count > 10 {
		reward += 1629
	}
	if p.HasItem(completeSkeleton) {
		reward += 543
	}
	s.TakeItems(p, zombieHead, -1)
	s.TakeItems(p, zombieHeart, -1)
	s.TakeItems(p, zombieLiver, -1)
	s.TakeItems(p, skull, -1)
	s.TakeItems(p, ribBone, -1)
	s.TakeItems(p, spine, -1)
	s.TakeItems(p, armBone, -1)
	s.TakeItems(p, thighBone, -1)
	s.TakeItems(p, completeSkeleton, -1)
	s.RewardItems(p, 57, reward)
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30336-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30434-03.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, anatomyDiagram, 1)
	case strings.EqualFold(e.Name, "30434-06.htm"):
		s.TakeItems(p, anatomyDiagram, -1)
		payback(s, p)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "30434-07.htm"):
		payback(s, p)
	case strings.EqualFold(e.Name, "30434-09.htm"):
		if skeletons := p.ItemCount(completeSkeleton); skeletons > 0 {
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, completeSkeleton, -1)
			s.RewardItems(p, 57, 543+341*skeletons)
		}
	case strings.EqualFold(e.Name, "30342-03.htm"):
		if !p.HasItems(spine, armBone, skull, ribBone, thighBone) {
			htmltext = "30342-02.htm"
		} else {
			s.TakeItems(p, spine, 1)
			s.TakeItems(p, skull, 1)
			s.TakeItems(p, armBone, 1)
			s.TakeItems(p, ribBone, 1)
			s.TakeItems(p, thighBone, 1)
			if s.Rnd(10) < 9 {
				s.GiveItems(p, completeSkeleton, 1)
			} else {
				htmltext = "30342-04.htm"
			}
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
			htmltext = "30336-01.htm"
		} else {
			htmltext = "30336-02.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case curtis:
			if !p.HasItem(anatomyDiagram) {
				htmltext = "30336-04.htm"
			} else {
				htmltext = "30336-05.htm"
			}
		case samed:
			switch {
			case !p.HasItem(anatomyDiagram):
				htmltext = "30434-01.htm"
			case numberOfPieces(p) == 0:
				htmltext = "30434-04.htm"
			case !p.HasItem(completeSkeleton):
				htmltext = "30434-05.htm"
			default:
				htmltext = "30434-08.htm"
			}
		case varsak:
			htmltext = "30342-01.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	if !p.HasItem(anatomyDiagram) {
		return
	}
	chance := int32(s.Rnd(100))
	for _, drop := range droplist()[e.NPC.NpcID()] {
		if chance < drop.chance {
			s.DropItemsAlways(p, drop.itemID, 1, 0)
			break
		}
	}
}
