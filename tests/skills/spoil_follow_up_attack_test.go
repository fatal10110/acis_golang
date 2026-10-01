package skills

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestSpoilRefusedOnKarmaPlayerStillAttacks pins the nextActionAttack
// hand-off after a SPOIL refusal (PlayerAI.thinkCast, PlayerAI.java:288-292,
// with PlayerCast.java:328-334): Spoil (254-1, nextActionAttack) on a karma
// player, who is attackable without Ctrl but not a Monster, answers
// INVALID_TARGET, starts no cast and spends no MP, and then the player
// attacks that player.
func TestSpoilRefusedOnKarmaPlayerStillAttacks(t *testing.T) {
	t.Parallel()
	def := shippedSkill(t, 254, 1)
	if !def.NextActionIsAttack {
		t.Fatal("shipped Spoil 254-1 lost nextActionAttack")
	}
	srv := bootTargetConditionCaster(t, def)
	c, objID := srv.Client, srv.SoleObjectID(t)
	victim := srv.SeedCharacterFor(t, "chaotic", "Chaotic", 5, 0)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET karma = ? WHERE obj_Id = ?", 240, victim.ID); err != nil {
		t.Fatalf("set karma: %v", err)
	}
	vc := srv.DialClient(t, "chaotic", 1)
	startInWorldAmongPlayers(t, vc)
	startInWorldAmongPlayers(t, c)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)
	selectTarget(t, c, victim.ID)
	mp := srv.PlayerCurrentMP(t, objID)

	c.Send(encodeRequestMagicSkillUse(254, false, false))
	refused := false
	for range 100 {
		frame := c.ReadWithTimeout(3 * time.Second)
		if frame == nil {
			t.Fatalf("Spoil on a karma player: no Attack by the player (INVALID_TARGET seen: %v)", refused)
		}
		switch {
		case frame[0] == serverpackets.OpcodeMagicSkillUse && wireReader(frame[1:]).ReadInt32() == objID:
			t.Fatal("Spoil on a karma player sent MagicSkillUse")
		case frame[0] == serverpackets.OpcodeSystemMessage:
			if _, ok := systemMessage(frame, serverpackets.SystemMessageInvalidTarget); ok {
				refused = true
			}
		case frame[0] == serverpackets.OpcodeAttack && wireReader(frame[1:]).ReadInt32() == objID:
			if !refused {
				t.Fatal("Attack came before INVALID_TARGET")
			}
			r := wireReader(frame[1:])
			r.ReadInt32() // attacker
			if got := r.ReadInt32(); got != victim.ID {
				t.Fatalf("attack after the refused Spoil target = %d, want the karma player %d", got, victim.ID)
			}
			if got := srv.PlayerCurrentMP(t, objID); got != mp {
				t.Fatalf("MP after refused Spoil = %d, want %d", got, mp)
			}
			return
		}
	}
	t.Fatal("Spoil on a karma player: no Attack by the player within 100 frames")
}
