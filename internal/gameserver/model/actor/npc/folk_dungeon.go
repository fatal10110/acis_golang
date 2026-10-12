package npc

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// dungeonGatekeeperCommand answers a Seven Signs dungeon gatekeeper's own
// commands, matched by prefix: necro asks down into a necropolis, governed
// by the Seal of Avarice, and cata into a catacomb, governed by the Seal of
// Gnosis; see sevensigns.Record.DungeonEntry. A refused talker is shown
// seven_signs/necro_no.htm or cata_no.htm. exit takes the talker out. ok is
// false for every other command, and at any other NPC.
func (f *Folk) dungeonGatekeeperCommand(pages Pages, kind InstanceKind, talker Talker, command string, reply BypassReply) (BypassReply, bool) {
	if kind != "DungeonGatekeeper" {
		return reply, false
	}
	var seal sevensigns.Seal
	var refusal string
	switch {
	case strings.HasPrefix(command, "necro"):
		seal, refusal = sevensigns.Avarice, "necro_no.htm"
	case strings.HasPrefix(command, "cata"):
		seal, refusal = sevensigns.Gnosis, "cata_no.htm"
	case strings.HasPrefix(command, "exit"):
		reply.Outcome = BypassDungeonExit
		return reply, true
	default:
		return reply, false
	}
	var record sevensigns.Record
	if talker.SevenSigns != nil {
		record = talker.SevenSigns()
	}
	switch record.DungeonEntry(seal) {
	case sevensigns.DungeonAdmitted:
		reply.Outcome = BypassDungeonEnter
		return reply, true
	case sevensigns.DungeonDawnOnly:
		reply.DungeonOnly = sevensigns.Dawn
	case sevensigns.DungeonDuskOnly:
		reply.DungeonOnly = sevensigns.Dusk
	case sevensigns.DungeonRefused:
	}
	reply.Outcome, reply.HTML = BypassDungeonRefused, f.page(pages, "data/html/seven_signs/"+refusal)
	return reply, true
}
