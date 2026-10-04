package npc

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// The classes an Olympiad manager ranks: the third occupations.
const (
	firstRankedClass = 88
	lastRankedClass  = 118
)

// olympiadNobleCommand answers an Olympiad manager's "OlympiadNoble <n>"
// command. A talker holding a cursed weapon, playing a subclass, or not a
// noble of a third occupation is turned away with its page, in that order,
// whatever the command; the cursed weapon's page is sent as is, its
// %objectId% unfilled. The choice is then read from the fifteenth character
// on: a command too short to hold one, or whose choice does not parse,
// stops the handling.
func (f *Folk) olympiadNobleCommand(pages Pages, talker Talker, command string, reply BypassReply) BypassReply {
	switch {
	case talker.CursedWeapon:
		reply.Outcome, reply.HTML = BypassPage, f.rawPage(pages, olympiadPages+"noble_cant_cw.htm")
		return reply
	case talker.SubclassActive:
		reply.Outcome, reply.HTML = BypassPage, f.page(pages, olympiadPages+"noble_cant_sub.htm")
		return reply
	case !talker.Noble || !talker.ThirdClass:
		reply.Outcome, reply.HTML = BypassPage, f.page(pages, olympiadPages+"noble_cant_thirdclass.htm")
		return reply
	}
	arg, ok := commandChars(command, 14, -1)
	if !ok {
		reply.Outcome = BypassAborted
		return reply
	}
	choice, err := commons.ParseInt(arg, 32)
	if err != nil {
		reply.Outcome = BypassAborted
		return reply
	}
	reply.Outcome, reply.Index = BypassOlympiadNoble, int(choice)
	return reply
}

// classRankingCommand answers "Olympiad 2_<class>", the class read from the
// twelfth character on: a class from 88 to 118 is ranked, any other answers
// nothing, and a command too short to hold one, or whose class does not
// parse, stops the handling.
func classRankingCommand(reply BypassReply, command string) BypassReply {
	arg, ok := commandChars(command, 11, -1)
	if !ok {
		reply.Outcome = BypassAborted
		return reply
	}
	classID, err := commons.ParseInt(arg, 32)
	if err != nil {
		reply.Outcome = BypassAborted
		return reply
	}
	if classID < firstRankedClass || classID > lastRankedClass {
		reply.Outcome = BypassRefused
		return reply
	}
	reply.Outcome, reply.Index = BypassClassRanking, int(classID)
	return reply
}

// OlympiadPage is the Olympiad page name, each placeholder of fill, given
// as placeholder and value pairs, replaced in order, then %objectId%
// naming this NPC.
func (f *Folk) OlympiadPage(pages Pages, name string, fill ...string) string {
	page, ok := pages.Get(olympiadPages + name)
	if !ok {
		return f.page(pages, olympiadPages+name)
	}
	for i := 0; i+1 < len(fill); i += 2 {
		page = strings.ReplaceAll(page, fill[i], fill[i+1])
	}
	return strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(f.ObjectID())))
}

// OlympiadRankingPage is noble_ranking.htm listing names, the first ten at
// most, numbered from 1; the places left over are blank.
func (f *Folk) OlympiadRankingPage(pages Pages, names []string) string {
	fill := make([]string, 0, 40)
	for i := 1; i <= 10; i++ {
		place, rank := "", ""
		if i <= len(names) {
			place, rank = strconv.Itoa(i), names[i-1]
		}
		n := strconv.Itoa(i)
		fill = append(fill, "%place"+n+"%", place, "%rank"+n+"%", rank)
	}
	return f.OlympiadPage(pages, "noble_ranking.htm", fill...)
}

// rawPage reads path as is; a missing page reads as a "My html is missing"
// notice naming it.
func (f *Folk) rawPage(pages Pages, path string) string {
	if page, ok := pages.Get(path); ok {
		return page
	}
	return "<html><body>My html is missing:<br>" + path + "</body></html>"
}
