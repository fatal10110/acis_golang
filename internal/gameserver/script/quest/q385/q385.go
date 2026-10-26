// Package q385 is the quest Yoke of the Past: the gatekeepers of the Ziggurat
// trade the ancient scrolls of the monsters for blank scrolls.
package q385

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q385_YokeOfThePast"

// Item.
const ancientScroll = 5902

// Reward.
const blankScroll = 5965

// chances returns the scroll drop chance of each monster, by NPC id.
func chances() map[int32]int32 {
	return map[int32]int32{
		21208: 70000,
		21209: 80000,
		21210: 110000,
		21211: 110000,
		21213: 140000,
		21214: 190000,
		21215: 190000,
		21217: 240000,
		21218: 300000,
		21219: 300000,
		21221: 370000,
		21222: 460000,
		21223: 450000,
		21224: 500000,
		21225: 540000,
		21226: 660000,
		21227: 640000,
		21228: 700000,
		21229: 750000,
		21230: 910000,
		21231: 860000,
		21236: 120000,
		21237: 140000,
		21238: 190000,
		21239: 190000,
		21240: 220000,
		21241: 240000,
		21242: 300000,
		21243: 300000,
		21244: 340000,
		21245: 370000,
		21246: 460000,
		21247: 450000,
		21248: 500000,
		21249: 540000,
		21250: 660000,
		21251: 640000,
		21252: 700000,
		21253: 750000,
		21254: 910000,
		21255: 860000,
	}
}

// New returns the quest.
func New() script.Script {
	gatekeeperZiggurat := []int32{
		31095, 31096, 31097, 31098, 31099, 31100, 31101, 31102, 31103, 31104, 31105, 31106, 31107, 31108, 31109,
		31110, 31114, 31115, 31116, 31117, 31118, 31119, 31120, 31121, 31122, 31123, 31124, 31125, 31126,
	}
	return script.Script{
		Title:   "Yoke of the Past",
		QuestID: 385,
		Items:   []int32{ancientScroll},
		Bind: script.Bindings{
			script.EventQuestStart: gatekeeperZiggurat,
			script.EventTalked:     gatekeeperZiggurat,
			script.EventMyDying: {
				21208, 21209, 21210, 21211, 21213, 21214, 21215, 21217, 21218, 21219, 21221, 21222, 21223, 21224, 21225,
				21226, 21227, 21228, 21229, 21230, 21231, 21236, 21237, 21238, 21239, 21240, 21241, 21242, 21243, 21244,
				21245, 21246, 21247, 21248, 21249, 21250, 21251, 21252, 21253, 21254, 21255,
			},
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
	switch {
	case strings.EqualFold(e.Name, "05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "10.htm"):
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
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
		if p.Level() < 20 {
			htmltext = "02.htm"
		} else {
			htmltext = "01.htm"
		}
	case questlog.StatusStarted:
		if !p.HasItem(ancientScroll) {
			htmltext = "08.htm"
		} else {
			htmltext = "09.htm"
			count := p.ItemCount(ancientScroll)
			s.TakeItems(p, ancientScroll, -1)
			s.RewardItems(p, blankScroll, count)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMemberState(script.ActingPlayer(e.Killer), e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItems(st.Player(), ancientScroll, 1, 0, chances()[e.NPC.NpcID()])
}
