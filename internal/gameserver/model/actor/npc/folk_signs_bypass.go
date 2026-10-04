package npc

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// signsPriestBypass answers the commands a Priest of Dawn, a Dusk Priestess
// or a Mammon NPC handles itself (DawnPriest, DuskPriest and
// SignsPriest.onBypassFeedback); ok is false for any other type, and for a
// command the generic dialog answers. A priest's Chat re-opens its cabal
// page for anyone. Every other command needs the talker to have selected
// the NPC, and answers nothing otherwise: SevenSignsDesc <n> opens the
// description page n, stopping the handling when n does not parse,
// SevenSigns runs the Seven Signs dialog, and a Mammon NPC's Chat <n> opens
// its chat page n with its refusals. The rest falls through to the generic
// dialog.
func (f *Folk) signsPriestBypass(pages Pages, kind InstanceKind, talker Talker, command string, reply BypassReply) (BypassReply, bool) {
	var own sevensigns.Cabal
	switch kind {
	case "DawnPriest":
		own = sevensigns.Dawn
	case "DuskPriest":
		own = sevensigns.Dusk
	case "SignsPriest":
	default:
		return reply, false
	}
	record := func() sevensigns.Record {
		if talker.SevenSigns == nil {
			return sevensigns.Record{}
		}
		return talker.SevenSigns()
	}
	if own != sevensigns.NoCabal && strings.HasPrefix(command, "Chat") {
		prefix := "dusk"
		if own == sevensigns.Dawn {
			prefix = "dawn"
		}
		reply.Outcome, reply.Chat = BypassSignsChat, ChatShownReleased
		reply.HTML = f.page(pages, sevenSignsPages+priestPage(prefix, own, record()))
		return reply, true
	}
	if !talker.CurrentFolk {
		reply.Outcome = BypassRefused
		return reply, true
	}
	switch {
	case strings.HasPrefix(command, "SevenSignsDesc"):
		arg, ok := commandChars(command, 15, -1)
		if !ok {
			reply.Outcome = BypassAborted
			return reply, true
		}
		n, err := commons.ParseInt(arg, 32)
		if err != nil {
			reply.Outcome = BypassAborted
			return reply, true
		}
		reply.Outcome = BypassChatWindow
		reply.HTML = f.page(pages, sevenSignsPages+"desc_"+strconv.FormatInt(n, 10)+".htm")
		return reply, true
	case strings.HasPrefix(command, "SevenSigns"):
		reply.Outcome, reply.DawnPriest = BypassSevenSigns, own == sevensigns.Dawn
		return reply, true
	case kind == "SignsPriest" && strings.HasPrefix(command, "Chat"):
		val := 0
		if arg, ok := commandChars(command, 5, -1); ok {
			if n, err := commons.ParseInt(arg, 32); err == nil {
				val = int(n)
			}
		}
		reply.Outcome = BypassSignsChat
		reply.HTML, reply.Chat = f.mammonChat(pages, record(), val)
		return reply, true
	}
	return reply, false
}
