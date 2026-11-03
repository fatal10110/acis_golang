package ai

import (
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// roamerTemplate is the fixture PrimevalRoamer of id, both physical
// special chances at 10, with ai added to its AI parameters.
func roamerTemplate(id int32, ai npc.AIParams) *npc.Template {
	params := npc.AIParams{"ProbPhysicalSpecial1": "10", "ProbPhysicalSpecial2": "10"}
	maps.Copy(params, ai)
	tmpl := ind3eTemplate(id, params, 0, false)
	tmpl.CanMove = true
	return tmpl
}

// TestPrimevalRoamerStrikesByItsChances ports PrimevalRoamer's created and
// attacked hooks. It clears i_ai2 and sets i_ai3 to 1 as it spawns, and
// its parent's created hook then sets i_ai2, which doubles as the summon
// flag of a leader with no SummonPrivateRate. A hit at full HP
// with a top desire target, every draw 0, broadcasts event 10002 with
// itself to those within 300 and wants both physical specials on that
// target, weight 1000000; draws over ProbCond5 (20) and over the chances
// (10) do neither. At 45% of its HP the chances double (i_ai3 2), so draws
// of 40 and 20 pass while 21 does not.
func TestPrimevalRoamerStrikesByItsChances(t *testing.T) {
	t.Parallel()
	tmpl := roamerTemplate(ind3eRoamerID, nil)
	tmpl.CanMove = false
	w := bootIndividual3e(t, raidBoot{}, tmpl)
	n := w.ind3eSpawn(t, ind3eRoamerID, 1500, 0)
	w.spawn(t, ind1ListenerID, 1500, 200)
	if a2, a3 := ind1Slot(n, script.SlotIntAI2), ind1Slot(n, script.SlotIntAI3); a2 != 1 || a3 != 1 {
		t.Fatalf("created i_ai2, i_ai3 = %d, %d, want 1, 1", a2, a3)
	}
	me := w.player.ObjectID()
	call := fmt.Sprintf("event=10002 arg1=%d arg2=0", n.ObjectID())
	for _, row := range []struct {
		name  string
		share float64
		draws []int
		calls bool
		want  map[modelskill.ID]ind1Cast
		ai3   int32
	}{
		{"full HP, every draw 0", 1, nil, true, map[modelskill.ID]ind1Cast{ind3eSpecial1Skill: {me, 1000000}, ind3eSpecial2Skill: {me, 1000000}}, 1},
		{"full HP, losing draws", 1, []int{21, 11, 11}, false, nil, 1},
		{"45% HP, draws 40, 20, 21", 0.45, []int{40, 20, 21}, true, map[modelskill.ID]ind1Cast{ind3eSpecial1Skill: {me, 1000000}}, 2},
	} {
		w.setHPShare(t, n, row.share)
		w.setTop(t, n, w.charOf(t))
		w.heard.take()
		w.draws.set(99, row.draws...)
		if row.draws == nil {
			w.draws.set(0)
		}
		w.hit(t, n, 1)
		w.wantCasts(t, row.name, n, row.want)
		var want []string
		if row.calls {
			want = []string{call}
		}
		if got := w.heard.take(); !slices.Equal(got, want) {
			t.Fatalf("%s: the listener heard %q, want %q", row.name, got, want)
		}
		if got := ind1Slot(n, script.SlotIntAI3); got != row.ai3 {
			t.Fatalf("%s: i_ai3 = %d, want %d", row.name, got, row.ai3)
		}
	}
}

// TestPrimevalRoamerBuffsOnceLow ports PrimevalRoamer's attacked and
// skill-finished hooks below HpChkRate5 (30%), with i_ai2 clear. A hit
// there, every draw 99,
// keeps its top desire target in c_ai1, drops the attack it wanted and
// wants its range buff on itself, weight 10000000. Once that buff is cast,
// i_ai2 is set, c_ai1 is cleared and it wants to attack the kept
// character with 100 more; a later hit there wants the buff no more.
func TestPrimevalRoamerBuffsOnceLow(t *testing.T) {
	t.Parallel()
	tmpl := roamerTemplate(ind3eRoamerID, nil)
	tmpl.CanMove = false
	w := bootIndividual3e(t, raidBoot{}, tmpl)
	n := w.ind3eSpawn(t, ind3eRoamerID, 150, 0)
	me := w.player.ObjectID()
	// The parent's summon flag set i_ai2 as the roamer spawned; idle out of
	// combat it is cleared again.
	w.onQueue(t, n, func() { script.NPCOf(n).SetInt(script.SlotIntAI2, 0) })
	w.setHPShare(t, n, 0.25)
	w.setTop(t, n, w.charOf(t))
	w.onQueue(t, n, func() { script.NPCOf(n).AddAttackDesire(script.PlayerOf(w.player), 5000) })
	w.draws.set(99)
	w.hit(t, n, 1)
	w.wantCasts(t, "a hit under 30%", n, map[modelskill.ID]ind1Cast{ind3eRangeBuff1: {n.ObjectID(), 10000000}})
	if got := slotID(n, script.SlotCreature1); got != me {
		t.Fatalf("c_ai1 after the hit = %d, want the character %d", got, me)
	}
	// The 5000 it wanted is dropped; the parent's hit hate, 1 / (level + 7)
	// x 100, follows.
	if d, ok := desire(n, actorai.IntentionAttack); !ok || d.Weight != 12.5 {
		t.Fatalf("attack desire after the hit = %+v (%v), want only the parent's 12.5", d, ok)
	}

	var frames [][]byte
	for i := 0; ind1Slot(n, script.SlotIntAI2) == 0; i++ {
		if i == 10 {
			t.Fatal("the range buff never landed within 10 cycles")
		}
		frames = append(frames, w.ticks(t, 1)...)
		w.srv.Advance(t, time.Second)
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
	}
	if got := castsOf(frames, n, ind3eRangeBuff1); !slices.Equal(got, []int32{n.ObjectID()}) {
		t.Fatalf("range buff MagicSkillUse targets = %v, want one on itself", got)
	}
	if got := slotID(n, script.SlotCreature1); got != 0 {
		t.Fatalf("c_ai1 after the buff = %d, want none", got)
	}
	if d, ok := desire(n, actorai.IntentionAttack); !ok || d.FinalTarget == nil || d.FinalTarget.ObjectID() != me {
		t.Fatalf("attack desire after the buff = %+v (%v), want the character", d, ok)
	}
	w.clearDesires(t, n)
	w.hit(t, n, 1)
	if _, ok := castDesireOf(n, ind3eRangeBuff1); ok {
		t.Fatal("a later hit under 30% wants the range buff again, want it once")
	}
}

// TestPrimevalRoamerAnswersAShoutedTarget ports PrimevalRoamer's script
// event 10016: naming a player with BroadCastReception 1, it drops its
// attack desires and runs its own attacked hook as a hit of 1000 by that
// player, which adds the Warrior hate, 1000 / (level + 7) x 100. Its
// parent then handles the same event, on a draw of 99 doing nothing.
// Without BroadCastReception only the parent reacts.
func TestPrimevalRoamerAnswersAShoutedTarget(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		reception string
		wantHate  float64
	}{{"1", 12500}, {"0", 0}} {
		t.Run("reception "+row.reception, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual3e(t, raidBoot{}, roamerTemplate(ind3eRoamerID, npc.AIParams{"BroadCastReception": row.reception}))
			n := w.ind3eSpawn(t, ind3eRoamerID, 1500, 0)
			w.draws.set(99)
			w.sendEvent(t, n, 10016, w.player.ObjectID(), 0)
			if got := n.AggroHate(w.player); got != row.wantHate {
				t.Fatalf("hate after event 10016 = %v, want %v", got, row.wantHate)
			}
		})
	}
}

// TestPrimevalRoamerAttacksOnSightByAgType ports PrimevalRoamer's
// see-creature hook. With ag_type 1 it wants its first physical special on
// the character it sees on a draw at most its chance (10), else its second
// on the next draw, and attacks it. With ag_type 0 it neither attacks nor
// calls its parent.
func TestPrimevalRoamerAttacksOnSightByAgType(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		agType string
		draws  []int
		want   modelskill.ID
	}{
		{"ag_type 1, draw 0", "1", []int{0}, ind3eSpecial1Skill},
		{"ag_type 1, draws 11 and 0", "1", []int{11, 0}, ind3eSpecial2Skill},
		{"ag_type 0", "0", nil, 0},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := roamerTemplate(ind3eRoamerID, npc.AIParams{"ag_type": row.agType})
			tmpl.CanMove = false
			w := bootIndividual3e(t, raidBoot{}, tmpl)
			n := w.ind3eSpawn(t, ind3eRoamerID, 1500, 0)
			w.clearDesires(t, n)
			w.draws.set(0, row.draws...)
			w.moveCharacter(t, n, -150, 0)
			w.ticks(t, 1)
			me := w.player.ObjectID()
			if row.want == 0 {
				w.wantCasts(t, row.name, n, nil)
				if got := n.AggroHate(w.player); got != 0 {
					t.Fatalf("hate of the character it sees = %v, want none", got)
				}
				return
			}
			w.wantCasts(t, row.name, n, map[modelskill.ID]ind1Cast{row.want: {me, 1000000}})
			if got := n.AggroHate(w.player); got <= 0 {
				t.Fatalf("hate of the character it sees = %v, want some", got)
			}
		})
	}
}

// TestPrimevalRoamerResetsAndWalksHomeWhenIdle ports PrimevalRoamer's
// no-desire hook: left with nothing to do out of combat it clears i_ai2,
// sets i_ai3 back to 1 and, with mobile_type 0, wants to walk back to its
// spawn point with weight 30; with mobile_type 1 it stays.
func TestPrimevalRoamerResetsAndWalksHomeWhenIdle(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		mobile string
		walks  bool
	}{{"0", true}, {"1", false}} {
		t.Run("mobile_type "+row.mobile, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual3e(t, raidBoot{}, roamerTemplate(ind3eRoamerID, npc.AIParams{"mobile_type": row.mobile}))
			n := w.ind3eSpawn(t, ind3eRoamerID, 3000, 0)
			x, y, _ := n.Position()
			w.teleport(t, n, 200, 0)
			w.onQueue(t, n, func() {
				n.AI().Desires().Clear()
				script.NPCOf(n).SetInt(script.SlotIntAI2, 1)
				script.NPCOf(n).SetInt(script.SlotIntAI3, 2)
			})
			w.ticks(t, 2)
			if a2, a3 := ind1Slot(n, script.SlotIntAI2), ind1Slot(n, script.SlotIntAI3); a2 != 0 || a3 != 1 {
				t.Fatalf("i_ai2, i_ai3 when idle = %d, %d, want 0, 1", a2, a3)
			}
			d, ok := desire(n, actorai.IntentionMoveTo)
			home := ok && d.Weight == 30 && d.Location.X == x && d.Location.Y == y
			if home != row.walks {
				t.Fatalf("move desire when idle = %+v (%v), want a walk home with weight 30: %v", d, ok, row.walks)
			}
		})
	}
}

// TestPrimevalRoamerWalkerWalksItsRoute ports PrimevalRoamerWalker's
// created hook: after PrimevalRoamer's (i_ai3 set to 1), it wants to walk
// the route its SuperPointName parameter names, with weight 50.
func TestPrimevalRoamerWalkerWalksItsRoute(t *testing.T) {
	t.Parallel()
	w := bootIndividual3e(t, raidBoot{}, roamerTemplate(ind3eWalkerID, npc.AIParams{"SuperPointName": "primeval_02"}))
	n := w.ind3eSpawn(t, ind3eWalkerID, 1500, 0)
	if got := ind1Slot(n, script.SlotIntAI3); got != 1 {
		t.Fatalf("created i_ai3 = %d, want 1", got)
	}
	d, ok := desire(n, actorai.IntentionMoveRoute)
	if !ok || d.RouteName != "primeval_02" || d.Weight != 50 {
		t.Fatalf("route desire once created = %+v (%v), want primeval_02 with weight 50", d, ok)
	}
}
