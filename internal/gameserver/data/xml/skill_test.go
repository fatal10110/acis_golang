package xml

import (
	"bytes"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	gameskill "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/rs/zerolog"
)

// fakeWearingActor is a minimal stat.Actor+conditions.Actor/PlayerActor
// double: only IsWearingType reports anything meaningful, which is all
// TestConditionalStatFuncsBuildForEveryShippedSkill's using-kind assertion
// needs.
type fakeWearingActor struct{ mask int }

func (fakeWearingActor) STR() int                          { return 0 }
func (fakeWearingActor) CON() int                          { return 0 }
func (fakeWearingActor) DEX() int                          { return 0 }
func (fakeWearingActor) INT() int                          { return 0 }
func (fakeWearingActor) WIT() int                          { return 0 }
func (fakeWearingActor) MEN() int                          { return 0 }
func (fakeWearingActor) LevelMod() float64                 { return 1 }
func (fakeWearingActor) IsSummon() bool                    { return false }
func (fakeWearingActor) Level() int                        { return 1 }
func (fakeWearingActor) HPRatio() float64                  { return 1 }
func (fakeWearingActor) MPRatio() float64                  { return 1 }
func (fakeWearingActor) X() int                            { return 0 }
func (fakeWearingActor) Y() int                            { return 0 }
func (fakeWearingActor) Z() int                            { return 0 }
func (fakeWearingActor) IsMoving() bool                    { return false }
func (fakeWearingActor) IsRunning() bool                   { return false }
func (fakeWearingActor) IsRiding() bool                    { return false }
func (fakeWearingActor) IsFlying() bool                    { return false }
func (fakeWearingActor) IsBehind(conditions.Actor) bool    { return false }
func (fakeWearingActor) IsInFrontOf(conditions.Actor) bool { return false }
func (fakeWearingActor) ActiveSkillLevel(int) (int, bool)  { return 0, false }
func (fakeWearingActor) ActiveEffectLevel(int) (int, bool) { return 0, false }
func (fakeWearingActor) IsSitting() bool                   { return false }
func (fakeWearingActor) IsInOlympiadMode() bool            { return false }
func (fakeWearingActor) IsHero() bool                      { return false }
func (fakeWearingActor) PkKills() int                      { return 0 }
func (fakeWearingActor) PledgeClass() int                  { return 0 }
func (fakeWearingActor) IsClanLeader() bool                { return false }
func (fakeWearingActor) HasClan() bool                     { return false }
func (fakeWearingActor) ClanCastleID() int                 { return 0 }
func (fakeWearingActor) ClanHasAnyCastle() bool            { return false }
func (fakeWearingActor) ClanHallID() int                   { return 0 }
func (fakeWearingActor) ClanHasAnyClanHall() bool          { return false }
func (fakeWearingActor) Race() int                         { return 0 }
func (fakeWearingActor) Sex() int                          { return 0 }
func (fakeWearingActor) WeightPenalty() int                { return 0 }
func (fakeWearingActor) InventorySize() int                { return 0 }
func (fakeWearingActor) InventoryLimit() int               { return 0 }
func (fakeWearingActor) Charges() int                      { return 0 }
func (a fakeWearingActor) IsWearingType(mask int) bool     { return a.mask&mask != 0 }

var _ stat.Actor = fakeWearingActor{}

// TestConditionalStatFuncsBuildForEveryShippedSkill covers issue #1499's
// acceptance criteria directly against the real datapack: every shipped
// skill's conditional stat funcs (statFuncs' <using>/<player>/<and>/<not>/
// <game> predicates) build without the former "not wired yet" refusal, and
// a character holding one of the affected core masteries enters the world
// (ApplyTransientPassiveSkill succeeds) with the bonus gated correctly by
// its equipped item type.
//
// effect.New also validates that the effect template's own name resolves to
// a known core effect kind — a broader claim than #1499 makes. The only
// shipped names that legitimately fail this are the unresolved "#table"
// reference names (e.g. "#effectname1") tracked by #1516 — a loader defect,
// not a missing effect kind — so this loop only tolerates
// effect.ErrUnsupportedCoreEffect for a name starting with "#". Any other
// unsupported-core-effect name, or any other error (including one from
// statFuncs), still fails the test: #1517 closed the last real coreKinds
// gap (the Signet family and ClanGate), so a new one here means a shipped
// template has genuinely gone unhandled again.
func TestConditionalStatFuncsBuildForEveryShippedSkill(t *testing.T) {
	dir := datapackPath(t, filepath.Join("data", "xml", "skills"))
	table, err := LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions(%q) error: %v", dir, err)
	}

	built := 0
	for _, def := range table.All() {
		switch def.Activation {
		case skill.ActivationPassive:
			if _, err := effect.PassiveFuncs(def); err != nil {
				t.Fatalf("PassiveFuncs(skill %d level %d %q): %v", def.ID, def.Level, def.Name, err)
			}
			built++
		default:
			for _, eff := range def.Effects {
				if _, err := effect.New(effect.SkillFromDefinition(def), eff); err != nil {
					if errors.Is(err, effect.ErrUnsupportedCoreEffect) && strings.HasPrefix(eff.Name, "#") {
						continue
					}
					t.Fatalf("effect.New(skill %d level %d %q, effect %q): %v", def.ID, def.Level, def.Name, eff.Name, err)
				}
				built++
			}
			for _, eff := range def.SelfEffects {
				if _, err := effect.New(effect.SkillFromDefinition(def), eff); err != nil {
					if errors.Is(err, effect.ErrUnsupportedCoreEffect) && strings.HasPrefix(eff.Name, "#") {
						continue
					}
					t.Fatalf("effect.New(skill %d level %d %q, self-effect %q): %v", def.ID, def.Level, def.Name, eff.Name, err)
				}
				built++
			}
		}
	}
	if built == 0 {
		t.Fatal("expected at least one built skill/effect, got 0 — datapack may not have loaded")
	}

	t.Run("Armor Mastery rEvas applies only while wearing light armor", func(t *testing.T) {
		def, ok := table.Get(142, 5)
		if !ok {
			t.Fatal("skill 142 level 5 not loaded")
		}
		funcs, err := effect.PassiveFuncs(def)
		if err != nil {
			t.Fatalf("PassiveFuncs(skill 142 level 5): %v", err)
		}
		var rEvasFunc, pDefFunc bool
		for _, fn := range funcs {
			if fn.Cond == nil {
				pDefFunc = true
				continue
			}
			rEvasFunc = true
			const lightMask = 1 << 15 // item.ArmorLight.Mask()
			if !fn.Cond.Test(fakeWearingActor{mask: lightMask}) {
				t.Error("rEvas bonus should apply while wearing light armor")
			}
			if fn.Cond.Test(fakeWearingActor{mask: 0}) {
				t.Error("rEvas bonus should not apply while not wearing light armor")
			}
		}
		if !rEvasFunc || !pDefFunc {
			t.Fatalf("expected both an unconditional pDef func and a conditional rEvas func, got funcs=%+v", funcs)
		}

		if err := gameskill.NewPersistence(nil, table).ApplyTransientPassiveSkill(&player.Character{}, 142, 0, 5); err != nil {
			t.Fatalf("ApplyTransientPassiveSkill(skill 142 level 5) error: %v", err)
		}
	})
}

func TestLoadSkillDefinitions(t *testing.T) {
	dir := datapackPath(t, filepath.Join("data", "xml", "skills"))

	table, err := LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions(%q) error: %v", dir, err)
	}

	// The full 34-file datapack loads 29742 distinct (id, level) skill
	// definitions with no dropped or duplicated entries.
	if got, want := table.Len(), 29742; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}

	t.Run("regular level with table-substituted fields", func(t *testing.T) {
		d, ok := table.Get(3, 1)
		if !ok {
			t.Fatal("skill 3 level 1 not loaded")
		}
		if d.Name != "Power Strike" || d.MagicLevel != 3 || d.Power != 25.0 || d.MPConsume != 10 {
			t.Fatalf("skill 3 level 1 = %+v", d)
		}
		if d.Target != skill.TargetOne || d.SkillType != "PDAM" || d.Activation != skill.ActivationActive {
			t.Fatalf("skill 3 level 1 tags = target=%v skillType=%v activation=%v", d.Target, d.SkillType, d.Activation)
		}
		if d.CastRange != 40 || d.EffectRange != 400 || d.ReuseDelay != 13000 || d.HitTime != 1080 || d.CoolTime != 720 {
			t.Fatalf("skill 3 level 1 timing = %+v", d)
		}
		if !d.Overhit || !d.NextActionIsAttack || d.SoulShotBoost != 2.0 {
			t.Fatalf("skill 3 level 1 flags = %+v", d)
		}
		// PDAM defaults BaseCritRate to 0 and Offensive to true when the
		// level doesn't set either explicitly.
		if d.BaseCritRate != 0 || !d.Offensive {
			t.Fatalf("skill 3 level 1 defaults = baseCritRate=%d offensive=%v", d.BaseCritRate, d.Offensive)
		}

		last, ok := table.Get(3, 9)
		if !ok {
			t.Fatal("skill 3 level 9 not loaded")
		}
		if last.MagicLevel != 15 || last.Power != 70.0 || last.MPConsume != 19 {
			t.Fatalf("skill 3 level 9 = %+v", last)
		}
		if table.MaxLevel(3) != 9 {
			t.Fatalf("MaxLevel(3) = %d, want 9", table.MaxLevel(3))
		}
	})

	t.Run("death penalty passive resolves at every level", func(t *testing.T) {
		for level := 1; level <= 15; level++ {
			def, ok := table.Get(5076, level)
			if !ok {
				t.Fatalf("skill 5076 level %d not loaded", level)
			}
			if def.Activation != skill.ActivationPassive {
				t.Fatalf("skill 5076 level %d activation = %v, want passive", level, def.Activation)
			}
			funcs, err := effect.PassiveFuncs(def)
			if err != nil {
				t.Fatalf("PassiveFuncs(skill 5076 level %d) error: %v", level, err)
			}
			if got, want := len(funcs), 9; got != want {
				t.Fatalf("PassiveFuncs(skill 5076 level %d) = %d funcs, want %d", level, got, want)
			}
		}

		if err := gameskill.NewPersistence(nil, table).ApplyTransientPassiveSkill(&player.Character{}, 5076, 0, 15); err != nil {
			t.Fatalf("ApplyTransientPassiveSkill(skill 5076 level 15) error: %v", err)
		}
	})

	t.Run("enchant levels reuse the max regular level except for their own table row", func(t *testing.T) {
		regular37, ok := table.Get(1, 37)
		if !ok {
			t.Fatal("skill 1 level 37 not loaded")
		}
		if regular37.MagicLevel != 74 || regular37.Power != 2131.0 || regular37.MPConsume != 97 {
			t.Fatalf("skill 1 level 37 = %+v", regular37)
		}

		// Level 101 (first enchantLevels1 step): magicLvl and power come
		// from the enchant1-specific tables at their own row; mpConsume,
		// untouched by any <enchant1> tag, carries over from level 37.
		ench1, ok := table.Get(1, 101)
		if !ok {
			t.Fatal("skill 1 level 101 not loaded")
		}
		if ench1.MagicLevel != 76 || ench1.Power != 2151.0 || ench1.MPConsume != 97 {
			t.Fatalf("skill 1 level 101 = %+v", ench1)
		}

		// Level 141 (first enchantLevels2 step): mpConsume comes from the
		// enchant2-specific table at its own row; power, untouched by any
		// <enchant2> tag, carries over from level 37 rather than 101.
		ench2, ok := table.Get(1, 141)
		if !ok {
			t.Fatal("skill 1 level 141 not loaded")
		}
		if ench2.MagicLevel != 76 || ench2.Power != 2131.0 || ench2.MPConsume != 96 {
			t.Fatalf("skill 1 level 141 = %+v", ench2)
		}

		// MaxLevel ignores enchant levels (>= 99), reporting only the
		// highest regular level.
		if table.MaxLevel(1) != 37 {
			t.Fatalf("MaxLevel(1) = %d, want 37", table.MaxLevel(1))
		}
	})

	t.Run("magic skill with literal and initial-consume fields", func(t *testing.T) {
		d, ok := table.Get(2, 1)
		if !ok {
			t.Fatal("skill 2 level 1 not loaded")
		}
		if d.Name != "Confusion" || d.MagicLevel != 24 || d.Power != 80.0 {
			t.Fatalf("skill 2 level 1 = %+v", d)
		}
		if d.MPConsume != 9 || d.MPInitialConsume != 3 || !d.Magic {
			t.Fatalf("skill 2 level 1 consume/magic = %+v", d)
		}
		if d.CastRange != 600 || d.EffectRange != 1100 {
			t.Fatalf("skill 2 level 1 range = %+v", d)
		}
		// CONFUSION is a classified-offensive skill type even without an
		// explicit "offensive" attribute.
		if !d.Offensive {
			t.Fatal("skill 2 level 1 Offensive = false, want true")
		}
		// Not PDAM/BLOW, so BaseCritRate defaults to -1.
		if d.BaseCritRate != -1 {
			t.Fatalf("skill 2 level 1 BaseCritRate = %d, want -1", d.BaseCritRate)
		}
	})

	t.Run("self-target buff with defaulted range and table-driven aggro", func(t *testing.T) {
		d, ok := table.Get(4, 1)
		if !ok {
			t.Fatal("skill 4 level 1 not loaded")
		}
		if d.Name != "Dash" || d.Target != skill.TargetSelf || d.SkillType != "BUFF" {
			t.Fatalf("skill 4 level 1 = %+v", d)
		}
		// power has no <set> entry at all, so it keeps the zero default.
		if d.Power != 0 {
			t.Fatalf("skill 4 level 1 Power = %v, want 0", d.Power)
		}
		// castRange/effectRange are absent, so they keep their own defaults
		// (0 and -1) rather than each other's.
		if d.CastRange != 0 || d.EffectRange != -1 {
			t.Fatalf("skill 4 level 1 range = castRange=%d effectRange=%d", d.CastRange, d.EffectRange)
		}
		if d.AggroPoints != 204 {
			t.Fatalf("skill 4 level 1 AggroPoints = %d, want 204", d.AggroPoints)
		}
		// effectNpcId is absent, so it defaults to -1.
		if d.EffectNpcID != -1 {
			t.Fatalf("skill 4 level 1 EffectNpcID = %d, want -1", d.EffectNpcID)
		}
		// BUFF isn't a classified-offensive type, isn't a debuff, and
		// doesn't target CORPSE_MOB, so Offensive defaults to false.
		if d.Offensive {
			t.Fatal("skill 4 level 1 Offensive = true, want false")
		}
	})

	t.Run("effect skill delegation fields are loaded", func(t *testing.T) {
		d, ok := table.Get(454, 1)
		if !ok {
			t.Fatal("skill 454 level 1 not loaded")
		}
		if d.EffectID != 5123 || d.EffectLevel != 0 {
			t.Fatalf("skill 454 delegation fields = effectID %d effectLevel %d, want 5123/0", d.EffectID, d.EffectLevel)
		}
		if d.EffectNpcID != 13018 {
			t.Fatalf("skill 454 EffectNpcID = %d, want 13018", d.EffectNpcID)
		}
		referenced, ok := table.Get(5123, 1)
		if !ok {
			t.Fatal("referenced skill 5123 level 1 not loaded")
		}
		if len(referenced.Effects) == 0 {
			t.Fatal("referenced skill 5123 level 1 has no effect templates")
		}
	})

	t.Run("for block preserves effect templates and nested stat funcs", func(t *testing.T) {
		d, ok := table.Get(4, 1)
		if !ok {
			t.Fatal("skill 4 level 1 not loaded")
		}
		if len(d.Effects) != 1 {
			t.Fatalf("skill 4 level 1 Effects = %+v, want 1 entry", d.Effects)
		}
		e := d.Effects[0]
		if e.Name != "Buff" || e.Time != 15 || e.Count != 1 || e.Value != 0 || e.StackType != "speed_up_special" || e.StackOrder != 1 || !e.Icon {
			t.Fatalf("skill 4 level 1 effect = %+v", e)
		}
		if len(e.Funcs) != 1 {
			t.Fatalf("skill 4 level 1 effect funcs = %+v, want 1 entry", e.Funcs)
		}
		fn := e.Funcs[0]
		if fn.Op != skill.FuncAdd || fn.Stat != "runSpd" || fn.Value != 40 {
			t.Fatalf("skill 4 level 1 effect func = %+v", fn)
		}

		level2, ok := table.Get(4, 2)
		if !ok {
			t.Fatal("skill 4 level 2 not loaded")
		}
		if got := level2.Effects[0].StackOrder; got != 2 {
			t.Fatalf("skill 4 level 2 StackOrder = %v, want 2", got)
		}
		if got := level2.Effects[0].Funcs[0].Value; got != 66 {
			t.Fatalf("skill 4 level 2 runSpd func value = %v, want 66", got)
		}
	})

	t.Run("conditions preserve message attributes and resolved predicate tables", func(t *testing.T) {
		d, ok := table.Get(8, 7)
		if !ok {
			t.Fatal("skill 8 level 7 not loaded")
		}
		if len(d.Conditions) != 1 {
			t.Fatalf("skill 8 level 7 Conditions = %+v, want 1 entry", d.Conditions)
		}
		cond := d.Conditions[0]
		if cond.MessageID != 113 || !cond.AddName {
			t.Fatalf("skill 8 level 7 condition message = %+v", cond)
		}
		if cond.Root.Kind != "not" || len(cond.Root.Children) != 1 {
			t.Fatalf("skill 8 level 7 condition root = %+v", cond.Root)
		}
		player := cond.Root.Children[0]
		if player.Kind != "player" || player.Attrs["Charges"] != "7" {
			t.Fatalf("skill 8 level 7 nested player condition = %+v", player)
		}
	})

	t.Run("enchant for blocks override regular effects per enchant route", func(t *testing.T) {
		ench1, ok := table.Get(42, 101)
		if !ok {
			t.Fatal("skill 42 level 101 not loaded")
		}
		if len(ench1.SelfEffects) != 1 || len(ench1.Effects) != 0 {
			t.Fatalf("skill 42 level 101 effects = normal %+v self %+v", ench1.Effects, ench1.SelfEffects)
		}
		if e := ench1.SelfEffects[0]; e.Name != "Heal" || e.Value != 3 || e.Icon || e.Time != 1 || e.Count != 1 {
			t.Fatalf("skill 42 level 101 self effect = %+v", e)
		}

		ench2, ok := table.Get(42, 141)
		if !ok {
			t.Fatal("skill 42 level 141 not loaded")
		}
		if len(ench2.SelfEffects) != 1 {
			t.Fatalf("skill 42 level 141 SelfEffects = %+v, want 1 entry", ench2.SelfEffects)
		}
		if e := ench2.SelfEffects[0]; e.Name != "ManaHeal" || e.Value != 1 || e.Icon {
			t.Fatalf("skill 42 level 141 self effect = %+v", e)
		}
	})

	// The shipped data spells booleans both ways and relies on a present
	// "false" beating a true default, so a level's own boolean value has to
	// win over the loader's default in both directions and regardless of
	// case.
	t.Run("explicit booleans override their defaults in both directions", func(t *testing.T) {
		// canBeReflected/canBeDispeled are the two attributes that default
		// to true, so only a present "false" can distinguish a real read
		// from the default.
		d, ok := table.Get(1375, 1)
		if !ok {
			t.Fatal("skill 1375 level 1 not loaded")
		}
		if d.Name != "Heroic Grandeur" {
			t.Fatalf("skill 1375 level 1 Name = %q, want \"Heroic Grandeur\"", d.Name)
		}
		if d.CanBeReflected || d.CanBeDispelled {
			t.Fatalf("skill 1375 level 1 = canBeReflected=%v canBeDispeled=%v, want both false", d.CanBeReflected, d.CanBeDispelled)
		}

		// Skill 455 writes offensive="True" with a capital T, and SIGNET is
		// neither a classified-offensive type nor a debuff nor a CORPSE_MOB
		// target, so its Offensive would default to false: only a
		// case-insensitive read of the attribute makes it true.
		signet, ok := table.Get(455, 1)
		if !ok {
			t.Fatal("skill 455 level 1 not loaded")
		}
		if signet.SkillType != "SIGNET" || signet.Debuff || signet.Target == skill.TargetCorpseMob {
			t.Fatalf("skill 455 level 1 no longer defaults Offensive to false: %+v", signet)
		}
		if !signet.Offensive {
			t.Fatal("skill 455 level 1 Offensive = false, want true from offensive=\"True\"")
		}
	})

	// Every attribute name is a bare string key in the loader, so a mistyped
	// one is a silent default rather than an error. These are the optional
	// attributes whose absent value is distinguishable from any real one.
	t.Run("optional attributes decode to their own fields", func(t *testing.T) {
		cubic, ok := table.Get(10, 1)
		if !ok {
			t.Fatal("skill 10 level 1 not loaded")
		}
		if !cubic.IsCubic || cubic.NpcID != 1 {
			t.Fatalf("skill 10 level 1 = isCubic=%v npcId=%d, want true/1", cubic.IsCubic, cubic.NpcID)
		}

		negateStats, ok := table.Get(7003, 1)
		if !ok {
			t.Fatal("skill 7003 level 1 not loaded")
		}
		if got, want := negateStats.NegateTypes, []string{"BUFF", "DEBUFF"}; !slices.Equal(got, want) {
			t.Fatalf("skill 7003 level 1 NegateTypes = %v, want %v", got, want)
		}

		negateIDs, ok := table.Get(5102, 1)
		if !ok {
			t.Fatal("skill 5102 level 1 not loaded")
		}
		if got, want := negateIDs.NegateIDs, []int{5106, 5107, 5108, 5098}; !slices.Equal(got, want) {
			t.Fatalf("skill 5102 level 1 NegateIDs = %v, want %v", got, want)
		}

		// 2288 shares 2287's reuse group, so the parsed reference must be
		// the other skill's id rather than its own.
		shared, ok := table.Get(2288, 1)
		if !ok {
			t.Fatal("skill 2288 level 1 not loaded")
		}
		if shared.SharedReuse == nil || *shared.SharedReuse != (skill.Ref{ID: 2287, Level: 1}) {
			t.Fatalf("skill 2288 level 1 SharedReuse = %v, want {2287 1}", shared.SharedReuse)
		}

		// flyType/element are the two non-required enums, and flyRadius is
		// table-substituted alongside them.
		fly, ok := table.Get(5015, 1)
		if !ok {
			t.Fatal("skill 5015 level 1 not loaded")
		}
		if fly.Flight == nil || *fly.Flight != skill.FlightCharge {
			t.Fatalf("skill 5015 level 1 Flight = %v, want FlightCharge", fly.Flight)
		}
		if fly.Element != skill.ElementDark {
			t.Fatalf("skill 5015 level 1 Element = %v, want ElementDark", fly.Element)
		}
		if fly.FlyRadius != 400 {
			t.Fatalf("skill 5015 level 1 FlyRadius = %d, want 400", fly.FlyRadius)
		}
	})
}

// skillFixture wraps extra as the only non-required content of a one-level
// <skill> element that is otherwise valid, so an error case isolates the
// attribute or child under test.
func skillFixture(extra string) string {
	return `<list><skill id="1" name="x" levels="1"><set name="target" val="ONE"/><set name="skillType" val="PDAM"/><set name="operateType" val="ACTIVE"/>` + extra + `</skill></list>`
}

// TestLoadSkillDefinitionsMalformedXMLFails checks that a file whose XML is
// not well-formed still aborts the whole load: only an individual <skill>
// element's data problem is tolerated (see
// TestLoadSkillDefinitionsSkipsMalformedSkills).
func TestLoadSkillDefinitionsMalformedXMLFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.xml")
	writeXMLFixture(t, path, `<list><skill id="1" name="x" levels="1" <set name="target" val="ONE"/></skill></list>`)
	if _, err := LoadSkillDefinitions(dir, zerolog.Nop()); err == nil {
		t.Fatal("expected an error for malformed xml, got nil")
	}
}

// TestLoadSkillDefinitionsSkipsMalformedSkills checks that a single <skill>
// element with a data problem is logged and skipped rather than aborting
// the whole load, matching DocumentSkill.java's per-level try/catch
// ("Failed parsing skill.").
func TestLoadSkillDefinitionsSkipsMalformedSkills(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "missing required skillType attribute",
			content: `<list><skill id="1" name="x" levels="1"><set name="target" val="ONE"/><set name="operateType" val="ACTIVE"/></skill></list>`,
		},
		{
			name:    "missing required target attribute",
			content: `<list><skill id="1" name="x" levels="1"><set name="skillType" val="PDAM"/><set name="operateType" val="ACTIVE"/></skill></list>`,
		},
		{
			name:    "unknown target tag",
			content: `<list><skill id="1" name="x" levels="1"><set name="target" val="NOT_A_TARGET"/><set name="skillType" val="PDAM"/><set name="operateType" val="ACTIVE"/></skill></list>`,
		},
		{
			// power's undefined-table substitution reads as "" (see
			// TestLoadSkillDefinitionsToleratesUndefinedTableRefInCondition
			// for the case where an unresolved reference doesn't itself
			// fail the level), and "" then fails power's own required
			// float64 parse — the level is skipped for that reason, not
			// because the unresolved reference errors directly.
			name:    "value references an undefined table, and the resulting empty value fails to parse",
			content: `<list><skill id="1" name="x" levels="1"><set name="target" val="ONE"/><set name="skillType" val="PDAM"/><set name="operateType" val="ACTIVE"/><set name="power" val="#missing"/></skill></list>`,
		},
		{
			name:    "table name missing the '#' prefix",
			content: `<list><skill id="1" name="x" levels="1"><table name="power"> 1 </table><set name="target" val="ONE"/><set name="skillType" val="PDAM"/><set name="operateType" val="ACTIVE"/></skill></list>`,
		},
		{
			name:    "non-numeric level count",
			content: `<list><skill id="1" name="x" levels="oops"><set name="target" val="ONE"/><set name="skillType" val="PDAM"/><set name="operateType" val="ACTIVE"/></skill></list>`,
		},
		{
			name:    "unknown operateType tag",
			content: skillFixture(`<set name="operateType" val="NOT_AN_OPERATE_TYPE"/>`),
		},
		{
			name:    "malformed integer attribute",
			content: skillFixture(`<set name="mpConsume" val="oops"/>`),
		},
		{
			name:    "malformed float attribute",
			content: skillFixture(`<set name="power" val="oops"/>`),
		},
		{
			name:    "unknown element tag",
			content: skillFixture(`<set name="element" val="NOT_AN_ELEMENT"/>`),
		},
		{
			name:    "unknown flyType tag",
			content: skillFixture(`<set name="flyType" val="NOT_A_FLY_TYPE"/>`),
		},
		{
			name:    "malformed sharedReuse pair",
			content: skillFixture(`<set name="sharedReuse" val="not-a-pair-of-ints"/>`),
		},
		{
			name:    "malformed negateId list",
			content: skillFixture(`<set name="negateId" val="1,oops"/>`),
		},
		{
			name:    "effect without a name",
			content: skillFixture(`<for><effect count="1" time="1" val="0"/></for>`),
		},
		{
			name:    "effect without a value",
			content: skillFixture(`<for><effect name="Buff" count="1" time="1"/></for>`),
		},
		{
			name:    "malformed effect count literal",
			content: skillFixture(`<for><effect name="Buff" val="0" count="oops"/></for>`),
		},
		{
			name:    "stat func without a value",
			content: skillFixture(`<for><add stat="runSpd"/></for>`),
		},
		{
			name:    "malformed condition msgId literal",
			content: skillFixture(`<cond msgId="oops"><player Charges="1"/></cond>`),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, "fixture.xml")
			writeXMLFixture(t, path, c.content)

			var buf bytes.Buffer
			table, err := LoadSkillDefinitions(dir, zerolog.New(&buf))
			if err != nil {
				t.Fatalf("LoadSkillDefinitions: unexpected error for %s: %v", c.name, err)
			}
			if _, ok := table.Get(1, 1); ok {
				t.Fatalf("%s: malformed skill 1 should have been skipped", c.name)
			}
			got := buf.String()
			if !strings.Contains(got, "fixture.xml") {
				t.Fatalf("%s: log output = %q, want it to name the file", c.name, got)
			}
		})
	}

	t.Run("empty directory", func(t *testing.T) {
		empty := t.TempDir()
		if _, err := LoadSkillDefinitions(empty, zerolog.Nop()); err == nil {
			t.Fatal("expected an error for an empty directory, got nil")
		}
	})

	t.Run("missing directory", func(t *testing.T) {
		if _, err := LoadSkillDefinitions(filepath.Join(dir, "does-not-exist"), zerolog.Nop()); err == nil {
			t.Fatal("expected an error for a missing directory, got nil")
		}
	})
}

// TestLoadSkillDefinitionsToleratesUndefinedTableRefInCondition checks that
// an unresolved "#name" table reference is itself logged and substituted
// with "" rather than failing the level, matching DocumentSkill.java's
// getTableValue/getTableValue(name,int) (DocumentSkill.java:55-81): both
// overloads catch the lookup failure, log, and return "" instead of
// propagating. Here the "" substitution flows into a condition attribute
// that isn't itself parsed as a number during load, so the level still
// builds successfully with the empty value — unlike the sibling case in
// TestLoadSkillDefinitionsSkipsMalformedSkills where the "" substitution
// feeds a required numeric attribute and fails there instead.
func TestLoadSkillDefinitionsToleratesUndefinedTableRefInCondition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.xml")
	writeXMLFixture(t, path, `<list><skill id="1" name="x" levels="1"><set name="target" val="ONE"/><set name="skillType" val="PDAM"/><set name="operateType" val="ACTIVE"/><cond><player Charges="#missing"/></cond></skill></list>`)

	var buf bytes.Buffer
	table, err := LoadSkillDefinitions(dir, zerolog.New(&buf))
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: unexpected error: %v", err)
	}

	def, ok := table.Get(1, 1)
	if !ok {
		t.Fatal("skill 1 level 1 should have loaded despite the unresolved table reference")
	}
	if len(def.Conditions) != 1 || def.Conditions[0].Root.Attrs["Charges"] != "" {
		t.Fatalf("skill 1 level 1 conditions = %+v, want Charges resolved to \"\"", def.Conditions)
	}

	got := buf.String()
	if !strings.Contains(got, "fixture.xml") || !strings.Contains(got, "\"skill\":1") {
		t.Fatalf("log output = %q, want it to name the file and skill id", got)
	}
}

// TestLoadSkillDefinitionsSkipsOnlyTheMalformedLevel checks that a bad
// level's table-substituted value only drops that one level, keeping the
// other, well-formed levels of the same skill — the granularity
// DocumentSkill.java's makeSkills (DocumentSkill.java:310-370) actually
// applies its per-level try/catch at, rather than dropping the whole
// <skill> element the way an earlier version of this loader did.
func TestLoadSkillDefinitionsSkipsOnlyTheMalformedLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.xml")
	writeXMLFixture(t, path, `<list><skill id="5" name="x" levels="3">
		<table name="#mp"> 10 oops 30 </table>
		<set name="target" val="ONE"/>
		<set name="skillType" val="PDAM"/>
		<set name="operateType" val="ACTIVE"/>
		<set name="mpConsume" val="#mp"/>
	</skill></list>`)

	var buf bytes.Buffer
	table, err := LoadSkillDefinitions(dir, zerolog.New(&buf))
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: unexpected error: %v", err)
	}

	if def, ok := table.Get(5, 1); !ok || def.MPConsume != 10 {
		t.Fatalf("skill 5 level 1 = %+v, %v, want MPConsume 10", def, ok)
	}
	if _, ok := table.Get(5, 2); ok {
		t.Fatal("skill 5 level 2 should have been skipped (mpConsume = \"oops\")")
	}
	if def, ok := table.Get(5, 3); !ok || def.MPConsume != 30 {
		t.Fatalf("skill 5 level 3 = %+v, %v, want MPConsume 30", def, ok)
	}

	got := buf.String()
	if !strings.Contains(got, "fixture.xml") || !strings.Contains(got, "\"skill\":5") || !strings.Contains(got, "\"level\":2") {
		t.Fatalf("log output = %q, want it to name the file, skill id, and level 2", got)
	}
}

// TestConditionMessagePrecedence covers pr-reviews/478.md finding 1: a
// regular-level <cond> reads msg or msgId, never both
// (DocumentSkill.java:216-224), and an enchant-route cond reads only msg,
// ignoring msgId/addName entirely even when present
// (DocumentSkill.java:245-247, 289-291).
func TestConditionMessagePrecedence(t *testing.T) {
	t.Run("regular cond: msg present wins over msgId", func(t *testing.T) {
		dir := t.TempDir()
		content := skillFixture(`<cond msg="both attrs present" msgId="113" addName="1"><player Charges="1"/></cond>`)
		writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), content)
		table, err := LoadSkillDefinitions(dir, zerolog.Nop())
		if err != nil {
			t.Fatalf("LoadSkillDefinitions: %v", err)
		}
		def, ok := table.Get(1, 1)
		if !ok {
			t.Fatal("skill 1 level 1 not loaded")
		}
		if len(def.Conditions) != 1 {
			t.Fatalf("Conditions = %+v, want 1 entry", def.Conditions)
		}
		cond := def.Conditions[0]
		if cond.Message != "both attrs present" || cond.MessageID != 0 || cond.AddName {
			t.Fatalf("condition message = %+v, want only Message set", cond)
		}
	})

	t.Run("enchant1cond: msgId is never read, even when msg is absent and msgId is malformed", func(t *testing.T) {
		dir := t.TempDir()
		content := `<list><skill id="1" name="x" levels="1" enchantLevels1="1">` +
			`<set name="target" val="ONE"/><set name="skillType" val="PDAM"/><set name="operateType" val="ACTIVE"/>` +
			`<enchant1cond msgId="oops"><player Charges="1"/></enchant1cond>` +
			`</skill></list>`
		writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), content)
		table, err := LoadSkillDefinitions(dir, zerolog.Nop())
		if err != nil {
			t.Fatalf("LoadSkillDefinitions: %v", err)
		}
		def, ok := table.Get(1, 101)
		if !ok {
			t.Fatal("skill 1 level 101 (enchant1) not loaded")
		}
		if len(def.Conditions) != 1 {
			t.Fatalf("Conditions = %+v, want 1 entry", def.Conditions)
		}
		cond := def.Conditions[0]
		if cond.Message != "" || cond.MessageID != 0 || cond.AddName {
			t.Fatalf("enchant1cond message = %+v, want zero value (msgId never consulted)", cond)
		}
	})
}

// TestSkillGrammarDegradesGracefully covers pr-reviews/478.md finding 2:
// an empty/predicate-less <cond/> and an unrecognized tag inside a <for>
// block tolerate the malformed content instead of failing the whole file,
// matching DocumentBase.java's parseCondition (returns null, attach(null) is
// a no-op) and parseTemplate (no fall-through branch for an unknown tag).
func TestSkillGrammarDegradesGracefully(t *testing.T) {
	t.Run("empty cond is skipped, not a load failure", func(t *testing.T) {
		dir := t.TempDir()
		content := skillFixture(`<cond/>`)
		writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), content)
		table, err := LoadSkillDefinitions(dir, zerolog.Nop())
		if err != nil {
			t.Fatalf("LoadSkillDefinitions: %v", err)
		}
		def, ok := table.Get(1, 1)
		if !ok {
			t.Fatal("skill 1 level 1 not loaded")
		}
		if len(def.Conditions) != 0 {
			t.Fatalf("Conditions = %+v, want none", def.Conditions)
		}
	})

	t.Run("unrecognized tag inside a for block is skipped, not a load failure", func(t *testing.T) {
		dir := t.TempDir()
		content := skillFixture(`<for><bogusTag/><add stat="runSpd" val="5"/></for>`)
		writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), content)
		table, err := LoadSkillDefinitions(dir, zerolog.Nop())
		if err != nil {
			t.Fatalf("LoadSkillDefinitions: %v", err)
		}
		def, ok := table.Get(1, 1)
		if !ok {
			t.Fatal("skill 1 level 1 not loaded")
		}
		if len(def.Funcs) != 1 || def.Funcs[0].Stat != "runSpd" || def.Funcs[0].Value != 5 {
			t.Fatalf("Funcs = %+v, want the add op to survive the skipped tag", def.Funcs)
		}
	})
}
