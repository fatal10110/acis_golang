// Package q509 is the quest The Clan's Prestige: a clan leader picks one
// raid boss from Valdis's list, and each proof of its death the leader's
// clan brings back raises the clan's reputation.
package q509

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q509_TheClansPrestige"

// valdis is the quest giver.
const valdis = 31331

// Items.
const (
	daimonEyes           = 8489
	hestiaFairyStone     = 8490
	nucleusOfLesserGolem = 8491
	falstonFang          = 8492
	shaidTalon           = 8493
)

// Raid bosses.
const (
	daimonTheWhiteEyed  = 25290
	hestiaGuardianDeity = 25293
	plagueGolem         = 25523
	demonAgentFalston   = 25322
	queenShyeed         = 25514
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
		{daimonTheWhiteEyed, daimonEyes, 180, 215},
		{hestiaGuardianDeity, hestiaFairyStone, 430, 465},
		{plagueGolem, nucleusOfLesserGolem, 380, 415},
		{demonAgentFalston, falstonFang, 220, 255},
		{queenShyeed, shaidTalon, 130, 165},
	}
}

// loc is a radar mark.
type loc struct{ x, y, z int32 }

// locs returns the radar mark of each raid, in the same order.
func locs() []loc {
	return []loc{
		{186320, -43904, -3175},
		{134672, -115600, -1216},
		{170000, -59900, -3848},
		{93296, -75104, -1824},
		{79635, -55612, -5980},
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "The Clan's Prestige",
		QuestID: 509,
		Items:   []int32{daimonEyes, hestiaFairyStone, nucleusOfLesserGolem, falstonFang, shaidTalon},
		Bind: script.Bindings{
			script.EventQuestStart: {valdis},
			script.EventTalked:     {valdis},
			script.EventMyDying: {
				daimonTheWhiteEyed, hestiaGuardianDeity, plagueGolem, demonAgentFalston, queenShyeed,
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
		htmltext = "31331-" + e.Name + ".htm"
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
	case strings.EqualFold(e.Name, "31331-6.htm"):
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
			htmltext = "31331-0a.htm"
		case e.Player.ClanLevel() < 6:
			htmltext = "31331-0b.htm"
		default:
			htmltext = "31331-0c.htm"
		}
	case questlog.StatusStarted:
		raid := st.GetInt("raid")
		r := rewards()[raid-1]
		page := "31331-" + strconv.Itoa(int(raid))
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
