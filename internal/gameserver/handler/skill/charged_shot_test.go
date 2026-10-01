package skill

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

func TestPdamAndMdamDischargeTheirChargedShots(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{}
	target := &skillTarget{
		hp: 1000,
		physicalInput: formulas.PhysicalSkillInput{
			AttackPower: 100, SkillPower: 50, Defence: 50,
			RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
		},
		physicalOK: true,
		magicInput: formulas.MagicDamageInput{
			MAtk: 100, MDef: 50, SkillPower: 20, PvPMul: 1, ElementalMul: 1,
		},
		magicOK: true,
	}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "PDAM"}, Targets: []Actor{target}})
	caster.charged = map[item.ShotKind]bool{item.ShotBlessedSpirit: true}
	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "MDAM"}, Targets: []Actor{target}})

	if got, want := caster.shots, []item.ShotKind{item.ShotSoul, item.ShotBlessedSpirit}; !slices.Equal(got, want) {
		t.Fatalf("discharged shots = %v, want %v", got, want)
	}
}

// TestNPCCasterSpendsItsSpiritshot drives HEAL and MDAM from a real
// *npc.Hostile, which exposes its charge through SpiritshotCharged and has
// no ChargedShot: the cast spends the NPC's spiritshot, and a static-reuse
// cast writes the bit back set, as Npc.setChargedShot does for any caster.
func TestNPCCasterSpendsItsSpiritshot(t *testing.T) {
	magicTarget := func() *skillTarget {
		return &skillTarget{
			hp:         1000,
			magicInput: formulas.MagicDamageInput{MAtk: 100, MDef: 50, SkillPower: 20, PvPMul: 1, ElementalMul: 1},
			magicOK:    true,
		}
	}
	for _, tc := range []struct {
		name        string
		skill       modelskill.Definition
		charged     bool
		target      func() *skillTarget
		wantCharged bool
	}{
		{"heal spends the charge", modelskill.Definition{SkillType: "HEAL", Power: 20}, true, func() *skillTarget {
			return &skillTarget{hp: 10, maxHP: 1000, healEffectiveness: 100}
		}, false},
		{"mdam spends the charge", modelskill.Definition{SkillType: "MDAM", Power: 20}, true, magicTarget, false},
		{"static-reuse heal writes the charge", modelskill.Definition{SkillType: "HEAL", Power: 20, StaticReuse: true}, false, func() *skillTarget {
			return &skillTarget{hp: 10, maxHP: 1000, healEffectiveness: 100}
		}, true},
		{"static-reuse mdam writes the charge", modelskill.Definition{SkillType: "MDAM", Power: 20, StaticReuse: true}, false, magicTarget, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caster := newTestHostile(t, 20001, 0)
			caster.SetChargedShot(item.ShotSpirit, tc.charged)
			target := tc.target()
			if !NewDefaultRegistry().Use(Cast{Caster: caster, Skill: tc.skill, Targets: []Actor{target}}) {
				t.Fatalf("Use() returned false for %s", tc.skill.SkillType)
			}
			if got := caster.SpiritshotCharged(); got != tc.wantCharged {
				t.Fatalf("NPC spiritshot charged = %v after %s, want %v", got, tc.name, tc.wantCharged)
			}
		})
	}
}

// TestNonDamageHandlersDischargeChargedShots pins the shot each non-damage
// handler spends after its target loop, and the static-reuse flag it writes
// back: CPDAMPERCENT spends the soulshot; HEAL, MANAHEAL, RESURRECT and the
// CANCEL family spend the blessed spiritshot when one is charged, otherwise
// the plain spiritshot; a heal also spends it in its BUFF pass first, so a
// static heal spends it once there and a potion leaves it alone.
// The continuous handler spends the spiritshot on every cast but a potion or
// a toggle; the disablers handler spends it unconditionally.
func TestNonDamageHandlersDischargeChargedShots(t *testing.T) {
	healTarget := func() *skillTarget { return &skillTarget{hp: 10, maxHP: 100, mp: 10, maxMP: 100, recharge: 1} }
	tests := []struct {
		name      string
		skill     modelskill.Definition
		blessed   bool
		healOK    bool
		cubic     bool
		targets   func() []Actor
		wantShots []item.ShotKind
	}{
		{
			name:      "cpdampercent with no targets",
			skill:     modelskill.Definition{SkillType: "CPDAMPERCENT", Power: 50},
			targets:   func() []Actor { return nil },
			wantShots: []item.ShotKind{item.ShotSoul},
		},
		{
			name:      "cpdampercent skips a non-player target",
			skill:     modelskill.Definition{SkillType: "CPDAMPERCENT", Power: 50},
			blessed:   true,
			targets:   func() []Actor { return []Actor{&skillTarget{cp: 100, maxCP: 100}} },
			wantShots: []item.ShotKind{item.ShotSoul},
		},
		{
			name:      "cpdampercent static reuse",
			skill:     modelskill.Definition{SkillType: "CPDAMPERCENT", Power: 50, StaticReuse: true},
			targets:   func() []Actor { return []Actor{&skillTarget{isPlayer: true, cp: 100, maxCP: 100}} },
			wantShots: []item.ShotKind{item.ShotSoul},
		},
		{
			name:      "heal plain spiritshot",
			skill:     modelskill.Definition{SkillType: "HEAL", Power: 20},
			healOK:    true,
			targets:   func() []Actor { return []Actor{healTarget()} },
			wantShots: []item.ShotKind{item.ShotSpirit, item.ShotSpirit},
		},
		{
			name:      "heal blessed spiritshot static reuse",
			skill:     modelskill.Definition{SkillType: "HEAL", Power: 20, StaticReuse: true},
			blessed:   true,
			healOK:    true,
			targets:   func() []Actor { return []Actor{healTarget()} },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit, item.ShotBlessedSpirit},
		},
		{
			name:      "heal without a resolvable amount",
			skill:     modelskill.Definition{SkillType: "HEAL", Power: 20},
			targets:   func() []Actor { return []Actor{healTarget()} },
			wantShots: []item.ShotKind{item.ShotSpirit, item.ShotSpirit},
		},
		{
			name:      "heal static spends only in the buff pass",
			skill:     modelskill.Definition{SkillType: "HEAL_STATIC", Power: 20},
			blessed:   true,
			healOK:    true,
			targets:   func() []Actor { return []Actor{healTarget()} },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit},
		},
		{
			name:    "heal potion keeps shot",
			skill:   modelskill.Definition{SkillType: "HEAL", Power: 20, Potion: true},
			healOK:  true,
			targets: func() []Actor { return []Actor{healTarget()} },
		},
		{
			name:      "manaheal plain spiritshot",
			skill:     modelskill.Definition{SkillType: "MANAHEAL", Power: 20},
			targets:   func() []Actor { return []Actor{healTarget()} },
			wantShots: []item.ShotKind{item.ShotSpirit},
		},
		{
			name:      "manarecharge blessed spiritshot",
			skill:     modelskill.Definition{SkillType: "MANARECHARGE", Power: 20, StaticReuse: true},
			blessed:   true,
			targets:   func() []Actor { return []Actor{healTarget()} },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit},
		},
		{
			name:    "manaheal potion keeps shot",
			skill:   modelskill.Definition{SkillType: "MANAHEAL", Power: 20, Potion: true},
			blessed: true,
			targets: func() []Actor { return []Actor{healTarget()} },
		},
		{
			name:      "resurrect by a caster without revive power",
			skill:     modelskill.Definition{SkillType: "RESURRECT", Power: 20},
			blessed:   true,
			targets:   func() []Actor { return nil },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit},
		},
		{
			name:      "cancel with no targets",
			skill:     modelskill.Definition{SkillType: "CANCEL", Power: 20},
			targets:   func() []Actor { return nil },
			wantShots: []item.ShotKind{item.ShotSpirit},
		},
		{
			name:      "mage bane skips a dead target",
			skill:     modelskill.Definition{SkillType: "MAGE_BANE", Power: 20, StaticReuse: true},
			blessed:   true,
			targets:   func() []Actor { return []Actor{&skillTarget{dead: true}} },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit},
		},
		{
			name:      "warrior bane with no targets",
			skill:     modelskill.Definition{SkillType: "WARRIOR_BANE", Power: 20},
			targets:   func() []Actor { return nil },
			wantShots: []item.ShotKind{item.ShotSpirit},
		},
		{
			name:      "buff plain spiritshot",
			skill:     modelskill.Definition{SkillType: "BUFF"},
			targets:   func() []Actor { return []Actor{&skillTarget{}} },
			wantShots: []item.ShotKind{item.ShotSpirit},
		},
		{
			name:      "debuff blessed spiritshot static reuse after a failed landing",
			skill:     modelskill.Definition{SkillType: "DEBUFF", Debuff: true, Offensive: true, StaticReuse: true},
			blessed:   true,
			targets:   func() []Actor { return []Actor{&skillTarget{}} },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit},
		},
		{
			name:      "hot with no targets",
			skill:     modelskill.Definition{SkillType: "HOT"},
			blessed:   true,
			targets:   func() []Actor { return nil },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit},
		},
		{
			name:    "buff potion keeps shot",
			skill:   modelskill.Definition{SkillType: "BUFF", Potion: true},
			blessed: true,
			targets: func() []Actor { return []Actor{&skillTarget{}} },
		},
		{
			name:    "toggle keeps shot",
			skill:   modelskill.Definition{SkillType: "CONT", Activation: modelskill.ActivationToggle},
			targets: func() []Actor { return []Actor{&skillTarget{}} },
		},
		{
			name:      "stun plain spiritshot",
			skill:     modelskill.Definition{SkillType: "STUN", Offensive: true},
			targets:   func() []Actor { return []Actor{&skillTarget{}} },
			wantShots: []item.ShotKind{item.ShotSpirit},
		},
		{
			name:      "sleep blessed spiritshot static reuse skips a dead target",
			skill:     modelskill.Definition{SkillType: "SLEEP", Offensive: true, StaticReuse: true},
			blessed:   true,
			targets:   func() []Actor { return []Actor{&skillTarget{dead: true}} },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit},
		},
		{
			name:    "cubic poison keeps the owner's shot",
			skill:   modelskill.Definition{SkillType: "POISON", Debuff: true, Offensive: true},
			blessed: true,
			cubic:   true,
			targets: func() []Actor { return []Actor{&skillTarget{}} },
		},
		{
			name:    "cubic stun keeps the owner's shot",
			skill:   modelskill.Definition{SkillType: "STUN", Offensive: true},
			blessed: true,
			cubic:   true,
			targets: func() []Actor { return []Actor{&skillTarget{}} },
		},
		{
			name:      "disabler potion still spends",
			skill:     modelskill.Definition{SkillType: "CANCEL_DEBUFF", Potion: true},
			blessed:   true,
			targets:   func() []Actor { return nil },
			wantShots: []item.ShotKind{item.ShotBlessedSpirit},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caster := &skillTarget{
				healAmount: 10, healOK: tt.healOK,
				charged: map[item.ShotKind]bool{item.ShotBlessedSpirit: tt.blessed, item.ShotSpirit: !tt.blessed, item.ShotSoul: true},
			}
			NewDefaultRegistry().Use(Cast{Caster: caster, Skill: tt.skill, Targets: tt.targets(), Cubic: tt.cubic})
			if !slices.Equal(caster.shots, tt.wantShots) {
				t.Fatalf("discharged shots = %v, want %v", caster.shots, tt.wantShots)
			}
			for i, flag := range caster.shotFlags {
				if flag != tt.skill.StaticReuse {
					t.Fatalf("shot write %d charged = %v, want static-reuse flag %v", i, flag, tt.skill.StaticReuse)
				}
			}
		})
	}
}

// TestCpDamPercentAlikeDeadCasterKeepsSoulshot pins the one early exit that
// precedes the soulshot discharge.
func TestCpDamPercentAlikeDeadCasterKeepsSoulshot(t *testing.T) {
	caster := &skillTarget{alikeDead: true}
	NewDefaultRegistry().Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "CPDAMPERCENT", Power: 50}})
	if len(caster.shots) != 0 {
		t.Fatalf("discharged shots = %v, want none from an alike-dead caster", caster.shots)
	}
}
