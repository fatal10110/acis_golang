package npc

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// folkCastRig is a civilian NPC at (100, 0, 0) wired to scripted cast
// seams, with one player-kind target it knows.
type folkCastRig struct {
	f       *Folk
	in      *sim.Inline
	control *scriptedFolkControl
	castAI  *scriptedFolkCastAI
	aiTask  *recordingFolkAI
	events  *recordingSink
	los     *switchLOS
	target  *hostileTarget
}

// scriptedFolkControl is a cast controller whose cast lasts until the test
// finishes or breaks it; either end reports to the NPC's cast sink the way
// the real controller does, with casting cleared first.
type scriptedFolkControl struct {
	f       *Folk
	casting bool
}

func (c *scriptedFolkControl) CastingNow() bool          { return c.casting }
func (c *scriptedFolkControl) CurrentSkillIsMagic() bool { return true }
func (c *scriptedFolkControl) InterruptCast()            {}
func (c *scriptedFolkControl) StopCast()                 {}
func (c *scriptedFolkControl) InterruptCastOnDamage(float64, int, func(float64) float64, int, bool) bool {
	if !c.casting {
		return false
	}
	c.casting = false
	sink := c.f.CastEvents()
	sink.Emit(event.CastAborted{})
	sink.Emit(event.CastFinished{Interrupted: true, Broken: true})
	return true
}

// finish ends the cast in flight at its natural end.
func (c *scriptedFolkControl) finish() {
	c.casting = false
	c.f.CastEvents().Emit(event.CastFinished{})
}

// scriptedFolkCastAI answers every cast gate from its fields and records
// the casts it starts.
type scriptedFolkCastAI struct {
	control   *scriptedFolkControl
	canDesire bool
	canCast   bool
	castRange int
	casts     []modelskill.Ref
}

func (a *scriptedFolkCastAI) FinalTarget(target attackable.Combatant, _ modelskill.Ref) attackable.Combatant {
	return target
}

func (a *scriptedFolkCastAI) Disabled() bool   { return a.control.casting }
func (a *scriptedFolkCastAI) CastingNow() bool { return a.control.casting }
func (a *scriptedFolkCastAI) Stop()            {}

func (a *scriptedFolkCastAI) CanDesire(attackable.Combatant, modelskill.Ref) bool { return a.canDesire }

func (a *scriptedFolkCastAI) MeetsHPMPDisabled(attackable.Combatant, modelskill.Ref) bool {
	return true
}
func (a *scriptedFolkCastAI) CanAttempt(attackable.Combatant, modelskill.Ref) bool { return true }
func (a *scriptedFolkCastAI) CanCast(attackable.Combatant, modelskill.Ref) bool    { return a.canCast }

func (a *scriptedFolkCastAI) Range(modelskill.Ref) int          { return a.castRange }
func (a *scriptedFolkCastAI) StopsMovement(modelskill.Ref) bool { return false }
func (a *scriptedFolkCastAI) SkillType(modelskill.Ref) string   { return "HEAL" }

func (a *scriptedFolkCastAI) Cast(_ attackable.Combatant, ref modelskill.Ref) {
	a.casts = append(a.casts, ref)
	a.control.casting = true
}

// recordingFolkAI records the NPC joining and leaving the AI task.
type recordingFolkAI struct{ adds, removes int }

func (a *recordingFolkAI) Add(task.AIActor)    { a.adds++ }
func (a *recordingFolkAI) Remove(task.AIActor) { a.removes++ }

type recordingSink struct{ events []event.Event }

func (s *recordingSink) Emit(ev event.Event) { s.events = append(s.events, ev) }

func (s *recordingSink) moveToPawns() []event.MoveToPawn {
	var out []event.MoveToPawn
	for _, ev := range s.events {
		if m, ok := ev.(event.MoveToPawn); ok {
			out = append(out, m)
		}
	}
	return out
}

type switchLOS struct{ blocked bool }

func (l *switchLOS) CanSeeActor(int, int, int, float64, int, int, int, float64) bool {
	return !l.blocked
}

// movingTarget is a target on the move.
type movingTarget struct{ *hostileTarget }

func (movingTarget) IsMoving() bool { return true }

// newFolkCastRig builds the rig with its target dist units east of the
// NPC. Every gate passes and the skill's range is 100 until a test says
// otherwise.
func newFolkCastRig(t *testing.T, dist int) *folkCastRig {
	t.Helper()
	inst, err := NewInstance(1, &Template{ID: 31226, TemplateID: 31226, Type: "Folk", Level: 70, HPMax: 2444})
	if err != nil {
		t.Fatal(err)
	}
	f, err := NewFolk(inst, false)
	if err != nil {
		t.Fatal(err)
	}
	r := &folkCastRig{
		f: f, in: sim.NewInline(time.Unix(0, 0)),
		aiTask: &recordingFolkAI{}, events: &recordingSink{}, los: &switchLOS{},
		target: &hostileTarget{id: 2, playable: true},
	}
	r.control = &scriptedFolkControl{f: f}
	r.castAI = &scriptedFolkCastAI{control: r.control, canDesire: true, canCast: true, castRange: 100}
	w := world.New()
	if err := f.Attach(FolkRuntime{World: w, Queue: r.in.NewQueue("folk"), Sink: r.events, AI: r.aiTask, LOS: r.los}); err != nil {
		t.Fatal(err)
	}
	f.SetCaster(r.control, r.castAI)
	w.Spawn(f, 100, 0, 0, 0)
	w.Spawn(r.target, 100+dist, 0, 0, 0)
	return r
}

// desire queues a cast of skill id at target and runs the NPC's queue.
func (r *folkCastRig) desire(target attackable.Combatant, id modelskill.ID, weight float64) {
	r.f.AddCastDesire(target, modelskill.Ref{ID: id, Level: 1}, weight)
	r.in.Run()
}

func (r *folkCastRig) tick(t *testing.T) {
	t.Helper()
	if err := r.f.TickThink(); err != nil {
		t.Fatalf("TickThink() = %v", err)
	}
}

// TestFolkCastWaitsForTargetOutOfReach follows CreatureAI.thinkCast's reach
// gate (CreatureMove.maybeStartOffensiveFollow): a target at the skill's
// range plus both bodies or farther, fifty more while it moves, is not cast
// on, and the desire stays queued for a later tick.
func TestFolkCastWaitsForTargetOutOfReach(t *testing.T) {
	for _, tc := range []struct {
		name     string
		dist     int
		moving   bool
		wantCast bool
	}{
		{"inside range", 99, false, true},
		{"at range", 100, false, false},
		{"past range", 120, false, false},
		{"past range, target moving", 120, true, true},
		{"past moving range", 150, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newFolkCastRig(t, tc.dist)
			var target attackable.Combatant = r.target
			if tc.moving {
				target = movingTarget{r.target}
			}
			r.desire(target, 4380, 1e6)
			r.tick(t)

			if got := len(r.castAI.casts) == 1; got != tc.wantCast {
				t.Fatalf("cast started = %v, want %v", got, tc.wantCast)
			}
			if tc.wantCast {
				return
			}
			if got := r.f.cast.desires.Len(); got != 1 {
				t.Fatalf("desires = %d after waiting, want the desire kept", got)
			}
			if got := r.events.moveToPawns(); len(got) != 0 {
				t.Fatalf("MoveToPawn %+v while waiting, want none", got)
			}
			if r.aiTask.removes != 0 {
				t.Fatal("NPC left the AI task while still holding a desire")
			}
		})
	}
}

// TestFolkCastRefusalTurnsToTarget follows CreatureAI.thinkCast's last
// gate: when canCast refuses, or the NPC cannot see its target, it shows
// MoveToPawn toward the target with their 3D distance, starts no cast and
// keeps the desire.
func TestFolkCastRefusalTurnsToTarget(t *testing.T) {
	for _, tc := range []struct {
		name    string
		canCast bool
		blocked bool
	}{
		{"canCast refuses", false, false},
		{"line of sight blocked", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newFolkCastRig(t, 60)
			r.castAI.canCast, r.los.blocked = tc.canCast, tc.blocked
			r.desire(r.target, 4380, 1e6)
			r.tick(t)

			if len(r.castAI.casts) != 0 {
				t.Fatalf("casts = %v, want none", r.castAI.casts)
			}
			want := event.MoveToPawn{TargetID: r.target.ObjectID(), Distance: 60}
			want.Origin.X = 100
			if got := r.events.moveToPawns(); len(got) != 1 || got[0] != want {
				t.Fatalf("MoveToPawn = %+v, want [%+v]", got, want)
			}
			if got := r.f.cast.desires.Len(); got != 1 {
				t.Fatalf("desires = %d, want the refused desire kept", got)
			}
		})
	}
}

// TestFolkCastDesireRefusedByConditions follows NpcAI.addCastDesire's
// checkConditions: a skill in reuse, or one whose MP or HP cost the NPC
// cannot pay, is never queued, and the NPC does not join the AI task.
func TestFolkCastDesireRefusedByConditions(t *testing.T) {
	r := newFolkCastRig(t, 60)
	r.castAI.canDesire = false
	r.desire(r.target, 4380, 1e6)

	if got := r.f.cast.desires.Len(); got != 0 {
		t.Fatalf("desires = %d, want none queued", got)
	}
	if r.aiTask.adds != 0 {
		t.Fatalf("AI task adds = %d, want 0", r.aiTask.adds)
	}
}

// TestFolkCastDesireDecaysAndLeavesAITask follows NpcAI.runAI's every
// third step decay of 66000 on cast desires (DesireQueue.decreaseWeightByType
// drops a desire the decay would take below zero): a weight-100000 desire
// that is never cast falls to 34000 on the third tick and is dropped on the
// sixth, and with nothing left the NPC leaves the AI task that same tick.
func TestFolkCastDesireDecaysAndLeavesAITask(t *testing.T) {
	r := newFolkCastRig(t, 500)
	r.desire(r.target, 4380, 100000)
	if r.aiTask.adds != 1 {
		t.Fatalf("AI task adds = %d, want 1", r.aiTask.adds)
	}

	weights := map[int]float64{2: 100000, 3: 34000, 5: 34000}
	for i := 1; i <= 5; i++ {
		r.tick(t)
		d, ok := r.f.cast.desires.Peek()
		if !ok {
			t.Fatalf("tick %d: desire dropped early", i)
		}
		if want, check := weights[i]; check && d.Weight != want {
			t.Fatalf("tick %d: weight = %v, want %v", i, d.Weight, want)
		}
		if r.aiTask.removes != 0 {
			t.Fatalf("tick %d: NPC left the AI task while holding a desire", i)
		}
	}
	r.tick(t)
	if got := r.f.cast.desires.Len(); got != 0 {
		t.Fatalf("desires = %d after the sixth tick, want the spent desire dropped", got)
	}
	if r.aiTask.removes != 1 {
		t.Fatalf("AI task removes = %d, want 1", r.aiTask.removes)
	}
	if len(r.castAI.casts) != 0 {
		t.Fatalf("casts = %v, want none", r.castAI.casts)
	}
}

// TestFolkCastBreakClosesDesireWithoutReselecting follows
// NpcCast.notifyCastFinishToAI: a cast broken by damage closes the desire
// behind it and picks nothing in the same callback; the next one waits for
// the AI tick. A cast that runs to its end closes its desire and picks the
// next one at once.
func TestFolkCastBreakClosesDesireWithoutReselecting(t *testing.T) {
	r := newFolkCastRig(t, 60)
	r.desire(r.target, 1, 300)
	r.desire(r.target, 2, 200)
	r.desire(r.target, 3, 100)

	r.tick(t)
	if got := r.castAI.casts; len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("casts = %v, want skill 1 first", got)
	}

	r.f.BreakCastOnDamage(50)
	if got := r.castAI.casts; len(got) != 1 {
		t.Fatalf("casts = %v after the break, want no recast inside the callback", got)
	}
	if got := r.f.cast.desires.Len(); got != 2 {
		t.Fatalf("desires = %d after the break, want 2", got)
	}
	if d, _ := r.f.cast.desires.Peek(); d.Skill.ID != 2 {
		t.Fatalf("heaviest desire after the break = skill %d, want 2: the broken cast's desire is closed", d.Skill.ID)
	}

	r.tick(t)
	if got := r.castAI.casts; len(got) != 2 || got[1].ID != 2 {
		t.Fatalf("casts = %v, want skill 2 on the next tick", got)
	}
	r.control.finish()
	if got := r.castAI.casts; len(got) != 3 || got[2].ID != 3 {
		t.Fatalf("casts = %v, want skill 3 picked at once after a finished cast", got)
	}
	if got := r.f.cast.desires.Len(); got != 1 {
		t.Fatalf("desires = %d, want only the cast in flight's", got)
	}
}

// TestFolkTemplateHeldMask follows Npc.getActiveWeaponItem and
// getSecondaryWeaponItem as L2Skill's weapon check reads them: a weapon in
// the right hand and any armor in the left contribute their type bits; an
// unknown id, or an item of the wrong kind for its hand, holds nothing.
func TestFolkTemplateHeldMask(t *testing.T) {
	items := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
		{ID: 2, Kind: item.KindArmor, Armor: &item.ArmorDetail{Type: item.ArmorShield}},
	})
	sword, shield := item.WeaponSword.Mask(), item.ArmorShield.Mask()
	for _, tc := range []struct {
		name        string
		right, left int
		items       *item.Table
		want        int32
	}{
		{"sword and shield", 1, 2, items, sword | shield},
		{"sword alone", 1, 0, items, sword},
		{"shield alone", 0, 2, items, shield},
		{"unknown ids", 999, 998, items, 0},
		{"shield in the right hand", 2, 0, items, 0},
		{"sword in the left hand", 0, 1, items, 0},
		{"no item table", 1, 2, nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := templateHeldMask(&Template{RightHand: tc.right, LeftHand: tc.left}, tc.items)
			if got != tc.want {
				t.Fatalf("templateHeldMask = %b, want %b", got, tc.want)
			}
		})
	}
}
