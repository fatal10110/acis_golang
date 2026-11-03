package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// handleExtended serves an extended frame: the uint16 sub-opcode after the
// opcode byte picks its handler in gameExInbound.
func (l *GameClientLink) handleExtended(ctx context.Context, cs *connState, payload []byte) error {
	client := cs.client
	r := wire.NewReader(payload[1:])
	second := r.ReadUint16()
	if r.Err() != nil {
		l.log.Warn().Str("state", client.State().String()).Msg("game client: extended opcode missing")
		cs.acted = false
		return nil
	}
	// While entering, only the manor-list sub-opcode is dispatched;
	// every other one counts toward the unknown-packet disconnect
	// threshold instead of being absorbed by an in-game no-op.
	if client.State() == StateEntering && second != clientpackets.OpcodeRequestManorList {
		l.log.Info().
			Uint16("opcode2", second).
			Str("state", client.State().String()).
			Msg("game client: accepted extended opcode not handled while entering")
		cs.acted = false
		if client.countUnknownPacket() {
			client.closeNow()
			return errDisconnect
		}
		return nil
	}
	if handle, ok := gameExInbound[second]; ok {
		return handle(l, ctx, cs, payload)
	}
	l.log.Info().
		Uint16("opcode2", second).
		Str("state", client.State().String()).
		Msg("game client: accepted extended opcode not implemented yet")
	// OpcodeExtended is only ever allowed in StateEntering/StateInGame
	// (see allowedOpcodes), so unlike the top-level Accept gate there is
	// no pre-auth immediate-disconnect case here; just count toward the
	// same sliding-60s threshold as an unknown top-level opcode.
	// An unknown packet never runs, so it is no action.
	cs.acted = false
	if client.countUnknownPacket() {
		client.closeNow()
		return errDisconnect
	}
	return nil
}

// pledgeMemberNameDecoder decodes the member-name request sub-opcode sub
// carries.
func pledgeMemberNameDecoder(sub uint16) func([]byte) (clientpackets.RequestPledgeMemberName, error) {
	return func(p []byte) (clientpackets.RequestPledgeMemberName, error) {
		return clientpackets.DecodeRequestPledgeMemberName(p, sub)
	}
}

// handleRequestManorList sends the manor list.
func (l *GameClientLink) handleRequestManorList(_ context.Context, cs *connState, _ struct{}) error {
	session := cs.client.Session
	session.SendFrame(serverpackets.FrameExSendManorList())
	return nil
}

// handleRequestSetSeed sets a castle's seed production for the next period.
//
// A refused list answers nothing, as specified: the
// setting window's submit leaves no client action
// pending.
func (l *GameClientLink) handleRequestSetSeed(_ context.Context, cs *connState, req clientpackets.RequestSetSeed) error {
	if cs.live != nil {
		onLive(cs.live, func() { l.requestSetSeed(cs.live, req) })
	}
	return nil
}

// handleRequestExPledgeCrestLarge sends a large clan crest.
func (l *GameClientLink) handleRequestExPledgeCrestLarge(_ context.Context, cs *connState, req clientpackets.RequestExPledgeCrestLarge) error {
	session := cs.client.Session
	if cs.live == nil {
		return nil
	}
	if frame, ok := l.frameExPledgeCrestLarge(req); ok {
		session.SendFrame(frame)
	}
	return nil
}

// handleRequestGetBossRecord sends the player's raid boss points.
func (l *GameClientLink) handleRequestGetBossRecord(_ context.Context, cs *connState, _ clientpackets.RequestGetBossRecord) error {
	if cs.live != nil {
		onLive(cs.live, func() { l.sendBossRecord(cs.live) })
	}
	return nil
}

// handleRequestCursedWeaponList sends the cursed weapon ids.
func (l *GameClientLink) handleRequestCursedWeaponList(_ context.Context, cs *connState, _ struct{}) error {
	session := cs.client.Session
	if cs.live == nil {
		return nil
	}
	session.SendFrame(serverpackets.FrameExCursedWeaponList(l.cursedWeapons.IDs()))
	return nil
}

// handleRequestDuelSurrender surrenders the player's duel. The request
// carries no body. A player in no duel surrenders nothing and hears
// nothing, as the menu action waits on no answer; the duel's end answers a
// surrender.
func (l *GameClientLink) handleRequestDuelSurrender(_ context.Context, cs *connState, _ struct{}) error {
	if cs.live != nil && l.duels != nil {
		onLive(cs.live, func() { l.duels.Surrender(cs.live) })
	}
	return nil
}

// handleRequestExitPartyMatchingWaitingRoom takes the player off the
// party-matching waiting list. The request carries no body; leaving the
// waiting list answers nothing, the client having closed its window
// already.
func (l *GameClientLink) handleRequestExitPartyMatchingWaitingRoom(_ context.Context, cs *connState, _ struct{}) error {
	if cs.live != nil {
		onLive(cs.live, func() { l.rooms.RemoveWaiting(cs.live) })
	}
	return nil
}
