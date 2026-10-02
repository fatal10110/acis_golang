package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// followPlayer clicks the player objectID the owner already has selected:
// one the owner may not attack without force is followed, so the owner's
// current intention acts on it.
func (h *petWorld) followPlayer(t *testing.T, objectID int32) {
	t.Helper()
	h.targetPlayer(t, objectID)
	h.srv.Settle(t)
	drainUntilQuiet(t, h.client)
}

func encodeJoinParty(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	w.WriteString(name)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeAnswerJoinParty(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinParty)
	w.WriteInt32(response)
	return w.Bytes()
}

// TestSummonCtrlStrikeNeedsOwnersMainTarget commands the wolf's damage
// strike with CTRL at an unflagged player and at a party member. The
// summon's single-target cast is judged as its owner's
// (TargetOne.meetCastConditions runs canCastOffensiveSkillOnPlayable on
// caster.getActingPlayer(), TargetOne.java:52-58), and a CTRL damage skill
// passes on either only when it aims at the main target, the final target
// of the owner's own current intention (Playable.java:405-412, 443): a
// player the owner merely selected is refused, one the owner follows is
// struck.
func TestSummonCtrlStrikeNeedsOwnersMainTarget(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		party   bool
		follow  bool
		started bool
	}{
		{"unflagged player the owner only selected", false, false, false},
		{"unflagged player the owner follows", false, true, true},
		{"party member the owner only selected", true, false, false},
		{"party member the owner follows", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, petActor, _ := bootWolfStrikerWith(t, wolfStrike(), gameservertest.WithAITask())
			playerID := h.srv.SeedCharacterFor(t, "player2", "Bystander", 5, 0).ID
			other := h.srv.DialClient(t, "player2", 1)
			startInWorld(t, other)
			drainUntilQuiet(t, h.client)
			drainUntilQuiet(t, other)
			if tc.party {
				h.client.Send(encodeJoinParty("Bystander"))
				drainUntilQuiet(t, h.client)
				other.Send(encodeAnswerJoinParty(1))
				drainUntilQuiet(t, other)
				drainUntilQuiet(t, h.client)
			}
			h.targetPlayer(t, playerID)
			if tc.follow {
				h.followPlayer(t, playerID)
			}

			h.client.Send(encodeRequestActionUse(wolfStrikeAction, true))
			frames := readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "strike ActionFailed")
			if tc.started {
				requireSummonStrikeStarted(t, frames, petActor, playerID)
			} else {
				requireSummonStrikeRefused(t, frames, petActor, playerID)
			}
			drainUntilQuiet(t, h.client)
		})
	}
}
