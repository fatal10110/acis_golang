package cast

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestPlayerPreAttemptGateRefusesTargetOutsideCasterDuel pins
// PlayerCast.canAttemptCast's duel side check (PlayerCast.java:227-234): a
// caster in a duel may aim at itself, a player of its duel or that player's
// summon, and gets INVALID_TARGET for a player, or a player's summon, from
// outside the duel. A caster in no duel aims anywhere.
func TestPlayerPreAttemptGateRefusesTargetOutsideCasterDuel(t *testing.T) {
	def := modelskill.Definition{ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne, SkillType: "DUMMY"}
	pet := func(t *testing.T, owner *player.Character) Target {
		t.Helper()
		p, err := summon.NewPet(summon.PetConfig{ObjectID: 3, Owner: owner})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	for _, tt := range []struct {
		name       string
		casterDuel int32
		otherDuel  int32
		aim        func(t *testing.T, caster, other *player.Character) Target
		want       error
	}{
		{name: "no duel, duellist target", otherDuel: 1, aim: func(_ *testing.T, _, o *player.Character) Target { return o }},
		{name: "self", casterDuel: 1, aim: func(_ *testing.T, c, _ *player.Character) Target { return c }},
		{name: "player of the same duel", casterDuel: 1, otherDuel: 1, aim: func(_ *testing.T, _, o *player.Character) Target { return o }},
		{name: "player outside any duel", casterDuel: 1, aim: func(_ *testing.T, _, o *player.Character) Target { return o }, want: ErrInvalidTarget},
		{name: "player of another duel", casterDuel: 1, otherDuel: 2, aim: func(_ *testing.T, _, o *player.Character) Target { return o }, want: ErrInvalidTarget},
		{name: "summon of the same duel", casterDuel: 1, otherDuel: 1, aim: func(t *testing.T, _, o *player.Character) Target { return pet(t, o) }},
		{name: "summon of an outsider", casterDuel: 1, aim: func(t *testing.T, _, o *player.Character) Target { return pet(t, o) }, want: ErrInvalidTarget},
		{name: "own summon", casterDuel: 1, aim: func(t *testing.T, c, _ *player.Character) Target { return pet(t, c) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl, caster := newGatePlayer(t)
			other := &player.Character{ID: 2}
			if tt.casterDuel != 0 {
				caster.JoinDuel(tt.casterDuel)
				caster.SetDuelState(duel.Duelling)
			}
			if tt.otherDuel != 0 {
				other.JoinDuel(tt.otherDuel)
				other.SetDuelState(duel.Duelling)
			}
			err := ctrl.CanPlayerAttemptCast(caster, tt.aim(t, caster, other), def)
			if tt.want == nil && err != nil {
				t.Fatalf("CanPlayerAttemptCast = %v, want nil", err)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("CanPlayerAttemptCast = %v, want %v", err, tt.want)
			}
		})
	}
}
