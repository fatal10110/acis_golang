// Package q328 is the quest Sense for Business: Sarien buys the monster eye
// carcasses and lenses and the basilisk gizzards.
package q328

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q328_SenseForBusiness"

// Items.
const (
	monsterEyeLens    = 1366
	monsterEyeCarcass = 1347
	basiliskGizzard   = 1348
)

// chances returns the drop chance of each monster, by NPC id: a percent
// for the monster eyes, out of a million for the basilisks.
func chances() map[int32]int32 {
	return map[int32]int32{
		20055: 48,
		20059: 52,
		20067: 68,
		20068: 76,
		20070: 500000,
		20072: 510000,
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Sense for Business",
		QuestID: 328,
		Items:   []int32{monsterEyeLens, monsterEyeCarcass, basiliskGizzard},
		Bind: script.Bindings{
			script.EventQuestStart: {30436}, // Sarien
			script.EventTalked:     {30436},
			script.EventMyDying:    {20055, 20059, 20067, 20068, 20070, 20072},
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
	case strings.EqualFold(e.Name, "30436-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30436-06.htm"):
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
		if p.Level() < 21 {
			htmltext = "30436-01.htm"
		} else {
			htmltext = "30436-02.htm"
		}
	case questlog.StatusStarted:
		carcasses := p.ItemCount(monsterEyeCarcass)
		lenses := p.ItemCount(monsterEyeLens)
		gizzards := p.ItemCount(basiliskGizzard)
		all := carcasses + lenses + gizzards
		if all == 0 {
			htmltext = "30436-04.htm"
		} else {
			htmltext = "30436-05.htm"
			s.TakeItems(p, monsterEyeCarcass, -1)
			s.TakeItems(p, monsterEyeLens, -1)
			s.TakeItems(p, basiliskGizzard, -1)
			bonus := int32(0)
			if all >= 10 {
				bonus = 618
			}
			s.RewardItems(p, 57, 25*carcasses+1000*lenses+60*gizzards+bonus)
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	npcID := e.NPC.NpcID()
	chance := chances()[npcID]
	if npcID < 20069 {
		rnd := int32(s.Rnd(100))
		if rnd < chance+1 {
			item := int32(monsterEyeLens)
			if rnd < chance {
				item = monsterEyeCarcass
			}
			s.DropItemsAlways(p, item, 1, 0)
		}
	} else {
		s.DropItems(p, basiliskGizzard, 1, 0, chance)
	}
}
