package cast

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/skill/skilltest"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	modelactor "github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ---- from effects_test.go ----
// effectsActor is a minimal skilltarget.Actor usable as a non-player
// caster, proving the resolution path ApplyEffects drives doesn't require
// the live-player packet-handling type the player cast flow uses.
type effectsActor struct {
	world.Presence
	skilltest.Creature
	id      int32
	x, y, z int
	kind    modelactor.Kind
	dead    bool
	corpse  bool
	monster bool
	summon  *effectsActor
}

type pvpEffectsActor struct {
	effectsActor
	calls []pvpSkillCall
}

type pvpSkillCall struct {
	targets   []attackable.Combatant
	offensive bool
	skillType string
}

func (a *pvpEffectsActor) NotePvPSkillTargets(targets []attackable.Combatant, offensive bool, skillType string) {
	a.calls = append(a.calls, pvpSkillCall{targets: append([]attackable.Combatant(nil), targets...), offensive: offensive, skillType: skillType})
}

type cursePvpEffectsActor struct {
	pvpEffectsActor
	block bool
}

func (a *cursePvpEffectsActor) TestCursesOnSkillSee(modelskill.Definition, []skilltarget.Actor) bool {
	return a.block
}

func (a *effectsActor) ObjectID() int32 { return a.id }

func (a *effectsActor) Position() (int, int, int) { return a.x, a.y, a.z }

func (a *effectsActor) Heading() int { return 0 }

func (a *effectsActor) Dead() bool { return a.dead }

func (a *effectsActor) Kind() modelactor.Kind { return a.kind }

func (a *effectsActor) AttackableBy(skilltarget.Actor) bool { return true }

func (a *effectsActor) AttackableWithoutForceBy(skilltarget.Actor) bool { return true }

func (a *effectsActor) HasCorpse() bool { return a.corpse }

func (a *effectsActor) MonsterKind() bool { return a.monster }

func (a *effectsActor) Summon() (skilltarget.Actor, bool) {
	if a.summon == nil {
		return nil, false
	}
	return a.summon, true
}

func TestApplyEffectsResultCarriesSkillHandlerAttackFailed(t *testing.T) {
	caster := &effectsActor{id: 1, kind: modelactor.KindPlayer}
	rec := &recordingSkillHandler{result: handlerskill.Result{AttackFailed: 2}}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 99, Target: modelskill.TargetSelf, SkillType: "DUMMY"}

	result := ApplyEffectsResult(handlers, caster, caster, def)
	if !result.Handled {
		t.Fatal("ApplyEffectsResult() handled = false, want true")
	}
	if result.AttackFailed != 2 {
		t.Fatalf("AttackFailed = %d, want 2", result.AttackFailed)
	}
}

func TestApplyEffectsResultCarriesCubicTargets(t *testing.T) {
	caster := &effectsActor{id: 1, kind: modelactor.KindPlayer}
	other := &effectsActor{id: 2, kind: modelactor.KindPlayer}
	rec := &recordingSkillHandler{result: handlerskill.Result{
		CubicTargets:      []handlerskill.Actor{other},
		CubicAddedTargets: []handlerskill.Actor{other},
	}}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 99, Target: modelskill.TargetSelf, SkillType: "DUMMY"}

	result := ApplyEffectsResult(handlers, caster, caster, def)
	if got := result.CubicTargets; len(got) != 1 || got[0] != other {
		t.Fatalf("CubicTargets = %v, want other", got)
	}
	if got := result.CubicAddedTargets; len(got) != 1 || got[0] != other {
		t.Fatalf("CubicAddedTargets = %v, want other", got)
	}
}

func TestApplyEffectsResultCarriesSkillHandlerCounterattack(t *testing.T) {
	caster := &effectsActor{id: 1, kind: modelactor.KindPlayer}
	rec := &recordingSkillHandler{result: handlerskill.Result{Counterattacks: []handlerskill.Counterattack{{
		AttackerID: 1, AttackerName: "Attacker", DefenderID: 2, DefenderName: "Defender",
	}}}}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 99, Target: modelskill.TargetSelf, SkillType: "DUMMY"}

	result := ApplyEffectsResult(handlers, caster, caster, def)
	if got := result.Counterattacks; len(got) != 1 || got[0].AttackerName != "Attacker" || got[0].DefenderName != "Defender" {
		t.Fatalf("Counterattacks = %+v, want attacker and defender", got)
	}
}

func TestApplyEffectsNotifiesPvPStatusBeforeSkillHandling(t *testing.T) {
	caster := &pvpEffectsActor{effectsActor: effectsActor{id: 1, kind: modelactor.KindPlayer}}
	target := &effectsActor{id: 2, kind: modelactor.KindPlayer}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 100, Target: modelskill.TargetOne, Offensive: true, SkillType: "DUMMY"}

	if !ApplyEffects(handlers, caster, target, def) {
		t.Fatal("ApplyEffects() = false, want true")
	}
	if len(caster.calls) != 1 {
		t.Fatalf("PvP status calls = %d, want 1", len(caster.calls))
	}
	call := caster.calls[0]
	if !call.offensive || call.skillType != "DUMMY" || len(call.targets) != 1 || call.targets[0] != attackable.Combatant(target) {
		t.Fatalf("PvP status call = %+v, want offensive selected target", call)
	}
}

func TestApplyEffectsAbortsWhenPlayableSkillSeeCurseBlocks(t *testing.T) {
	caster := &cursePvpEffectsActor{
		pvpEffectsActor: pvpEffectsActor{effectsActor: effectsActor{id: 1, kind: modelactor.KindPlayer}},
		block:           true,
	}
	target := &effectsActor{id: 2, kind: modelactor.KindNPC}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 100, Target: modelskill.TargetOne, Offensive: true, SkillType: "DUMMY"}

	if ApplyEffects(handlers, caster, target, def) {
		t.Fatal("ApplyEffects() = true, want false after skill-see curse")
	}
	if len(rec.calls) != 0 {
		t.Fatalf("skill handler calls = %d, want 0", len(rec.calls))
	}
	if len(caster.calls) != 0 {
		t.Fatalf("PvP status calls = %d, want 0", len(caster.calls))
	}
}

func TestApplyEffectsContinuesWhenPlayableSkillSeeCurseAllows(t *testing.T) {
	caster := &cursePvpEffectsActor{
		pvpEffectsActor: pvpEffectsActor{effectsActor: effectsActor{id: 1, kind: modelactor.KindPlayer}},
	}
	target := &effectsActor{id: 2, kind: modelactor.KindNPC}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 100, Target: modelskill.TargetOne, Offensive: true, SkillType: "DUMMY"}

	if !ApplyEffects(handlers, caster, target, def) {
		t.Fatal("ApplyEffects() = false, want true")
	}
	if len(rec.calls) != 1 {
		t.Fatalf("skill handler calls = %d, want 1", len(rec.calls))
	}
}

func TestApplyEffectsToggleSkipsSkillSeeCurse(t *testing.T) {
	caster := &cursePvpEffectsActor{
		pvpEffectsActor: pvpEffectsActor{effectsActor: effectsActor{id: 1, kind: modelactor.KindPlayer}},
		block:           true,
	}
	target := &effectsActor{id: 2, kind: modelactor.KindPlayer}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 100, Target: modelskill.TargetOne, Activation: modelskill.ActivationToggle, SkillType: "DUMMY"}

	if !ApplyEffects(handlers, caster, target, def) {
		t.Fatal("ApplyEffects(toggle) = false, want true")
	}
	if len(rec.calls) != 1 {
		t.Fatalf("skill handler calls = %d, want 1", len(rec.calls))
	}
}

func TestApplyEffectsAreaTargetReachesEveryAffectedCreature(t *testing.T) {
	caster := &effectsActor{id: 1, kind: modelactor.KindNPC}
	selected := &effectsActor{id: 2, x: 10, kind: modelactor.KindPlayer}
	bystander := &effectsActor{id: 3, x: 20, kind: modelactor.KindPlayer}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{caster, selected, bystander}, "DUMMY", rec)
	def := modelskill.Definition{ID: 100, Target: modelskill.TargetArea, Offensive: true, Radius: 900, SkillType: "DUMMY"}

	if !ApplyEffects(handlers, caster, selected, def) {
		t.Fatal("ApplyEffects(area) = false, want true")
	}
	if len(rec.calls) != 1 {
		t.Fatalf("skill handler calls = %d, want 1", len(rec.calls))
	}
	if got := rec.calls[0].Caster; got != any(caster) {
		t.Fatalf("recorded caster = %v, want %v", got, caster)
	}
	if len(rec.calls[0].Targets) != 2 {
		t.Fatalf("recorded targets = %d, want 2 (selected + bystander)", len(rec.calls[0].Targets))
	}
}

func TestApplyEffectsAuraTargetSweepsRadiusAroundCaster(t *testing.T) {
	caster := &effectsActor{id: 1, kind: modelactor.KindNPC}
	nearby := &effectsActor{id: 2, kind: modelactor.KindPlayer}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{caster, nearby}, "DUMMY", rec)
	def := modelskill.Definition{ID: 101, Target: modelskill.TargetAura, Radius: 300, SkillType: "DUMMY"}

	// Aura skills have no selected target: the caster is both the anchor
	// and the resolved target.
	if !ApplyEffects(handlers, caster, nil, def) {
		t.Fatal("ApplyEffects(aura) = false, want true")
	}
	if len(rec.calls) != 1 || len(rec.calls[0].Targets) != 1 {
		t.Fatalf("recorded call = %+v, want one call with one target", rec.calls)
	}
	if rec.calls[0].Targets[0] != any(nearby) {
		t.Fatalf("recorded target = %v, want %v", rec.calls[0].Targets[0], nearby)
	}
}

func TestApplyEffectsCorpseMobTargetRequiresPendingCorpse(t *testing.T) {
	caster := &effectsActor{id: 1, kind: modelactor.KindPlayer}
	corpse := &effectsActor{id: 2, kind: modelactor.KindNPC, dead: true, corpse: true, monster: true}
	live := &effectsActor{id: 3, kind: modelactor.KindNPC, corpse: false}
	def := modelskill.Definition{ID: 102, Target: modelskill.TargetCorpseMob, SkillType: "SWEEP"}

	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{}, "SWEEP", rec)
	if !ApplyEffects(handlers, caster, corpse, def) {
		t.Fatal("ApplyEffects(corpse mob, has corpse) = false, want true")
	}
	if len(rec.calls) != 1 || len(rec.calls[0].Targets) != 1 || rec.calls[0].Targets[0] != any(corpse) {
		t.Fatalf("recorded call = %+v, want one call targeting the corpse", rec.calls)
	}

	rec2 := &recordingSkillHandler{}
	handlers2 := newEffectHandlers(effectsKnown{}, "SWEEP", rec2)
	if ApplyEffects(handlers2, caster, live, def) {
		t.Fatal("ApplyEffects(corpse mob, no corpse) = true, want false")
	}
	if len(rec2.calls) != 0 {
		t.Fatalf("skill handler calls = %d, want 0 for a target with no corpse", len(rec2.calls))
	}
}

func TestApplyEffectsSummonTargetResolvesCasterOwnedSummon(t *testing.T) {
	summon := &effectsActor{id: 2, kind: modelactor.KindPlayer}
	caster := &effectsActor{id: 1, kind: modelactor.KindPlayer, summon: summon}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 103, Target: modelskill.TargetSummon, SkillType: "DUMMY"}

	if !ApplyEffects(handlers, caster, nil, def) {
		t.Fatal("ApplyEffects(summon) = false, want true")
	}
	if len(rec.calls) != 1 || len(rec.calls[0].Targets) != 1 || rec.calls[0].Targets[0] != any(summon) {
		t.Fatalf("recorded call = %+v, want one call targeting the caster's summon", rec.calls)
	}

	// A caster without a summon must not reach the skill handler at all.
	rec2 := &recordingSkillHandler{}
	handlers2 := newEffectHandlers(effectsKnown{}, "DUMMY", rec2)
	summonless := &effectsActor{id: 4, kind: modelactor.KindPlayer}
	if ApplyEffects(handlers2, summonless, nil, def) {
		t.Fatal("ApplyEffects(summon, no summon) = true, want false")
	}
	if len(rec2.calls) != 0 {
		t.Fatalf("skill handler calls = %d, want 0 for a caster without a summon", len(rec2.calls))
	}
}

func TestApplyEffectsUnresolvedTargetTypeIsNoop(t *testing.T) {
	caster := &effectsActor{id: 1, kind: modelactor.KindPlayer}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 104, Target: modelskill.TargetEnemySummon, SkillType: "DUMMY"}

	if ApplyEffects(handlers, caster, nil, def) {
		t.Fatal("ApplyEffects(unregistered target type) = true, want false")
	}
	if len(rec.calls) != 0 {
		t.Fatalf("skill handler calls = %d, want 0", len(rec.calls))
	}
}

func TestApplyEffectsNilRegistriesAreNoop(t *testing.T) {
	caster := &effectsActor{id: 1, kind: modelactor.KindPlayer}
	def := modelskill.Definition{ID: 105, Target: modelskill.TargetSelf, SkillType: "DUMMY"}

	if ApplyEffects(EffectHandlers{}, caster, nil, def) {
		t.Fatal("ApplyEffects(zero-value handlers) = true, want false")
	}
	if ApplyEffects(EffectHandlers{}, nil, nil, def) {
		t.Fatal("ApplyEffects(nil caster) = true, want false")
	}
}

// nonCreatureSelection is a world.Tracked value that does not satisfy
// skilltarget.Actor — a door, static object, or similar world object a
// player can select but that carries no combat-relevant state.
type nonCreatureSelection struct {
	world.Presence
	id int32
}

func (s *nonCreatureSelection) ObjectID() int32 { return s.id }

func (*nonCreatureSelection) Kind() modelactor.Kind { return modelactor.KindStatic }

var (
	_ world.Tracked = (*nonCreatureSelection)(nil)
	_ Target        = (*nonCreatureSelection)(nil)
)

// TestApplyEffectsRejectsNonCreatureSelection pins the quirk #1502 preserves:
// typing Selected to world.Tracked doesn't tighten what TargetOne admits at
// selection time (a door still satisfies cast.Target, so SelectTarget still
// accepts it), but resolveAffected's skilltarget.Actor narrowing still
// rejects it before any skill handler runs — the same branch as before the
// caster/selection types were tightened.
func TestApplyEffectsRejectsNonCreatureSelection(t *testing.T) {
	caster := &effectsActor{id: 1, kind: modelactor.KindPlayer}
	door := &nonCreatureSelection{id: 2}
	rec := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", rec)
	def := modelskill.Definition{ID: 106, Target: modelskill.TargetOne, SkillType: "DUMMY"}

	if _, ok := SelectTarget(caster, door, def); !ok {
		t.Fatal("SelectTarget(door) ok = false, want true: a non-creature Tracked value still selects")
	}
	if ApplyEffects(handlers, caster, door, def) {
		t.Fatal("ApplyEffects(door selection) = true, want false")
	}
	if len(rec.calls) != 0 {
		t.Fatalf("skill handler calls = %d, want 0 (door never reaches a skill handler)", len(rec.calls))
	}
}

type castDoorShape struct{}

func (castDoorShape) GeoX() int { return 0 }

func (castDoorShape) GeoY() int { return 0 }

func (castDoorShape) GeoZ() int { return 0 }

func (castDoorShape) Height() int { return 1 }

func (castDoorShape) GeoData() [][]block.NSWE { return [][]block.NSWE{{block.AllDirections}} }

type castHostileMove struct{}

func (castHostileMove) MaybeStartOffensiveFollow(attackable.Combatant, int) (bool, error) {
	return false, nil
}

func (castHostileMove) MoveHome(location.Location) error { return nil }

func (castHostileMove) Stop() {}

func (castHostileMove) CancelFollow() {}

type castHostileAttack struct{}

func (castHostileAttack) BowCoolingDown() bool { return false }

func (castHostileAttack) AttackingNow() bool { return false }

func (castHostileAttack) CanAttack(attackable.Combatant) bool { return false }

func (castHostileAttack) DoAttack(attackable.Combatant) {}

func (castHostileAttack) Stop() {}

func newCastHostile(t *testing.T, id int32, kind string) *npc.Hostile {
	t.Helper()
	live, err := creature.NewLive(location.Location{}, 100, permissiveGeo{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	live.SetQueue(idleQueue())
	hostile, err := npc.NewHostile(&npc.Instance{ObjectID: id, Template: &npc.Template{ID: int(id), Type: kind}, Kind: npc.InstanceKind(kind)}, live, castHostileMove{}, castHostileAttack{})
	if err != nil {
		t.Fatal(err)
	}
	return hostile
}

func TestApplyEffectsAcceptsOnlyUnlockableRuntimeTargets(t *testing.T) {
	caster := &effectsActor{id: 1, kind: modelactor.KindPlayer}
	recorder := &recordingSkillHandler{}
	handlers := newEffectHandlers(effectsKnown{}, "DUMMY", recorder)

	unlockableDoor, err := door.NewObject(2, &door.Template{ID: 2, OpenKind: door.OpenSkill}, castDoorShape{})
	if err != nil {
		t.Fatal(err)
	}
	lockedDoor, err := door.NewObject(3, &door.Template{ID: 3, OpenKind: door.OpenClick}, castDoorShape{})
	if err != nil {
		t.Fatal(err)
	}
	chest := newCastHostile(t, 4, "Chest")
	monster := newCastHostile(t, 5, "Monster")
	for _, tc := range []struct {
		name   string
		target world.Tracked
		def    modelskill.Definition
		want   bool
	}{
		{"skill door", unlockableDoor, modelskill.Definition{Target: modelskill.TargetUnlockable, SkillType: "DUMMY"}, true},
		{"click door rejected", lockedDoor, modelskill.Definition{Target: modelskill.TargetUnlockable, SkillType: "DUMMY"}, false},
		{"chest", chest, modelskill.Definition{Target: modelskill.TargetUnlockable, SkillType: "DUMMY"}, true},
		{"monster rejected", monster, modelskill.Definition{Target: modelskill.TargetUnlockable, SkillType: "DUMMY"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected, ok := SelectTarget(caster, tc.target, tc.def)
			if !ok {
				t.Fatal("SelectTarget() ok = false, want true")
			}
			if got := ApplyEffects(handlers, caster, selected, tc.def); got != tc.want {
				t.Fatalf("ApplyEffects() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTargetRejectionsDistinguishInvalidTargetsFromLockedDoors(t *testing.T) {
	caster := &effectsActor{id: 1, kind: modelactor.KindPlayer}
	skillDoor, err := door.NewObject(2, &door.Template{ID: 2, OpenKind: door.OpenSkill}, castDoorShape{})
	if err != nil {
		t.Fatal(err)
	}
	lockedDoor, err := door.NewObject(3, &door.Template{ID: 3, OpenKind: door.OpenClick}, castDoorShape{})
	if err != nil {
		t.Fatal(err)
	}
	monster := newCastHostile(t, 4, "Monster")

	if got := skilltarget.CastRejectionFor(modelskill.TargetOne, caster, skillDoor, &modelskill.Definition{Offensive: true}, false); got != skilltarget.CastRejectInvalidTarget {
		t.Fatalf("TargetOne skill door rejection = %v, want invalid target", got)
	}
	if got := skilltarget.CastRejectionFor(modelskill.TargetUnlockable, caster, lockedDoor, &modelskill.Definition{}, false); got != skilltarget.CastRejectSilent {
		t.Fatalf("locked door rejection = %v, want silent", got)
	}
	if got := skilltarget.CastRejectionFor(modelskill.TargetUnlockable, caster, nil, &modelskill.Definition{}, false); got != skilltarget.CastRejectNone {
		t.Fatalf("nil unlockable rejection = %v, want none", got)
	}
	if got := skilltarget.CastRejectionFor(modelskill.TargetUnlockable, caster, monster, &modelskill.Definition{}, false); got != skilltarget.CastRejectInvalidTarget {
		t.Fatalf("monster unlockable rejection = %v, want invalid target", got)
	}
	if got := skilltarget.CastRejectionFor(modelskill.TargetHoly, caster, monster, &modelskill.Definition{}, false); got != skilltarget.CastRejectInvalidTarget {
		t.Fatalf("monster holy rejection = %v, want invalid target", got)
	}
}

func (castHostileMove) CanMoveTo(location.Location) bool { return true }

func (castHostileMove) MoveToLocation(location.Location) (bool, error) { return false, nil }

func (*effectsActor) NotePvPSkillTargets([]attackable.Combatant, bool, string) {}

func (*effectsActor) TestCursesOnSkillSee(modelskill.Definition, []skilltarget.Actor) bool {
	return false
}
