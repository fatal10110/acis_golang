// Package q642 is the quest A Powerful Primeval Creature: Dinn, for a
// character of level 75 or more, pays adena for the dinosaur tissues the
// primeval creatures drop, and an item of the page's pick for a hundred and
// fifty tissues with a dinosaur egg.
package q642

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q642_APowerfulPrimevalCreature"

// NPC.
const dinn = 32105

// Items.
const (
	dinosaurTissue = 8774
	dinosaurEgg    = 8775
)

// ancientEgg is the monster that drops the dinosaur egg.
const ancientEgg = 18344

// rewards returns the items the page picks by index.
func rewards() []int32 {
	return []int32{8690, 8692, 8694, 8696, 8698, 8700, 8702, 8704, 8706, 8708, 8710}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "A Powerful Primeval Creature",
		QuestID: 642,
		Items:   []int32{dinosaurTissue, dinosaurEgg},
		Bind: script.Bindings{
			script.EventQuestStart: {dinn},
			script.EventTalked:     {dinn},
			script.EventMyDying: {
				22196, 22197, 22198, 22199, 22200, 22201, 22202, 22203, 22204, 22205, 22218, 22219,
				22220, 22223, 22224, 22225, ancientEgg,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, pays adena for the tissues, or trades a
// hundred and fifty tissues and an egg for adena and the item the page sent
// the index of; the client can only send a link of the page it was shown.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "32105-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "32105-08.htm"):
		if e.Player.ItemCount(dinosaurTissue) >= 150 && e.Player.HasItem(dinosaurEgg) {
			htmltext = "32105-06.htm"
		}
	case strings.EqualFold(e.Name, "32105-07.htm"):
		tissues := e.Player.ItemCount(dinosaurTissue)
		if tissues > 0 {
			s.TakeItems(e.Player, dinosaurTissue, -1)
			s.RewardItems(e.Player, 57, tissues*5000)
		} else {
			htmltext = "32105-08.htm"
		}
	case strings.Contains(e.Name, "event_"):
		if e.Player.ItemCount(dinosaurTissue) >= 150 && e.Player.HasItem(dinosaurEgg) {
			htmltext = "32105-07.htm"

			s.TakeItems(e.Player, dinosaurTissue, 150)
			s.TakeItems(e.Player, dinosaurEgg, 1)
			s.RewardItems(e.Player, 57, 44000)

			index, err := strconv.Atoi(strings.Split(e.Name, "_")[1])
			if err != nil {
				panic(err)
			}
			s.GiveItems(e.Player, rewards()[index], 1)
		} else {
			htmltext = "32105-08.htm"
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
		if e.Player.Level() < 75 {
			htmltext = "32105-00.htm"
		} else {
			htmltext = "32105-01.htm"
		}
	case questlog.StatusStarted:
		if !e.Player.HasItem(dinosaurTissue) {
			htmltext = "32105-08.htm"
		} else {
			htmltext = "32105-05.htm"
		}
	}
	return htmltext
}

// onMyDying gives the killer, who must have the quest, a dinosaur egg from
// the ancient egg or a dinosaur tissue from the other monsters, by chance.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	if e.NPC.NpcID() == ancientEgg {
		if s.Rnd(100) < 1 {
			s.GiveItems(p, dinosaurEgg, 1)

			if p.ItemCount(dinosaurTissue) >= 150 {
				s.PlaySound(p, script.SoundMiddle)
			} else {
				s.PlaySound(p, script.SoundItemGet)
			}
		}
	} else if s.Rnd(100) < 33 {
		s.RewardItems(p, dinosaurTissue, 1)

		if p.ItemCount(dinosaurTissue) >= 150 && p.HasItem(dinosaurEgg) {
			s.PlaySound(p, script.SoundMiddle)
		} else {
			s.PlaySound(p, script.SoundItemGet)
		}
	}
}
