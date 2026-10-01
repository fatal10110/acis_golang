package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// Party and command channel packet opcodes.
const (
	OpcodeAskJoinParty              = 0x39
	OpcodeJoinParty                 = 0x3a
	OpcodePartySmallWindowAll       = 0x4e
	OpcodePartySmallWindowAdd       = 0x4f
	OpcodePartySmallWindowDeleteAll = 0x50
	OpcodePartySmallWindowDelete    = 0x51
	OpcodePartySmallWindowUpdate    = 0x52
	OpcodePartyMemberPosition       = 0xa7

	OpcodeExOpenMPCC                uint16 = 0x0025
	OpcodeExCloseMPCC               uint16 = 0x0026
	OpcodeExAskJoinMPCC             uint16 = 0x0027
	OpcodeExMPCCShowPartyMemberInfo uint16 = 0x004a
	OpcodeExMPCCPartyInfoUpdate     uint16 = 0x005a
)

// Party and command channel system message ids.
const (
	SystemMessageYouInvitedS1ToParty                  = 105
	SystemMessageYouJoinedS1Party                     = 106
	SystemMessageS1JoinedParty                        = 107
	SystemMessageS1LeftParty                          = 108
	SystemMessageYouHaveInvitedTheWrongTarget         = 152
	SystemMessageOnlyLeaderCanInvite                  = 154
	SystemMessagePartyFull                            = 155
	SystemMessageS1IsAlreadyInParty                   = 160
	SystemMessageWaitingForAnotherReply               = 164
	SystemMessageFirstSelectUserToInviteToParty       = 185
	SystemMessageYouLeftParty                         = 200
	SystemMessageS1WasExpelledFromParty               = 201
	SystemMessageHaveBeenExpelledFromParty            = 202
	SystemMessagePartyDispersed                       = 203
	SystemMessageS1HasBecomeAPartyLeader              = 1384
	SystemMessageOnlyPartyLeaderCanTransferRights     = 1399
	SystemMessageYouCannotTransferRightsToYourself    = 1401
	SystemMessageCommandChannelConfirmFromS1          = 1529
	SystemMessageCommandChannelOnlyByLevel5ClanLeader = 1575
	SystemMessageCommandChannelFormed                 = 1580
	SystemMessageCommandChannelDisbanded              = 1581
	SystemMessageJoinedCommandChannel                 = 1582
	SystemMessageDismissedFromCommandChannel          = 1583
	SystemMessageS1PartyDismissedFromCommandChannel   = 1584
	SystemMessageCommandChannelLeaderNowS1            = 1589
	SystemMessageCannotInviteToCommandChannel         = 1593
	SystemMessageS1AlreadyMemberOfCommandChannel      = 1594
	SystemMessageS1DeclinedChannelInvitation          = 1680
	SystemMessageTargetCantFound                      = 50
)

// PartyMember is one member row of a party window packet.
type PartyMember struct {
	ObjectID  int32
	Name      string
	CP, MaxCP int32
	HP, MaxHP int32
	MP, MaxMP int32
	Level     int32
	ClassID   int32
	Race      int32
}

// PartyMemberAt is one member's position in a PartyMemberPosition packet.
type PartyMemberAt struct {
	ObjectID int32
	X, Y, Z  int32
}

// PartyChannelMember is one member row of an ExMPCCShowPartyMemberInfo
// packet.
type PartyChannelMember struct {
	Name     string
	ObjectID int32
	ClassID  int32
}

// FrameAskJoinParty asks the receiver to join requester's party under the
// loot rule.
func FrameAskJoinParty(requester string, loot int32) wire.Frame {
	w := newFrameWriter(OpcodeAskJoinParty)
	w.WriteString(requester)
	w.WriteInt32(loot)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameJoinParty tells an inviter the invited player's answer.
func FrameJoinParty(response int32) wire.Frame {
	w := newFrameWriter(OpcodeJoinParty)
	w.WriteInt32(response)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePartySmallWindowAll fills the receiver's party window with every
// other member.
func FramePartySmallWindowAll(leaderID, loot int32, others []PartyMember) wire.Frame {
	w := newFrameWriter(OpcodePartySmallWindowAll)
	w.WriteInt32(leaderID)
	w.WriteInt32(loot)
	w.WriteInt32(int32(len(others)))
	for _, m := range others {
		writePartyMemberVitals(w, m)
		w.WriteInt32(0)
		w.WriteInt32(m.Race)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePartySmallWindowAdd adds one member to the receiver's party window.
func FramePartySmallWindowAdd(leaderID, loot int32, m PartyMember) wire.Frame {
	w := newFrameWriter(OpcodePartySmallWindowAdd)
	w.WriteInt32(leaderID)
	w.WriteInt32(loot)
	writePartyMemberVitals(w, m)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePartySmallWindowDelete takes one member off the receiver's party
// window.
func FramePartySmallWindowDelete(objectID int32, name string) wire.Frame {
	w := newFrameWriter(OpcodePartySmallWindowDelete)
	w.WriteInt32(objectID)
	w.WriteString(name)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePartySmallWindowDeleteAll clears the receiver's party window.
func FramePartySmallWindowDeleteAll() wire.Frame {
	w := newFrameWriter(OpcodePartySmallWindowDeleteAll)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePartySmallWindowUpdate refreshes one member's row in the receiver's
// party window.
func FramePartySmallWindowUpdate(m PartyMember) wire.Frame {
	w := newFrameWriter(OpcodePartySmallWindowUpdate)
	writePartyMemberVitals(w, m)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func writePartyMemberVitals(w *wire.Writer, m PartyMember) {
	w.WriteInt32(m.ObjectID)
	w.WriteString(m.Name)
	w.WriteInt32(m.CP)
	w.WriteInt32(m.MaxCP)
	w.WriteInt32(m.HP)
	w.WriteInt32(m.MaxHP)
	w.WriteInt32(m.MP)
	w.WriteInt32(m.MaxMP)
	w.WriteInt32(m.Level)
	w.WriteInt32(m.ClassID)
}

// FramePartyMemberPosition carries every party member's position.
func FramePartyMemberPosition(members []PartyMemberAt) wire.Frame {
	w := newFrameWriter(OpcodePartyMemberPosition)
	w.WriteInt32(int32(len(members)))
	for _, m := range members {
		w.WriteInt32(m.ObjectID)
		w.WriteInt32(m.X)
		w.WriteInt32(m.Y)
		w.WriteInt32(m.Z)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExAskJoinMPCC asks the receiver to bring its party into requester's
// command channel.
func FrameExAskJoinMPCC(requester string) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExAskJoinMPCC)
	w.WriteString(requester)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExOpenMPCC shows the command channel window.
func FrameExOpenMPCC() wire.Frame { return frameExtendedOnly(OpcodeExOpenMPCC) }

// FrameExCloseMPCC closes the command channel window.
func FrameExCloseMPCC() wire.Frame { return frameExtendedOnly(OpcodeExCloseMPCC) }

// FrameExMPCCPartyInfoUpdate adds (added) or removes the party leaderName
// leads on the receiver's command channel window.
func FrameExMPCCPartyInfoUpdate(leaderName string, leaderID, members int32, added bool) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExMPCCPartyInfoUpdate)
	w.WriteString(leaderName)
	w.WriteInt32(leaderID)
	w.WriteInt32(members)
	w.WriteInt32(boolInt32(added))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameExMPCCShowPartyMemberInfo lists one party's members.
func FrameExMPCCShowPartyMemberInfo(members []PartyChannelMember) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExMPCCShowPartyMemberInfo)
	w.WriteInt32(int32(len(members)))
	for _, m := range members {
		w.WriteString(m.Name)
		w.WriteInt32(m.ObjectID)
		w.WriteInt32(m.ClassID)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
