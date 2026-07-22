package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Compile-time check that playerClockEffects satisfies task.PlayerClockEffects.
var _ task.PlayerClockEffects = (*playerClockEffects)(nil)

// playerClockEffects routes task.PlayerClock's per-player SystemMessage
// sends to the in-world player session identified by actor id, looking the
// player up through world.State and writing the frame via livePlayer.SendFrame.
//
// The player-clock task never sees the session directly; it routes through
// actor ids and this adapter, so task stays free of network-package types
// (and of the converse cycle that would otherwise form).
type playerClockEffects struct {
	state *world.State
}

// NewPlayerClockEffects returns a player-clock effects adapter that
// delivers SystemMessage frames to whatever in-world session is registered
// for the given actor id. Missing or already-detached sessions are no-ops.
func NewPlayerClockEffects(state *world.State) *playerClockEffects {
	return &playerClockEffects{state: state}
}

// NotifyPlayingTooLong sends the no-parameter PLAYING_FOR_LONG_TIME message
// to the named actor's session.
func (e *playerClockEffects) NotifyPlayingTooLong(actorID int32) {
	e.deliver(actorID, serverpackets.FrameSystemMessage(serverpackets.SystemMessagePlayingForLongTime))
}

// NotifyDayNightSkillTransition sends the skill-name SystemMessage that
// announces Shadow Sense has either come into effect (night has just
// fallen) or faded (day has just broken), naming skillID at level.
func (e *playerClockEffects) NotifyDayNightSkillTransition(actorID int32, night bool, skillID, level int32) {
	messageID := serverpackets.SystemMessageDaySkillEffectDisappears
	if night {
		messageID = serverpackets.SystemMessageNightSkillEffectApplies
	}
	e.deliver(actorID, serverpackets.FrameSystemMessageSkillName(messageID, skillID, level))
}

// deliver writes frame to actorID's currently registered in-world session.
// A lookup that misses (the player has logged out between PlayerClock's
// decision to notify and the session lookup here) silently drops the frame.
func (e *playerClockEffects) deliver(actorID int32, frame wire.Frame) {
	if e.state == nil {
		return
	}
	obj, ok := e.state.Player(actorID)
	if !ok {
		return
	}
	if live, ok := obj.(*livePlayer); ok {
		live.SendFrame(frame)
	}
}
