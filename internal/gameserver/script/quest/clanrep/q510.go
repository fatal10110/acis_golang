package clanrep

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const q510Name = "Q510_AClansReputation"

// tyrannosaurusClaw is the proof of Q510.
const tyrannosaurusClaw = 8767

// Q510 is the quest A Clan's Reputation (Valdis): the leader of a
// clan of level 5 or more hunts tyrannosaurs, and every claw the leader's
// clan brings back is worth 50 reputation points.
func Q510() script.Script {
	return script.Script{
		Title:   "A Clan's Reputation",
		QuestID: 510,
		Items:   []int32{tyrannosaurusClaw},
		Bind: script.Bindings{
			script.EventQuestStart: {valdis},
			script.EventTalked:     {valdis},
			script.EventMyDying:    {22215, 22216, 22217},
		},
		Hooks: script.Hooks{OnEvent: q510Event, OnTalk: q510Talk, OnMyDying: q510Dying},
	}
}

func q510Event(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, q510Name)
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

// q510Talk pays 50 reputation points a claw. A payer who no longer leads a
// clan is refused before any claw is taken (#5167).
func q510Talk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, q510Name)
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
		if !e.Player.IsClanLeader() {
			// The payer left the clan or its lead: nothing is taken (#5167).
			return "31331-0.htm"
		}
		count := 50 * s.TakeAllItems(e.Player, tyrannosaurusClaw)
		if count > 0 {
			htmltext = "31331-7.htm"
			payClan(e.Player, count)
		} else {
			htmltext = "31331-4.htm"
		}
	}
	return htmltext
}

// q510Dying gives a claw to the killer's clan leader, when the leader
// stands near with the quest started.
func q510Dying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.ClanLeaderQuestState(p, e.NPC)
	if st == nil || st.Status() != questlog.StatusStarted {
		return
	}
	s.DropItemsAlways(st.Player(), tyrannosaurusClaw, 1, 0)
}
