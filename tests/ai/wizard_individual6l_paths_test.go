package ai

import (
	"strconv"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// i6lWalkTo spawns an NPC of id dx, dy from the character, hating it, and
// walks it 100 east, before running first on its queue.
func (w *individual6World) i6lWalkTo(t *testing.T, id int32, dx, dy int, first func(h *npc.Hostile)) *npc.Hostile {
	t.Helper()
	h := w.spawn(t, id, dx, dy)
	w.srv.InstallNPCCastSeam(t, h, i6lSkills())
	x, y, z := h.Position()
	w.onQueue(t, h, func() {
		h.AI().Desires().Clear()
		h.AI().Hates().Add(w.player, 100)
		first(h)
	})
	if !h.AI().AddMoveToDesire(location.Location{X: x + 100, Y: y, Z: z}, 1e6) {
		t.Fatal("the walk was not queued")
	}
	w.i6eWalk(t, h)
	return h
}

// i6lCastEnds runs AI cycles until h casts, then runs then on its queue
// with its desires emptied and lets the cast end.
func (w *individual6World) i6lCastEnds(t *testing.T, h *npc.Hostile, then func()) {
	t.Helper()
	for i := 0; h.AI().CurrentIntention() != actorai.IntentionCast; i++ {
		if i == 5 {
			t.Fatalf("setup: npc %d does not cast", h.NpcID())
		}
		w.ticks(t, 1)
	}
	w.onQueue(t, h, func() { h.AI().Desires().Clear(); then() })
	w.srv.Advance(t, time.Second)
	w.srv.Settle(t)
}

// TestWizardSaintSelfRangeDDMagicStrikesNearAtWalkEnd runs
// WizardSaintSelfRangeDDMagic's move-finished hook for a walk ending 20
// from the character it hates, on a draw of 0: it wants its debuff and its
// short range DD magic on the character, weight 1000000, and no middle
// range one. With its short range DD magic waiting for its reuse, the
// checked want of it is refused, and no middle range one comes instead.
func TestWizardSaintSelfRangeDDMagicStrikesNearAtWalkEnd(t *testing.T) {
	t.Parallel()
	w := bootIndividual6l(t, i6lTemplate(i6lSelfID))
	me := w.player.ObjectID()

	near := w.i6lWalkTo(t, i6lSelfID, -120, 0, func(*npc.Hostile) {})
	if !castOn(near, i6gDebuff, me) || !castOn(near, wShortDDID, me) {
		t.Fatal("saint arriving 20 from its most hated wants no debuff and short range DD magic on it")
	}
	if _, ok := castDesireOf(near, i6lMiddle); ok {
		t.Fatal("saint arriving 20 from its most hated wants its middle range DD magic")
	}

	shortKey := actorcast.ReuseKey(modelskill.Definition{ID: wShortDDID, Level: 1})
	waiting := w.i6lWalkTo(t, i6lSelfID, -120, 30, func(h *npc.Hostile) { h.DisableSkill(shortKey, time.Minute) })
	if !castOn(waiting, i6gDebuff, me) {
		t.Fatal("saint arriving near its most hated, short range DD magic in reuse, wants no debuff on it")
	}
	for _, id := range []modelskill.ID{wShortDDID, i6lMiddle} {
		if d, ok := castDesireOf(waiting, id); ok {
			t.Fatalf("saint arriving near its most hated, short range DD magic in reuse, wants skill %d: %+v", id, d)
		}
	}
}

// TestWizardSaintSelfRangeDDMagicDebuffsFarAfterCast runs
// WizardSaintSelfRangeDDMagic's skill-finished hook with the character it
// hates 300 away, on a draw of 0: the end of its cast wants its debuff and
// its middle range DD magic on the character, weight 1000000, and no run.
func TestWizardSaintSelfRangeDDMagicDebuffsFarAfterCast(t *testing.T) {
	t.Parallel()
	w := bootIndividual6l(t, i6lTemplate(i6lSelfID))
	me := w.player.ObjectID()

	w.draw.Store(80)
	h := w.i6lArmed(t, i6lSelfID, 300, 0)
	w.hit(t, h, 10)
	w.i6lCastEnds(t, h, func() { w.draw.Store(0) })
	i6fWantCasts(t, "after the cast", h, map[modelskill.ID]i6fCast{
		i6gDebuff: {me, 1000000, false}, i6lMiddle: {me, 1000000, false},
	})
	if d, ok := desire(h, actorai.IntentionFlee); ok {
		t.Fatalf("saint 300 from its most hated wants to run after its cast: %+v", d)
	}
}

// TestWizardSaintsOfThePRAttackOnceTheyGaveUp runs the skill-finished
// hooks of WizardSaintDDMagic2 and WizardSaintSelfRangeDDMagic for a saint
// that gave up its spells during its cast: the end of the cast wants to
// attack its most hated, the character, weight 1000, and no cast.
func TestWizardSaintsOfThePRAttackOnceTheyGaveUp(t *testing.T) {
	t.Parallel()
	for _, id := range []int32{i6lDDMagic2ID, i6lSelfID} {
		t.Run(strconv.Itoa(int(id)), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual6l(t, i6lTemplate(id))
			w.draw.Store(80)
			h := w.i6lArmed(t, id, 300, 0)
			w.hit(t, h, 10)
			w.i6lCastEnds(t, h, func() { h.Scratch().SetInt(npc.IntAI0, 1); w.draw.Store(0) })
			if got := i6cAttackWeight(h, w.player.ObjectID()); got != 1000 {
				t.Fatalf("attack desire on the character after the cast = %v, want 1000", got)
			}
			i6fWantCasts(t, "after the cast", h, nil)
		})
	}
}
