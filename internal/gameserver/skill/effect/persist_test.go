package effect

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func ref(id int32, level int) modelskill.Ref {
	return modelskill.Ref{ID: modelskill.ID(id), Level: level}
}

func TestBuildSaveRows_ActiveEffectWithoutTimer(t *testing.T) {
	rows := BuildSaveRows(
		[]ActiveEffect{{Skill: ref(1001, 1), ReuseGroup: 1, Count: 3, Time: 20}},
		nil, 0, true,
	)

	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	want := SaveRow{Skill: ref(1001, 1), EffectCount: 3, EffectCurTime: 20, RestoreType: RestoreTypeEffect, ClassIndex: 0, BuffIndex: 1}
	if rows[0] != want {
		t.Errorf("rows[0] = %+v, want %+v", rows[0], want)
	}
}

func TestBuildSaveRows_ActiveEffectCarriesItsReuseTimer(t *testing.T) {
	rows := BuildSaveRows(
		[]ActiveEffect{{Skill: ref(1001, 1), ReuseGroup: 1, Count: 3, Time: 20}},
		[]ReuseTimer{{Skill: ref(1001, 1), ReuseGroup: 1, Delay: 5000, ExpiresAt: 999999}},
		2, true,
	)

	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].ReuseDelay != 5000 || rows[0].SystemTime != 999999 {
		t.Errorf("rows[0] reuse = (%d, %d), want (5000, 999999)", rows[0].ReuseDelay, rows[0].SystemTime)
	}
	if rows[0].ClassIndex != 2 {
		t.Errorf("rows[0].ClassIndex = %d, want 2", rows[0].ClassIndex)
	}
}

func TestBuildSaveRows_ExcludedEffectKinds(t *testing.T) {
	cases := []struct {
		name string
		eff  ActiveEffect
	}{
		{"toggle", ActiveEffect{Skill: ref(1, 1), ReuseGroup: 1, Toggle: true}},
		{"herb", ActiveEffect{Skill: ref(2, 1), ReuseGroup: 2, Herb: true}},
		{"continuous", ActiveEffect{Skill: ref(3, 1), ReuseGroup: 3, Continuous: true}},
		{"healOverTime", ActiveEffect{Skill: ref(4, 1), ReuseGroup: 4, HealOverTime: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := BuildSaveRows([]ActiveEffect{tc.eff}, nil, 0, true)
			if len(rows) != 0 {
				t.Errorf("len(rows) = %d, want 0 for excluded kind %s", len(rows), tc.name)
			}
		})
	}
}

func TestBuildSaveRows_ExcludedEffectStillLeavesReuseOnlyRow(t *testing.T) {
	rows := BuildSaveRows(
		[]ActiveEffect{{Skill: ref(1001, 1), ReuseGroup: 1, Herb: true}},
		[]ReuseTimer{{Skill: ref(1001, 1), ReuseGroup: 1, Delay: 5000, ExpiresAt: 999999}},
		0, true,
	)

	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].RestoreType != RestoreTypeReuseOnly {
		t.Errorf("rows[0].RestoreType = %v, want RestoreTypeReuseOnly", rows[0].RestoreType)
	}
	if rows[0].EffectCount != -1 || rows[0].EffectCurTime != -1 {
		t.Errorf("rows[0] effect fields = (%d, %d), want (-1, -1)", rows[0].EffectCount, rows[0].EffectCurTime)
	}
}

func TestBuildSaveRows_DedupBySharedReuseGroup(t *testing.T) {
	rows := BuildSaveRows(
		[]ActiveEffect{
			{Skill: ref(1001, 1), ReuseGroup: 1, Count: 1, Time: 10},
			{Skill: ref(1002, 1), ReuseGroup: 1, Count: 2, Time: 20},
		},
		nil, 0, true,
	)

	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Skill != ref(1001, 1) {
		t.Errorf("rows[0].Skill = %+v, want the first encountered effect's skill", rows[0].Skill)
	}
}

func TestBuildSaveRows_IncludeEffectsFalseOnlyStoresReuseTimers(t *testing.T) {
	rows := BuildSaveRows(
		[]ActiveEffect{{Skill: ref(1001, 1), ReuseGroup: 1, Count: 1, Time: 10}},
		[]ReuseTimer{{Skill: ref(2002, 1), ReuseGroup: 2, Delay: 1000, ExpiresAt: 5000}},
		0, false,
	)

	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Skill != ref(2002, 1) || rows[0].RestoreType != RestoreTypeReuseOnly {
		t.Errorf("rows[0] = %+v, want the reuse timer's row only", rows[0])
	}
}

func TestBuildSaveRows_BuffIndexOrdersEffectsBeforeReuseOnly(t *testing.T) {
	rows := BuildSaveRows(
		[]ActiveEffect{{Skill: ref(1001, 1), ReuseGroup: 1, Count: 1, Time: 10}},
		[]ReuseTimer{{Skill: ref(2002, 1), ReuseGroup: 2, Delay: 1000, ExpiresAt: 5000}},
		0, true,
	)

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].BuffIndex != 1 || rows[0].RestoreType != RestoreTypeEffect {
		t.Errorf("rows[0] = %+v, want the effect row at BuffIndex 1", rows[0])
	}
	if rows[1].BuffIndex != 2 || rows[1].RestoreType != RestoreTypeReuseOnly {
		t.Errorf("rows[1] = %+v, want the reuse-only row at BuffIndex 2", rows[1])
	}
}

func TestBuildSaveRows_TimerAlreadyClaimedIsNotDuplicated(t *testing.T) {
	rows := BuildSaveRows(
		[]ActiveEffect{{Skill: ref(1001, 1), ReuseGroup: 1, Count: 1, Time: 10}},
		[]ReuseTimer{{Skill: ref(1001, 1), ReuseGroup: 1, Delay: 1000, ExpiresAt: 5000}},
		0, true,
	)

	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1 (the effect row absorbs the reuse timer, no separate reuse-only row)", len(rows))
	}
}

func TestBuildRestorePlan_UnknownSkillIsSkippedEntirely(t *testing.T) {
	rows := []SaveRow{{Skill: ref(9999, 1), SystemTime: 1_000_000, RestoreType: RestoreTypeEffect}}
	plan := BuildRestorePlan(rows, 0, func(modelskill.Ref) (bool, bool) { return false, false })

	if len(plan.Reuse) != 0 || len(plan.Effects) != 0 {
		t.Errorf("plan = %+v, want empty for an unresolved skill", plan)
	}
}

func TestBuildRestorePlan_EffectRowWithRemainingReuseRestoresBoth(t *testing.T) {
	rows := []SaveRow{{
		Skill: ref(1001, 1), EffectCount: 3, EffectCurTime: 20,
		ReuseDelay: 5000, SystemTime: 100_100, RestoreType: RestoreTypeEffect,
	}}
	plan := BuildRestorePlan(rows, 100_000, func(modelskill.Ref) (bool, bool) { return true, true })

	if len(plan.Reuse) != 1 || plan.Reuse[0] != (ReusePlan{Skill: ref(1001, 1), Delay: 5000, ExpiresAt: 100_100}) {
		t.Errorf("plan.Reuse = %+v, want one reinstated reuse timer", plan.Reuse)
	}
	if len(plan.Effects) != 1 || plan.Effects[0] != (EffectPlan{Skill: ref(1001, 1), Count: 3, Time: 20}) {
		t.Errorf("plan.Effects = %+v, want one reapplied effect", plan.Effects)
	}
}

func TestBuildRestorePlan_ReuseWithin10msIsNotRestored(t *testing.T) {
	rows := []SaveRow{{Skill: ref(1001, 1), SystemTime: 100_005, RestoreType: RestoreTypeEffect}}
	plan := BuildRestorePlan(rows, 100_000, func(modelskill.Ref) (bool, bool) { return true, true })

	if len(plan.Reuse) != 0 {
		t.Errorf("plan.Reuse = %+v, want none when only 5ms remain", plan.Reuse)
	}
	// The effect itself still restores independent of the reuse delay.
	if len(plan.Effects) != 1 {
		t.Errorf("plan.Effects = %+v, want the effect to still restore", plan.Effects)
	}
}

func TestBuildRestorePlan_ReuseOnlyRowNeverRestoresAnEffect(t *testing.T) {
	rows := []SaveRow{{
		Skill: ref(1001, 1), EffectCount: -1, EffectCurTime: -1,
		SystemTime: 200_000, RestoreType: RestoreTypeReuseOnly,
	}}
	plan := BuildRestorePlan(rows, 100_000, func(modelskill.Ref) (bool, bool) { return true, true })

	if len(plan.Reuse) != 1 {
		t.Errorf("plan.Reuse = %+v, want the reuse timer restored", plan.Reuse)
	}
	if len(plan.Effects) != 0 {
		t.Errorf("plan.Effects = %+v, want none for a reuse-only row", plan.Effects)
	}
}

func TestBuildRestorePlan_SkillWithoutEffectsSkipsEffectRestore(t *testing.T) {
	rows := []SaveRow{{
		Skill: ref(1001, 1), EffectCount: 3, EffectCurTime: 20,
		SystemTime: 200_000, RestoreType: RestoreTypeEffect,
	}}
	plan := BuildRestorePlan(rows, 100_000, func(modelskill.Ref) (bool, bool) { return true, false })

	if len(plan.Effects) != 0 {
		t.Errorf("plan.Effects = %+v, want none when the skill carries no effect templates", plan.Effects)
	}
}
