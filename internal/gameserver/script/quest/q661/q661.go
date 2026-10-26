// Package q661 is the quest Making the Harvest Grounds Safe: Norman pays
// adena for the stings, gems and talons the monsters of the harvest grounds
// drop.
package q661

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q661_MakingTheHarvestGroundsSafe"

// NPC.
const norman = 30210

// Items.
const (
	stingOfGiantPoisonBee = 8283
	cloudyGem             = 8284
	talonOfYoungAraneid   = 8285
)

// Monsters. Each drops the item whose id is its own minus dropOffset.
const (
	giantPoisonBee = 21095
	cloudyBeast    = 21096
	youngAraneid   = 21097
	dropOffset     = 12812
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Making the Harvest Grounds Safe",
		QuestID: 661,
		Items:   []int32{stingOfGiantPoisonBee, cloudyGem, talonOfYoungAraneid},
		Bind: script.Bindings{
			script.EventQuestStart: {norman},
			script.EventTalked:     {norman},
			script.EventMyDying:    {giantPoisonBee, cloudyBeast, youngAraneid},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest, pays adena for the items held, or ends the
// quest.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.EqualFold(e.Name, "30210-02.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "30210-04.htm"):
		item1 := e.Player.ItemCount(stingOfGiantPoisonBee)
		item2 := e.Player.ItemCount(cloudyGem)
		item3 := e.Player.ItemCount(talonOfYoungAraneid)
		sum := item1*57 + item2*56 + item3*60
		if item1+item2+item3 >= 10 {
			sum += 2871
		}
		s.TakeItems(e.Player, stingOfGiantPoisonBee, item1)
		s.TakeItems(e.Player, cloudyGem, item2)
		s.TakeItems(e.Player, talonOfYoungAraneid, item3)
		s.RewardItems(e.Player, 57, sum)
	case strings.EqualFold(e.Name, "30210-06.htm"):
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
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 21 {
			htmltext = "30210-01a.htm"
		} else {
			htmltext = "30210-01.htm"
		}
	case questlog.StatusStarted:
		if e.Player.HasAtLeastOneItem(stingOfGiantPoisonBee, cloudyGem, talonOfYoungAraneid) {
			htmltext = "30210-03.htm"
		} else {
			htmltext = "30210-05.htm"
		}
	}
	return htmltext
}

// onMyDying drops the item of the monster to the killer, who must have the
// quest.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItems(p, e.NPC.NpcID()-dropOffset, 1, 0, 500000)
}
