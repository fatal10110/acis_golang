// Package q508 is the quest A Clan's Reputation: a clan leader picks one
// raid boss from Sir Eric Rodemai's list, and each proof of its death the
// leader's clan brings back raises the clan's reputation.
package q508

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q508_AClansReputation"

// sirEricRodemai is the quest giver.
const sirEricRodemai = 30868

// Items.
const (
	nucleusOfFlamestoneGiant = 8494
	themisScale              = 8277
	nucleusOfHekatonPrime    = 8279
	tiphonShard              = 8280
	glakiNucleus             = 8281
	rahhaFang                = 8282
)

// Raid bosses.
const (
	flamestoneGiant      = 25524
	palibatiQueenThemis  = 25252
	hekatonPrime         = 25140
	gargoyleLordTiphon   = 25255
	lastLesserGiantGlaki = 25245
	rahha                = 25051
)

// clanQuestCompleted is the system message of the reputation a clan quest
// earned.
const clanQuestCompleted = 1777

// reward is one raid of the list: its boss, the proof it drops and the
// reputation range the proof is worth.
type reward struct {
	boss, item, minPoints, maxPoints int32
}

// rewards returns the raids, in the order the "raid" variable numbers them
// from 1.
func rewards() []reward {
	return []reward{
		{palibatiQueenThemis, themisScale, 65, 100},
		{hekatonPrime, nucleusOfHekatonPrime, 40, 75},
		{gargoyleLordTiphon, tiphonShard, 30, 65},
		{lastLesserGiantGlaki, glakiNucleus, 105, 140},
		{rahha, rahhaFang, 40, 75},
		{flamestoneGiant, nucleusOfFlamestoneGiant, 60, 95},
	}
}

// loc is a radar mark.
type loc struct{ x, y, z int32 }

// locs returns the radar mark of each raid, in the same order.
func locs() []loc {
	return []loc{
		{192346, 21528, -3648},
		{191979, 54902, -7658},
		{170038, -26236, -3824},
		{171762, 55028, -5992},
		{117232, -9476, -3320},
		{144218, -5816, -4722},
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "A Clan's Reputation",
		QuestID: 508,
		Items:   []int32{themisScale, nucleusOfHekatonPrime, tiphonShard, glakiNucleus, rahhaFang, nucleusOfFlamestoneGiant},
		Bind: script.Bindings{
			script.EventQuestStart: {sirEricRodemai},
			script.EventTalked:     {sirEricRodemai},
			script.EventMyDying: {
				flamestoneGiant, palibatiQueenThemis, hekatonPrime, gargoyleLordTiphon,
				lastLesserGiantGlaki, rahha,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// onAdvEvent starts the quest on the raid the event numbers. A number off
// the list panics once the quest has started.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case script.IsDigit(e.Name):
		htmltext = "30868-" + e.Name + ".htm"
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		st.Set("raid", e.Name)
		s.PlaySound(e.Player, script.SoundAccept)
		raid, err := strconv.Atoi(e.Name)
		if err != nil {
			panic(err)
		}
		mark := locs()[raid-1]
		e.Player.AddRadarMarker(mark.x, mark.y, mark.z)
	case strings.EqualFold(e.Name, "30868-7.htm"):
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
	switch st.Status() {
	case questlog.StatusCreated:
		switch {
		case !e.Player.IsClanLeader():
			htmltext = "30868-0a.htm"
		case e.Player.ClanLevel() < 5:
			htmltext = "30868-0b.htm"
		default:
			htmltext = "30868-0c.htm"
		}
	case questlog.StatusStarted:
		raid := st.GetInt("raid")
		r := rewards()[raid-1]
		page := "30868-" + strconv.Itoa(int(raid))
		if !e.Player.HasItem(r.item) {
			htmltext = page + "a.htm"
		} else {
			points := r.minPoints + int32(s.Rnd(int(r.maxPoints-r.minPoints+1)))
			htmltext = page + "b.htm"
			s.TakeItems(e.Player, r.item, 1)
			e.Player.AddClanReputation(points)
			e.Player.SystemMessageNumbers(clanQuestCompleted, points)
			e.Player.BroadcastClanInfo()
		}
	}
	return htmltext
}

// onMyDying drops the proof to the killer's clan leader, when the leader
// stands near with the quest started on that boss.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.ClanLeaderQuestState(p, e.NPC)
	if st == nil || st.Status() != questlog.StatusStarted {
		return
	}
	raid := st.GetInt("raid")
	if r := rewards()[raid-1]; r.boss == e.NPC.NpcID() {
		s.DropItemsAlways(st.Player(), r.item, 1, 1)
	}
}
