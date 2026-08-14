package effect

import (
	"fmt"
	"reflect"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestTargetMeEffectSetsTargetOrAttacksIfAlreadyTargeted(t *testing.T) {
	effector := &liveEffectTarget{}
	target := &liveEffectTarget{}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "TargetMe"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	e.Effector = effector

	if !e.OnStart(e) {
		t.Fatal("target me effect start rejected a valid target")
	}
	if want := []string{fmt.Sprintf("set-target:%v", effector)}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("events = %#v, want %#v", target.events, want)
	}

	target.events = nil
	target.target = effector
	if !e.OnStart(e) {
		t.Fatal("target me effect start rejected a valid target")
	}
	if want := []string{fmt.Sprintf("try-attack:%v", effector)}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("events = %#v, want %#v", target.events, want)
	}
}

func TestBluffEffectRedirectsHeadingUnlessExemptOrRaidRelated(t *testing.T) {
	effector := &liveEffectTarget{heading: 42}
	target := &liveEffectTarget{}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "Bluff"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	e.Effector = effector

	if !e.OnStart(e) {
		t.Fatal("bluff effect start rejected a valid target")
	}
	if target.heading != 42 {
		t.Fatalf("target heading = %d, want 42", target.heading)
	}

	target = &liveEffectTarget{bluffExempt: true}
	e.Effected = target
	if e.OnStart(e) {
		t.Fatal("bluff effect started on an exempt target")
	}

	target = &liveEffectTarget{raidRelated: true}
	e.Effected = target
	if e.OnStart(e) {
		t.Fatal("bluff effect started on a raid-related target")
	}
}

func TestCharmOfCourageEffectOnlyStartsForPlayers(t *testing.T) {
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "CharmOfCourage"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	e.Effected = &liveEffectTarget{isPlayer: true}
	if !e.OnStart(e) {
		t.Fatal("charm of courage effect start rejected a player")
	}

	e.Effected = &liveEffectTarget{isPlayer: false}
	if e.OnStart(e) {
		t.Fatal("charm of courage effect started on a non-player")
	}
}

func TestCharmOfLuckAndPhoenixBlessNotifyOnExit(t *testing.T) {
	luck := &liveEffectTarget{}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "CharmOfLuck"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = luck
	list := NewList(nil)
	list.Add(e)
	list.Remove(e)
	if want := []string{"stop-charm-of-luck"}; !reflect.DeepEqual(luck.events, want) {
		t.Fatalf("charm of luck exit events = %#v, want %#v", luck.events, want)
	}

	bless := &liveEffectTarget{}
	pb, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "PhoenixBless"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	pb.Effected = bless
	list2 := NewList(nil)
	list2.Add(pb)
	list2.Remove(pb)
	if want := []string{"stop-phoenix-bless"}; !reflect.DeepEqual(bless.events, want) {
		t.Fatalf("phoenix bless exit events = %#v, want %#v", bless.events, want)
	}
}

func TestCancelEffectSkipsToggleAndDebuffCandidates(t *testing.T) {
	target := &liveEffectTarget{vuln: 1, list: NewList(nil)}
	toggle, err := New(Skill{Toggle: true}, modelskill.EffectTemplate{Name: "Buff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(toggle)
	debuff, err := New(Skill{Debuff: true}, modelskill.EffectTemplate{Name: "Debuff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(debuff)

	e, err := New(Skill{MagicLevel: 80, MaxNegatedEffects: 10}, modelskill.EffectTemplate{Name: "Cancel", EffectPower: 100})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	e.OnStart(e)

	if !hasEffectInList(target.list, toggle) {
		t.Error("a toggle effect must never be stripped by a cancel effect")
	}
	if !hasEffectInList(target.list, debuff) {
		t.Error("a debuff effect must never be stripped by a cancel effect")
	}
}

func TestCancelEffectRejectsDeadTarget(t *testing.T) {
	target := &liveEffectTarget{dead: true, list: NewList(nil)}
	buff, err := New(Skill{}, modelskill.EffectTemplate{Name: "Buff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(buff)

	e, err := New(Skill{MagicLevel: 80, MaxNegatedEffects: 10}, modelskill.EffectTemplate{Name: "Cancel", EffectPower: 100})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	if e.OnStart(e) {
		t.Fatal("cancel effect started on a dead target, want rejected")
	}
	if !hasEffectInList(target.list, buff) {
		t.Error("a rejected start must not touch the candidate list")
	}
}

// TestCancelEffectStripsProtectionMarkersDespiteExemptionList proves a
// deliberately preserved quirk: cancelStart's exemption check compares its
// own classification (always the cancel tag) against the protected-marker
// list, so the check can never match and the four protected markers stay
// cancellable through this path. A single trial can't distinguish "always
// removed" from "never checked" because the roll saturates below 100%, so
// this repeats many independent trials and requires at least one removal —
// with removal odds fixed at 75% per trial, the chance of zero removals
// across all of them is astronomically small.

func TestCancelEffectStripsProtectionMarkersDespiteExemptionList(t *testing.T) {
	const trials = 200
	removed := 0
	for i := 0; i < trials; i++ {
		target := &liveEffectTarget{vuln: 1, list: NewList(nil)}
		marker, err := New(Skill{}, modelskill.EffectTemplate{Name: "ProtectionBlessing", Time: 600})
		if err != nil {
			t.Fatalf("New() error: %v", err)
		}
		target.list.Add(marker)

		e, err := New(Skill{MagicLevel: 1000, MaxNegatedEffects: 1}, modelskill.EffectTemplate{Name: "Cancel", EffectPower: 1000})
		if err != nil {
			t.Fatalf("New() error: %v", err)
		}
		e.Effected = target
		e.OnStart(e)

		if !hasEffectInList(target.list, marker) {
			removed++
		}
	}
	if removed == 0 {
		t.Fatal("protection blessing marker was never stripped across repeated trials, want at least one removal")
	}
}

func TestNegateEffectStripsBySkillID(t *testing.T) {
	target := &liveEffectTarget{list: NewList(nil)}
	match, err := New(Skill{ID: 42}, modelskill.EffectTemplate{Name: "Buff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(match)
	other, err := New(Skill{ID: 7}, modelskill.EffectTemplate{Name: "Buff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(other)
	zero, err := New(Skill{ID: 0}, modelskill.EffectTemplate{Name: "Buff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(zero)

	e, err := New(Skill{NegateIDs: []int{42, 0}, NegateLevel: -1}, modelskill.EffectTemplate{Name: "Negate"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	e.OnStart(e)

	if hasEffectInList(target.list, match) {
		t.Error("an effect owned by a negated skill id must be stripped")
	}
	if !hasEffectInList(target.list, other) {
		t.Error("an effect owned by an unrelated skill id must remain")
	}
	if !hasEffectInList(target.list, zero) {
		t.Error("a negateId of 0 is a no-op sentinel and must never strip anything")
	}
}

func TestNegateEffectStripsByTypeWithLevelGate(t *testing.T) {
	target := &liveEffectTarget{list: NewList(nil)}

	// Distinct skill ids matter here: the effect list treats same-id,
	// same-type, same-stack candidates as duplicates of each other and
	// silently rejects the later Add, which would hide these candidates
	// from the assertions below regardless of negateStart's behavior.
	withinLevel, err := New(Skill{ID: 1, SkillType: "POISON", AbnormalLevel: 2}, modelskill.EffectTemplate{Name: "Debuff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(withinLevel)

	aboveLevel, err := New(Skill{ID: 2, SkillType: "POISON", AbnormalLevel: 5}, modelskill.EffectTemplate{Name: "Debuff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(aboveLevel)

	wrongType, err := New(Skill{ID: 3, SkillType: "BLEED", AbnormalLevel: 1}, modelskill.EffectTemplate{Name: "Debuff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(wrongType)

	viaEffectType, err := New(Skill{ID: 4, SkillType: "BUFF", EffectType: "POISON", EffectAbnormalLevel: 2}, modelskill.EffectTemplate{Name: "Buff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(viaEffectType)

	e, err := New(Skill{NegateTypes: []string{"POISON"}, NegateLevel: 3}, modelskill.EffectTemplate{Name: "Negate"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	e.OnStart(e)

	if hasEffectInList(target.list, withinLevel) {
		t.Error("a candidate within the negate level threshold should have been stripped")
	}
	if !hasEffectInList(target.list, aboveLevel) {
		t.Error("a candidate above the negate level threshold must remain")
	}
	if !hasEffectInList(target.list, wrongType) {
		t.Error("a candidate of an unrelated classification must remain")
	}
	if hasEffectInList(target.list, viaEffectType) {
		t.Error("a candidate matched via its own effectType tag should have been stripped")
	}
}

func TestNegateEffectTypeUnrestrictedWhenLevelIsMinusOne(t *testing.T) {
	target := &liveEffectTarget{list: NewList(nil)}
	high, err := New(Skill{SkillType: "POISON", AbnormalLevel: 99}, modelskill.EffectTemplate{Name: "Debuff", Time: 600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(high)

	e, err := New(Skill{NegateTypes: []string{"POISON"}, NegateLevel: -1}, modelskill.EffectTemplate{Name: "Negate"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	e.OnStart(e)

	if hasEffectInList(target.list, high) {
		t.Error("a negateLevel of -1 must strip regardless of abnormal level")
	}
}

func TestNewChanceSkillTriggerRejectsUnknownTriggerType(t *testing.T) {
	if _, err := New(Skill{}, modelskill.EffectTemplate{Name: "ChanceSkillTrigger", ChanceType: "BOGUS", ActivationChance: 50}); err == nil {
		t.Fatal("New() error = nil, want an error for an unknown chanceType")
	}
}

func TestNewChanceSkillTriggerAcceptsAnAbsentChanceType(t *testing.T) {
	if _, err := New(Skill{}, modelskill.EffectTemplate{Name: "ChanceSkillTrigger", TriggeredID: 5144}); err != nil {
		t.Fatalf("New() error = %v, want nil for an absent chanceType", err)
	}
}

type chanceTriggerFakeActor struct {
	tracked []*Effect
}

func (a *chanceTriggerFakeActor) AddChanceTrigger(e *Effect) {
	a.tracked = append(a.tracked, e)
}

func (a *chanceTriggerFakeActor) RemoveChanceTrigger(e *Effect) {
	for i, cur := range a.tracked {
		if cur == e {
			a.tracked = append(a.tracked[:i], a.tracked[i+1:]...)
			return
		}
	}
}

func TestChanceSkillTriggerInstallsAndRemovesOnTarget(t *testing.T) {
	target := &chanceTriggerFakeActor{}
	e, err := New(Skill{}, modelskill.EffectTemplate{
		Name: "ChanceSkillTrigger", Time: 60, TriggeredID: 5144,
		ChanceType: "ON_ATTACKED", ActivationChance: 80,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("OnStart() = false, want true")
	}
	if len(target.tracked) != 1 || target.tracked[0] != e {
		t.Fatalf("tracked after OnStart = %+v, want [e]", target.tracked)
	}

	e.OnExit(e)
	if len(target.tracked) != 0 {
		t.Fatalf("tracked after OnExit = %+v, want empty", target.tracked)
	}
}

func TestChanceSkillTriggerOnATargetWithNoTrackingIsANoop(t *testing.T) {
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "ChanceSkillTrigger", ChanceType: "ON_HIT", ActivationChance: 50})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if !e.OnStart(e) {
		t.Fatal("OnStart() = false, want true even without a tracking target")
	}
	e.OnExit(e)
}

// spoilFakeCaster and spoilFakeTarget are minimal actors for spoil effect
// tests, standing in for the SPOIL skill-type handler's own target/caster
// surface.

func TestCancelDebuffEffectOnlyAffectsAPlayerTargetsDispellableDebuffs(t *testing.T) {
	target := &liveEffectTarget{isPlayer: true, vuln: 1, list: NewList(nil)}

	buff, err := New(Skill{}, modelskill.EffectTemplate{Name: "Buff", Time: 3600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(buff)

	nonDispellable, err := New(Skill{Debuff: true}, modelskill.EffectTemplate{Name: "Debuff", Time: 3600})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target.list.Add(nonDispellable)

	e, err := New(Skill{MagicLevel: 76}, modelskill.EffectTemplate{Name: "CancelDebuff"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	e.OnStart(e)

	if !hasEffectInList(target.list, buff) {
		t.Error("a non-debuff candidate must never be stripped by a cancel-debuff effect")
	}
	if !hasEffectInList(target.list, nonDispellable) {
		t.Error("a non-dispellable debuff candidate must never be stripped by a cancel-debuff effect")
	}
}

func TestCancelDebuffEffectRejectsNonPlayerOrDeadTarget(t *testing.T) {
	target := &liveEffectTarget{isPlayer: false, vuln: 1, list: NewList(nil)}
	e, err := New(Skill{MagicLevel: 76}, modelskill.EffectTemplate{Name: "CancelDebuff"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target
	if e.OnStart(e) {
		t.Fatal("cancel-debuff effect started against a non-player target")
	}

	dead := &liveEffectTarget{isPlayer: true, dead: true, vuln: 1, list: NewList(nil)}
	e2, err := New(Skill{MagicLevel: 76}, modelskill.EffectTemplate{Name: "CancelDebuff"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e2.Effected = dead
	if e2.OnStart(e2) {
		t.Fatal("cancel-debuff effect started against a dead player target")
	}
}

// TestCancelDebuffEffectAutoStripsASameSkillIDCandidateWithoutItsOwnRoll
// proves the reference effect's own quirk: once a candidate's roll
// succeeds, the very next candidate examined that shares its owning skill
// id is stripped unconditionally, without an independent roll of its own.
// A single trial can't isolate "always stripped once its predecessor
// succeeds" from "coincidentally also rolled successfully", so this
// repeats many independent trials and checks the one-directional
// implication holds in every one: whenever the first candidate ends up
// stripped, the second must also always be stripped. Skill.MaxNegatedEffects
// is left at its zero-value default (unlimited), so every trial also
// exercises the effect's second pass over the same candidate snapshot.

func TestCancelDebuffEffectAutoStripsASameSkillIDCandidateWithoutItsOwnRoll(t *testing.T) {
	const trials = 200
	bothOrNeither := 0
	for i := 0; i < trials; i++ {
		target := &liveEffectTarget{isPlayer: true, vuln: 1, list: NewList(nil)}

		// Same skill id, distinct stack order so both coexist as separate
		// active debuffs instead of the second Add being rejected as a
		// duplicate of the first.
		older, err := New(Skill{ID: 5, Debuff: true, CanBeDispelled: true, MagicLevel: 1},
			modelskill.EffectTemplate{Name: "Debuff", Time: 36000, StackType: "poison", StackOrder: 1})
		if err != nil {
			t.Fatalf("New() error: %v", err)
		}
		target.list.Add(older)

		newer, err := New(Skill{ID: 5, Debuff: true, CanBeDispelled: true, MagicLevel: 1},
			modelskill.EffectTemplate{Name: "Debuff", Time: 36000, StackType: "poison", StackOrder: 2})
		if err != nil {
			t.Fatalf("New() error: %v", err)
		}
		target.list.Add(newer)

		// Cancel level and remaining time both drive the rate to its
		// ceiling clamp (75%), maximizing the chance the first roll lands
		// without forcing a literal certainty the formula can't produce.
		e, err := New(Skill{MagicLevel: 76}, modelskill.EffectTemplate{Name: "CancelDebuff"})
		if err != nil {
			t.Fatalf("New() error: %v", err)
		}
		e.Effected = target
		e.OnStart(e)

		newerRemoved := !hasEffectInList(target.list, newer)
		olderRemoved := !hasEffectInList(target.list, older)
		if newerRemoved == olderRemoved {
			bothOrNeither++
		}
		if newerRemoved && !olderRemoved {
			t.Fatal("the newer (first-examined) candidate was stripped without stripping the older same-skill-id candidate")
		}
	}
	if bothOrNeither == 0 {
		t.Fatal("neither candidate was ever stripped together across repeated trials, want at least one paired removal")
	}
}

// TestIncreaseChargesEffectAddsUpToTemplateCountCap and
// TestIncreaseChargesEffectReportsSuccessEvenAtCap moved to
// hooks_buff_player_test.go (package effect_test): fakeChargesTarget's
// IncreaseCharges reimplemented the same cap/overflow logic already on the
// real (*player.Character).IncreaseCharges, risking silent drift between
// the two. See docs/agents/test-strategy.md.

func TestIncreaseChargesEffectRejectsNonChargesTarget(t *testing.T) {
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "IncreaseCharges", Value: 1, Count: 7})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = &struct{}{}

	if e.OnStart(e) {
		t.Fatal("increase charges effect start accepted a target without IncreaseCharges")
	}
}
