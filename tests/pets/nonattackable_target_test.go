package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// commandPetOnto has the owner select the second player and command the pet
// to attack it with force, then advances until the pet's first swing.
//
// Every pet swing misses, so the victim outlives any number of them and only
// the keep-attacking rule decides whether the pet swings again.
func commandPetOnto(t *testing.T, h *petWorld, petActor *summon.Actor, victim secondPlayer) {
	t.Helper()
	setSummonRoll(t, h.srv, h.ownerID, petActor, func(n int) int { return n - 1 })
	x, y, z := victim.actor.Position()
	h.client.Send(encodeAction(victim.id, int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeRequestActionUse(petAttackAction, true))
	h.srv.AdvanceUntil(t, "the pet's first swing at the player", petActor.IsAttackingNow)
}

// TestPetForcedOntoUnflaggedPlayerSwingsOnce commands a pet onto an
// unflagged player outside any PvP zone. After its one swing it goes idle
// and follows its owner again; it never swings a second time.
func TestPetForcedOntoUnflaggedPlayerSwingsOnce(t *testing.T) {
	t.Parallel()
	h, petActor := bootFollowingPet(t)
	victim := h.joinSecondPlayer(t, "White")
	commandPetOnto(t, h, petActor, victim)

	frames := advanceCollecting(t, h, 4*time.Second)
	if n := petAttacks(frames, petActor); n != 1 {
		t.Fatalf("pet swings at the unflagged player = %d, want 1", n)
	}
	if got := petActor.Intent(); got != summon.IntentFollowOwner {
		t.Fatalf("pet intent after its swing = %v, want following its owner", got)
	}
	if victim.actor.AlikeDead() {
		t.Fatal("the unflagged player died: the swing count proves nothing")
	}
}

// TestPetForcedOntoKarmaPlayerKeepsSwinging commands a pet onto a player with
// karma: the pet can keep attacking it and swings again after its first.
func TestPetForcedOntoKarmaPlayerKeepsSwinging(t *testing.T) {
	t.Parallel()
	h, petActor := bootFollowingPet(t)
	victim := h.joinKarmaPlayer(t, "Pk", 500)
	commandPetOnto(t, h, petActor, victim)

	frames := advanceCollecting(t, h, 4*time.Second)
	if n := petAttacks(frames, petActor); n < 2 {
		t.Fatalf("pet swings at the karma player = %d, want it to keep attacking", n)
	}
	if got := petActor.Intent(); got != summon.IntentAttackTarget {
		t.Fatalf("pet intent = %v, want still attacking", got)
	}
}

// TestPlainClickOnAnotherPlayersPetFollowsIt has a second player click an
// unflagged owner's pet twice without force. The second click is answered
// ActionFailed and attacks nothing; once the owner walks off and the pet
// follows it, the clicking player walks after the pet with MoveToPawn at
// the 70-unit follow offset.
func TestPlainClickOnAnotherPlayersPetFollowsIt(t *testing.T) {
	t.Parallel()
	h, petActor := bootFollowingPet(t)
	other := h.joinSecondPlayer(t, "Follower")
	x, y, z := petActor.Position()
	other.client.Send(encodeAction(petActor.ObjectID(), int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, other.client)
	full := petActor.HP()

	other.client.Send(encodeAction(petActor.ObjectID(), int32(x), int32(y), int32(z), false))
	assertFrameOpcode(t, mustRead(t, other.client, "follow ActionFailed"), serverpackets.OpcodeActionFailed, "follow ActionFailed")

	ox, oy, oz := h.srv.PlayerPosition(t, h.ownerID)
	h.client.Send(encodeMoveBackwardToLocation(int32(ox+600), int32(oy), int32(oz)))
	var frames [][]byte
	for range 8 {
		// The follow task rechecks on the movement-correction ticks; the
		// pet follows its owner on the AI think.
		for range time.Second / move.PositionUpdateInterval {
			h.srv.Advance(t, move.PositionUpdateInterval)
			h.srv.TickPositions()
		}
		if err := h.srv.AI.Tick(); err != nil {
			t.Fatalf("AI.Tick() = %v", err)
		}
		h.srv.Settle(t)
		frames = append(frames, drainFrames(t, other.client)...)
	}
	if petActor.HP() != full {
		t.Fatal("plain click on an unflagged player's pet attacked it")
	}
	pawn := -1
	for i, f := range frames {
		if f[0] == serverpackets.OpcodeAttack && wire.NewReader(f[1:]).ReadInt32() == other.id {
			t.Fatalf("follower swung: opcodes %x", frameOpcodes(frames))
		}
		if f[0] != serverpackets.OpcodeMoveToPawn {
			continue
		}
		r := wire.NewReader(f[1:])
		if mover, target, offset := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); mover == other.id && target == petActor.ObjectID() && offset == 70 && pawn < 0 {
			pawn = i
		}
	}
	if pawn < 0 {
		t.Fatalf("follower never walked after the pet: opcodes %x", frameOpcodes(frames))
	}
	fx, _, _ := h.srv.PlayerPosition(t, other.id)
	px, _, _ := petActor.Position()
	if px-fx > 200 {
		t.Fatalf("follower at x=%d, pet at x=%d: want the follower close behind", fx, px)
	}
}

// joinKarmaPlayer is joinSecondPlayer for a level-40 character carrying
// karma, sturdy enough to outlast a few pet swings.
func (h *petWorld) joinKarmaPlayer(t *testing.T, name string, karma int) secondPlayer {
	t.Helper()
	id := h.srv.SeedCharacterFor(t, "player2", name, 40, 0).ID
	ch, err := h.srv.Chars.Get(petCtx(), id)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	ch.KarmaPoints = karma
	if err := h.srv.Chars.Save(petCtx(), ch.SaveState()); err != nil {
		t.Fatalf("save %s: %v", name, err)
	}
	c := h.srv.DialClient(t, "player2", 1)
	startInWorld(t, c)
	obj, ok := h.srv.State.Player(id)
	if !ok {
		t.Fatalf("%s not in world", name)
	}
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, c)
	return secondPlayer{client: c, id: id, actor: obj.(attackable.Combatant), queue: h.srv.PlayerQueue(t, id)}
}
