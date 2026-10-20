package npc

import (
	"strconv"
	"strings"
)

// festivalGuideBypass answers the commands a festival guide or witch
// handles itself; ok is false for any other type, and for a command the
// generic dialog answers. FestivalDesc <n> reads n from the fourteenth
// character on and Festival <n> reads the one character after "Festival ":
// a command too short to hold it, or whose number does not parse, stops the
// handling. Any command starting with Chat re-opens the guide's or witch's
// page, whatever page it names.
func (f *Folk) festivalGuideBypass(pages Pages, kind InstanceKind, talker Talker, command string, reply BypassReply) (BypassReply, bool) {
	if kind != "FestivalGuide" {
		return reply, false
	}
	switch {
	case strings.HasPrefix(command, "FestivalDesc"):
		arg, ok := commandChars(command, 13, -1)
		if !ok {
			reply.Outcome = BypassAborted
			return reply, true
		}
		n, err := strconv.ParseInt(arg, 10, 32)
		if err != nil {
			reply.Outcome = BypassAborted
			return reply, true
		}
		reply.Outcome, reply.Index, reply.FestivalDesc = BypassFestival, int(n), true
	case strings.HasPrefix(command, "Festival"):
		arg, ok := commandChars(command, 9, 10)
		if !ok {
			reply.Outcome = BypassAborted
			return reply, true
		}
		n, err := strconv.Atoi(arg)
		if err != nil {
			reply.Outcome = BypassAborted
			return reply, true
		}
		reply.Outcome, reply.Index = BypassFestival, n
	case strings.HasPrefix(command, "Chat"):
		notice := ""
		if talker.FestivalNotice != nil {
			notice = talker.FestivalNotice()
		}
		reply.Outcome = BypassChatWindow
		reply.HTML = strings.ReplaceAll(f.page(pages, festivalGuidePath(f.NpcID())), "%festivalMins%", notice)
	default:
		return reply, false
	}
	return reply, true
}
