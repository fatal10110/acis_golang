package summon

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// ---- from actor_cancel_vulnerability_test.go ----
// TestSummonCancelVulnerabilityAppliesCancelVulnStat proves CANCEL_VULN
// (Formulas.java:949-951) reaches Actor.CancelVulnerability through the live
// stat calculator — see the parallel player-package test for the downstream
// formula behavior this plumbing enables.
func TestSummonCancelVulnerabilityAppliesCancelVulnStat(t *testing.T) {
	a := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 44, Roll: zeroSummonRoll})

	if got := a.CancelVulnerability("CANCEL"); got != 1 {
		t.Fatalf("CancelVulnerability() with no modifier = %v, want 1 (unmodified)", got)
	}

	a.AddStatFuncs([]effect.Mod{{Stat: stat.CancelVuln, Op: effect.OpMul, Value: 0.2}})
	if got := a.CancelVulnerability("CANCEL"); got != 0.2 {
		t.Fatalf("CancelVulnerability() after 0.2x modifier = %v, want 0.2", got)
	}
}

// ---- from command_test.go ----
func TestResolveToggleFollow(t *testing.T) {
	tests := []struct {
		name string
		req  Request
		want Outcome
	}{
		{"no summon", Request{Command: CommandToggleFollow}, OutcomeIgnored},
		{"following too far to recall", Request{Command: CommandToggleFollow, HasSummon: true, FollowActive: true, OwnerWithinFollowRange: false}, OutcomeIgnored},
		{"out of control", Request{Command: CommandToggleFollow, HasSummon: true, OutOfControl: true}, OutcomeRefusedOutOfControl},
		{"applies", Request{Command: CommandToggleFollow, HasSummon: true, FollowActive: true, OwnerWithinFollowRange: true}, OutcomeApplied},
		{"applies while not following", Request{Command: CommandToggleFollow, HasSummon: true, FollowActive: false}, OutcomeApplied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.req); got != tt.want {
				t.Errorf("Resolve() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveAttack(t *testing.T) {
	base := Request{Command: CommandAttack, HasSummon: true, HasTarget: true}

	tests := []struct {
		name string
		req  Request
		want Outcome
	}{
		{"no target", Request{Command: CommandAttack, HasSummon: true}, OutcomeIgnored},
		{"no summon", Request{Command: CommandAttack, HasTarget: true}, OutcomeIgnored},
		{"target is own summon", withReq(base, func(r *Request) { r.TargetIsSummon = true }), OutcomeIgnored},
		{"target is owner", withReq(base, func(r *Request) { r.TargetIsOwner = true }), OutcomeIgnored},
		{"target already dead", withReq(base, func(r *Request) { r.TargetIsDeadCreature = true }), OutcomeIgnored},
		{"passive summon can't attack", withReq(base, func(r *Request) { r.IsPassiveSummon = true }), OutcomeIgnored},
		{"out of control", withReq(base, func(r *Request) { r.OutOfControl = true }), OutcomeRefusedOutOfControl},
		{"pet outgrew owner", withReq(base, func(r *Request) { r.IsPet = true; r.SummonLevel = 50; r.OwnerLevel = 20 }), OutcomeRefusedLevelGap},
		{"pet within level gap applies", withReq(base, func(r *Request) { r.IsPet = true; r.SummonLevel = 40; r.OwnerLevel = 20 }), OutcomeApplied},
		{"servitor ignores level gap rule", withReq(base, func(r *Request) { r.IsPet = false; r.SummonLevel = 90; r.OwnerLevel = 1 }), OutcomeApplied},
		{"applies", base, OutcomeApplied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.req); got != tt.want {
				t.Errorf("Resolve() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveStop(t *testing.T) {
	if got := Resolve(Request{Command: CommandStop}); got != OutcomeIgnored {
		t.Errorf("no summon: Resolve() = %v, want OutcomeIgnored", got)
	}
	if got := Resolve(Request{Command: CommandStop, HasSummon: true, OutOfControl: true}); got != OutcomeRefusedOutOfControl {
		t.Errorf("out of control: Resolve() = %v, want OutcomeRefusedOutOfControl", got)
	}
	if got := Resolve(Request{Command: CommandStop, HasSummon: true}); got != OutcomeApplied {
		t.Errorf("Resolve() = %v, want OutcomeApplied", got)
	}
}

func TestResolveReturnPet(t *testing.T) {
	base := Request{Command: CommandReturnPet, IsPet: true}

	tests := []struct {
		name string
		req  Request
		want Outcome
	}{
		{"not a pet", Request{Command: CommandReturnPet, IsPet: false}, OutcomeIgnored},
		{"dead", withReq(base, func(r *Request) { r.SummonIsDead = true }), OutcomeRefusedDead},
		{"out of control", withReq(base, func(r *Request) { r.OutOfControl = true }), OutcomeRefusedOutOfControl},
		{"attacking", withReq(base, func(r *Request) { r.IsAttackingNow = true }), OutcomeRefusedInCombat},
		{"in combat", withReq(base, func(r *Request) { r.InCombat = true }), OutcomeRefusedInCombat},
		{"too hungry", withReq(base, func(r *Request) { r.BelowUnsummonFeedShare = true }), OutcomeRefusedHungry},
		{"applies", base, OutcomeApplied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.req); got != tt.want {
				t.Errorf("Resolve() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveUnsummonServitor(t *testing.T) {
	base := Request{Command: CommandUnsummonServitor, HasSummon: true}

	tests := []struct {
		name string
		req  Request
		want Outcome
	}{
		{"targets a pet, not a servitor", withReq(base, func(r *Request) { r.IsPet = true }), OutcomeIgnored},
		{"no summon", Request{Command: CommandUnsummonServitor}, OutcomeIgnored},
		{"dead", withReq(base, func(r *Request) { r.SummonIsDead = true }), OutcomeRefusedDead},
		{"out of control", withReq(base, func(r *Request) { r.OutOfControl = true }), OutcomeRefusedOutOfControl},
		{"in combat", withReq(base, func(r *Request) { r.InCombat = true }), OutcomeRefusedInCombat},
		{"applies", base, OutcomeApplied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.req); got != tt.want {
				t.Errorf("Resolve() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveMoveToTarget(t *testing.T) {
	base := Request{Command: CommandMoveToTarget, HasSummon: true, HasTarget: true}

	tests := []struct {
		name string
		req  Request
		want Outcome
	}{
		{"no target", Request{Command: CommandMoveToTarget, HasSummon: true}, OutcomeIgnored},
		{"target is own summon", withReq(base, func(r *Request) { r.TargetIsSummon = true }), OutcomeIgnored},
		{"out of control", withReq(base, func(r *Request) { r.OutOfControl = true }), OutcomeRefusedOutOfControl},
		{"applies", base, OutcomeApplied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.req); got != tt.want {
				t.Errorf("Resolve() = %v, want %v", got, tt.want)
			}
		})
	}
}

// withReq returns a copy of base mutated by fn, so table-driven cases can
// vary one field at a time from a shared baseline without aliasing it.
func withReq(base Request, fn func(*Request)) Request {
	r := base
	fn(&r)
	return r
}

// ---- from conditions_test.go ----
// openGeo allows every straight-line move and echoes back requested
// heights/targets unmodified, enough to drive CreatureMove.MoveToLocation
// deterministically without real geodata.
type openGeo struct{}

func (openGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool { return true }
func (openGeo) Height(x, y, z int) int16                { return int16(z) }
func (openGeo) FindPath(origin, target location.Location) ([]location.Location, bool) {
	return nil, false
}
func (openGeo) Walkable(int, int, int) bool { return true }
func (openGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}

// movingGate is a minimal effect.Condition mirroring skill/effect's
// conditionGate for a <player moving="true"/> tag: it resolves effector to
// a conditions.Actor and requires IsMoving(). Exercises #1510: before it,
// summonStatActor.IsMoving() was hardcoded false, so this gate could never
// pass regardless of real movement state.
type movingGate struct{}

func (g movingGate) Test(effector stat.Actor) bool {
	actor, ok := effector.(conditions.Actor)
	return ok && actor.IsMoving()
}

func TestSummonConditionalStatFuncGatesOnRealMovement(t *testing.T) {
	a := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 44, Roll: zeroSummonRoll})

	// HealEffectiveness carries no default summon stat func (see
	// defaultStatFuncs), so its finalized value is driven purely by what
	// AddStatFuncs attaches here.
	a.AddStatFuncs([]effect.Mod{
		{Stat: stat.HealEffectiveness, Op: effect.OpAdd, Value: 25, Cond: movingGate{}},
	})

	if err := a.InitMovement(location.Location{X: 0, Y: 0, Z: 0}, 100, openGeo{}); err != nil {
		t.Fatalf("InitMovement: %v", err)
	}

	if got := a.CalcStat(stat.HealEffectiveness, 100); got != 100 {
		t.Errorf("CalcStat(HealEffectiveness) before moving = %v, want 100 unchanged (gate should fail while still)", got)
	}

	if _, err := a.Move().MoveToLocation(location.Location{X: 1000, Y: 0, Z: 0}); err != nil {
		t.Fatalf("MoveToLocation: %v", err)
	}
	if !a.Move().Moving() {
		t.Fatal("Move().Moving() = false right after an accepted non-zero-distance move request")
	}
	if got := a.CalcStat(stat.HealEffectiveness, 100); got != 125 {
		t.Errorf("CalcStat(HealEffectiveness) while moving = %v, want 125 (gate should pass while moving)", got)
	}

	a.Move().SetPosition(location.Location{X: 1000, Y: 0, Z: 0})
	if a.Move().Moving() {
		t.Fatal("Move().Moving() = true after SetPosition reports arrival at the destination")
	}
	if got := a.CalcStat(stat.HealEffectiveness, 100); got != 100 {
		t.Errorf("CalcStat(HealEffectiveness) after stopping = %v, want 100 unchanged (gate should withdraw once movement stops)", got)
	}
}

// levelGate is a minimal effect.Condition mirroring skill/effect's
// conditionGate: it resolves effector to a conditions.Actor and requires
// its Level() to meet min. Before #1509, summonStatActor didn't implement
// conditions.Actor, so this always resolved to false regardless of min — a
// conditional stat func on a summon-owned skill silently never applied.
type levelGate struct{ min int }

func (g levelGate) Test(effector stat.Actor) bool {
	actor, ok := effector.(conditions.Actor)
	return ok && actor.Level() >= g.min
}

func TestSummonConditionalStatFuncGatesOnRealLevel(t *testing.T) {
	a := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 44, Roll: zeroSummonRoll})

	// HealEffectiveness carries no default summon stat func (see
	// defaultStatFuncs), so its finalized value is driven purely by what
	// AddStatFuncs attaches here.
	a.AddStatFuncs([]effect.Mod{
		{Stat: stat.HealEffectiveness, Op: effect.OpAdd, Value: 25, Cond: levelGate{min: 10}},
	})
	a.AddStatFuncs([]effect.Mod{
		{Stat: stat.RechargeMPRate, Op: effect.OpAdd, Value: 25, Cond: levelGate{min: 100}},
	})

	if got := a.CalcStat(stat.HealEffectiveness, 100); got != 125 {
		t.Errorf("CalcStat(HealEffectiveness) = %v, want 125 (level 44 >= 10 gate should pass)", got)
	}
	if got := a.CalcStat(stat.RechargeMPRate, 100); got != 100 {
		t.Errorf("CalcStat(RechargeMPRate) = %v, want 100 unchanged (level 44 >= 100 gate should fail)", got)
	}
}

func TestSummonStatActorImplementsConditionsActor(t *testing.T) {
	stats := CombatStats{MaxHP: 500, MaxMP: 200}
	a := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 44, Stats: stats, Roll: zeroSummonRoll})

	var actor conditions.Actor = summonStatActor{a: a}
	if actor.Level() != 44 {
		t.Errorf("Level() = %v, want 44", actor.Level())
	}
	if actor.HPRatio() <= 0 {
		t.Errorf("HPRatio() = %v, want > 0 for a freshly spawned summon", actor.HPRatio())
	}
	if !actor.IsRunning() {
		t.Error("IsRunning() = false, want true (non-player actors default to run stance)")
	}
	if actor.IsRiding() || actor.IsFlying() || actor.IsMoving() {
		t.Error("IsRiding()/IsFlying()/IsMoving() should default false for a summon")
	}
	if _, ok := actor.ActiveSkillLevel(1); ok {
		t.Error("ActiveSkillLevel(1) ok = true, want false for a summon with no active effects")
	}
}

// ---- from effects_test.go ----
func TestSummonMaxBuffCountIncludesTemplateDivineInspiration(t *testing.T) {
	servitor := mustServitor(t, ServitorConfig{
		ObjectID: 1,
		Skills:   map[int]int{int(modelskill.DivineInspirationSkillID): 3},
	})

	if got := servitor.MaxBuffCount(); got != 23 {
		t.Fatalf("MaxBuffCount() = %d, want 23", got)
	}
}

func TestSummonMaxBuffCountUsesConfiguredBase(t *testing.T) {
	servitor := mustServitor(t, ServitorConfig{ObjectID: 1, MaxBuffsAmount: 2})

	if got := servitor.MaxBuffCount(); got != 2 {
		t.Fatalf("MaxBuffCount() = %d, want 2", got)
	}
}

func TestSummonEffectListHoldsAppliedEffects(t *testing.T) {
	servitor := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 40, Stats: CombatStats{MaxHP: 500, MaxMP: 200}, Roll: zeroSummonRoll})

	if servitor.MaxBuffCount() != baseBuffSlots {
		t.Fatalf("MaxBuffCount() = %d, want %d", servitor.MaxBuffCount(), baseBuffSlots)
	}

	list := servitor.EffectList()
	if list == nil {
		t.Fatal("EffectList() = nil, want a live list wired at construction")
	}

	e, err := effect.New(effect.Skill{ID: 2280, Level: 1, SkillType: "BUFF"}, modelskill.EffectTemplate{Name: "BlockBuff", Count: 1, Time: 60})
	if err != nil {
		t.Fatalf("effect.New() error = %v", err)
	}
	e.Effector = servitor
	e.Effected = servitor
	list.Add(e)

	if got := len(list.All()); got != 1 {
		t.Fatalf("EffectList().All() len = %d, want 1", got)
	}
}

func TestSummonDenyAIActionHonorsCrowdControlEffects(t *testing.T) {
	for _, tt := range []struct {
		name string
		flag effect.Flag
	}{
		{"stun", effect.FlagStunned},
		{"immobile until attacked", effect.FlagMeditating},
		{"sleep", effect.FlagSleep},
		{"paralyze", effect.FlagParalyzed},
		{"fear", effect.FlagFear},
	} {
		t.Run(tt.name, func(t *testing.T) {
			summon := mustServitor(t, ServitorConfig{ObjectID: 1})
			summon.EffectList().Add(&effect.Effect{Flag: tt.flag})

			if !summon.DenyAIAction() {
				t.Fatal("DenyAIAction() = false while crowd controlled")
			}
		})
	}
}

func TestSummonDenyAIActionHonorsTransientControlStates(t *testing.T) {
	summon := mustServitor(t, ServitorConfig{ObjectID: 1})

	if !summon.SetParalyzed(true) || !summon.DenyAIAction() {
		t.Fatal("paralyzed summon must deny AI actions")
	}
	if !summon.SetParalyzed(false) || summon.DenyAIAction() {
		t.Fatal("clearing paralysis must allow AI actions")
	}
	if !summon.SetTeleporting(true) || !summon.DenyAIAction() {
		t.Fatal("teleporting summon must deny AI actions")
	}
}

// TestSummonOutOfControlHonorsBetrayedFlag is the regression test for the
// review finding that OutOfControl only read a.disabled, so a betrayed
// summon kept accepting owner commands instead of refusing them with
// PET_REFUSING_ORDER, matching Summon.isOutOfControl (Summon.java:296-298):
// super.isOutOfControl() || isBetrayed().
func TestSummonOutOfControlHonorsBetrayedFlag(t *testing.T) {
	summon := mustServitor(t, ServitorConfig{ObjectID: 1})

	if summon.OutOfControl() {
		t.Fatal("OutOfControl() = true before any betray effect, want false")
	}

	summon.EffectList().Add(&effect.Effect{Flag: effect.FlagBetrayed})

	if !summon.OutOfControl() {
		t.Fatal("OutOfControl() = false while betrayed, want true")
	}

	result := summon.ApplyCommand(CommandContext{Command: CommandStop})
	if result.Outcome != OutcomeRefusedOutOfControl {
		t.Fatalf("ApplyCommand(Stop) outcome = %v while betrayed, want OutcomeRefusedOutOfControl", result.Outcome)
	}
}

func TestPetEffectListHoldsAppliedEffects(t *testing.T) {
	pet := mustPet(t, PetConfig{ObjectID: 1, Level: 40, Stats: CombatStats{MaxHP: 500, MaxMP: 200}, Roll: zeroSummonRoll})

	if pet.EffectList() == nil {
		t.Fatal("EffectList() = nil, want a live list wired at construction")
	}
	if pet.MaxBuffCount() != baseBuffSlots {
		t.Fatalf("MaxBuffCount() = %d, want %d", pet.MaxBuffCount(), baseBuffSlots)
	}
}

// ---- from formula_golden_test.go ----
// goldenSummonScenarios is summon.Actor's half of the stat pipeline parity
// oracle described in issue #1527: see the player and npc packages' golden
// tests for the same shape of coverage (same-order attach-sequence
// sensitivity, Set rebasing, attach/detach round-tripping).
func goldenSummonScenarios(t testing.TB) map[string]float64 {
	t.Helper()
	out := make(map[string]float64)

	stats := CombatStats{
		STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20,
		PAtk: 100, PDef: 50, MAtk: 64, MDef: 40,
		MaxHP: 500, MaxMP: 200, BaseRandomDamage: 5,
	}

	{
		a1 := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 44, Stats: stats, Roll: zeroSummonRoll})
		a1.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1e16}})
		a1.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpSub, Value: 1e16}})
		a1.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1}})
		out["order30_forward"] = a1.PDef()

		a2 := mustServitor(t, ServitorConfig{ObjectID: 2, Level: 44, Stats: stats, Roll: zeroSummonRoll})
		a2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1}})
		a2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpSub, Value: 1e16}})
		a2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1e16}})
		out["order30_reverse"] = a2.PDef()
	}

	{
		a := mustPet(t, PetConfig{ObjectID: 3, Level: 44, Stats: stats, Roll: zeroSummonRoll})
		a.AddStatFuncs([]effect.Mod{
			{Stat: stat.MagicDefence, Op: effect.OpSet, Value: 500},
			{Stat: stat.MagicDefence, Op: effect.OpBaseMul, Value: 0.5},
		})
		out["set_rebase_mdef"] = a.MDef()
	}

	{
		a := mustPet(t, PetConfig{ObjectID: 4, Level: 44, Stats: stats, Roll: zeroSummonRoll})
		base := a.PAtk()
		owner := effect.ModOwnerEffect(&effect.Effect{})
		a.AddStatFuncs([]effect.Mod{
			{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 7, Owner: owner},
			{Stat: stat.PowerAttack, Op: effect.OpMul, Value: 1.25, Owner: owner},
		})
		out["attach_detach_before"] = base
		out["attach_detach_during"] = a.PAtk()
		a.RemoveStatsByOwner(owner)
		out["attach_detach_after"] = a.PAtk()
	}

	return out
}

func TestGoldenSummonStatPipelineCapture(t *testing.T) {
	if os.Getenv("ACIS_CAPTURE_GOLDEN") == "" {
		t.Skip("set ACIS_CAPTURE_GOLDEN=1 to (re)capture the golden fixture from the current implementation")
	}
	got := goldenSummonScenarios(t)
	writeSummonGolden(t, "testdata/golden_stats.json", got)
}

func TestGoldenSummonStatPipelineParity(t *testing.T) {
	want := readSummonGolden(t, "testdata/golden_stats.json")
	got := goldenSummonScenarios(t)
	compareSummonGolden(t, want, got)
}

func writeSummonGolden(t testing.TB, path string, values map[string]float64) {
	t.Helper()
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	bitsMap := make(map[string]uint64, len(values))
	for _, k := range keys {
		bitsMap[k] = math.Float64bits(values[k])
	}
	data, err := json.MarshalIndent(bitsMap, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write golden fixture %s: %v", path, err)
	}
}

func readSummonGolden(t testing.TB, path string) map[string]float64 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden fixture %s: %v (capture it first with ACIS_CAPTURE_GOLDEN=1)", path, err)
	}
	var bitsMap map[string]uint64
	if err := json.Unmarshal(data, &bitsMap); err != nil {
		t.Fatalf("unmarshal golden fixture %s: %v", path, err)
	}
	out := make(map[string]float64, len(bitsMap))
	for k, v := range bitsMap {
		out[k] = math.Float64frombits(v)
	}
	return out
}

func compareSummonGolden(t testing.TB, want, got map[string]float64) {
	t.Helper()
	for k, w := range want {
		g, ok := got[k]
		if !ok {
			t.Errorf("golden case %q missing from current run", k)
			continue
		}
		if math.Float64bits(g) != math.Float64bits(w) {
			t.Errorf("golden case %q = %v (bits %x), want %v (bits %x)", k, g, math.Float64bits(g), w, math.Float64bits(w))
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("golden case %q present in current run but not in fixture", k)
		}
	}
}

// ---- from formula_input_test.go ----
func TestSummonFormulaInputsResolveStatsAndResources(t *testing.T) {
	stats := CombatStats{
		STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20,
		PAtk: 100, PDef: 50, MAtk: 64, MDef: 40,
		MaxHP: 500, MaxMP: 200, BaseRandomDamage: 5,
	}
	caster := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 44, Stats: stats, Roll: func(n int) int {
		if n == 10000 {
			return 9999
		}
		return 0
	}})
	target := mustPet(t, PetConfig{ObjectID: 2, Level: 44, Stats: stats, Roll: zeroSummonRoll})

	owner := effect.ModOwnerEffect(&effect.Effect{})
	caster.AddStatFuncs([]effect.Mod{
		{Stat: stat.PvPPhysSkillDmg, Op: effect.OpMul, Value: 0.8, Owner: owner},
		{Stat: stat.PvPMagicalDmg, Op: effect.OpMul, Value: 1.3, Owner: owner},
		{Stat: stat.HealProficiency, Op: effect.OpAdd, Value: 11, Owner: owner},
	})
	target.AddStatFuncs([]effect.Mod{
		{Stat: stat.FireRes, Op: effect.OpMul, Value: 0.36, Owner: owner},
		{Stat: stat.StunVuln, Op: effect.OpMul, Value: 0.5, Owner: owner},
		{Stat: stat.DaggerWpnVuln, Op: effect.OpMul, Value: 0.8, Owner: owner},
		{Stat: stat.RechargeMPRate, Op: effect.OpMul, Value: 1.5, Owner: owner},
		{Stat: stat.HealEffectiveness, Op: effect.OpMul, Value: 1.2, Owner: owner},
	})

	if got := target.Category(); got != skilltarget.CategoryPlayable {
		t.Fatalf("Category() = %v, want playable", got)
	}
	if !target.Playable() {
		t.Fatal("Playable() = false for a pet")
	}

	phys, ok := target.PhysicalSkillInput(caster, modelskill.Definition{
		Power:        30,
		SkillType:    "PDAM",
		Element:      modelskill.ElementFire,
		BaseCritRate: 1,
	})
	if !ok {
		t.Fatal("PhysicalSkillInput() ok = false")
	}
	if !phys.Crit {
		t.Fatal("PhysicalSkillInput Crit = false, want true with zero roll")
	}
	if got, want := phys.RandomMul, 0.95; !closeSummonFloat(got, want) {
		t.Fatalf("PhysicalSkillInput RandomMul = %v, want %v", got, want)
	}
	if got, want := phys.RaceMul, 1.0; !closeSummonFloat(got, want) {
		t.Fatalf("PhysicalSkillInput RaceMul = %v, want %v", got, want)
	}
	if got, want := phys.PvPMul, 0.8; !closeSummonFloat(got, want) {
		t.Fatalf("PhysicalSkillInput PvPMul = %v, want %v", got, want)
	}
	if got, want := phys.ElementalMul, 0.36; !closeSummonFloat(got, want) {
		t.Fatalf("PhysicalSkillInput ElementalMul = %v, want %v", got, want)
	}

	magic, ok := target.MagicDamageInput(caster, modelskill.Definition{
		Power:     40,
		SkillType: "MDAM",
		Magic:     true,
		Element:   modelskill.ElementFire,
	})
	if !ok {
		t.Fatal("MagicDamageInput() ok = false")
	}
	if !magic.MagicCrit {
		t.Fatal("MagicDamageInput MagicCrit = false, want true with zero roll")
	}
	if got, want := magic.PvPMul, 1.3; !closeSummonFloat(got, want) {
		t.Fatalf("MagicDamageInput PvPMul = %v, want %v", got, want)
	}
	if got, want := magic.ElementalMul, 0.36; !closeSummonFloat(got, want) {
		t.Fatalf("MagicDamageInput ElementalMul = %v, want %v", got, want)
	}

	blow, ok := target.BlowInput(caster, modelskill.Definition{Power: 30, SkillType: "BLOW"})
	if !ok {
		t.Fatal("BlowInput() ok = false")
	}
	if !blow.IsPvP {
		t.Fatal("BlowInput IsPvP = false for summon-vs-pet")
	}
	if got, want := blow.RandomMul, 0.95; !closeSummonFloat(got, want) {
		t.Fatalf("BlowInput RandomMul = %v, want %v", got, want)
	}
	if got, want := blow.DaggerVulnMul, 0.8; !closeSummonFloat(got, want) {
		t.Fatalf("BlowInput DaggerVulnMul = %v, want %v", got, want)
	}

	mana, ok := target.ManaDamageInput(caster, modelskill.Definition{
		Power:     20,
		SkillType: "MANADAM",
		Element:   modelskill.ElementFire,
	})
	if !ok {
		t.Fatal("ManaDamageInput() ok = false")
	}
	if mana.MAtk <= 0 || mana.MDef <= 0 || mana.TargetMaxMp <= 0 {
		t.Fatalf("ManaDamageInput non-positive values = %+v", mana)
	}
	if got, want := mana.VulnMul, 0.6; !closeSummonFloat(got, want) {
		t.Fatalf("ManaDamageInput VulnMul = %v, want %v", got, want)
	}

	success, ok := target.SkillSuccessInput(caster, modelskill.Definition{
		SkillType:    "STUN",
		EffectType:   "STUN",
		Magic:        true,
		BaseLandRate: 50,
		Element:      modelskill.ElementFire,
	}, false, formulas.ShieldPerfect)
	if !ok {
		t.Fatal("SkillSuccessInput() ok = false")
	}
	if success.BaseChance != 50 || success.Shield != formulas.ShieldPerfect {
		t.Fatalf("SkillSuccessInput base/shield = %+v", success)
	}
	if got, want := success.VulnModifier, 0.3; !closeSummonFloat(got, want) {
		t.Fatalf("SkillSuccessInput VulnModifier = %v, want %v", got, want)
	}
	if rate := formulas.SkillSuccessRate(success); rate != 0 {
		t.Fatalf("SkillSuccessRate() = %v, want 0 for perfect shield", rate)
	}

	target.SetHP(100)
	if got := target.AddHP(25); got != 25 {
		t.Fatalf("AddHP() = %v, want 25", got)
	}
	target.ReduceHP(20.5, caster, modelskill.Definition{SkillType: "PDAM"})
	if got, want := target.HP(), 104.5; !closeSummonFloat(got, want) {
		t.Fatalf("HP after ReduceHP = %v, want %v", got, want)
	}
	mp := target.MPValue()
	if got := target.ReduceMP(15); got != 15 {
		t.Fatalf("ReduceMP() = %v, want 15", got)
	}
	if got := target.AddMP(10); got != 10 {
		t.Fatalf("AddMP() = %v, want 10", got)
	}
	if got, want := target.MPValue(), mp-5; !closeSummonFloat(got, want) {
		t.Fatalf("MP after ReduceMP/AddMP = %v, want %v", got, want)
	}
	if !target.CanBeHealed() || target.Invul() || target.Invulnerable() {
		t.Fatalf("healing/invulnerability flags: CanBeHealed=%v Invul=%v Invulnerable=%v", target.CanBeHealed(), target.Invul(), target.Invulnerable())
	}
	if got, want := target.HealEffectiveness(), 120.0; !closeSummonFloat(got, want) {
		t.Fatalf("HealEffectiveness() = %v, want %v", got, want)
	}
	if got, want := target.RechargeMP(10), 15.0; !closeSummonFloat(got, want) {
		t.Fatalf("RechargeMP() = %v, want %v", got, want)
	}

	heal, ok := caster.HealAmount(modelskill.Definition{SkillType: "HEAL", Power: 25})
	if !ok {
		t.Fatal("HealAmount() ok = false")
	}
	wantHeal := 25.0 + 11 + math.Sqrt(float64(int(caster.MAtk())))
	if !closeSummonFloat(heal, wantHeal) {
		t.Fatalf("HealAmount() = %v, want %v", heal, wantHeal)
	}
	static, ok := caster.HealAmount(modelskill.Definition{SkillType: "HEAL_STATIC", Power: 25})
	if !ok {
		t.Fatal("HealAmount(static) ok = false")
	}
	if got, want := static, 36.0; !closeSummonFloat(got, want) {
		t.Fatalf("HealAmount(static) = %v, want %v", got, want)
	}
}

func TestSummonSkillReflectInputUsesMagicSpecificStat(t *testing.T) {
	target := mustPet(t, PetConfig{ObjectID: 1, Stats: CombatStats{}})
	refOwner := effect.ModOwnerEffect(&effect.Effect{})
	target.AddStatFuncs([]effect.Mod{
		{Stat: stat.ReflectSkillMagic, Op: effect.OpSet, Value: 17, Owner: refOwner},
		{Stat: stat.ReflectSkillPhysic, Op: effect.OpSet, Value: 29, Owner: refOwner},
	})

	magic := target.SkillReflectInput(modelskill.Definition{Magic: true, CanBeReflected: true, CastRange: 900})
	if magic.ReflectChance != 17 || !magic.CanBeReflected || magic.CastRange != 900 {
		t.Fatalf("magic SkillReflectInput() = %+v", magic)
	}
	if !formulas.SkillReflects(magic, 0) {
		t.Fatal("magic SkillReflectInput() does not reflect")
	}
	physical := target.SkillReflectInput(modelskill.Definition{CanBeReflected: true, CastRange: 40, IgnoreResists: true})
	if physical.ReflectChance != 29 || !physical.IgnoreResists || !physical.CanBeReflected || physical.CastRange != 40 {
		t.Fatalf("physical SkillReflectInput() = %+v", physical)
	}
	if formulas.SkillReflects(physical, 0) {
		t.Fatal("physical SkillReflectInput() reflects despite IgnoreResists")
	}
}

func TestSummonCalcStatFloorsNonPositiveValues(t *testing.T) {
	for _, value := range []float64{0, -1} {
		a := mustPet(t, PetConfig{ObjectID: 1})
		a.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpSet, Value: value}})

		if got := a.CalcStat(stat.PowerAttack, 10); got != 1 {
			t.Errorf("CalcStat(PowerAttack, %v) = %v, want 1", value, got)
		}
	}
}

func TestDeadConcurrentReduceHPAndRead(t *testing.T) {
	actor := mustPet(t, PetConfig{Stats: CombatStats{MaxHP: 1}})
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(started)
		actor.ReduceHP(1, nil, modelskill.Definition{})
		close(done)
	}()
	<-started
	for range 1000 {
		_ = actor.Dead()
		_ = actor.AlikeDead()
	}
	<-done
}

func zeroSummonRoll(int) int { return 0 }

func closeSummonFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// ---- from formula_speed_test.go ----
func TestActorPAtkSpdHungryHalvesBase(t *testing.T) {
	hungry := mustPet(t, PetConfig{ObjectID: 1, Level: 10, MaxMeal: 100, Fed: 10, HungryLimit: 0.3, Roll: zeroSummonRoll})
	fed := mustPet(t, PetConfig{ObjectID: 2, Level: 10, MaxMeal: 100, Fed: 100, HungryLimit: 0.3, Roll: zeroSummonRoll})

	hungryValue := hungry.PAtkSpd(300)
	fedValue := fed.PAtkSpd(300)
	if hungryValue*2 != fedValue {
		t.Fatalf("PAtkSpd(300) hungry=%v fed=%v, want hungry == fed/2 (base halved while under-fed)", hungryValue, fedValue)
	}
}

func TestActorMAtkSpdHungryHalvesBase(t *testing.T) {
	hungry := mustPet(t, PetConfig{ObjectID: 1, Level: 10, MaxMeal: 100, Fed: 10, HungryLimit: 0.3, Roll: zeroSummonRoll})
	fed := mustPet(t, PetConfig{ObjectID: 2, Level: 10, MaxMeal: 100, Fed: 100, HungryLimit: 0.3, Roll: zeroSummonRoll})

	hungryValue := hungry.MAtkSpd()
	fedValue := fed.MAtkSpd()
	if hungryValue*2 != fedValue {
		t.Fatalf("MAtkSpd() hungry=%v fed=%v, want hungry == fed/2 (base halved while under-fed)", hungryValue, fedValue)
	}
}

func TestActorCriticalRateCapsAt500(t *testing.T) {
	pet := mustPet(t, PetConfig{ObjectID: 1, Level: 10, Roll: zeroSummonRoll})
	if got := pet.CriticalRate(600); got != 500 {
		t.Fatalf("CriticalRate(600) = %v, want capped at 500", got)
	}
}

// TestActorCriticalRateTruncatesBeforeCap pins the boundary from
// CreatureStatus.getCriticalHit (CreatureStatus.java:551-553):
// `Math.min((int) calcStat(...), 500)`. A base critical rate of 8.48
// finalizes to 84.8 (base*10, no DEX bonus for summons); truncating first
// yields the int 84, not the untruncated 84.8.
func TestActorCriticalRateTruncatesBeforeCap(t *testing.T) {
	pet := mustPet(t, PetConfig{ObjectID: 1, Level: 10, Roll: zeroSummonRoll})
	if got := pet.CriticalRate(8.48); got != 84 {
		t.Fatalf("CriticalRate(8.48) = %v, want truncated to 84", got)
	}
}

func TestActorMAtkSpdServitorNeverHalved(t *testing.T) {
	// A servitor has no feeding state at all (isPet is false), so its
	// magic attack speed must equal a well-fed pet's, never a hungry one's.
	servitor := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 10, Roll: zeroSummonRoll})
	fed := mustPet(t, PetConfig{ObjectID: 2, Level: 10, MaxMeal: 100, Fed: 100, HungryLimit: 0.3, Roll: zeroSummonRoll})
	if servitor.MAtkSpd() != fed.MAtkSpd() {
		t.Fatalf("MAtkSpd() servitor=%v, want unhalved (matching a well-fed pet) %v", servitor.MAtkSpd(), fed.MAtkSpd())
	}
}

// ---- from lethal_test.go ----
type deniedLethalCaster struct{ *Actor }

func (deniedLethalCaster) CanGiveDamage() bool { return false }

func TestActorLethalSurfaceBuildsInputAndAppliesOutcomes(t *testing.T) {
	caster := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 40, Stats: CombatStats{MaxHP: 500}})
	target := mustServitor(t, ServitorConfig{ObjectID: 2, Level: 45, Stats: CombatStats{MaxHP: 500}})
	skill := modelskill.Definition{LethalChance1: 30, LethalChance2: 10, MagicLevel: 40}

	in, ok := target.LethalInput(caster, skill)
	if !ok {
		t.Fatal("LethalInput() ok = false")
	}
	if in.Chance1 != 30 || in.Chance2 != 10 || in.MagicLevel != 40 || in.AttackerLevel != 40 || in.TargetLevel != 45 || in.LethalMul != 1 {
		t.Fatalf("LethalInput() = %+v, want skill fields and 40/45/1 actor values", in)
	}

	hp := target.MaxHPValue()
	target.SetHP(hp)
	target.ApplyLethalOutcome(formulas.LethalHalf, caster, skill)
	if got := target.HP(); got != hp/2 {
		t.Fatalf("half lethal HP = %v, want %v", got, hp/2)
	}

	target.SetHP(hp)
	target.ApplyLethalOutcome(formulas.LethalFull, caster, skill)
	if got := target.HP(); got != 1 {
		t.Fatalf("full lethal HP = %v, want 1", got)
	}
}

func TestActorLethalInputRejectsGuardedDamage(t *testing.T) {
	caster := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 40, Stats: CombatStats{MaxHP: 500}})
	target := mustServitor(t, ServitorConfig{ObjectID: 2, Level: 45, Stats: CombatStats{MaxHP: 500}})
	skill := modelskill.Definition{LethalChance1: 30}
	target.SetInvul(true)
	if _, ok := target.LethalInput(caster, skill); ok {
		t.Fatal("LethalInput accepted an invulnerable summon")
	}
	target.SetInvul(false)
	if _, ok := target.LethalInput(deniedLethalCaster{caster}, skill); ok {
		t.Fatal("LethalInput accepted an attacker without damage permission")
	}
}

// ---- from lifetime_test.go ----
func TestInitialNextConsumeTime(t *testing.T) {
	tests := []struct {
		name                         string
		totalLifeTime, steps, itemID int
		want                         int
	}{
		{"no consume item", 10000, 1, 0, -1},
		{"zero steps", 10000, 0, 57, -1},
		{"even split", 10000, 1, 57, 5000},
		{"three steps", 1200000, 3, 57, 900000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InitialNextConsumeTime(tt.totalLifeTime, tt.steps, tt.itemID); got != tt.want {
				t.Errorf("InitialNextConsumeTime() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestTick_Sequence walks a small servitor through its full life, checking
// each tick's expired/dueForUpkeep flags and the running countdown state.
// totalLifeTime=10000 with 1 consume step means the first (only) upkeep
// checkpoint sits at 5000; each 2000-cost tick should cross a checkpoint on
// the ticks where the remaining time steps from above the checkpoint to at
// or below it.
func TestTick_Sequence(t *testing.T) {
	state := LifetimeState{
		TimeRemaining:       10000,
		TotalLifeTime:       10000,
		NextItemConsumeTime: InitialNextConsumeTime(10000, 1, 57),
		ItemConsumeSteps:    1,
	}
	if state.NextItemConsumeTime != 5000 {
		t.Fatalf("setup: NextItemConsumeTime = %d, want 5000", state.NextItemConsumeTime)
	}

	type step struct {
		wantRemaining int
		wantExpired   bool
		wantUpkeep    bool
	}
	steps := []step{
		{8000, false, false},
		{6000, false, false},
		{4000, false, true}, // crosses the 5000 checkpoint
		{2000, false, false},
		{0, false, true}, // crosses the second checkpoint at 0
		{-2000, true, false},
	}

	for i, want := range steps {
		next, expired, upkeep := Tick(state, 2000)
		if expired != want.wantExpired {
			t.Errorf("tick %d: expired = %v, want %v", i+1, expired, want.wantExpired)
		}
		if upkeep != want.wantUpkeep {
			t.Errorf("tick %d: dueForUpkeep = %v, want %v", i+1, upkeep, want.wantUpkeep)
		}
		if next.TimeRemaining != want.wantRemaining {
			t.Errorf("tick %d: TimeRemaining = %d, want %d", i+1, next.TimeRemaining, want.wantRemaining)
		}
		state = next
		if expired {
			break
		}
	}
}

func TestTick_NeverConsumes(t *testing.T) {
	state := LifetimeState{
		TimeRemaining:       10000,
		TotalLifeTime:       10000,
		NextItemConsumeTime: -1, // no consume item
		ItemConsumeSteps:    0,
	}
	for i := 0; i < 5; i++ {
		next, expired, upkeep := Tick(state, 1000)
		if upkeep {
			t.Fatalf("tick %d: dueForUpkeep = true, want false (no consume item)", i+1)
		}
		if expired {
			t.Fatalf("tick %d: expired unexpectedly at TimeRemaining=%d", i+1, next.TimeRemaining)
		}
		state = next
	}
}

// ---- from shots_test.go ----
func TestSummonChargedShotStateAndCounts(t *testing.T) {
	servitor := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 40, Stats: CombatStats{MaxHP: 500, MaxMP: 200, SSCount: 5, SPSCount: 3}, Roll: zeroSummonRoll})

	if servitor.SoulshotCharged() || servitor.SpiritshotCharged() || servitor.BlessedSpiritshotCharged() {
		t.Fatal("new summon should carry no shot charge")
	}
	if servitor.SSCount() != 5 {
		t.Fatalf("SSCount() = %d, want 5", servitor.SSCount())
	}
	if servitor.SPSCount() != 3 {
		t.Fatalf("SPSCount() = %d, want 3", servitor.SPSCount())
	}

	servitor.SetChargedShot(item.ShotSoul, true)
	if !servitor.SoulshotCharged() {
		t.Fatal("SoulshotCharged() = false after SetChargedShot(ShotSoul, true)")
	}
	if servitor.SpiritshotCharged() || servitor.BlessedSpiritshotCharged() {
		t.Fatal("charging soulshot must not charge spiritshot kinds")
	}

	servitor.SetChargedShot(item.ShotSoul, false)
	if servitor.SoulshotCharged() {
		t.Fatal("SoulshotCharged() = true after SetChargedShot(ShotSoul, false)")
	}
}

// ---- from status_update_test.go ----
type namedDamageAttacker struct{ name string }

func (a namedDamageAttacker) ObjectID() int32       { return 1 }
func (a namedDamageAttacker) Dead() bool            { return false }
func (a namedDamageAttacker) CharacterName() string { return a.name }

// anonymousAttacker satisfies effect.Participant but not the
// CharacterName-having surface notifyDamage looks for, so it stands in for
// an attacker whose identity the notifier doesn't recognize.
type anonymousAttacker struct{}

func (anonymousAttacker) ObjectID() int32 { return 0 }
func (anonymousAttacker) Dead() bool      { return false }

func TestReduceHPUpdatesStatusAfterDirectAndDOTDamage(t *testing.T) {
	for _, damage := range []struct {
		name  string
		apply func(*Actor)
	}{
		{"direct", func(a *Actor) { a.ReduceHP(10, nil, modelskill.Definition{}) }},
		{"dot", func(a *Actor) { a.ReduceHPByDOT(10, nil, true) }},
	} {
		t.Run(damage.name, func(t *testing.T) {
			a := mustPet(t, PetConfig{Stats: CombatStats{MaxHP: 100}})
			updates := 0
			a.SetStatusUpdater(func() { updates++ })

			damage.apply(a)

			if updates != 1 {
				t.Fatalf("status updates = %d, want 1", updates)
			}
		})
	}
}

func TestReduceHPNotifiesKnownDirectAttackerOnly(t *testing.T) {
	for _, tc := range []struct {
		name   string
		new    func() *Actor
		apply  func(*Actor, effect.Participant)
		called bool
	}{
		{"pet direct", func() *Actor { return mustPet(t, PetConfig{Stats: CombatStats{MaxHP: 100}}) }, func(a *Actor, attacker effect.Participant) { a.ReduceHP(12.9, attacker, modelskill.Definition{}) }, true},
		{"servitor direct", func() *Actor { return mustServitor(t, ServitorConfig{Stats: CombatStats{MaxHP: 100}}) }, func(a *Actor, attacker effect.Participant) { a.ReduceHP(12.9, attacker, modelskill.Definition{}) }, true},
		{"dot", func() *Actor { return mustPet(t, PetConfig{Stats: CombatStats{MaxHP: 100}}) }, func(a *Actor, attacker effect.Participant) { a.ReduceHPByDOT(12.9, attacker, true) }, false},
		{"unknown attacker", func() *Actor { return mustPet(t, PetConfig{Stats: CombatStats{MaxHP: 100}}) }, func(a *Actor, attacker effect.Participant) { a.ReduceHP(12.9, attacker, modelskill.Definition{}) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotName string
			var gotDamage int32
			calls := 0
			a := tc.new()
			a.SetDamageNotifier(func(name string, damage int32) { calls++; gotName, gotDamage = name, damage })
			var attacker effect.Participant = namedDamageAttacker{name: "Attacker"}
			if tc.name == "unknown attacker" {
				attacker = anonymousAttacker{}
			}
			tc.apply(a, attacker)
			if tc.called {
				if calls != 1 || gotName != "Attacker" || gotDamage != 12 {
					t.Fatalf("notification = (%d, %q, %d), want (1, Attacker, 12)", calls, gotName, gotDamage)
				}
			} else if calls != 0 {
				t.Fatalf("notifications = %d, want 0", calls)
			}
		})
	}
}

func TestNewServitorAppliesTemplatePassivesBeforeHPSeed(t *testing.T) {
	stats := CombatStats{MaxHP: 1000, CON: 40}
	base := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 44, Stats: stats, Roll: zeroSummonRoll})

	passive := modelskill.Ref{ID: 99, Level: 1}
	table := modelskill.NewTable([]modelskill.Definition{{
		ID:         passive.ID,
		Level:      passive.Level,
		Activation: modelskill.ActivationActive,
		Funcs:      []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: 250}},
	}})
	got := mustServitor(t, ServitorConfig{
		ObjectID:  2,
		Level:     44,
		Stats:     stats,
		Roll:      zeroSummonRoll,
		Passives:  []modelskill.Ref{passive},
		SkillDefs: table,
	})

	wantMax := base.MaxHPValue() + 250
	if got.MaxHPValue() != wantMax {
		t.Fatalf("MaxHPValue() = %v, want %v (base %v + 250 add after CON mul)", got.MaxHPValue(), wantMax, base.MaxHPValue())
	}
	if got.HP() != wantMax {
		t.Fatalf("HP() = %v, want %v (seed after funcs attach)", got.HP(), wantMax)
	}
}

func TestNewPetAppliesTemplatePassivesBeforeHPSeed(t *testing.T) {
	stats := CombatStats{MaxHP: 500, CON: 40}
	base := mustPet(t, PetConfig{ObjectID: 1, Level: 40, Stats: stats, Roll: zeroSummonRoll})

	passive := modelskill.Ref{ID: 99, Level: 1}
	table := modelskill.NewTable([]modelskill.Definition{{
		ID:         passive.ID,
		Level:      passive.Level,
		Activation: modelskill.ActivationActive,
		Funcs:      []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: 250}},
	}})
	got := mustPet(t, PetConfig{
		ObjectID:  2,
		Level:     40,
		Stats:     stats,
		Roll:      zeroSummonRoll,
		Passives:  []modelskill.Ref{passive},
		SkillDefs: table,
	})

	wantMax := base.MaxHPValue() + 250
	if got.MaxHPValue() != wantMax {
		t.Fatalf("MaxHPValue() = %v, want %v", got.MaxHPValue(), wantMax)
	}
	if got.HP() != wantMax {
		t.Fatalf("HP() = %v, want %v", got.HP(), wantMax)
	}
}

func TestNewServitorFailsOnTemplatePassiveBuildError(t *testing.T) {
	table := modelskill.NewTable([]modelskill.Definition{{
		ID:    99,
		Level: 1,
		Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "notAStat", Value: 1}},
	}})
	_, err := NewServitor(ServitorConfig{
		ObjectID:  1,
		Passives:  []modelskill.Ref{{ID: 99, Level: 1}},
		SkillDefs: table,
	})
	if err == nil {
		t.Fatal("NewServitor() error = nil, want template-passive build error")
	}
}
