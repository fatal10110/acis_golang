// Package blackjudge is the Black Judge, who lifts one level of a player's
// death penalty for a fee that grows with the player's level.
package blackjudge

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// judge is the Black Judge.
const judge = 30981

// deathPenalty is the fee, in adena, for lifting one level of death
// penalty from a player of minLevel or above.
type deathPenalty struct{ minLevel, fee int32 }

// deathPenalties are the fees, by the index a fee page's link names.
func deathPenalties() []deathPenalty {
	return []deathPenalty{
		{76, 144000},
		{61, 86400},
		{52, 50400},
		{40, 25200},
		{20, 8640},
		{1, 3600},
	}
}

// New returns the feature.
func New() script.Script {
	return script.Script{
		Dir:   "feature",
		Bind:  script.Bindings{script.EventFirstTalk: {judge}, script.EventTalked: {judge}},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnFirstTalk: onFirstTalk},
	}
}

// onAdvEvent answers the judge's links. "test_dp" opens the fee page of the
// player's level. "remove_dp <index>" lifts one level of death penalty for
// the fee of that index: a player with none is told so, one short of the
// fee is told that; a malformed index, or a fee for a higher level than the
// player's, answers nothing. Any other event is answered as a page.
func onAdvEvent(s *script.Script, e script.Event) string {
	event := e.Name
	if strings.EqualFold(event, "test_dp") {
		switch level := e.Player.Level(); {
		case level >= 76:
			event = "black_judge007.htm"
		case level >= 61:
			event = "black_judge006.htm"
		case level >= 52:
			event = "black_judge005.htm"
		case level >= 40:
			event = "black_judge004.htm"
		case level >= 20:
			event = "black_judge003.htm"
		case level >= 1:
			event = "black_judge002.htm"
		}
	} else if strings.HasPrefix(event, "remove_dp") {
		if e.Player.DeathPenaltyLevel() <= 0 {
			event = "black_judge009.htm"
		} else {
			split := strings.Split(event, " ")
			if len(split) < 2 || !script.IsDigit(split[1]) {
				return ""
			}
			index, err := strconv.Atoi(split[1])
			if err != nil {
				panic(err)
			}
			// An index past the fees aborts the event here.
			dp := deathPenalties()[index]
			if e.Player.Level() < dp.minLevel {
				return ""
			}
			if e.Player.Adena() < dp.fee {
				event = "black_judge008.htm"
			} else {
				s.TakeItems(e.Player, 57, dp.fee)
				e.Player.ReduceDeathPenaltyLevel()
				return ""
			}
		}
	}
	return event
}

func onFirstTalk(*script.Script, script.FirstTalk) string {
	return "black_judge001.htm"
}
