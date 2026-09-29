package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// bootWolfStrikerWithBystander is bootWolfStriker with a second, unflagged
// player in the world and a hand-driven AI cycle. The owner still has the
// monster targeted.
func bootWolfStrikerWithBystander(t *testing.T, strike modelskill.Definition) (*petWorld, *summon.Actor, int32) {
	t.Helper()
	h, petActor, _ := bootWolfStrikerWith(t, strike, gameservertest.WithAITask())
	bystanderID := h.srv.SeedCharacterFor(t, "player2", "Bystander", 5, 0).ID
	bystander := h.srv.DialClient(t, "player2", 1)
	startInWorld(t, bystander)
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, bystander)
	return h, petActor, bystanderID
}

// targetPlayer makes the owner select the player objectID.
func (h *petWorld) targetPlayer(t *testing.T, objectID int32) {
	t.Helper()
	x, y, z := h.srv.PlayerPosition(t, objectID)
	h.client.Send(encodeAction(objectID, int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, h.client)
}

// requireSummonStrikeRefused finds INVALID_TARGET in frames, directly
// followed by the pet's MoveToPawn toward targetID, and no MagicSkillUse.
func requireSummonStrikeRefused(t *testing.T, frames [][]byte, petActor *summon.Actor, targetID int32) {
	t.Helper()
	if _, ok := firstOpcode(frames, serverpackets.OpcodeMagicSkillUse); ok {
		t.Fatalf("refused strike started a cast: opcodes %x", frameOpcodes(frames))
	}
	for i, frame := range frames {
		if frame[0] != serverpackets.OpcodeSystemMessage || systemMessageID(t, frame) != serverpackets.SystemMessageInvalidTarget {
			continue
		}
		assertStaticSystemMessage(t, frame, serverpackets.SystemMessageInvalidTarget)
		if i+1 >= len(frames) {
			t.Fatalf("INVALID_TARGET was the last frame, want the pet's MoveToPawn after it: opcodes %x", frameOpcodes(frames))
		}
		assertFrameOpcode(t, frames[i+1], serverpackets.OpcodeMoveToPawn, "pet MoveToPawn")
		r := wire.NewReader(frames[i+1][1:])
		if mover, target := r.ReadInt32(), r.ReadInt32(); mover != petActor.ObjectID() || target != targetID {
			t.Fatalf("MoveToPawn = %d toward %d, want pet %d toward %d", mover, target, petActor.ObjectID(), targetID)
		}
		return
	}
	t.Fatalf("no INVALID_TARGET for the refused strike: opcodes %x", frameOpcodes(frames))
}

// requireSummonStrikeStarted finds the pet's MagicSkillUse onto targetID in
// frames, with no INVALID_TARGET.
func requireSummonStrikeStarted(t *testing.T, frames [][]byte, petActor *summon.Actor, targetID int32) {
	t.Helper()
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, frame) == serverpackets.SystemMessageInvalidTarget {
			t.Fatalf("forced strike was refused with INVALID_TARGET: opcodes %x", frameOpcodes(frames))
		}
	}
	use, ok := firstOpcode(frames, serverpackets.OpcodeMagicSkillUse)
	if !ok {
		t.Fatalf("forced strike sent no MagicSkillUse: opcodes %x", frameOpcodes(frames))
	}
	r := wire.NewReader(use[1:])
	if caster, target, skill := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != petActor.ObjectID() || target != targetID || skill != wolfStrikeSkill {
		t.Fatalf("MagicSkillUse = caster %d target %d skill %d, want pet %d onto %d with %d",
			caster, target, skill, petActor.ObjectID(), targetID, wolfStrikeSkill)
	}
}

// TestSummonStrikeOnUnflaggedPlayerNeedsCtrl commands the pet's offensive
// single-target strike at an unflagged player. Without CTRL the owner reads
// INVALID_TARGET and the pet turns toward the player; with CTRL, the forced
// attack, the cast starts.
func TestSummonStrikeOnUnflaggedPlayerNeedsCtrl(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		ctrl bool
	}{
		{"without ctrl", false},
		{"with ctrl", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, petActor, bystanderID := bootWolfStrikerWithBystander(t, wolfStrike())
			h.targetPlayer(t, bystanderID)

			h.client.Send(encodeRequestActionUse(wolfStrikeAction, tc.ctrl))
			frames := readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "strike ActionFailed")
			if tc.ctrl {
				requireSummonStrikeStarted(t, frames, petActor, bystanderID)
			} else {
				requireSummonStrikeRefused(t, frames, petActor, bystanderID)
			}
			drainUntilQuiet(t, h.client)
		})
	}
}

// TestQueuedSummonStrikeKeepsCtrl commands the strike at an unflagged player
// while the pet is still casting at the monster, so the command is queued as
// the pet's next intention. When the first cast is over and the queued one
// runs, it is judged with the CTRL flag the command carried.
func TestQueuedSummonStrikeKeepsCtrl(t *testing.T) {
	t.Parallel()
	// No reuse, so the queued strike can be the same skill, and a hit time
	// long enough that the second command lands while the first is casting.
	const hitTime = 3000
	strike := wolfStrike()
	strike.HitTime = hitTime
	strike.ReuseDelay = 0

	for _, tc := range []struct {
		name string
		ctrl bool
	}{
		{"without ctrl", false},
		{"with ctrl", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, petActor, bystanderID := bootWolfStrikerWithBystander(t, strike)
			h.client.Send(encodeRequestActionUse(wolfStrikeAction, false))
			first := readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "first strike ActionFailed")
			if _, ok := firstOpcode(first, serverpackets.OpcodeMagicSkillUse); !ok {
				t.Fatalf("first strike sent no MagicSkillUse: opcodes %x", frameOpcodes(first))
			}
			h.targetPlayer(t, bystanderID)

			h.client.Send(encodeRequestActionUse(wolfStrikeAction, tc.ctrl))
			queued := readUntilOpcode(t, h.client, serverpackets.OpcodeActionFailed, "queued strike ActionFailed")
			if _, ok := firstOpcode(queued, serverpackets.OpcodeMagicSkillUse); ok {
				t.Fatalf("strike commanded mid-cast started at once: opcodes %x", frameOpcodes(queued))
			}
			if _, ok := firstOpcode(queued, serverpackets.OpcodeSystemMessage); ok {
				t.Fatalf("strike commanded mid-cast was judged at once: opcodes %x", frameOpcodes(queued))
			}

			// The first cast ends, then the AI's next think runs the queued one.
			h.srv.Advance(t, hitTime*time.Millisecond)
			drainFrames(t, h.client)
			h.think(t)
			frames := drainFrames(t, h.client)
			if tc.ctrl {
				requireSummonStrikeStarted(t, frames, petActor, bystanderID)
			} else {
				requireSummonStrikeRefused(t, frames, petActor, bystanderID)
			}
		})
	}
}
