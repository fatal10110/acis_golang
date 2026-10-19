package character

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// bootRegenMultiplied is bootRegenTerms with extra boot options.
func bootRegenMultiplied(t *testing.T, extra ...gameservertest.Option) (*gameservertest.Server, *player.Character) {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		regenTermsTemplate(),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
	}, extra...)...)
	enterWorld(t, srv.Client)
	drainQuiet(t, srv.Client)
	obj, ok := srv.State.Player(srv.SoleObjectID(t))
	if !ok {
		t.Fatal("player not in the world")
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("%T is not an online character", obj)
	}
	return srv, c
}

// TestRegenMultipliersScalePlayerRegen: players.properties'
// HpRegenMultiplier, MpRegenMultiplier and CpRegenMultiplier multiply a
// player's base HP, MP and CP regeneration before the stat and the posture
// multiplier (CreatureStatus.getRegenHp/getRegenMp, PlayerStatus.getRegenCp),
// so the standing rates at 2, 3 and 0.5 are those multiples of the shipped
// ones, and a regeneration tick restores them.
func TestRegenMultipliersScalePlayerRegen(t *testing.T) {
	t.Parallel()
	// Each subtest boots its own server; they run in order so the second
	// compares against the shipped rates the first reads.
	var base regenRates
	t.Run("shipped", func(t *testing.T) {
		_, plain := bootRegenMultiplied(t)
		base = readRegenRates(plain)
	})
	if base.hp <= 0 || base.mp <= 0 || base.cp <= 0 {
		t.Fatalf("shipped regen = %+v, want every rate positive", base)
	}
	t.Run("multiplied", func(t *testing.T) { multipliedPlayerRegen(t, base) })
}

// multipliedPlayerRegen boots the player at multipliers 2, 3 and 0.5 and
// checks its rates and one regeneration tick against the shipped rates.
func multipliedPlayerRegen(t *testing.T, base regenRates) {
	srv, c := bootRegenMultiplied(t, gameservertest.WithRegenMultipliers(2, 3, 0.5))
	rates := readRegenRates(c)
	want := regenRates{base.hp * 2, base.mp * 3, base.cp * 0.5}
	if !rates.near(want) {
		t.Fatalf("regen at multipliers 2/3/0.5 = %+v, want %+v (shipped %+v)", rates, want, base)
	}

	if !srv.DrivesClock() {
		t.Skip("pinning the regeneration tick needs the driven clock")
	}
	c.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: 100, MaxMP: 100, CurrentMP: 100, MaxCP: 100, CurrentCP: 100})
	c.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: 10, MaxMP: 100, CurrentMP: 10, MaxCP: 100, CurrentCP: 10})
	srv.ReadQueued(t, srv.Client)
	regen := task.NewNPCRegen(srv.State)
	srv.Advance(t, task.NPCRegenTick)
	regen.Tick()
	srv.Settle(t)
	got := c.ResourceValues()
	if math.Abs(got.CurrentHP-(10+math.Max(1, rates.hp))) > 1e-9 || math.Abs(got.CurrentMP-(10+math.Max(1, rates.mp))) > 1e-9 ||
		math.Abs(got.CurrentCP-(10+math.Max(1, rates.cp))) > 1e-9 {
		t.Fatalf("resources after one tick = %+v, want 10 plus max(1, %+v)", got, rates)
	}
}
