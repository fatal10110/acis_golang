package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// User command system message ids.
const (
	SystemMessageHungryStriderNotMount          = 1008 // no parameter
	SystemMessageStuckTransportInFiveMinutes    = 809  // no parameter
	SystemMessageTimeS1S2InTheDay               = 927  // number, text
	SystemMessageTimeS1S2InTheNight             = 928  // number, text
	SystemMessagePartyInformation               = 1030 // no parameter
	SystemMessageLootingFindersKeepers          = 1031 // no parameter
	SystemMessageNoUnstuckPleaseSendPetition    = 1043 // no parameter
	SystemMessageCannotDismountFromElevation    = 1158 // no parameter
	SystemMessageS1S2Alliance                   = 1200 // two text parameters
	SystemMessageS1NoAllianceExists             = 1202 // text parameter
	SystemMessageNoDismountHere                 = 1385 // no parameter
	SystemMessageClansYouDeclaredWarOn          = 1571 // no parameter
	SystemMessageClansThatHaveDeclaredWarOnYou  = 1572 // no parameter
	SystemMessageYouArentInClanWars             = 1573 // no parameter
	SystemMessageNoClanWarsVsYou                = 1574 // no parameter
	SystemMessageLeftCommandChannel             = 1586 // no parameter
	SystemMessageS1PartyLeftCommandChannel      = 1587 // text parameter
	SystemMessagePartyLeaderS1                  = 1611 // text parameter
	SystemMessageWarList                        = 1612 // no parameter
	SystemMessageOnlyClanLeaderCanIssueCommands = 1966 // no parameter
)

// SystemMessageLooting returns the message naming the party loot rule
// rule, numbered as the client numbers loot rules: 0 finders keepers, 1
// random, 2 random including spoil, 3 by turn, 4 by turn including spoil
// (1031 to 1035).
func SystemMessageLooting(rule int32) int {
	return SystemMessageLootingFindersKeepers + int(rule)
}

// OpcodeExMultiPartyCommandChannelInfo is the extended opcode of the
// command channel overview.
const OpcodeExMultiPartyCommandChannelInfo uint16 = 0x0030

// ChannelParty is one party row of the command channel overview.
type ChannelParty struct {
	LeaderName string
	LeaderID   int32
	Members    int32
}

// FrameExMultiPartyCommandChannelInfo is the command channel overview: its
// leader, its member count over every party, and each party. The channel
// loot field is always 0.
func FrameExMultiPartyCommandChannelInfo(leaderName string, members int32, parties []ChannelParty) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExMultiPartyCommandChannelInfo)
	w.WriteString(leaderName)
	w.WriteInt32(0)
	w.WriteInt32(members)
	w.WriteInt32(int32(len(parties)))
	for _, p := range parties {
		w.WriteString(p.LeaderName)
		w.WriteInt32(p.LeaderID)
		w.WriteInt32(p.Members)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
