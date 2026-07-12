package xml

import (
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestLoadSkillDefinitions(t *testing.T) {
	dir := datapackPath(t, filepath.Join("data", "xml", "skills"))

	table, err := LoadSkillDefinitions(dir)
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
		// BUFF isn't a classified-offensive type, isn't a debuff, and
		// doesn't target CORPSE_MOB, so Offensive defaults to false.
		if d.Offensive {
			t.Fatal("skill 4 level 1 Offensive = true, want false")
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
}

func TestLoadSkillDefinitionsErrors(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "malformed xml",
			content: `<list><skill id="1" name="x" levels="1" <set name="target" val="ONE"/></skill></list>`,
		},
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
			name:    "value references an undefined table",
			content: `<list><skill id="1" name="x" levels="1"><set name="target" val="ONE"/><set name="skillType" val="PDAM"/><set name="operateType" val="ACTIVE"/><set name="power" val="#missing"/></skill></list>`,
		},
		{
			name:    "condition references an undefined table",
			content: `<list><skill id="1" name="x" levels="1"><set name="target" val="ONE"/><set name="skillType" val="PDAM"/><set name="operateType" val="ACTIVE"/><cond><player Charges="#missing"/></cond></skill></list>`,
		},
		{
			name:    "table name missing the '#' prefix",
			content: `<list><skill id="1" name="x" levels="1"><table name="power"> 1 </table><set name="target" val="ONE"/><set name="skillType" val="PDAM"/><set name="operateType" val="ACTIVE"/></skill></list>`,
		},
		{
			name:    "non-numeric level count",
			content: `<list><skill id="1" name="x" levels="oops"><set name="target" val="ONE"/><set name="skillType" val="PDAM"/><set name="operateType" val="ACTIVE"/></skill></list>`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, "fixture.xml")
			writeXMLFixture(t, path, c.content)
			if _, err := LoadSkillDefinitions(dir); err == nil {
				t.Fatalf("expected an error for %s, got nil", c.name)
			}
		})
	}

	t.Run("empty directory", func(t *testing.T) {
		empty := t.TempDir()
		if _, err := LoadSkillDefinitions(empty); err == nil {
			t.Fatal("expected an error for an empty directory, got nil")
		}
	})

	t.Run("missing directory", func(t *testing.T) {
		if _, err := LoadSkillDefinitions(filepath.Join(dir, "does-not-exist")); err == nil {
			t.Fatal("expected an error for a missing directory, got nil")
		}
	})
}
