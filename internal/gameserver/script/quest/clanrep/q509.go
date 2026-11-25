package clanrep

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const q509Name = "Q509_TheClansPrestige"

// Proofs of Q509.
const (
	daimonEyes           = 8489
	hestiaFairyStone     = 8490
	nucleusOfLesserGolem = 8491
	falstonFang          = 8492
	shaidTalon           = 8493
)

// Raid bosses of Q509.
const (
	daimonTheWhiteEyed  = 25290
	hestiaGuardianDeity = 25293
	plagueGolem         = 25523
	demonAgentFalston   = 25322
	queenShyeed         = 25514
)

// q509 is the quest; the raid list pays the proofs.
type q509 struct{ raidList }

// Q509 is the quest The Clan's Prestige: a clan leader picks one raid boss
// from Valdis's list, and each proof of its death the leader's clan brings
// back raises the clan's reputation.
func Q509() script.Script {
	q := q509{raidList{name: q509Name, raids: []raid{
		{daimonTheWhiteEyed, daimonEyes, 180, 215, loc{186320, -43904, -3175}},
		{hestiaGuardianDeity, hestiaFairyStone, 430, 465, loc{134672, -115600, -1216}},
		{plagueGolem, nucleusOfLesserGolem, 380, 415, loc{170000, -59900, -3848}},
		{demonAgentFalston, falstonFang, 220, 255, loc{93296, -75104, -1824}},
		{queenShyeed, shaidTalon, 130, 165, loc{79635, -55612, -5980}},
	}}}
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
		Hooks: script.Hooks{OnEvent: q.onAdvEvent, OnTalk: q.onTalk, OnMyDying: q.onMyDying},
	}
}

// onAdvEvent starts the quest on the raid the event numbers.
func (q q509) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, q509Name)
	if st == nil {
		return htmltext
	}
	switch {
	case script.IsDigit(e.Name):
		htmltext = "31331-" + e.Name + ".htm"
		q.start(s, e.Player, st, e.Name)
	case strings.EqualFold(e.Name, "31331-6.htm"):
		s.PlaySound(e.Player, script.SoundFinish)
		st.Exit(true)
	}
	return htmltext
}

func (q q509) onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, q509Name)
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
		if !e.Player.IsClanLeader() {
			// The owner left the clan or its lead: nothing is taken (#5167).
			return "31331-0a.htm"
		}
		raid, paid := q.pay(s, e.Player, st)
		page := "31331-" + strconv.Itoa(int(raid))
		if paid {
			htmltext = page + "b.htm"
		} else {
			htmltext = page + "a.htm"
		}
	}
	return htmltext
}
