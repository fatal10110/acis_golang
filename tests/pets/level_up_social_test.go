package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// socialActionLevelUp is the level-up animation id a pet plays when its
// level increases (PetStatus.addLevel broadcasts SocialAction(pet, 15)).
const socialActionLevelUp = 15

// petSocialActions returns the action ids of every SocialAction frame in
// frames that animates objectID, and the index of the first one (-1 if none).
func petSocialActions(t *testing.T, frames [][]byte, objectID int32) ([]int32, int) {
	t.Helper()
	var actions []int32
	first := -1
	for i, frame := range frames {
		if frame[0] != serverpackets.OpcodeSocialAction {
			continue
		}
		r := wire.NewReader(frame[1:])
		id, action := r.ReadInt32(), r.ReadInt32()
		if err := r.Err(); err != nil {
			t.Fatalf("read SocialAction: %v", err)
		}
		if id != objectID {
			continue
		}
		if first < 0 {
			first = i
		}
		actions = append(actions, action)
	}
	return actions, first
}

// TestPetLevelUpBroadcastsSocialAction drives a kill-reward level-up: the
// owner and a nearby player each see the pet play action 15 once. For the
// owner it follows the PetInfo refresh the level change sends and precedes
// the exp-earned message, the order the reference's addExp -> addLevel ->
// addExpAndSp chain produces.
func TestPetLevelUpBroadcastsSocialAction(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	actor, _ := h.spawnWolf(t)

	h.srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	observer := h.srv.DialClient(t, "player2", 1)
	startInWorld(t, observer)
	drainUntilQuiet(t, observer)
	drainUntilQuiet(t, h.client)

	actor.AddExpAndSp(wolfNextLevelExp, 0)
	if actor.Level() != wolfNextLevel {
		t.Fatalf("pet level after exp = %d, want %d", actor.Level(), wolfNextLevel)
	}

	frames := drainFrames(t, h.client)
	actions, socialAt := petSocialActions(t, frames, actor.ObjectID())
	if len(actions) != 1 || actions[0] != socialActionLevelUp {
		t.Fatalf("owner pet SocialAction ids = %v, want exactly [%d]; opcodes %x", actions, socialActionLevelUp, frameOpcodes(frames))
	}
	petInfoAt, earnedAt := -1, -1
	for i, frame := range frames {
		switch {
		case frame[0] == serverpackets.OpcodePetInfo && petInfoAt < 0:
			petInfoAt = i
		case frame[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, frame) == serverpackets.SystemMessagePetEarnedS1Exp:
			earnedAt = i
		}
	}
	if petInfoAt < 0 || earnedAt < 0 || petInfoAt > socialAt || socialAt > earnedAt {
		t.Fatalf("owner level-up order: PetInfo@%d SocialAction@%d PET_EARNED_S1_EXP@%d, want PetInfo < SocialAction < exp message; opcodes %x",
			petInfoAt, socialAt, earnedAt, frameOpcodes(frames))
	}

	observed := drainFrames(t, observer)
	if actions, _ := petSocialActions(t, observed, actor.ObjectID()); len(actions) != 1 || actions[0] != socialActionLevelUp {
		t.Fatalf("observer pet SocialAction ids = %v, want exactly [%d]; opcodes %x", actions, socialActionLevelUp, frameOpcodes(observed))
	}
}

// TestPetExpWithoutLevelUpSendsOnlyTheEarnedMessage grants exp that stays
// inside the current level. Pet.addExpAndSp -> PetStatus.addExpAndSp ->
// PlayableStatus.addExpAndSp (PlayableStatus.java:119-130) binds to
// addExp(long) (:70-93), not PetStatus.addExp(int), whose
// updateAndBroadcastStatus never runs here; addSp only sets the value. So
// the owner is told the exp earned and nothing else: no PetStatusUpdate and
// no animation, and a watching player gets no NpcInfo.
func TestPetExpWithoutLevelUpSendsOnlyTheEarnedMessage(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	actor, _ := h.spawnWolf(t)
	watcher := h.joinSecondPlayer(t, "Watcher")
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, watcher.client)

	runOn(t, actor.Queue(), func() { actor.AddExpAndSp(100, 10) })
	if actor.Level() != wolfLevel {
		t.Fatalf("pet level after small exp = %d, want unchanged %d", actor.Level(), wolfLevel)
	}

	frames := drainFrames(t, h.client)
	if len(frames) != 1 || findSystemMessage(t, frames, serverpackets.SystemMessagePetEarnedS1Exp) == nil {
		t.Fatalf("owner frames after a plain exp grant = opcodes %x, want PET_EARNED_S1_EXP alone", frameOpcodes(frames))
	}
	if n := countNPCInfoFor(drainFrames(t, watcher.client), actor.ObjectID()); n != 0 {
		t.Fatalf("watcher got %d pet NpcInfo after a plain exp grant, want none", n)
	}
}
