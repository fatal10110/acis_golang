package skills

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The spell-watching NPCs: A is the skills' target, B stands beside it and
// Far stands out of the 1000 sight radius of the player.
const (
	spellWatcherA   = int32(20061)
	spellWatcherB   = int32(20062)
	spellWatcherFar = int32(20063)

	// The probe skills: a good skill on one target, a debuff on one target
	// and a good self buff.
	spellGoodOne  = modelskill.ID(4101)
	spellDebuff   = modelskill.ID(4102)
	spellSelfBuff = modelskill.ID(4103)
)

// spellLog records, per NPC, the see-spell, spelled and abnormal-status
// hooks of the behavior bound to the watchers, and the abnormal-status
// hook of a script bound to that event alone. Hooks run on the caster's
// queue.
type spellLog struct {
	mu sync.Mutex
	// names maps an object id to the name the lines use.
	names map[int32]string
	lines map[string][]string
}

func newSpellLog() *spellLog {
	return &spellLog{names: map[int32]string{}, lines: map[string][]string{}}
}

func (l *spellLog) name(id int32, name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.names[id] = name
}

func (l *spellLog) nameOf(c script.Creature) string {
	if c == nil {
		return "none"
	}
	if n, ok := l.names[c.ObjectID()]; ok {
		return fmt.Sprintf("%s(%T)", n, c)
	}
	return fmt.Sprintf("%d(%T)", c.ObjectID(), c)
}

func (l *spellLog) add(n *script.NPC, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, a := range args {
		if c, ok := a.(script.Creature); ok {
			args[i] = l.nameOf(c)
		}
	}
	who := l.names[n.ObjectID()]
	l.lines[who] = append(l.lines[who], fmt.Sprintf(format, args...))
}

// take returns the lines since the last call, per NPC name.
func (l *spellLog) take() map[string][]string {
	l.mu.Lock()
	defer l.mu.Unlock()
	got := l.lines
	l.lines = map[string][]string{}
	return got
}

func (l *spellLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, lines := range l.lines {
		n += len(lines)
	}
	return n
}

func (l *spellLog) catalog() script.Catalog {
	return script.Catalog{
		"ai.Seer": func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{spellWatcherA, spellWatcherB, spellWatcherFar}, Hooks: script.Hooks{
				OnSeeSpell: func(_ *script.Script, e script.SeeSpell) {
					targets := make([]string, len(e.Targets))
					l.mu.Lock()
					for i, c := range e.Targets {
						targets[i] = l.nameOf(c)
					}
					l.mu.Unlock()
					l.add(e.NPC, "see %d-%d caster=%v targets=[%s] pet=%v", e.Skill.ID, e.Skill.Level, e.Caster, strings.Join(targets, " "), e.IsPet)
				},
				OnSpelled: func(_ *script.Script, e script.Spelled) {
					l.add(e.NPC, "spelled %d-%d caster=%v", e.Skill.ID, e.Skill.Level, e.Caster)
				},
				OnAbnormalStatusChanged: func(_ *script.Script, e script.AbnormalStatusChanged) {
					l.add(e.NPC, "abnormal %d-%d caster=%v", e.Skill.ID, e.Skill.Level, e.Caster)
				},
			}}
		},
		// Bound to the abnormal-status event alone: the hook is dispatched
		// over the see-spell list, so it never runs.
		"quest.Blind": func() script.Script {
			return script.Script{Bind: script.Bindings{script.EventAbnormalStatusChanged: {spellWatcherA}}, Hooks: script.Hooks{
				OnAbnormalStatusChanged: func(_ *script.Script, e script.AbnormalStatusChanged) {
					l.add(e.NPC, "blind abnormal %d-%d", e.Skill.ID, e.Skill.Level)
				},
			}}
		},
	}
}

func (l *spellLog) option() gameservertest.Option {
	kinds := map[int32]script.NPCKind{spellWatcherA: script.KindHostile, spellWatcherB: script.KindHostile, spellWatcherFar: script.KindHostile}
	return gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: "ai.Seer"}, {Path: "quest.Blind"}}, l.catalog())
}

// spawnWatchers spawns A at the fixture monster point, B beside it and Far
// out of the player's 1000 sight radius, and names them and the player.
func (l *spellLog) spawnWatchers(t *testing.T, srv *gameservertest.Server, objID int32) *npc.Hostile {
	t.Helper()
	spawn := func(id int32, name string, at location.Location) *npc.Hostile {
		tmpl := conditionNPCTemplate("Monster", 0)
		tmpl.ID, tmpl.TemplateID = int(id), int(id)
		h := srv.SpawnHostileNPCTemplateAt(t, tmpl, at)
		l.name(h.ObjectID(), name)
		return h
	}
	l.name(objID, "P")
	a := spawn(spellWatcherA, "A", location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	spawn(spellWatcherB, "B", location.Location{X: hostileX, Y: hostileY + 100, Z: hostileZ})
	spawn(spellWatcherFar, "Far", location.Location{X: hostileX + 1500, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, srv.Client)
	return a
}

// expect waits for the hooks of want, lets the server settle and compares
// every NPC's lines with want's. The order between NPCs is not part of it.
func (l *spellLog) expect(t *testing.T, srv *gameservertest.Server, what string, want map[string][]string) {
	t.Helper()
	n := 0
	for _, lines := range want {
		n += len(lines)
	}
	srv.AdvanceUntil(t, what, func() bool { return l.count() >= n })
	srv.Settle(t)
	got := l.take()
	if !maps.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("%s: hooks = %q, want %q", what, got, want)
	}
}

func spellProbeSkills() []modelskill.Definition {
	return []modelskill.Definition{
		{
			ID: spellGoodOne, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "BUFF", CastRange: 900, HitTime: 500, StaticHitTime: true, StaticReuse: true,
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "probe_good", StackOrder: 1}},
		},
		{
			ID: spellDebuff, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true, Offensive: true, CastRange: 900, EffectRange: 1400,
			HitTime: 500, StaticHitTime: true, StaticReuse: true, EffectPower: 100, IgnoreResists: true,
			Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 30, Icon: true, StackType: "probe_bad", StackOrder: 1}},
		},
		{
			ID: spellSelfBuff, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true,
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "probe_self", StackOrder: 1}},
		},
	}
}

// TestPlayerSkillReachesSpellWatchers (CreatureCast.callSkill, Creature.
// addEffect): a player's skill reaches the behavior of every NPC within
// 1000 of it as a seen spell, and a good skill reaches its target NPC as a
// spell landing on it. A good skill on one NPC is not seen by that NPC; an
// effect about to land on an NPC reaches the scripts that see its spells,
// not a script bound to the abnormal-status event alone.
func TestPlayerSkillReachesSpellWatchers(t *testing.T) {
	t.Parallel()
	log := newSpellLog()
	defs := spellProbeSkills()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, defs)),
		log.option(),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	for _, def := range defs {
		seedKnownSkill(t, srv, objID, int(def.ID), 1)
	}
	startInWorld(t, c)
	setPlayerRollSource(t, srv, objID, func(int) int { return 0 })
	a := log.spawnWatchers(t, srv, objID)
	player := "P(*script.Player)"

	// A self buff: everyone in range sees it, on the player.
	castSelf(t, srv, c, objID, int32(spellSelfBuff))
	seen := fmt.Sprintf("see %d-1 caster=%s targets=[%s] pet=false", spellSelfBuff, player, player)
	log.expect(t, srv, "the self buff", map[string][]string{"A": {seen}, "B": {seen}})
	drainUntilQuiet(t, c)

	// A good skill on A, forced as a monster takes one only with CTRL: its
	// effect and the spell land on A, which does not see it; B does.
	targetHostile(t, c, a.ObjectID())
	drainUntilQuiet(t, c)
	c.Send(encodeRequestMagicSkillUse(int32(spellGoodOne), true, false))
	log.expect(t, srv, "the good skill on A", map[string][]string{
		"A": {
			fmt.Sprintf("abnormal %d-1 caster=%s", spellGoodOne, player),
			fmt.Sprintf("spelled %d-1 caster=%s", spellGoodOne, player),
		},
		"B": {fmt.Sprintf("see %d-1 caster=%s targets=[A(*script.NPC)] pet=false", spellGoodOne, player)},
	})
	drainUntilQuiet(t, c)

	// A debuff on A: its effect lands on A, and A and B see it; nothing
	// lands as a good spell.
	c.Send(encodeRequestMagicSkillUse(int32(spellDebuff), false, false))
	seen = fmt.Sprintf("see %d-1 caster=%s targets=[A(*script.NPC)] pet=false", spellDebuff, player)
	log.expect(t, srv, "the debuff on A", map[string][]string{
		"A": {fmt.Sprintf("abnormal %d-1 caster=%s", spellDebuff, player), seen},
		"B": {seen},
	})
}

// TestWeaponOnMagicSkillSeenBySpellWatchers (Weapon.castSkillOnMagic): the
// good skill a player's weapon sets off when the player's good self buff
// lands is seen by the NPCs around the player, on the player, before the
// buff that set it off.
func TestWeaponOnMagicSkillSeenBySpellWatchers(t *testing.T) {
	t.Parallel()
	log := newSpellLog()
	catalog := procWeaponCatalog(t, func(w *item.WeaponDetail) {
		w.OnCastSkill = &item.SkillTrigger{Skill: item.SkillRef{ID: onGoodTriggered, Level: 1}, Chance: -1}
	})
	srv, c, objID := bootArmedOpts(t, []gameservertest.Option{log.option()}, chanceSkills(), catalog, func(int) int { return 0 }, []int32{fixtureSwordID}, selfBuffSkill)
	log.spawnWatchers(t, srv, objID)
	player := "P(*script.Player)"

	castSelf(t, srv, c, objID, selfBuffSkill)
	seen := func(id int32) string {
		return fmt.Sprintf("see %d-1 caster=%s targets=[%s] pet=false", id, player, player)
	}
	want := []string{seen(onGoodTriggered), seen(selfBuffSkill)}
	log.expect(t, srv, "the buff and its weapon skill", map[string][]string{"A": want, "B": want})
}
