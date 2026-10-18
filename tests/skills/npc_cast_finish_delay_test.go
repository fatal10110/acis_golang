package skills

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

const npcFinishDelaySkill = modelskill.ID(9105)

// npcFinishDelayCases are the two timelines an NPC cast's finish is pinned
// on: a cast with no hit time and no cool time still ends 250 ms after its
// hit, and one with both ends its cool time plus 250 ms after its hit,
// which lands at its hit time.
var npcFinishDelayCases = []struct {
	name              string
	hitTime, coolTime int
	finish            time.Duration
}{
	{name: "no hit and no cool time", finish: 250 * time.Millisecond},
	{name: "hit and cool time", hitTime: 1000, coolTime: 500, finish: 1750 * time.Millisecond},
}

// npcFinishDelaySkills holds a self buff with hitTime and coolTime, both
// read as they are.
func npcFinishDelaySkills(hitTime, coolTime int) []modelskill.Definition {
	return []modelskill.Definition{{
		ID: npcFinishDelaySkill, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetSelf, SkillType: "BUFF",
		HitTime: hitTime, CoolTime: coolTime, StaticHitTime: true, StaticReuse: true,
	}}
}

// assertCastEndsAt lets finish pass from the cast's start on the driven
// clock and checks the cast is still in flight 1 ms before then and over at
// it.
func assertCastEndsAt(t *testing.T, advance func(time.Duration), casting func() bool, finish time.Duration) {
	t.Helper()
	if !casting() {
		t.Fatal("cast not in flight once started")
	}
	advance(finish - time.Millisecond)
	if !casting() {
		t.Fatalf("cast over before %v", finish)
	}
	advance(time.Millisecond)
	if casting() {
		t.Fatalf("cast still in flight %v after its start", finish)
	}
}

// TestHostileNPCCastFinishesCoolTimePlus250AfterHit pins
// NpcCast.onMagicHitTimer for a monster: the finish runs _coolTime + 250 ms
// after the hit, whatever the hit and cool times.
func TestHostileNPCCastFinishesCoolTimePlus250AfterHit(t *testing.T) {
	t.Parallel()
	for _, tt := range npcFinishDelayCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, hostile, cast := bootRealDamageCaster(t, npcFinishDelaySkills(tt.hitTime, tt.coolTime)...)
			cast(hostile, npcFinishDelaySkill)
			srv.Settle(t)
			assertCastEndsAt(t, func(d time.Duration) { srv.Advance(t, d) }, hostile.CastingNow, tt.finish)
		})
	}
}

// TestFolkCastFinishesCoolTimePlus250AfterHit pins the same finish for a
// civilian NPC's cast.
func TestFolkCastFinishesCoolTimePlus250AfterHit(t *testing.T) {
	t.Parallel()
	for _, tt := range npcFinishDelayCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, _, folk := bootFolkCaster(t, npcFinishDelaySkills(tt.hitTime, tt.coolTime))
			addCastDesire(t, folk, modelskill.Ref{ID: npcFinishDelaySkill, Level: 1})
			tickAI(t, srv)
			assertCastEndsAt(t, func(d time.Duration) { srv.Advance(t, d) }, folk.CastingNow, tt.finish)
		})
	}
}
