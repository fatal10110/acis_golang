package ai

import (
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Aden Castle's standing guards (GludioStand) and standing bow guards
// (GludioBowGuardStand).
const (
	adenStandID         = int32(35248)
	adenOtherStandID    = int32(35251)
	adenBowGuardStandID = int32(35254)
)

// attackDesireOn returns h's queued attack desire on objectID, and whether
// it has one.
func attackDesireOn(h *npc.Hostile, objectID int32) (actorai.Desire, bool) {
	for _, d := range h.AI().Desires().Snapshot() {
		if d.Kind == actorai.IntentionAttack && d.FinalTarget != nil && d.FinalTarget.ObjectID() == objectID {
			return d, true
		}
	}
	return actorai.Desire{}, false
}

// holdsAttack fails unless h has an attack desire on the character, with
// weight, from where h stands.
func (w *warriorWorld) holdsAttack(t *testing.T, name string, h *npc.Hostile, weight float64) {
	t.Helper()
	d, ok := attackDesireOn(h, w.player.ObjectID())
	if !ok || d.MoveToTarget || d.Weight != weight {
		t.Fatalf("%s attack desire = %+v, %v, want one on the character from where it stands, weight %v", name, d, ok, weight)
	}
}

// TestGludioStandAttacksFromItsPost ports the attacked and clan-attacked
// hooks of GludioStand and GludioBowGuardStand. A hit taking a twentieth of
// a standing guard's hit points makes it want to attack the character from
// where it stands by 100 (attacked); the hit's clan calls, the guard's own
// first, add 100 to each standing guard (GludioStand weighs a clan mate's
// hit as its own) and 50 to the standing bow guard. A hit on the bow guard
// adds its own 100 and 50, and 100 to each standing guard. The built-in
// attacked hate stays off for a bound id, so the hate is the hooks' weights
// alone.
func TestGludioStandAttacksFromItsPost(t *testing.T) {
	t.Parallel()
	w := bootSiegeGuards(t, raidBoot{},
		siegeGuardTemplate(adenStandID), siegeGuardTemplate(adenOtherStandID), siegeGuardTemplate(adenBowGuardStandID))
	stand := w.spawn(t, adenStandID, 100, 0)
	other := w.spawn(t, adenOtherStandID, 0, 100)
	bow := w.spawn(t, adenBowGuardStandID, 0, -100)
	names := map[*npc.Hostile]string{stand: "standing guard", other: "other standing guard", bow: "standing bow guard"}

	w.hit(t, stand, siegeTwentieth)
	w.hates(t, "after the standing guard's hit", map[*npc.Hostile]float64{stand: 200, other: 100, bow: 50}, names)
	w.holdsAttack(t, "standing guard", stand, 200)
	w.holdsAttack(t, "other standing guard", other, 100)
	w.holdsAttack(t, "standing bow guard", bow, 50)

	w.hit(t, bow, siegeTwentieth)
	w.hates(t, "after the standing bow guard's hit", map[*npc.Hostile]float64{stand: 300, other: 200, bow: 200}, names)
}

// TestGludioStandHeightTestsTheGuardHit pins whose height GludioStand's
// clan-attacked hook tests: the guard hit, not the one called. The
// character hits a standing guard 150 below it: that guard wants nothing
// (attacked, and its own clan call), and neither does a standing guard
// level with the character, which the call reaches as the hit guard's clan
// mate. The standing bow guard, as far below, answers both its own hit
// (100 and 50) and the clan call (50): it tests no height. A hit on the
// level guard then adds 100 to the low guard too.
func TestGludioStandHeightTestsTheGuardHit(t *testing.T) {
	t.Parallel()
	w := bootSiegeGuards(t, raidBoot{},
		siegeGuardTemplate(adenStandID), siegeGuardTemplate(adenOtherStandID), siegeGuardTemplate(adenBowGuardStandID))
	low := w.spawnBelow(t, adenStandID, 100, 0, 150)
	level := w.spawn(t, adenOtherStandID, 0, 100)
	bow := w.spawnBelow(t, adenBowGuardStandID, 0, -100, 150)
	names := map[*npc.Hostile]string{low: "low standing guard", level: "level standing guard", bow: "low standing bow guard"}

	w.hit(t, low, siegeTwentieth)
	w.hates(t, "after the low guard's hit", map[*npc.Hostile]float64{low: 0, level: 0, bow: 50}, names)

	w.hit(t, bow, siegeTwentieth)
	w.hates(t, "after the bow guard's hit", map[*npc.Hostile]float64{low: 0, level: 0, bow: 200}, names)

	w.hit(t, level, siegeTwentieth)
	w.hates(t, "after the level guard's hit", map[*npc.Hostile]float64{low: 100, level: 200, bow: 250}, names)
}

// TestGludioStandAttacksWhatItSees ports GludioStand's see-creature hook:
// a standing guard that sees the character, from its first AI cycle on,
// wants to attack it from where it stands with weight 200.
func TestGludioStandAttacksWhatItSees(t *testing.T) {
	t.Parallel()
	w := bootSiegeGuards(t, raidBoot{}, siegeGuardTemplate(adenStandID))
	stand := w.spawn(t, adenStandID, 150, 0)

	w.ticks(t, 1)
	w.holdsAttack(t, "standing guard", stand, 200)
	if got := stand.AggroHate(w.player); got != 200 {
		t.Fatalf("standing guard hate of the character it sees = %v, want 200", got)
	}
}

// TestGludioStandAttacksWhoSpellsWhatItFights ports GludioStand's
// see-spell hook. A monster's hit makes a standing guard, out of the
// character's sight, want to attack the monster (its attacked hook spares
// no creature for not being a player's), so it fights the monster. The
// character then casts on that monster, from 1000 away, an offensive spell
// whose aggro points come to a twentieth of the guard's hit points: the
// guard wants to attack the character from where it stands by 50. A good
// skill on the monster adds nothing.
func TestGludioStandAttacksWhoSpellsWhatItFights(t *testing.T) {
	t.Parallel()
	dummy := warriorTemplate(siegeDummyID, nil)
	// It outlives the fight and the character's spells.
	dummy.HPMax, dummy.PDef, dummy.MDef = 1_000_000, 1000, 1000
	boot := siegeSkillBoot(t)
	boot.opts = append(boot.opts, gameservertest.WithAttackStanceClock(time.Now))
	tmpl := siegeGuardTemplate(adenStandID)
	// Clanless: the monster's hit calls no one, the guard included.
	tmpl.Clans = nil
	w := bootSiegeGuards(t, boot, tmpl, dummy)
	stand := w.spawn(t, adenStandID, 1000, 0)
	monster := w.spawn(t, siegeDummyID, 1020, 0)

	w.onQueue(t, monster, func() { stand.TakeDamage(siegeTwentieth, monster) })
	// damage / max HP / 0.05 * 100, in float arithmetic.
	if got, want := stand.AggroHate(monster), float64(siegeTwentieth)/siegeGuardMaxHP/0.05*100; got != want {
		t.Fatalf("standing guard hate of the monster that hit it = %v, want %v", got, want)
	}
	w.srv.AdvanceUntil(t, "the standing guard's fight with the monster", func() bool {
		cycle(t, w.srv)
		w.srv.TickPositions()
		return stand.InCombat()
	})

	w.selectTarget(t, monster)
	w.cast(t, siegeBlessID)
	if got := stand.AggroHate(w.player); got != 0 {
		t.Fatalf("standing guard hate after a good skill on the monster = %v, want none", got)
	}
	w.cast(t, siegeBoltID)
	if x, _, _ := w.srv.PlayerPosition(t, w.player.ObjectID()); x != w.at.X {
		t.Fatalf("setup: the character moved to cast, x %d, want %d", x, w.at.X)
	}
	if got := stand.AggroHate(w.player); got != 50 {
		t.Fatalf("standing guard hate after a spell on the monster it fights = %v, want 50", got)
	}
	w.holdsAttack(t, "standing guard", stand, 50)
}

// TestGludioBowGuardStandAttacksTheSealOfRulerWithin1000 ports
// GludioBowGuardStand's spelled hook: Seal of Ruler landing on a standing
// bow guard from the character 700 away makes it want to attack the
// character from where it stands with weight 50000; another good skill
// does nothing, and from 1100 away neither does Seal of Ruler.
func TestGludioBowGuardStandAttacksTheSealOfRulerWithin1000(t *testing.T) {
	t.Parallel()
	w := bootSiegeGuards(t, siegeSkillBoot(t), siegeGuardTemplate(adenBowGuardStandID))
	near := w.spawn(t, adenBowGuardStandID, 700, 0)
	far := w.spawn(t, adenBowGuardStandID, -1100, 0)

	w.selectTarget(t, far)
	w.cast(t, sealOfRulerID)
	if x, _, _ := w.srv.PlayerPosition(t, w.player.ObjectID()); x != w.at.X {
		t.Fatalf("setup: the character moved to cast, x %d, want %d", x, w.at.X)
	}
	if got := far.AggroHate(w.player); got != 0 {
		t.Fatalf("standing bow guard 1100 away hate after Seal of Ruler = %v, want none", got)
	}

	w.selectTarget(t, near)
	w.cast(t, siegeBlessID)
	if got := near.AggroHate(w.player); got != 0 {
		t.Fatalf("standing bow guard hate after another good skill = %v, want none", got)
	}
	w.cast(t, sealOfRulerID)
	if got := near.AggroHate(w.player); got != 50000 {
		t.Fatalf("standing bow guard 700 away hate after Seal of Ruler = %v, want 50000", got)
	}
	w.holdsAttack(t, "standing bow guard", near, 50000)
}
