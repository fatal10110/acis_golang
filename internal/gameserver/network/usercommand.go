package network

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// User command ids, as the client numbers the slash commands it resolves.
const (
	userCommandLoc               = 0
	userCommandEscape            = 52
	userCommandMount             = 61
	userCommandDismount          = 62
	userCommandTime              = 77
	userCommandPartyInfo         = 81
	userCommandAttackList        = 88
	userCommandUnderAttackList   = 89
	userCommandWarList           = 90
	userCommandChannelDelete     = 93
	userCommandChannelLeave      = 96
	userCommandChannelListUpdate = 97
	userCommandSiegeStatus       = 99
	userCommandClanPenalty       = 100
	userCommandOlympiadStat      = 109
)

// Escape's casts: five minutes for a player, one second for a GM.
var (
	escapeSkillRef   = modelskill.Ref{ID: 2099, Level: 1}
	escapeGMSkillRef = modelskill.Ref{ID: 2100, Level: 1}
)

// escapeSound is the voice line /unstuck plays before its long cast.
const escapeSound = "systemmsg_e.809"

// userCommands maps each user command id to its handler.
var userCommands = map[int32]func(l *GameClientLink, live *livePlayer, id int32){
	userCommandLoc:               (*GameClientLink).userCommandLoc,
	userCommandEscape:            (*GameClientLink).userCommandEscape,
	userCommandMount:             (*GameClientLink).userCommandMount,
	userCommandDismount:          (*GameClientLink).userCommandDismount,
	userCommandTime:              (*GameClientLink).userCommandTime,
	userCommandPartyInfo:         (*GameClientLink).userCommandPartyInfo,
	userCommandAttackList:        (*GameClientLink).userCommandClanWarsList,
	userCommandUnderAttackList:   (*GameClientLink).userCommandClanWarsList,
	userCommandWarList:           (*GameClientLink).userCommandClanWarsList,
	userCommandChannelDelete:     (*GameClientLink).userCommandChannelDelete,
	userCommandChannelLeave:      (*GameClientLink).userCommandChannelLeave,
	userCommandChannelListUpdate: (*GameClientLink).userCommandChannelListUpdate,
	userCommandSiegeStatus:       (*GameClientLink).userCommandSiegeStatus,
	userCommandClanPenalty:       (*GameClientLink).userCommandClanPenalty,
	userCommandOlympiadStat:      (*GameClientLink).userCommandOlympiadStat,
}

// requestUserCommand runs user command id for live.
//
// A command id no handler claims is ignored, as specified, and so are the
// commands' own silent branches documented below: a user command leaves no
// client action pending, so silence locks nothing.
func (l *GameClientLink) requestUserCommand(live *livePlayer, id int32) {
	if handle, ok := userCommands[id]; ok {
		handle(l, live, id)
	}
}

// userCommandGap answers a user command whose system is not ported yet: the
// gap is logged and the client released.
func (l *GameClientLink) userCommandGap(live *livePlayer, id int32, issue string) {
	l.log.Warn().Int32("command_id", id).Int32("object_id", live.ObjectID()).Str("issue", issue).
		Msg("game client: user command not implemented yet")
	live.SendFrame(serverpackets.FrameActionFailed())
}

// userCommandLoc (/loc) tells live where it stands, in the message naming
// the region its restart point covers. With no restart point resolved
// nothing is said.
func (l *GameClientLink) userCommandLoc(live *livePlayer, _ int32) {
	if l.restarts == nil {
		return
	}
	at := live.CurrentLocation()
	point, ok := l.restarts.CalculatedPoint(at, live.Race)
	if !ok {
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessageParams(point.LocName,
		serverpackets.NumberParam(int32(at.X)), serverpackets.NumberParam(int32(at.Y)), serverpackets.NumberParam(int32(at.Z))))
}

// userCommandEscape (/unstuck) casts live's way back to town: five minutes
// for a player, after its voice line and notice, one second for a GM. A
// player at an Olympiad match, observing, in the festival, jailed or in a
// boss zone is told to petition instead. No one is jailed until the
// punishment state exists (#3155), and the Escape skills' recall at the
// cast's end has no handler yet (#3213).
func (l *GameClientLink) userCommandEscape(live *livePlayer, _ int32) {
	inBossZone := live.zoneActor != nil && live.zoneActor.ZoneFlags().Has(zone.FlagBoss)
	if live.OlympiadMode() || live.ObserverMode() || live.FestivalParticipant() || inBossZone {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoUnstuckPleaseSendPetition))
		return
	}
	ref := escapeGMSkillRef
	if !live.accessLevel().IsGM {
		ref = escapeSkillRef
		live.SendFrame(serverpackets.FramePlaySound(escapeSound))
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageStuckTransportInFiveMinutes))
	}
	def, ok := l.skills.Definition(ref)
	if !ok {
		return
	}
	l.castItemSkills(live, live.Inventory(), nil, []modelskill.Definition{def}, false, false)
}

// userCommandMount (/mount) mounts live on its strider pet, or takes it off
// its mount. Neither on a mount nor with a strider to ride, nothing
// happens.
func (l *GameClientLink) userCommandMount(live *livePlayer, id int32) {
	if l.hasMountablePet(live) && !live.Mounted() && !live.EffectList().IsAffected(effect.FlagBetrayed) {
		l.userCommandGap(live, id, "strider mount (#3210)")
		return
	}
	if !live.Mounted() {
		return
	}
	if live.MountType() == player.MountTypeWyvern {
		if live.zoneActor != nil && live.zoneActor.ZoneFlags().Has(zone.FlagNoLanding) {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoDismountHere))
			return
		}
		if l.unsafeDismountHeight(live) {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotDismountFromElevation))
			return
		}
	}
	if live.MountHungry() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageHungryStriderNotMount))
		return
	}
	live.Character.Dismount()
}

// hasMountablePet reports whether live's summon is a pet it can ride.
func (l *GameClientLink) hasMountablePet(live *livePlayer) bool {
	if l.world == nil {
		return false
	}
	obj, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return false
	}
	s, ok := obj.(*summon.Actor)
	return ok && s.IsPet() && pet.IsMountable(s.NPCID())
}

// unsafeDismountHeight reports whether live hangs higher above the ground
// than its class may fall unhurt.
func (l *GameClientLink) unsafeDismountHeight(live *livePlayer) bool {
	if l.geo == nil {
		return false
	}
	at := live.CurrentLocation()
	safe := live.Template().SafeFallHeightFemale
	if live.Sex == player.SexMale {
		safe = live.Template().SafeFallHeightMale
	}
	drop := at.Z - int(l.geo.Height(at.X, at.Y, at.Z))
	return max(drop, -drop) > safe
}

// userCommandDismount (/dismount) takes live off its mount at once, with
// none of /mount's checks. Off a mount nothing happens.
func (l *GameClientLink) userCommandDismount(live *livePlayer, _ int32) {
	if live.Mounted() {
		live.Character.Dismount()
	}
}

// userCommandTime (/time) tells live the in-game time, day or night.
func (l *GameClientLink) userCommandTime(live *livePlayer, _ int32) {
	if l.gameClock == nil {
		return
	}
	id := serverpackets.SystemMessageTimeS1S2InTheDay
	if l.gameClock.IsNight() {
		id = serverpackets.SystemMessageTimeS1S2InTheNight
	}
	live.SendFrame(serverpackets.FrameSystemMessageParams(id,
		serverpackets.NumberParam(int32(l.gameClock.Hour())), serverpackets.TextParam(fmt.Sprintf("%02d", l.gameClock.Minute()))))
}

// userCommandPartyInfo (/partyinfo) describes live's party: its loot rule,
// its leader and its size. Out of a party nothing is said.
func (l *GameClientLink) userCommandPartyInfo(live *livePlayer, _ int32) {
	view, ok := l.parties.View(live.ObjectID())
	if !ok {
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePartyInformation))
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageLooting(int32(view.Loot))))
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessagePartyLeaderS1, view.Leader.Name))
	sendText(live, fmt.Sprintf("Members: %d/%d", len(view.Members), party.MaxMembers))
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFriendListFooter))
}

// userCommandChannelDelete (/channeldelete) disbands the command channel
// live leads. Anyone else is ignored.
func (l *GameClientLink) userCommandChannelDelete(live *livePlayer, _ int32) {
	l.applyPartyNotices(l.parties.DisbandChannel(live))
}

// userCommandChannelLeave (/channelleave) takes the party live leads out of
// its command channel. Anyone else is ignored.
func (l *GameClientLink) userCommandChannelLeave(live *livePlayer, _ int32) {
	l.applyPartyNotices(l.parties.LeaveChannel(live))
}

// userCommandChannelListUpdate (/channellistupdate) shows live the command
// channel its party is in. Out of one nothing is shown.
func (l *GameClientLink) userCommandChannelListUpdate(live *livePlayer, _ int32) {
	view, ok := l.parties.Channel(live.ObjectID())
	if !ok {
		return
	}
	parties := make([]serverpackets.ChannelParty, len(view.Parties))
	for i, p := range view.Parties {
		parties[i] = serverpackets.ChannelParty{LeaderName: p.Leader.Name, LeaderID: p.Leader.ObjectID(), Members: int32(p.Count)}
	}
	live.SendFrame(serverpackets.FrameExMultiPartyCommandChannelInfo(view.Leader.Name, int32(view.MembersCount), parties))
}

// userCommandOlympiadStat (/olympiadstat) tells a noble its Olympiad record.
// Neither noble status nor Olympiad records exist yet.
func (l *GameClientLink) userCommandOlympiadStat(live *livePlayer, id int32) {
	l.userCommandGap(live, id, "olympiad record (#3212)")
}
