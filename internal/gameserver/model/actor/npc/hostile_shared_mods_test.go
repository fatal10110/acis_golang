package npc

import (
	"runtime"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// sharedPassivesTable holds three template passives touching MaxHP (every
// op order the chain sorts by), P.Def and run speed.
func sharedPassivesTable() (*skill.Table, []skill.Ref) {
	refs := []skill.Ref{{ID: 4401, Level: 1}, {ID: 4402, Level: 3}, {ID: 4403, Level: 1}}
	table := skill.NewTable([]skill.Definition{
		{ID: 4401, Level: 1, Funcs: []skill.FuncTemplate{
			{Op: skill.FuncAdd, Stat: "maxHp", Value: 250},
			{Op: skill.FuncMul, Stat: "maxHp", Value: 1.1},
			{Op: skill.FuncAdd, Stat: "pDef", Value: 10},
		}},
		{ID: 4402, Level: 3, Funcs: []skill.FuncTemplate{
			{Op: skill.FuncBaseMul, Stat: "maxHp", Value: 0.2},
			{Op: skill.FuncAdd, Stat: "maxHp", Value: 33.3},
			{Op: skill.FuncMul, Stat: "pDef", Value: 1.3},
		}},
		{ID: 4403, Level: 1, Funcs: []skill.FuncTemplate{
			{Op: skill.FuncSub, Stat: "runSpd", Value: 20},
			{Op: skill.FuncAddMul, Stat: "maxHp", Value: 0.05},
		}},
	})
	return table, refs
}

func sharedPassivesTemplate(refs []skill.Ref) *Template {
	return NewTemplate(Template{ID: 9101, Type: "Monster", Level: 40, HPMax: 2000, MPMax: 500, CON: 43, MEN: 25, RunSpeed: 160}, TemplateSkills{Passives: refs})
}

// BenchmarkHostileSpawnStats spawns hostiles of one template with passives
// and reads the stats a spawn reads, keeping every NPC alive: retained-B/npc
// is the heap each one holds.
func BenchmarkHostileSpawnStats(b *testing.B) {
	table, refs := sharedPassivesTable()
	tpl := sharedPassivesTemplate(refs)
	live := make([]*Hostile, 0, b.N)
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		h, err := NewHostile(&Instance{ObjectID: int32(i + 1), Template: tpl, Kind: "Monster"}, newHostileLive(b), &hostileMove{}, &hostileAttack{}, table)
		if err != nil {
			b.Fatal(err)
		}
		_ = h.MaxHPValue() + h.MaxMPValue() + float64(h.RunSpeed())
		live = append(live, h)
	}
	b.StopTimer()
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	b.ReportMetric(float64(after.HeapAlloc-before.HeapAlloc)/float64(b.N), "retained-B/npc")
	runtime.KeepAlive(live)
}

// sharedPassiveStats are the stats the shared passives touch, plus one they
// leave to the builtin alone, with the base each is finalized from.
var sharedPassiveStats = []struct {
	stat stat.Stat
	base float64
}{
	{stat.MaxHP, 2000},
	{stat.PowerDefence, 300},
	{stat.RunSpeed, 160},
	{stat.MaxMP, 500},
}

// attachedOneByOne finalizes base for s through a fresh Calculator that got
// mods one at a time, the way a chain was built before passives were
// shared.
func attachedOneByOne(mods []effect.Mod, s stat.Stat, actor stat.Actor, base float64) float64 {
	calc := effect.NewCalculator(defaultBuiltin(s))
	for _, m := range mods {
		if m.Stat == s {
			calc.AddMod(m)
		}
	}
	value := calc.Calc(actor, base, nil, nil)
	if s.CantBeNegative() && value <= 0 {
		return 1
	}
	return value
}

func newSharedPassivesHostile(t testing.TB, tpl *Template, table *skill.Table, objectID int32) *Hostile {
	t.Helper()
	h, err := NewHostile(&Instance{ObjectID: objectID, Template: tpl, Kind: "Monster"}, newHostileLive(t), &hostileMove{}, &hostileAttack{}, table)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func templatePassiveMods(t *testing.T, table *skill.Table, refs []skill.Ref) []effect.Mod {
	t.Helper()
	mods, err := effect.TemplatePassiveMods(table, refs)
	if err != nil {
		t.Fatal(err)
	}
	return mods
}

func assertStatsMatch(t *testing.T, label string, h *Hostile, mods []effect.Mod) {
	t.Helper()
	for _, tc := range sharedPassiveStats {
		want := attachedOneByOne(mods, tc.stat, hostileStatActor{h: h}, tc.base)
		if got := h.CalcStat(tc.stat, tc.base); got != want {
			t.Errorf("%s: CalcStat(%v, %v) = %v, want %v", label, tc.stat, tc.base, got, want)
		}
	}
}

// TestHostileSharedPassivesMatchOneByOneAttach: an NPC computing from its
// template's shared passive chains, cold or after a buff gives a stat its
// own chain, gets exactly what attaching each passive func in turn gave.
func TestHostileSharedPassivesMatchOneByOneAttach(t *testing.T) {
	table, refs := sharedPassivesTable()
	tpl := sharedPassivesTemplate(refs)
	mods := templatePassiveMods(t, table, refs)
	h := newSharedPassivesHostile(t, tpl, table, 1)

	assertStatsMatch(t, "cold", h, mods)

	buff := []effect.Mod{
		{Stat: stat.MaxHP, Op: effect.OpMul, Value: 1.07, Owner: effect.ModOwnerSkill(skill.Ref{ID: 7001, Level: 1})},
		{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 12.5, Owner: effect.ModOwnerSkill(skill.Ref{ID: 7001, Level: 1})},
	}
	h.AddStatFuncs(buff)
	assertStatsMatch(t, "buffed", h, append(append([]effect.Mod(nil), mods...), buff...))
}

// TestHostileBuiltinOnlyReadsStayCold: NPCs of one template share its
// compiled passives, and reading their stats creates no stat chain and
// allocates nothing.
func TestHostileBuiltinOnlyReadsStayCold(t *testing.T) {
	table, refs := sharedPassivesTable()
	tpl := sharedPassivesTemplate(refs)
	a := newSharedPassivesHostile(t, tpl, table, 1)
	b := newSharedPassivesHostile(t, tpl, table, 2)
	if a.passives == nil || a.passives != b.passives {
		t.Fatalf("passives = %p and %p, want one shared non-nil set", a.passives, b.passives)
	}
	allocs := testing.AllocsPerRun(100, func() {
		_ = a.MaxHPValue() + a.MaxMPValue() + float64(a.RunSpeed()) + a.CalcStat(stat.PowerDefence, 300)
	})
	if allocs != 0 {
		t.Errorf("stat reads allocated %v times per run, want 0", allocs)
	}
	if a.statCalcs != nil {
		t.Error("stat reads created stat chains, want none")
	}
}

// TestHostileRemovingTemplatePassiveKeepsSharedChain: dropping a passive
// skill's funcs from one NPC leaves the other NPCs of its template, and
// those spawned later, with them.
func TestHostileRemovingTemplatePassiveKeepsSharedChain(t *testing.T) {
	table, refs := sharedPassivesTable()
	tpl := sharedPassivesTemplate(refs)
	mods := templatePassiveMods(t, table, refs)
	a := newSharedPassivesHostile(t, tpl, table, 1)
	b := newSharedPassivesHostile(t, tpl, table, 2)

	a.RemoveStatsByOwner(effect.ModOwnerSkill(refs[0]))

	assertStatsMatch(t, "after removal", a, templatePassiveMods(t, table, refs[1:]))
	assertStatsMatch(t, "other NPC", b, mods)
	assertStatsMatch(t, "later spawn", newSharedPassivesHostile(t, tpl, table, 3), mods)
}

// TestHostilePassivesFollowSkillReload: NPCs spawned after a skill reload
// get the reloaded passive funcs; NPCs already spawned keep theirs.
func TestHostilePassivesFollowSkillReload(t *testing.T) {
	table, refs := sharedPassivesTable()
	tpl := sharedPassivesTemplate(refs)
	before := templatePassiveMods(t, table, refs)
	old := newSharedPassivesHostile(t, tpl, table, 1)

	table.Replace(skill.NewTable([]skill.Definition{
		{ID: 4401, Level: 1, Funcs: []skill.FuncTemplate{{Op: skill.FuncAdd, Stat: "maxHp", Value: 900}}},
	}))
	after := templatePassiveMods(t, table, refs)
	fresh := newSharedPassivesHostile(t, tpl, table, 2)

	assertStatsMatch(t, "spawned before reload", old, before)
	assertStatsMatch(t, "spawned after reload", fresh, after)
	if fresh.passives == old.passives {
		t.Error("spawn after reload reused the passives compiled before it")
	}
}

// TestHostilePassiveWithFailingAttachConditionLeftOut: a passive func whose
// attached condition fails for the NPC is not attached, while the others
// are.
func TestHostilePassiveWithFailingAttachConditionLeftOut(t *testing.T) {
	targetSide := &skill.ConditionClause{Root: skill.Condition{Kind: "target", Attrs: map[string]string{"hp_min_max": "0,100"}}}
	ref := skill.Ref{ID: 4410, Level: 1}
	table := skill.NewTable([]skill.Definition{{ID: ref.ID, Level: ref.Level, Funcs: []skill.FuncTemplate{
		{Op: skill.FuncAdd, Stat: "maxHp", Value: 10},
		{Op: skill.FuncAdd, Stat: "pDef", Value: 100, AttachCondition: targetSide},
	}}})
	tpl := sharedPassivesTemplate([]skill.Ref{ref})
	h := newSharedPassivesHostile(t, tpl, table, 1)

	mods := templatePassiveMods(t, table, []skill.Ref{ref})
	assertStatsMatch(t, "attached", h, mods[:1])
}

// TestHostileConcurrentBuffsOnColdNPC: two casters buffing a cold NPC at
// once, while others read its stats, each get their funcs onto the chain
// the template passives start, and dropping them leaves the passives
// alone. Run with -race.
func TestHostileConcurrentBuffsOnColdNPC(t *testing.T) {
	table, refs := sharedPassivesTable()
	tpl := sharedPassivesTemplate(refs)
	mods := templatePassiveMods(t, table, refs)
	h := newSharedPassivesHostile(t, tpl, table, 1)

	buffs := [2][]effect.Mod{}
	for i := range buffs {
		owner := effect.ModOwnerSkill(skill.Ref{ID: skill.ID(7100 + i), Level: 1})
		buffs[i] = []effect.Mod{
			{Stat: stat.MaxHP, Op: effect.OpAdd, Value: float64(100 * (i + 1)), Owner: owner},
			{Stat: stat.RunSpeed, Op: effect.OpAdd, Value: float64(5 * (i + 1)), Owner: owner},
			{Stat: stat.MaxMP, Op: effect.OpAdd, Value: float64(7 * (i + 1)), Owner: owner},
		}
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range buffs {
		wg.Go(func() {
			<-start
			h.AddStatFuncs(buffs[i])
		})
		wg.Go(func() {
			<-start
			for range 50 {
				_ = h.MaxHPValue() + h.MaxMPValue() + float64(h.RunSpeed())
			}
		})
	}
	close(start)
	wg.Wait()

	buffed := append(append(append([]effect.Mod(nil), mods...), buffs[0]...), buffs[1]...)
	assertStatsMatch(t, "both buffs", h, buffed)

	start = make(chan struct{})
	for i := range buffs {
		wg.Go(func() {
			<-start
			h.RemoveStatsByOwner(buffs[i][0].Owner)
		})
	}
	close(start)
	wg.Wait()

	assertStatsMatch(t, "buffs removed", h, mods)
	assertStatsMatch(t, "template chain", newSharedPassivesHostile(t, tpl, table, 2), mods)
}

// TestFolkSharesTemplatePassives: civilian NPCs of one template share its
// compiled passives, compute what attaching each passive func in turn gave
// and keep reads cold.
func TestFolkSharesTemplatePassives(t *testing.T) {
	table, refs := sharedPassivesTable()
	tpl := NewTemplate(Template{ID: 30102, TemplateID: 30102, Type: "Merchant", Level: 40, HPMax: 2000, MPMax: 500, CON: 43, MEN: 25, RunSpeed: 160}, TemplateSkills{Passives: refs})
	mods := templatePassiveMods(t, table, refs)
	var folks [2]*Folk
	for i := range folks {
		inst, err := NewInstance(int32(i+1), tpl)
		if err != nil {
			t.Fatal(err)
		}
		if folks[i], err = NewFolk(inst, table); err != nil {
			t.Fatal(err)
		}
	}
	if folks[0].passives == nil || folks[0].passives != folks[1].passives {
		t.Fatalf("passives = %p and %p, want one shared non-nil set", folks[0].passives, folks[1].passives)
	}
	f := folks[0]
	for _, tc := range sharedPassiveStats {
		want := attachedOneByOne(mods, tc.stat, f.statActor(), tc.base)
		if got := f.CalcStat(tc.stat, tc.base); got != want {
			t.Errorf("CalcStat(%v, %v) = %v, want %v", tc.stat, tc.base, got, want)
		}
	}
	if f.statCalcs != nil {
		t.Error("stat reads created stat chains, want none")
	}
}
