package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const overloadFolkID = 31_888

// bootOverloaded boots a player carrying far past its weight limit (weight
// band 4, so its move speed is zero) and knowing the ONE-target skill.
func bootOverloaded(t *testing.T) (*gameservertest.Server, *scriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithWeightLimitMultiplier(1),
		gameservertest.WithSkills(combatPersistence(t, []modelskill.Definition{queueOneSkill()})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, queueOneSkillID, 1)
	srv.GiveItem(t, objID, 9500, 100_000)
	// The overload adds its status and band refresh to the world-entry
	// burst, so the burst is drained rather than matched.
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	drainUntilQuiet(t, c)
	pc := onlinePlayer(t, srv, objID)
	if pc.WeightPenalty() != 4 || pc.MoveSpeed() != 0 {
		t.Fatalf("fixture: weight penalty %d, move speed %v; want 4 and 0", pc.WeightPenalty(), pc.MoveSpeed())
	}
	return srv, c, objID
}

// assertRefusedApproach sends req and requires no approach (MoveToPawn) or
// swing, then or later; wantFailed requires the answer to carry ActionFailed,
// otherwise none may come.
func assertRefusedApproach(t *testing.T, srv *gameservertest.Server, c *scriptedClient, req []byte, wantFailed bool, what string) {
	t.Helper()
	frames := testsupport.SyncBarrierFrames(t, c, func() {
		c.Send(req)
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}, serverpackets.OpcodeItemList)
	failed := false
	for _, frame := range frames {
		switch frame[0] {
		case serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeMoveToLocation, serverpackets.OpcodeAttack, serverpackets.OpcodeMagicSkillUse:
			t.Fatalf("%s: overloaded player answered opcode %#x", what, frame[0])
		case serverpackets.OpcodeActionFailed:
			failed = true
		}
	}
	if failed != wantFailed {
		t.Fatalf("%s: ActionFailed sent = %t, want %t", what, failed, wantFailed)
	}
	srv.Advance(t, 2*time.Second)
	for _, frame := range c.ReadQueued() {
		switch frame[0] {
		case serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeMoveToLocation, serverpackets.OpcodeAttack, serverpackets.OpcodeMagicSkillUse:
			t.Fatalf("%s: overloaded player later sent opcode %#x", what, frame[0])
		}
	}
}

// TestOverloadedPlayerApproachesNothing pins Playable.isMovementDisabled
// (Playable.java:210-213, super || getMoveSpeed() == 0): a fully overloaded
// player is movement-disabled, so an attack, a cast and an interaction on a
// target out of reach start no approach (PlayerMove.maybeMoveToPawn,
// PlayerMove.java:335-352). The attack keeps its intention silently, as the
// reference does; the cast and the interaction release the client with
// ActionFailed.
func TestOverloadedPlayerApproachesNothing(t *testing.T) {
	t.Parallel()
	srv, c, _ := bootOverloaded(t)
	far := location.Location{X: playerOrigin.X + 1500, Y: playerOrigin.Y, Z: playerOrigin.Z}
	hostile := srv.SpawnHostileNPCAt(t, far)
	folk := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", overloadFolkID), location.Location{X: far.X, Y: far.Y + 300, Z: far.Z})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)
	assertRefusedApproach(t, srv, c, encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false), false, "attack")
	assertRefusedApproach(t, srv, c, encodeRequestMagicSkillUse(queueOneSkillID, false, false), true, "cast")
	if got := hostile.CurrentHP(); got != hostile.MaxHP() {
		t.Fatalf("hostile HP = %d, want untouched %d", got, hostile.MaxHP())
	}

	c.Send(encodeAction(folk.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	drainUntilQuiet(t, c)
	assertRefusedApproach(t, srv, c, encodeAction(folk.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false), true, "interact")
}
