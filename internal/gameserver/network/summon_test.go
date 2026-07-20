package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestGameClientLinkRoutesSummonActionUseToLiveSummon(t *testing.T) {
	c, chars, _, state := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	ownerObject, ok := state.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	owner, ok := ownerObject.(summon.Owner)
	if !ok {
		t.Fatalf("world.Player(%d) does not satisfy summon.Owner", objID)
	}
	liveSummon := summon.NewServitor(summon.ServitorConfig{ObjectID: 500, Owner: owner, Level: 40})
	summon.SpawnBesideOwner(state, liveSummon, owner, location.Location{})

	c.send(encodeRequestActionUse(52, false, false))
	reply := c.read()
	if reply[0] != serverpackets.OpcodePetDelete {
		t.Fatalf("post-action opcode = %#x, want PetDelete (%#x)", reply[0], serverpackets.OpcodePetDelete)
	}

	if _, ok := state.Summon(objID); ok {
		t.Fatalf("owner %d still has active summon after action 52", objID)
	}
	if _, ok := state.Object(liveSummon.ObjectID()); ok {
		t.Fatalf("summon object %d still exists after action 52", liveSummon.ObjectID())
	}
}

// TestGameClientLinkSummonActionUseWithNoActiveSummonAnswersActionFailed is
// the regression test for a pet-command shortcut (attack/follow/stop/
// return) pressed with no summon out: handleSummonActionUse recognized the
// action id and claimed the request as handled, but sent nothing back,
// silently swallowing the ActionFailed fallback the dispatch loop would
// otherwise have sent for an unclaimed action-bar command.
func TestGameClientLinkSummonActionUseWithNoActiveSummonAnswersActionFailed(t *testing.T) {
	c, chars, _, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	chars.soleObjectID(t)
	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	// Action id 16 is the pet-attack shortcut; no summon has been spawned.
	c.send(encodeRequestActionUse(16, false, false))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("pet-command opcode with no active summon = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
}
