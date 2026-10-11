// Package q510 is the quest A Clan's Reputation (Valdis): the leader of a
// clan of level 5 or more hunts tyrannosaurs, and every claw the leader's
// clan brings back is worth 50 reputation points.
package q510

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q510_AClansReputation"

// valdis is the quest giver.
const valdis = 31331

// tyrannosaurusClaw is the quest item.
const tyrannosaurusClaw = 8767

// clanQuestCompleted is the system message of the reputation a clan quest
// earned.
const clanQuestCompleted = 1777

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "A Clan's Reputation",
		QuestID: 510,
		Items:   []int32{tyrannosaurusClaw},
		Bind: script.Bindings{
			script.EventQuestStart: {valdis},
			script.EventTalked:     {valdis},
			script.EventMyDying:    {22215, 22216, 22217},
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
	case strings.EqualFold(e.Name, "31331-3.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.EqualFold(e.Name, "31331-6.htm"):
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
	}
	return htmltext
}

// onTalk pays 50 reputation points a claw. A payer who left the clan
// panics once the claws are taken, as the reference throws there.
func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if !e.Player.IsClanLeader() || e.Player.ClanLevel() < 5 {
			htmltext = "31331-0.htm"
		} else {
			htmltext = "31331-1.htm"
		}
	case questlog.StatusStarted:
		count := 50 * e.Player.ItemCount(tyrannosaurusClaw)
		if count > 0 {
			htmltext = "31331-7.htm"
			s.TakeItems(e.Player, tyrannosaurusClaw, -1)
			e.Player.AddClanReputation(count)
			e.Player.SystemMessageNumbers(clanQuestCompleted, count)
			e.Player.BroadcastClanInfo()
		} else {
			htmltext = "31331-4.htm"
		}
	}
	return htmltext
}

// onMyDying gives a claw to the killer's clan leader, when the leader
// stands near with the quest started.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.ClanLeaderQuestState(p, e.NPC)
	if st == nil || st.Status() != questlog.StatusStarted {
		return
	}
	s.DropItemsAlways(st.Player(), tyrannosaurusClaw, 1, 0)
}
