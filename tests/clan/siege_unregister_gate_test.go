package clan

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// TestSiegeUnregisterRefusedOnceRegistrationsClose: Knights, attacking
// Gludio, asks to leave the siege while it runs. The request is refused
// with 845 naming Gludio, then the siege window: Knights stays an attacker,
// its row stays stored and its leader keeps the attacker state, which the
// siege's end then clears, so no headquarters or siege state outlives it.
// Leaving once the registrations closed is an owner-approved divergence
// (#5111): the clan would otherwise keep the attacker state and its
// headquarters past the end, which clears only the clans still registered.
func TestSiegeUnregisterRefusedOnceRegistrationsClose(t *testing.T) {
	t.Parallel()
	w := bootSiegeClanWorld(t, nil, []string{registerOnGludio(knightsClanID, "ATTACKER")})
	gludio := startGludio(t, w)
	if got := w.srv.PlayerSiegeState(t, w.leaderID); got != 1 {
		t.Fatalf("leader's siege state = %d, want 1", got)
	}

	w.leader.Send(encodeSiegeRequest(clientpackets.OpcodeRequestJoinSiege, 1, 1, 0))
	frames := drainFrames(t, w.leader)
	if got, want := sysMsgParams(t, frames), [][]string{{"845", "1"}}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("messages = %v, want %v", got, want)
	}
	var order []byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage || f[0] == serverpackets.OpcodeSiegeInfo {
			order = append(order, f[0])
		}
	}
	if !slices.Equal(order, []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSiegeInfo}) {
		t.Fatalf("frames %x: want the refusal, then the siege window", opcodes(frames))
	}
	if side := gludio.Side(knightsClanID); side != siege.SideAttacker {
		t.Fatalf("Knights' side after the refused drop = %v, want attacker", side)
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT COUNT(*) FROM siege_clans WHERE castle_id = 1 AND clan_id = `+itoa(knightsClanID)); got != 1 {
		t.Fatalf("stored registrations = %d, want Knights' kept", got)
	}
	if got := w.srv.PlayerSiegeState(t, w.leaderID); got != 1 {
		t.Fatalf("leader's siege state after the refused drop = %d, want 1", got)
	}

	gludio.End()
	drainFrames(t, w.leader)
	if got := w.srv.PlayerSiegeState(t, w.leaderID); got != 0 {
		t.Fatalf("leader's siege state after the end = %d, want 0", got)
	}
}
