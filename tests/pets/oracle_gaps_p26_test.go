package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestSummonKillRaisesTheDeathPenalty pins the death-penalty gate's killer
// test: only a player killer blocks the raise, not the player's summon. A
// karma victim skips the chance roll, so a kill by the owner's wolf raises
// its level to 1 and tells the victim, while the owner's own kill, the
// control, leaves it at 0.
func TestSummonKillRaisesTheDeathPenalty(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		bySummon  bool
		wantLevel int
	}{
		{"summon killer", true, 1},
		{"player killer", false, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := bootOwnerWithCollar(t)
			wolf, _ := h.spawnWolf(t)
			victim := h.joinSecondPlayer(t, "Victim")
			target := victim.actor.(interface {
				HP() float64
				SetKarma(int)
				DeathPenaltyLevel() int
				ReduceHP(float64, attackable.Combatant, modelskill.Definition)
			})
			runOn(t, victim.queue, func() { target.SetKarma(100) })
			owner, _ := h.srv.State.Player(h.ownerID)
			killer := owner.(attackable.Combatant)
			if tt.bySummon {
				killer = wolf
			}
			drainUntilQuiet(t, victim.client)

			runOn(t, victim.queue, func() { target.ReduceHP(target.HP()+1, killer, modelskill.Definition{}) })
			h.srv.Settle(t)

			if got := target.DeathPenaltyLevel(); got != tt.wantLevel {
				t.Fatalf("death-penalty level after the kill = %d, want %d", got, tt.wantLevel)
			}
			if got := hasSystemMessage(drainFrames(t, victim.client), serverpackets.SystemMessageDeathPenaltyLevelS1Added); got != (tt.wantLevel > 0) {
				t.Fatalf("DEATH_PENALTY_LEVEL_S1_ADDED sent = %v, want %v", got, tt.wantLevel > 0)
			}
		})
	}
}
