package effect

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// RestoreType distinguishes a saved row that carries a live effect's
// remaining duration from one that carries only a leftover skill reuse
// delay with no effect still running.
type RestoreType int8

const (
	// RestoreTypeEffect marks a row whose effect_count/effect_cur_time
	// describe a still-active effect to reapply on restore.
	RestoreTypeEffect RestoreType = 0
	// RestoreTypeReuseOnly marks a row that carries nothing but a pending
	// reuse delay; its effect_count/effect_cur_time are meaningless (-1).
	RestoreTypeReuseOnly RestoreType = 1
)

// SaveRow is one row of a character's persisted skill/buff state: either an
// active effect's remaining duration plus whatever reuse delay its skill
// carries (RestoreTypeEffect), or a reuse delay surviving with no live
// effect behind it (RestoreTypeReuseOnly). Its fields map 1:1 to the
// character_skills_save table's columns other than the owning character id,
// which callers key rows by separately.
type SaveRow struct {
	Skill         modelskill.Ref
	EffectCount   int32
	EffectCurTime int32
	// ReuseDelay is the skill's full configured reuse delay in
	// milliseconds, persisted verbatim for display purposes.
	ReuseDelay int64
	// SystemTime is the unix millisecond timestamp the reuse delay ends.
	SystemTime  int64
	RestoreType RestoreType
	ClassIndex  int32
	BuffIndex   int32
}

// ActiveEffect is the per-effect state BuildSaveRows needs to decide
// whether one live effect survives a relog, and what row it becomes if it
// does. A caller builds one from a live effect plus the skill that applied
// it: ReuseGroup is the key that collapses every effect and reuse timer
// sharing one reuse cooldown (shared-reuse skills, or the same skill cast
// at different levels) into a single saved row.
type ActiveEffect struct {
	Skill      modelskill.Ref
	ReuseGroup int32
	Count      int32
	Time       int32

	// Toggle, Herb, Continuous, and HealOverTime each mark a category of
	// effect that never survives a relog, mirroring an active toggle
	// skill, a consumable-herb effect, a continuous/channeled skill effect,
	// and a heal-over-time effect respectively.
	Toggle       bool
	Herb         bool
	Continuous   bool
	HealOverTime bool
}

func (e ActiveEffect) excluded() bool {
	return e.Toggle || e.Herb || e.Continuous || e.HealOverTime
}

// ReuseTimer is one skill's remaining reuse delay, independent of whether
// an effect from it is still active. Callers pass only timers that have not
// yet expired — BuildSaveRows treats an expired or absent timer as no
// pending reuse at all, so filtering happens before the call rather than
// inside it.
type ReuseTimer struct {
	Skill      modelskill.Ref
	ReuseGroup int32
	Delay      int64
	ExpiresAt  int64
}

// BuildSaveRows converts one character's active effects and pending reuse
// timers into the rows a relog restore later replays. Each reuse group
// contributes at most one row: when includeEffects is true and the group's
// first (in encounter order) active effect isn't excluded, that effect's
// own row wins and carries the group's reuse timer alongside it; otherwise
// any unclaimed reuse timer for that group adds a trailing
// RestoreTypeReuseOnly row. buff_index numbers the combined rows 1-based in
// that order, so an effect row always sorts before the reuse-only rows that
// follow it.
//
// includeEffects lets a caller store leftover reuse delays without storing
// any effect state — used on paths that intentionally drop active buffs
// (e.g. a duel) but still want skills to remember their cooldown.
func BuildSaveRows(effects []ActiveEffect, timers []ReuseTimer, classIndex int32, includeEffects bool) []SaveRow {
	var rows []SaveRow
	claimed := make(map[int32]bool)
	var index int32

	if includeEffects {
		for _, e := range effects {
			if claimed[e.ReuseGroup] || e.excluded() {
				continue
			}
			claimed[e.ReuseGroup] = true

			var delay, expires int64
			for _, t := range timers {
				if t.ReuseGroup == e.ReuseGroup {
					delay, expires = t.Delay, t.ExpiresAt
					break
				}
			}

			index++
			rows = append(rows, SaveRow{
				Skill: e.Skill, EffectCount: e.Count, EffectCurTime: e.Time,
				ReuseDelay: delay, SystemTime: expires,
				RestoreType: RestoreTypeEffect, ClassIndex: classIndex, BuffIndex: index,
			})
		}
	}

	for _, t := range timers {
		if claimed[t.ReuseGroup] {
			continue
		}
		claimed[t.ReuseGroup] = true

		index++
		rows = append(rows, SaveRow{
			Skill: t.Skill, EffectCount: -1, EffectCurTime: -1,
			ReuseDelay: t.Delay, SystemTime: t.ExpiresAt,
			RestoreType: RestoreTypeReuseOnly, ClassIndex: classIndex, BuffIndex: index,
		})
	}

	return rows
}

// RestorePlan is the work a relog restore performs, computed from one
// character's saved rows: which skills need their reuse delay reinstated,
// and, among those, which also need their effect state reapplied with the
// remaining duration it had at logout.
type RestorePlan struct {
	Reuse   []ReusePlan
	Effects []EffectPlan
}

// ReusePlan reinstates one skill's pending reuse delay.
type ReusePlan struct {
	Skill     modelskill.Ref
	Delay     int64
	ExpiresAt int64
}

// EffectPlan reapplies one skill's effect with the count/time it had at
// logout, rather than the fresh values a new cast would start from.
type EffectPlan struct {
	Skill modelskill.Ref
	Count int32
	Time  int32
}

// BuildRestorePlan turns rows — loaded in buff_index order — into a
// RestorePlan. A row's reuse delay is reinstated whenever more than 10ms of
// it remains at nowMillis, regardless of restore type. A
// RestoreTypeEffect row additionally has its effect reapplied, provided
// lookup still resolves the row's skill (found) and reports that skill as
// one that carries effects (hasEffects) — a row whose skill id/level no
// longer resolves against current skill data is dropped entirely, matching
// a stale save surviving a data update.
func BuildRestorePlan(rows []SaveRow, nowMillis int64, lookup func(modelskill.Ref) (found, hasEffects bool)) RestorePlan {
	var plan RestorePlan
	for _, row := range rows {
		found, hasEffects := lookup(row.Skill)
		if !found {
			continue
		}

		if remaining := row.SystemTime - nowMillis; remaining > 10 {
			plan.Reuse = append(plan.Reuse, ReusePlan{Skill: row.Skill, Delay: row.ReuseDelay, ExpiresAt: row.SystemTime})
		}

		if row.RestoreType != RestoreTypeEffect {
			continue
		}
		if hasEffects {
			plan.Effects = append(plan.Effects, EffectPlan{Skill: row.Skill, Count: row.EffectCount, Time: row.EffectCurTime})
		}
	}
	return plan
}
