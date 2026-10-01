package skill

import (
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

func TestDisablersSkipsDeadAndUnparalyzedInvulTargets(t *testing.T) {
	registry := NewDefaultRegistry()
	dead := newDisablerFake(1)
	dead.dead = true
	invul := newDisablerFake(2)
	invul.invul = true

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "FAKE_DEATH", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{dead, invul},
	})

	if len(dead.list.All()) != 0 || len(invul.list.All()) != 0 {
		t.Fatal("a dead or unparalyzed-invulnerable target must never receive an effect")
	}
}

func TestDisablersRespectsBlockDebuffForOffensiveSkills(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)
	blocker, err := effect.New(effect.Skill{}, modelskill.EffectTemplate{Name: "Buff", EffectType: "BLOCK_DEBUFF"})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	blocker.Effected = target
	target.list.Add(blocker)

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "FAKE_DEATH", Offensive: true, Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})

	if len(target.list.All()) != 1 {
		t.Fatalf("target under BLOCK_DEBUFF should not receive a new offensive effect, got %d effects", len(target.list.All()))
	}
}

func TestDisablersRespectsBlockDebuffFromRealMarkerEffect(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)

	// A real BlockDebuff marker loaded from the datapack carries no effectType
	// attribute; its debuff immunity is resolved from the runtime kind.
	blocker, err := effect.New(effect.Skill{}, modelskill.EffectTemplate{Name: "BlockDebuff", Time: 600})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	blocker.Effected = target
	target.list.Add(blocker)

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "FAKE_DEATH", Offensive: true, Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})

	if len(target.list.All()) != 1 {
		t.Fatalf("target under BlockDebuff should not receive a new offensive effect, got %d effects", len(target.list.All()))
	}
}

func TestFakeDeathAppliesUnconditionally(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)
	target.successOK = false // even without a success source, FAKE_DEATH doesn't roll

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "FAKE_DEATH", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})
	if len(target.list.All()) != 1 {
		t.Fatal("FAKE_DEATH should apply its effects with no success check")
	}
}

func TestStunAppliesOnGuaranteedSuccess(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "STUN", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})
	if len(target.list.All()) != 1 {
		t.Fatal("STUN should apply its effect on a guaranteed-success roll")
	}
}

func TestControlDisablersReportFailedRollToPlayer(t *testing.T) {
	for _, skillType := range []string{"ROOT", "STUN", "SLEEP", "PARALYZE", "MUTE"} {
		t.Run(skillType, func(t *testing.T) {
			caster := &skillTarget{isPlayer: true}
			target := &skillTarget{name: "Victim", effects: newTestList(nil), skillSuccessOK: true, skillSuccessChance: chanceOf(0)}
			def := modelskill.Definition{ID: 44, Level: 7, SkillType: skillType}
			result, ok := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
			if !ok || len(result.Resisted) != 1 || result.Resisted[0] != (Resisted{TargetName: "Victim", SkillID: 44, SkillLevel: 1}) {
				t.Fatalf("UseResult() = %+v, %t; want one level-1 resisted message", result.Resisted, ok)
			}
			if len(result.Messages) != 1 || result.Messages[0] != result.Resisted[0] {
				t.Fatalf("Messages = %+v, want the resisted message in order", result.Messages)
			}
		})
	}
}

// TestDisablersReportFailedRollAtCastLevel covers the Disablers types
// whose resist names the skill with its level: a failed landing roll tells a
// player caster each target resisted the skill at the cast level, in target
// order, a landed roll tells it nothing, and a non-player caster hears
// nothing either way.
func TestDisablersReportFailedRollAtCastLevel(t *testing.T) {
	for _, skillType := range []string{"BETRAY", "CONFUSION", "AGGREDUCE_CHAR", "AGGREMOVE", "ERASE"} {
		t.Run(skillType, func(t *testing.T) {
			def := modelskill.Definition{ID: 1380, Level: 7, SkillType: skillType}
			targets := func(fail bool) []Actor {
				var out []Actor
				for i, name := range []string{"First", "Second"} {
					target := newDisablerFake(int32(i + 1))
					target.name = name
					target.attackableFlag = true
					target.failRoll = fail
					out = append(out, target)
				}
				return out
			}

			result, ok := NewDefaultRegistry().UseResult(Cast{Caster: &skillTarget{isPlayer: true}, Skill: def, Targets: targets(true)})
			want := []Resisted{{TargetName: "First", SkillID: 1380, SkillLevel: 7}, {TargetName: "Second", SkillID: 1380, SkillLevel: 7}}
			if !ok || !slices.Equal(result.Resisted, want) {
				t.Fatalf("player caster, failed rolls: Resisted = %+v, handled = %t; want %+v", result.Resisted, ok, want)
			}
			if len(result.Messages) != 2 || result.Messages[0] != any(want[0]) || result.Messages[1] != any(want[1]) {
				t.Fatalf("player caster, failed rolls: Messages = %+v, want the resists in target order", result.Messages)
			}

			result, _ = NewDefaultRegistry().UseResult(Cast{Caster: &skillTarget{isPlayer: true}, Skill: def, Targets: targets(false)})
			if len(result.Resisted) != 0 || len(result.Messages) != 0 {
				t.Fatalf("player caster, landed rolls: Resisted = %+v, Messages = %+v; want none", result.Resisted, result.Messages)
			}

			result, _ = NewDefaultRegistry().UseResult(Cast{Caster: &skillTarget{}, Skill: def, Targets: targets(true)})
			if len(result.Resisted) != 0 || len(result.Messages) != 0 {
				t.Fatalf("NPC caster, failed rolls: Resisted = %+v, Messages = %+v; want none", result.Resisted, result.Messages)
			}
		})
	}
}

// TestConfusionOnNonNPCTellsPlayerInvalidTarget: CONFUSION rolls nothing
// against a target that is no NPC and tells a player caster the target is
// invalid, in target order among the cast's resists; an NPC caster hears
// nothing.
func TestConfusionOnNonNPCTellsPlayerInvalidTarget(t *testing.T) {
	def := modelskill.Definition{ID: 2, Level: 3, SkillType: "CONFUSION", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}}
	targets := func() (*disablerFake, *disablerFake) {
		player := newDisablerFake(1)
		npc := newDisablerFake(2)
		npc.name = "Monster"
		npc.attackableFlag = true
		npc.failRoll = true
		return player, npc
	}

	player, npc := targets()
	result, ok := NewDefaultRegistry().UseResult(Cast{Caster: &skillTarget{isPlayer: true}, Skill: def, Targets: []Actor{player, npc}})
	resisted := Resisted{TargetName: "Monster", SkillID: 2, SkillLevel: 3}
	if !ok || len(result.Messages) != 2 || result.Messages[0] != any(InvalidTargetMessage{}) || result.Messages[1] != any(resisted) {
		t.Fatalf("Messages = %+v, handled = %t; want invalid target, then %+v", result.Messages, ok, resisted)
	}
	if player.shieldRolls != 1 || len(player.list.All()) != 0 {
		t.Fatalf("non-NPC target: shield rolls %d, effects %d; want its shield rolled and no effect", player.shieldRolls, len(player.list.All()))
	}

	player, npc = targets()
	result, _ = NewDefaultRegistry().UseResult(Cast{Caster: &skillTarget{}, Skill: def, Targets: []Actor{player, npc}})
	if len(result.Messages) != 0 {
		t.Fatalf("NPC caster: Messages = %+v, want none", result.Messages)
	}
}

func TestControlDisablersDoNotReportFailedRollForNPCCaster(t *testing.T) {
	caster := &skillTarget{}
	target := &skillTarget{effects: newTestList(nil), skillSuccessOK: true, skillSuccessChance: chanceOf(0)}
	result, ok := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: modelskill.Definition{ID: 44, Level: 7, SkillType: "STUN"}, Targets: []Actor{target}})
	if !ok || len(result.Resisted) != 0 {
		t.Fatalf("UseResult() = %+v, %t; want no player-only resist", result.Resisted, ok)
	}
}

func TestReflectedStunReportsResistedCasterAsTarget(t *testing.T) {
	caster := &skillTarget{isPlayer: true, name: "Caster", skillSuccessOK: true, skillSuccessChance: chanceOf(0)}
	target := newDisablerFake(1)
	target.reflects = true
	result, ok := NewDefaultRegistry().UseResult(Cast{
		Caster: caster, Skill: modelskill.Definition{ID: 44, Level: 7, SkillType: "STUN"}, Targets: []Actor{target},
	})
	if !ok || len(result.Resisted) != 1 || result.Resisted[0] != (Resisted{TargetName: "Caster", SkillID: 44, SkillLevel: 1}) {
		t.Fatalf("Resisted = %+v, handled = %t; want reflected caster named at level 1", result.Resisted, ok)
	}
}

// TestReflectedStunUsesOriginalTargetsPreSwapShieldBlock proves the shield
// roll that gates a reflected STUN/ROOT/SLEEP/PARALYZE cast is resolved
// against the original target before the reflect swap, matching
// Disablers.java:64 (calcShldUse against targetCreature, once, ahead of the
// switch) and :80-83 (the reflect reassignment happens after sDef is fixed).
// The original target perfect-blocks and reflects; the caster's own shield
// state must never be consulted for the reflected cast.
func TestReflectedStunUsesOriginalTargetsPreSwapShieldBlock(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)
	target.shield = formulas.ShieldPerfect
	target.reflects = true
	caster := newDisablerFake(2)
	caster.shield = formulas.ShieldFailed // must never be consulted

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "STUN", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})

	if caster.lastShield != formulas.ShieldPerfect {
		t.Fatalf("lastShield = %v, want the original target's pre-swap ShieldPerfect", caster.lastShield)
	}
	if len(caster.list.All()) != 0 {
		t.Fatal("original target's perfect block must fail the reflected cast against the caster")
	}
}

// TestReflectedMuteUsesOriginalTargetsPreSwapShieldBlock is
// TestReflectedStunUsesOriginalTargetsPreSwapShieldBlock for the MUTE case
// (Disablers.java:90-93), which resolves shield the same way.
func TestReflectedMuteUsesOriginalTargetsPreSwapShieldBlock(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)
	target.shield = formulas.ShieldPerfect
	target.reflects = true
	caster := newDisablerFake(2)
	caster.shield = formulas.ShieldFailed // must never be consulted

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "MUTE", Effects: []modelskill.EffectTemplate{{Name: "Mute", Time: 10}}},
		Targets: []Actor{target},
	})

	if caster.lastShield != formulas.ShieldPerfect {
		t.Fatalf("lastShield = %v, want the original target's pre-swap ShieldPerfect", caster.lastShield)
	}
	if len(caster.list.All()) != 0 {
		t.Fatal("original target's perfect block must fail the reflected cast against the caster")
	}
}

func TestControlDisablersApplyToHostileNPC(t *testing.T) {
	registry := NewDefaultRegistry()
	tests := []struct {
		skillType  string
		effectName string
	}{
		{skillType: "STUN", effectName: "Stun"},
		{skillType: "ROOT", effectName: "Root"},
		{skillType: "SLEEP", effectName: "Sleep"},
		{skillType: "PARALYZE", effectName: "Paralyze"},
	}

	for _, tt := range tests {
		t.Run(tt.skillType, func(t *testing.T) {
			target := newTestHostile(t, 100, 0)

			registry.Use(Cast{
				Caster: &bssCasterFake{},
				Skill: modelskill.Definition{
					SkillType:     tt.skillType,
					EffectType:    tt.skillType,
					BaseLandRate:  100,
					IgnoreResists: true,
					Effects: []modelskill.EffectTemplate{{
						Name: tt.effectName,
						Time: 10,
					}},
				},
				Targets: []Actor{target},
			})

			if len(target.EffectList().All()) != 1 {
				t.Fatalf("%s should apply its effect to a hostile NPC target", tt.skillType)
			}
		})
	}
}

func TestCancelDebuffStripsOnlyDispellableDebuffsUpToLimit(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)

	// Distinct skill ids keep the effect list from treating these as
	// duplicate applications of "the same" effect and silently dropping
	// one (List.Add's identical-effect collision handling).
	a, _ := effect.New(effect.Skill{ID: 1, Debuff: true, CanBeDispelled: true}, modelskill.EffectTemplate{Name: "Debuff"})
	b, _ := effect.New(effect.Skill{ID: 2, Debuff: true, CanBeDispelled: true}, modelskill.EffectTemplate{Name: "Debuff"})
	notDispellable, _ := effect.New(effect.Skill{ID: 3, Debuff: true, CanBeDispelled: false}, modelskill.EffectTemplate{Name: "Debuff"})
	notDebuff, _ := effect.New(effect.Skill{ID: 4, Debuff: false, CanBeDispelled: true}, modelskill.EffectTemplate{Name: "Buff"})
	for _, e := range []*effect.Effect{a, b, notDispellable, notDebuff} {
		e.Effected = target
		target.list.Add(e)
	}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "CANCEL_DEBUFF", MaxNegatedEffects: 1},
		Targets: []Actor{target},
	})

	remaining := target.list.All()
	if len(remaining) != 3 {
		t.Fatalf("expected exactly 1 debuff stripped (limit=1), got %d effects remaining", len(remaining))
	}
	if !hasEffect(target.list, notDispellable) {
		t.Error("a non-dispellable debuff must never be stripped")
	}
	if !hasEffect(target.list, notDebuff) {
		t.Error("a non-debuff effect must never be stripped by CANCEL_DEBUFF")
	}
	if hasEffect(target.list, a) && hasEffect(target.list, b) {
		t.Error("exactly one of the two dispellable debuffs should have been stripped (limit=1)")
	}
}

func TestNegateByIDStripsMatchingEffect(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)

	targeted, _ := effect.New(effect.Skill{ID: 42}, modelskill.EffectTemplate{Name: "Buff"})
	untouched, _ := effect.New(effect.Skill{ID: 43}, modelskill.EffectTemplate{Name: "Buff"})
	targeted.Effected, untouched.Effected = target, target
	target.list.Add(targeted)
	target.list.Add(untouched)

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "NEGATE", NegateIDs: []int{42}},
		Targets: []Actor{target},
	})

	if hasEffect(target.list, targeted) {
		t.Error("NEGATE should strip the effect matching its negate id list")
	}
	if !hasEffect(target.list, untouched) {
		t.Error("NEGATE should not strip an effect outside its negate id list")
	}
}

func TestAggRemoveSkipsNonAttackableAndRaidRelatedTargets(t *testing.T) {
	registry := NewDefaultRegistry()

	notAttackable := newDisablerFake(1)
	notAttackable.aggro.AddDamage(newDisablerFake(9), 50, 50)

	raidRelated := newDisablerFake(2)
	raidRelated.attackableFlag = true
	raidRelated.raidRelated = true
	raidRelated.aggro.AddDamage(newDisablerFake(9), 50, 50)

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "AGGREMOVE"},
		Targets: []Actor{notAttackable, raidRelated},
	})

	if notAttackable.aggro.IsEmpty() {
		t.Error("a non-attackable target's aggro should be untouched")
	}
	if raidRelated.aggro.IsEmpty() {
		t.Error("a raid-related target's aggro should be untouched")
	}
}

func TestAggDamageNotifiesAttackableTargetAndAppliesEffectsUnconditionally(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDisablerFake(9)
	target := newDisablerFake(1)
	target.attackableFlag = true
	target.level = 43
	target.successOK = false // AGGDAMAGE never rolls; effects apply regardless

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "AGGDAMAGE", Power: 100, Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})

	if target.aggressionSource != caster {
		t.Fatalf("aggression source = %v, want caster", target.aggressionSource)
	}
	// power/(targetLevel+7)*150 = 100/(43+7)*150 = 300.
	if target.aggressionPower != 300 {
		t.Fatalf("aggression power = %d, want 300", target.aggressionPower)
	}
	if len(target.list.All()) != 1 {
		t.Fatalf("AGGDAMAGE must apply its effects unconditionally, got %d effects", len(target.list.All()))
	}
}

func TestAggDamageSkipsAggroNotificationForNonAttackableTarget(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDisablerFake(9)
	target := newDisablerFake(1) // not attackable

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "AGGDAMAGE", Power: 100, Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})

	if target.aggressionSource != nil {
		t.Fatalf("a non-attackable target must never receive an aggro notification, got source %v", target.aggressionSource)
	}
	if len(target.list.All()) != 1 {
		t.Fatalf("AGGDAMAGE must still apply its effects to a non-attackable target, got %d effects", len(target.list.All()))
	}
}

func TestAggRemoveClearsBothTablesOnSuccess(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)
	target.attackableFlag = true
	attacker := newDisablerFake(9)
	target.aggro.AddDamage(attacker, 50, 50)
	target.hate.Add(attacker, 50)

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "AGGREMOVE"},
		Targets: []Actor{target},
	})

	if !target.aggro.IsEmpty() || !target.hate.IsEmpty() {
		t.Fatal("AGGREMOVE should clear both hate tables on a guaranteed-success roll")
	}
}

func TestCheckSkillSuccessFailsOnPerfectShieldBlockDespiteGuaranteedRate(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)
	target.shield = formulas.ShieldPerfect

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "STUN", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})

	if len(target.list.All()) != 0 {
		t.Fatal("a perfect shield block must fail the roll even though the target reports a guaranteed-success rate")
	}
	if target.lastShield != formulas.ShieldPerfect {
		t.Fatalf("lastShield = %v, want ShieldPerfect", target.lastShield)
	}
}

func TestCheckSkillSuccessUsesLivePlayerShieldDefense(t *testing.T) {
	registry := NewDefaultRegistry()
	items := liveShieldItems()
	caster := liveShieldCharacter(t, 1, items)
	unblocked := liveShieldCharacter(t, 2, items)
	blocked := liveShieldCharacter(t, 3, items, &item.Instance{
		ObjectID: 30, TemplateID: 3, Location: item.LocationPaperdoll, LocationData: itemcontainer.LHand,
	})
	caster.SetLastKnownPosition(location.Location{X: 80, Y: 0, Z: 0}, 0)
	for _, target := range []*player.Character{unblocked, blocked} {
		target.SetLastKnownPosition(location.Location{X: 0, Y: 0, Z: 0}, 0)
		owner := effect.ModOwnerSkill(modelskill.Ref{ID: 1, Level: 1})
		target.AddStatFuncs([]effect.Mod{
			{Stat: stat.ShieldRate, Op: effect.OpSet, Value: 20, Owner: owner},
			{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: 120, Owner: owner},
		})
	}
	blocked.SetRollSource(func(n int) int {
		if n != 100 {
			t.Fatalf("shield roll bound = %d, want 100", n)
		}
		return 0
	})

	skill := modelskill.Definition{
		SkillType:     "STUN",
		EffectType:    "STUN",
		BaseLandRate:  100,
		IgnoreResists: true,
		Effects:       []modelskill.EffectTemplate{{Name: "Stun", Time: 10}},
	}
	tests := []struct {
		name   string
		target *player.Character
		want   int
	}{
		{name: "unblocked", target: unblocked, want: 1},
		{name: "perfect shield blocked", target: blocked, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry.Use(Cast{Caster: caster, Skill: skill, Targets: []Actor{tt.target}})
			if got := len(tt.target.EffectList().All()); got != tt.want {
				t.Fatalf("target effects = %d, want %d", got, tt.want)
			}
		})
	}
}

type liveShieldGeo struct{}

func (liveShieldGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (liveShieldGeo) Height(_, _, _ int) int16          { return 0 }
func (liveShieldGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
func (liveShieldGeo) Walkable(int, int, int) bool { return true }
func (liveShieldGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

func liveShieldItems() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponFist}},
		{ID: 3, Kind: item.KindArmor, Slot: item.SlotLHand, Armor: &item.ArmorDetail{Type: item.ArmorShield}},
	})
}

func liveShieldTemplate() *player.Template {
	return &player.Template{
		ID: 0, FistsItemID: 1,
		STR: 40, CON: 43, DEX: 30, INT: 21, WIT: 11, MEN: 25,
		PAtk: 5, PDef: 50, MAtk: 25, MDef: 40,
		CollisionRadius: 9, CollisionHeight: 23,
		HPTable: []float64{100}, MPTable: []float64{30}, CPTable: []float64{0},
	}
}

func liveShieldCharacter(t *testing.T, id int32, items *item.Table, equipped ...*item.Instance) *player.Character {
	t.Helper()
	tmpl := liveShieldTemplate()
	c := &player.Character{
		ID: id, Name: "char", BaseClassID: tmpl.ID,
		Race: player.RaceHuman, Sex: player.SexMale, CharLevel: 1,
		Location: location.Location{X: int(id) * 100, Y: 0, Z: 0},
	}
	c.SetClassID(tmpl.ID)
	c.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: 100, MaxMP: 30, CurrentMP: 30})
	c.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventory(c.ID, items, equipped))
	live, err := creature.NewLive(c.Location, 0, liveShieldGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	live.SetQueue(idleQueue())
	c.Live = live
	c.SetRollSource(func(int) int { return 99 })
	c.Configure(player.Runtime{Rules: player.Rules{PerfectShieldBlockRate: 5}})
	return c
}

func TestCheckSkillSuccessResolvesCasterBlessedSpiritshotCharge(t *testing.T) {
	registry := NewDefaultRegistry()
	target := newDisablerFake(1)
	caster := &bssCasterFake{bss: true}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "STUN", Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}},
		Targets: []Actor{target},
	})

	if !target.lastBss {
		t.Fatal("checkSkillSuccess should have resolved the caster's blessed-spiritshot charge as true")
	}
}

// TestDisablersPassBlessedShotAndShieldToTemplateLanding pins the Disablers
// landing inputs: one shield roll per target ahead of the type switch (even
// for a type that never rolls its own landing), reused with the cast-start
// blessed spiritshot by every per-template roll; a perfect block refuses
// every effect, including a type with no landing roll of its own.
func TestDisablersPassBlessedShotAndShieldToTemplateLanding(t *testing.T) {
	for _, tc := range []struct {
		skillType  string
		shield     formulas.ShieldDefense
		attackable bool
		wantInputs []templateLanding
		wantLanded int
	}{
		{skillType: "STUN", shield: formulas.ShieldSuccess, wantInputs: []templateLanding{{bss: true, shield: formulas.ShieldSuccess}}, wantLanded: 1},
		{skillType: "BETRAY", shield: formulas.ShieldSuccess, wantInputs: []templateLanding{{bss: true, shield: formulas.ShieldSuccess}}, wantLanded: 1},
		{skillType: "NEGATE", shield: formulas.ShieldSuccess, wantInputs: []templateLanding{{bss: true, shield: formulas.ShieldSuccess}}, wantLanded: 1},
		{skillType: "AGGDAMAGE", shield: formulas.ShieldSuccess, wantInputs: []templateLanding{{bss: true, shield: formulas.ShieldSuccess}}, wantLanded: 1},
		{skillType: "AGGREDUCE", shield: formulas.ShieldSuccess, attackable: true, wantInputs: []templateLanding{{bss: true, shield: formulas.ShieldSuccess}}, wantLanded: 1},
		{skillType: "FAKE_DEATH", shield: formulas.ShieldPerfect},
		{skillType: "NEGATE", shield: formulas.ShieldPerfect},
		{skillType: "CANCEL_DEBUFF", shield: formulas.ShieldSuccess},
	} {
		t.Run(fmt.Sprintf("%s/shield%d", tc.skillType, tc.shield), func(t *testing.T) {
			caster := &bssCasterFake{bss: true}
			target := newDisablerFake(2)
			target.shield = tc.shield
			target.attackableFlag = tc.attackable
			disablersHandler{}.Use(Cast{
				Caster:  caster,
				Skill:   modelskill.Definition{ID: 1, SkillType: tc.skillType, Magic: true, Effects: rolledStun()},
				Targets: []Actor{target},
			})
			if target.shieldRolls != 1 {
				t.Fatalf("shield rolls = %d, want 1 per target", target.shieldRolls)
			}
			if !slices.Equal(target.templateLandings, tc.wantInputs) {
				t.Fatalf("template landing inputs = %+v, want %+v", target.templateLandings, tc.wantInputs)
			}
			if got := len(target.list.All()); got != tc.wantLanded {
				t.Fatalf("landed effects = %d, want %d", got, tc.wantLanded)
			}
		})
	}
}
