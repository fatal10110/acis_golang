package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestSpoilRefusedOnNonMonsterTarget pins PlayerCast.canCast's SPOIL and
// DRAIN_SOUL case (PlayerCast.java:328-334): a player's Spoil (254-1, ONE,
// aCis_datapack/data/xml/skills/0200-0299.xml:603-620) whose target is not
// a Monster answers INVALID_TARGET and nothing else, spending no MP and
// starting no cast or reuse. A SiegeGuard and a FriendlyMonster are
// attackable but not Monster-family, so the ONE target check lets the
// offensive skill through and only this rule refuses it; a town Guard is
// already refused by the ONE check itself. A Monster is spoiled as before.
func TestSpoilRefusedOnNonMonsterTarget(t *testing.T) {
	t.Parallel()
	def := shippedSkill(t, 254, 1)
	for _, tc := range []struct {
		kind   string
		refuse bool
	}{
		{kind: "SiegeGuard", refuse: true},
		{kind: "FriendlyMonster", refuse: true},
		{kind: "Guard", refuse: true},
		{kind: "Monster", refuse: false},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			srv := bootTargetConditionCaster(t, def)
			c, objID := srv.Client, srv.SoleObjectID(t)
			startInWorld(t, c)
			// Inside Spoil's 40 cast range, so no approach walk comes first.
			target := srv.SpawnHostileNPCKindAt(t, tc.kind, location.Location{X: 30, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, c)
			selectTarget(t, c, target.ObjectID())
			mp := srv.PlayerCurrentMP(t, objID)

			c.Send(encodeRequestMagicSkillUse(254, false, false))
			if !tc.refuse {
				assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "Spoil on a Monster")
				return
			}
			assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageInvalidTarget)
			// Nor does the refusal hand on to Spoil's nextActionAttack: none
			// of these NPCs is attackable without Ctrl.
			for frame := c.ReadWithTimeout(300 * time.Millisecond); frame != nil; frame = c.ReadWithTimeout(300 * time.Millisecond) {
				switch {
				case frame[0] == serverpackets.OpcodeActionFailed:
					t.Fatalf("Spoil on a %s sent ActionFailed after the rejection message", tc.kind)
				case frame[0] == serverpackets.OpcodeAttack && wireReader(frame[1:]).ReadInt32() == objID:
					t.Fatalf("Spoil on a %s handed on to an attack", tc.kind)
				}
			}
			if srv.PlayerCastingNow(t, objID) {
				t.Fatalf("Spoil on a %s started a cast", tc.kind)
			}
			if got := srv.PlayerCurrentMP(t, objID); got != mp {
				t.Fatalf("MP after refused Spoil = %d, want %d", got, mp)
			}
			// No reuse was started: the same request is refused the same
			// way again, not with a reuse message.
			c.Send(encodeRequestMagicSkillUse(254, false, false))
			assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageInvalidTarget)
		})
	}
}
