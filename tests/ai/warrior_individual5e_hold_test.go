package ai

import (
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// ind5eBowID is the fixture item table's bow.
const ind5eBowID = int32(14)

// bootIndividual5eArcher boots the batch beside templates with the
// character holding the fixture bow.
func bootIndividual5eArcher(t *testing.T, templates ...*npc.Template) *ind1World {
	t.Helper()
	var bow int32
	w := bootIndividual5e(t, raidBoot{ready: func(srv *gameservertest.Server, objID int32) { bow = srv.GiveItem(t, objID, ind5eBowID, 1) }}, templates...)
	use := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	use.WriteInt32(bow)
	use.WriteInt32(0)
	w.srv.Client.Send(use.Bytes())
	w.srv.AdvanceUntil(t, "the bow in the hands", func() bool {
		var hand int32
		w.srv.RunScript(t, w.player.ObjectID(), "Ind1Director", func(_ *script.Script, p *script.Player) { hand = p.RightHandItemID() })
		return hand == ind5eBowID
	})
	w.srv.ReadQueued(t, w.srv.Client)
	return w
}

// ind5eHeld fails unless h's i_ai0 marks whether it held someone.
func ind5eHeld(t *testing.T, when string, h *npc.Hostile, held bool) {
	t.Helper()
	want := int32(0)
	if held {
		want = 1
	}
	if got := ind1Slot(h, script.SlotIntAI0); got != want {
		t.Fatalf("%s: i_ai0 = %d, want %d", when, got, want)
	}
}

// TestCastHoldMagicHoldsByRange ports WarriorCastHoldMagic's created and
// attacked hooks against the unarmed character. It appears with i_ai0 0.
// From 400 away a draw under 50 wants the hold on the character and marks
// i_ai0, after which no hit wants it again; a draw of 50 wants nothing.
// From 200 away the first hit, wanting to attack no one, wants nothing;
// the next, by the one it most wants to attack, wants the hold on a draw
// under 50. With a decoy its top desire target, a draw of 9 wants the
// hold, a draw of 10 not. From 50 away no hit wants it.
func TestCastHoldMagicHoldsByRange(t *testing.T) {
	t.Parallel()
	w := bootIndividual5e(t, raidBoot{}, ind5eTemplate(ind5eHoldID))
	me := w.player.ObjectID()

	far := w.spawn(t, ind5eHoldID, 400, 0)
	ind5eHeld(t, "once created", far, false)
	w.hit(t, far, 10)
	ind5bWantCast(t, "a hit from 400 away, draw 0", far, ind5eHoldMagic, me)
	ind5eHeld(t, "a hit from 400 away, draw 0", far, true)
	w.ind5dClear(t, far)
	w.hit(t, far, 10)
	ind5bNoCast(t, "a hit once it held", far, ind5eHoldMagic)

	missed := w.spawn(t, ind5eHoldID, -400, 0)
	w.draws.set(50)
	w.hit(t, missed, 10)
	ind5bNoCast(t, "a hit from 400 away, draw 50", missed, ind5eHoldMagic)
	ind5eHeld(t, "a hit from 400 away, draw 50", missed, false)

	w.draws.set(49)
	mid := w.spawn(t, ind5eHoldID, 0, 200)
	w.hit(t, mid, 10)
	ind5bNoCast(t, "a first hit from 200 away", mid, ind5eHoldMagic)
	w.hit(t, mid, 10)
	ind5bWantCast(t, "a hit from 200 away by the one it most wants to attack, draw 49", mid, ind5eHoldMagic, me)

	decoyed := w.spawn(t, ind5eHoldID, 0, -200)
	decoy := w.spawn(t, unboundID, 0, 1500)
	for _, draw := range []int{10, 9} {
		w.onQueue(t, decoyed, func() { script.NPCOf(decoyed).SetTopDesireTarget(script.NPCOf(decoy)) })
		w.draws.set(draw)
		w.hit(t, decoyed, 10)
		when := fmt.Sprintf("a hit from 200 away, a decoy its top desire target, draw %d", draw)
		if draw == 10 {
			ind5bNoCast(t, when, decoyed, ind5eHoldMagic)
		} else {
			ind5bWantCast(t, when, decoyed, ind5eHoldMagic, me)
		}
	}
	w.draws.set(0)
	near := w.spawn(t, ind5eHoldID, 50, 50)
	w.hit(t, near, 10)
	w.hit(t, near, 10)
	ind5bNoCast(t, "hits from 50 away", near, ind5eHoldMagic)
	ind5eHeld(t, "hits from 50 away", near, false)
}

// TestCastHoldMagicHoldsAnArcher ports WarriorCastHoldMagic's bow branch:
// from 50 away, where no range band holds, the character attacking with a
// bow draws the hold on a second draw under 50, not on 50.
func TestCastHoldMagicHoldsAnArcher(t *testing.T) {
	t.Parallel()
	w := bootIndividual5eArcher(t, ind5eTemplate(ind5eHoldID))
	n := w.spawn(t, ind5eHoldID, 50, 0)
	w.draws.set(0, 0, 50)
	w.hit(t, n, 10)
	ind5bNoCast(t, "a hit by an archer, second draw 50", n, ind5eHoldMagic)
	w.draws.set(0)
	w.hit(t, n, 10)
	ind5bWantCast(t, "a hit by an archer, second draw 0", n, ind5eHoldMagic, w.player.ObjectID())
	ind5eHeld(t, "a hit by an archer", n, true)
}

// TestCastHoldMagicHoldsItsLastAttackerLater ports WarriorCastHoldMagic's
// timer "2001", started 12 s after a hit: unless it held someone, it wants
// the hold on its last attacker on a draw under 80, not on 80.
func TestCastHoldMagicHoldsItsLastAttackerLater(t *testing.T) {
	t.Parallel()
	w := bootIndividual5e(t, raidBoot{}, ind5eTemplate(ind5eHoldID))
	me := w.player.ObjectID()
	missed := w.spawn(t, ind5eHoldID, 50, 0)
	w.hit(t, missed, 10)
	ind5bNoCast(t, "the hit from 50 away", missed, ind5eHoldMagic)
	w.draws.set(80)
	w.srv.Advance(t, 12*time.Second)
	ind5bNoCast(t, "the timer, draw 80", missed, ind5eHoldMagic)

	w.draws.set(0)
	n := w.spawn(t, ind5eHoldID, -50, 0)
	w.hit(t, n, 10)
	w.srv.Advance(t, 11*time.Second)
	ind5bNoCast(t, "11 s after the hit", n, ind5eHoldMagic)
	w.srv.Advance(t, time.Second)
	ind5bWantCast(t, "the timer, draw 0", n, ind5eHoldMagic, me)
	ind5eHeld(t, "the timer, draw 0", n, true)
}

// TestCastHoldMagicAggressiveAttacksOnSight ports
// WarriorCastHoldMagicAggressive's see-creature hook: it attacks the
// character it sees.
func TestCastHoldMagicAggressiveAttacksOnSight(t *testing.T) {
	t.Parallel()
	w := bootIndividual5e(t, raidBoot{}, ind5eTemplate(ind5eHoldAggrID))
	n := w.spawn(t, ind5eHoldAggrID, 150, 0)
	w.ticks(t, 1)
	if got := n.AggroHate(w.player); got <= 0 {
		t.Fatalf("hate of the character seen = %v, want it attacked", got)
	}
}

// TestOrfenPrivateRibaCursesWithoutTheParent ports OrfenPrivateRiba's
// attacked hook, which replaces its parents': a level 1 Riba hit by the
// level 20 character, more than 8 levels above it, wants the raid curse on
// it and neither the hold nor any hate; a level 12 one wants nothing.
func TestOrfenPrivateRibaCursesWithoutTheParent(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		level int
		curse bool
	}{{1, true}, {12, false}} {
		t.Run(fmt.Sprintf("level %d", row.level), func(t *testing.T) {
			t.Parallel()
			tmpl := ind5eTemplate(ind5eRibaID)
			tmpl.Level = row.level
			w := bootIndividual5e(t, raidBoot{}, tmpl)
			n := w.spawn(t, ind5eRibaID, 400, 0)
			w.hit(t, n, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.curse {
				want[raidCurseID] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "after the hit", n, want)
			ind5eHeld(t, "after the hit", n, false)
			if got := n.AggroHate(w.player); got != 0 {
				t.Fatalf("hate after the hit = %v, want none", got)
			}
			if d, ok := desire(n, actorai.IntentionAttack); ok {
				t.Fatalf("attack desire after the hit = %+v, want none", d)
			}
		})
	}
}

// TestOrfenPrivateRibaMutesOverLevelledCasters ports OrfenPrivateRiba's
// see-spell hook: a level 11 Riba seeing the level 20 character cast,
// more than 8 levels above it, wants the raid mute on it; a level 12 one
// does not, once a doom trooper beside it, which holds against every
// spell it sees, shows the spell was seen.
func TestOrfenPrivateRibaMutesOverLevelledCasters(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		level int
		mute  bool
	}{{11, true}, {12, false}} {
		t.Run(fmt.Sprintf("level %d", row.level), func(t *testing.T) {
			t.Parallel()
			tmpl := ind5eTemplate(ind5eRibaID)
			tmpl.Level = row.level
			w := bootIndividual5e(t, ind1AggroSpellBoot(t), tmpl, ind5eTemplate(ind5eTrooperID))
			n := w.spawn(t, ind5eRibaID, 150, 0)
			witness := w.spawn(t, ind5eTrooperID, -150, 0)
			w.ticks(t, 1)
			seen, _ := desire(witness, actorai.IntentionAttack)
			w.castAggroBuff()
			w.srv.AdvanceUntil(t, "the trooper's hold against the spell", func() bool {
				d, ok := desire(witness, actorai.IntentionAttack)
				return ok && d.Weight > seen.Weight
			})
			if !row.mute {
				ind5bNoCast(t, "a spell by one 8 levels above", n, premoMuteID)
				return
			}
			ind5bWantCast(t, "a spell by one 9 levels above", n, premoMuteID, w.player.ObjectID())
		})
	}
}

// TestOrfenPrivateRibaShufflesItsHate ports OrfenPrivateRiba's created and
// timer hooks: it arms timer "5001" 90 s plus a draw of up to 240 s after
// it spawns (draw 0: 90 s), and on a draw under 66 the tick shuffles its
// hate, raising the character it hates less than another NPC to that
// NPC's hate plus 200.
func TestOrfenPrivateRibaShufflesItsHate(t *testing.T) {
	t.Parallel()
	tmpl := ind5eTemplate(ind5eRibaID)
	tmpl.CanMove = false
	w := bootIndividual5e(t, raidBoot{}, tmpl)
	n := w.spawn(t, ind5eRibaID, 400, 0)
	other := w.spawn(t, unboundID, 400, 200)
	ind2Hate(t, w, n, other, 1000, 100)
	w.srv.Advance(t, 89*time.Second)
	if got := n.AggroHate(w.player); got != 100 {
		t.Fatalf("hate of the character before 90 s = %v, want 100", got)
	}
	w.draws.set(0, 65, 1000)
	w.srv.Advance(t, time.Second)
	if got := n.AggroHate(w.player); got != 1200 {
		t.Fatalf("hate of the character after the tick = %v, want 1200", got)
	}
}
