package clanrep

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const q508Name = "Q508_AClansReputation"

// sirEricRodemai gives Q508.
const sirEricRodemai = 30868

// Proofs of Q508.
const (
	nucleusOfFlamestoneGiant = 8494
	themisScale              = 8277
	nucleusOfHekatonPrime    = 8279
	tiphonShard              = 8280
	glakiNucleus             = 8281
	rahhaFang                = 8282
)

// Raid bosses of Q508.
const (
	flamestoneGiant      = 25524
	palibatiQueenThemis  = 25252
	hekatonPrime         = 25140
	gargoyleLordTiphon   = 25255
	lastLesserGiantGlaki = 25245
	rahha                = 25051
)

// q508 is the quest; the raid list pays the proofs.
type q508 struct{ raidList }

// Q508 is the quest A Clan's Reputation: a clan leader picks one raid boss
// from Sir Eric Rodemai's list, and each proof of its death the leader's clan
// brings back raises the clan's reputation.
func Q508() script.Script {
	q := q508{raidList{name: q508Name, raids: []raid{
		{palibatiQueenThemis, themisScale, 65, 100, loc{192346, 21528, -3648}},
		{hekatonPrime, nucleusOfHekatonPrime, 40, 75, loc{191979, 54902, -7658}},
		{gargoyleLordTiphon, tiphonShard, 30, 65, loc{170038, -26236, -3824}},
		{lastLesserGiantGlaki, glakiNucleus, 105, 140, loc{171762, 55028, -5992}},
		{rahha, rahhaFang, 40, 75, loc{117232, -9476, -3320}},
		{flamestoneGiant, nucleusOfFlamestoneGiant, 60, 95, loc{144218, -5816, -4722}},
	}}}
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
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: q.onTalk, OnMyDying: q.onMyDying},
	}
}

// onAdvEvent starts the quest on the raid the event numbers.
func (q q508) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, q508Name)
	if st == nil {
		return htmltext
	}
	switch {
	case script.IsDigit(e.Name):
		htmltext = "30868-" + e.Name + ".htm"
		q.start(s, e.Player, st, e.Name)
	case strings.EqualFold(e.Name, "30868-7.htm"):
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
	}
	return htmltext
}

func (q q508) onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, q508Name)
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
		if !e.Player.IsClanLeader() {
			// The owner left the clan or its lead: nothing is taken (#5167).
			return "30868-0a.htm"
		}
		raid, paid := q.pay(s, e.Player, st)
		page := "30868-" + strconv.Itoa(int(raid))
		if paid {
			htmltext = page + "b.htm"
		} else {
			htmltext = page + "a.htm"
		}
	}
	return htmltext
}
