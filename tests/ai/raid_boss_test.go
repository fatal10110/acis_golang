package ai

import (
	"slices"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/raid"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The raid boss fixtures' skills by type: a physical special on one target,
// two specials around the boss and its self buff, and the raid mute (the
// strider slow is below).
const (
	rbPhysical = modelskill.ID(9941)
	rbRange    = modelskill.ID(9942)
	rbCancel   = modelskill.ID(9943)
	rbSelfBuff = modelskill.ID(9944)
	rbMuteID   = modelskill.ID(4215)
)

// rbSkills are the skills the raid boss fixtures cast, the raid curse
// excepted.
func rbSkills() []modelskill.Definition {
	return []modelskill.Definition{
		ind1Skill(rbPhysical, 2000, false), ind1Skill(rbRange, 0, true), ind1Skill(rbCancel, 0, true),
		ind1Skill(rbSelfBuff, 0, true), ind1Skill(rbMuteID, 600, false), ind1Skill(rbStriderSlow, 600, false),
	}
}

// rbTemplate is a raid boss fixture of id and level, with ai, listing
// skills by type.
func rbTemplate(id int32, level int, ai npc.AIParams, skills map[npc.SkillType]modelskill.ID) *npc.Template {
	tmpl := wusTemplate(id, ai, skills)
	tmpl.Type, tmpl.Level = "RaidBoss", level
	return tmpl
}

// bootRaidBosses boots the raid boss chain (RaidBossStandard,
// RaidBossType1) beside templates, with extra, every NPC casting the raid
// boss fixture skills and the raid curse, every draw 0 until a test queues
// others.
func bootRaidBosses(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind7World {
	t.Helper()
	paths := []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.individual.Monster.MonsterAI",
		"script.ai.individual.Monster.RaidBoss.RaidBossStandard",
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1",
	}
	catalog := script.Catalog{
		paths[0]: defaultnpc.New, paths[1]: monster.New, paths[2]: raid.BossStandard, paths[3]: raid.BossType1,
	}
	var list []script.Listing
	for _, p := range paths {
		list = append(list, script.Listing{Path: p})
	}
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	w := bootRaid(t, raidBoot{ready: extra.ready, opts: append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(append(rbSkills(), raidCurseDefinition()))),
		gameservertest.WithScriptSkills(rbSkills()...),
	}, extra.opts...)}, templates...)
	return &ind7World{ind1World: &ind1World{warriorWorld: w, draws: draws, heard: &ind1Heard{}}}
}

// rbHate is the attack weight a raid boss of level gives a hit of damage,
// computed at run time as the boss computes it.
func rbHate(damage float64, level int) float64 { return damage / float64(level+7) * 20000 }

// TestRaidBossStandardPlaysItsMusicAndHatesByLevel ports RaidBossStandard's
// created and attacked hooks through RaidBossType1. A raid boss appears
// playing its RaidSpawnMusic, Rm01_A by default, bound to itself where it
// stands. A hit of 10 from the character makes a level 20 boss want to
// attack it at 10 / (20 + 7) * 20000; a hit of 0 counts as 1.
func TestRaidBossStandardPlaysItsMusicAndHatesByLevel(t *testing.T) {
	t.Parallel()
	w := bootRaidBosses(t, raidBoot{},
		rbTemplate(25019, 20, nil, nil), rbTemplate(25050, 20, npc.AIParams{"RaidSpawnMusic": "Rm02_B"}, nil))
	for _, row := range []struct {
		id    int32
		music string
	}{{25019, "Rm01_A"}, {25050, "Rm02_B"}} {
		s := w.spawnFrames(t, row.id, 400, 0)
		want := ind8MusicPayload(s.h.ObjectID(), row.music, positionOf(s.h))
		if got := ind8Pick(s.frames, serverpackets.OpcodePlaySound); len(got) != 1 || !slices.Equal(got[0], want) {
			t.Fatalf("npc %d PlaySound frames = %x, want one of %s bound to it", row.id, got, row.music)
		}
	}
	boss := w.liveNPCs(25019)[0]

	w.hit(t, boss, 10)
	if got, want := w.attackWeight(boss), rbHate(10, 20); got != want {
		t.Fatalf("attack desire after a hit of 10 = %v, want %v", got, want)
	}
	w.zeroHit(t, boss)
	if got, want := w.attackWeight(boss), rbHate(10, 20)+rbHate(1, 20); got != want {
		t.Fatalf("attack desire after a hit of 0 = %v, want %v", got, want)
	}
}

// TestRaidBossStandardCursesAndMutesOverLevelled ports the attacked and
// see-spell hooks' level rule: against the level 20 character, a level 11
// boss (9 levels below, more than 8) wants to cast its raid curse on the
// character when hit, forgets its hate and wants no attack; seeing the
// character cast, it wants its raid mute on it. A level 12 boss fights it.
func TestRaidBossStandardCursesAndMutesOverLevelled(t *testing.T) {
	t.Parallel()
	w := bootRaidBosses(t, raidSpellBoot(t), rbTemplate(25019, 11, nil, nil), rbTemplate(25063, 12, nil, nil))
	me := w.player.ObjectID()
	low := w.spawn(t, 25019, 400, 0)
	even := w.spawn(t, 25063, -400, 0)

	w.hit(t, low, 10)
	w.wantCasts(t, "after the hit", low, map[modelskill.ID]ind1Cast{raidCurseID: {me, 1000000}})
	if got := low.AggroHate(w.player); got != 0 {
		t.Errorf("hate after the hit = %v, want none", got)
	}
	if d, ok := desire(low, actorai.IntentionAttack); ok {
		t.Errorf("attack desire after the hit = %+v, want none", d)
	}
	w.hit(t, even, 10)
	w.wantCasts(t, "level 12 after the hit", even, nil)
	if got, want := w.attackWeight(even), rbHate(10, 12); got != want {
		t.Errorf("level 12 attack desire = %v, want %v", got, want)
	}

	w.castSelfBuff()
	w.srv.AdvanceUntil(t, "the raid mute desire", func() bool {
		_, ok := ind1Casts(low)[rbMuteID]
		return ok
	})
	w.wantCasts(t, "after the spell", low, map[modelskill.ID]ind1Cast{raidCurseID: {me, 1000000}, rbMuteID: {me, 1000000}})
}

// TestRaidBossGoesBackHomeEachMinute ports the timer RaidBossStandard
// starts as the boss appears, with RaidBossType1's part: 1 s after it
// appears, then every minute, a boss out of its territory and out of a
// fight goes back to its spawn point on an even draw, and one time in five
// wants its self buff (SELF_BUFF_A) at 1000000. Every draw is 0.
func TestRaidBossGoesBackHomeEachMinute(t *testing.T) {
	t.Parallel()
	tmpl := rbTemplate(25019, 20, nil, map[npc.SkillType]modelskill.ID{npc.SkillTypeSelfBuffA: rbSelfBuff})
	tmpl.CanMove = false
	w := bootRaidBosses(t, raidBoot{}, tmpl)
	boss := w.spawn(t, 25019, 400, 0)
	home := positionOf(boss)
	away := location.Location{X: home.X + 1000, Y: home.Y, Z: home.Z}
	w.onQueue(t, boss, func() { script.NPCOf(boss).InstantTeleportTo(script.Loc{X: away.X, Y: away.Y, Z: away.Z}) })
	if got := positionOf(boss); got.Distance2D(home) < 900 {
		t.Fatalf("the boss stands at %v, want it 1000 from its spawn point %v", got, home)
	}

	w.srv.AdvanceUntil(t, "the boss back home", func() bool { return positionOf(boss).Distance2D(home) < 1 })
	w.wantCasts(t, "after the check", boss, map[modelskill.ID]ind1Cast{rbSelfBuff: {boss.ObjectID(), 1000000}})
}

// TestRaidBossType1UsesItsSpecials ports RaidBossType1's attacked hook,
// with the character 100 away. The first hit, from one that is not yet its
// top desire target, makes it want its ranged physical special around
// itself on a draw of 0 in 375; its ranged cancel, on a draw of 1 in 750,
// stays off. That wish, at 1000000, outweighs its attack, so the boss
// itself is its top desire target, and a second hit would want it again.
// With its desires dropped and the character made its top desire target,
// the next hit wants its physical special on the character on a draw of 0
// in 15 and its ranged cancel on one of 0 in 750, each at 1000000.
func TestRaidBossType1UsesItsSpecials(t *testing.T) {
	t.Parallel()
	tmpl := rbTemplate(25019, 20, nil, map[npc.SkillType]modelskill.ID{
		npc.SkillTypePhysicalSpecialA: rbPhysical, npc.SkillTypeSelfRangePhysicalSpecialA: rbRange,
		npc.SkillTypeSelfRangeCancelA: rbCancel, npc.SkillTypeSelfBuffA: rbSelfBuff,
	})
	tmpl.CanMove = false
	w := bootRaidBosses(t, raidBoot{}, tmpl)
	me := w.player.ObjectID()
	boss := w.spawn(t, 25019, 100, 0)

	w.draws.set(9999, 0, 1)
	w.hit(t, boss, 10)
	w.wantCasts(t, "first hit", boss, map[modelskill.ID]ind1Cast{rbRange: {boss.ObjectID(), 1000000}})
	if top := script.NPCOf(boss).TopDesireTarget(); !script.Same(top, script.NPCOf(boss)) {
		t.Fatalf("top desire target after the first hit = %v, want the boss itself", top)
	}
	w.setTop(t, boss, script.PlayerOf(w.player))
	w.draws.set(9999, 0, 0)
	w.hit(t, boss, 10)
	w.wantCasts(t, "hit by its top desire target", boss, map[modelskill.ID]ind1Cast{rbPhysical: {me, 1000000}, rbCancel: {boss.ObjectID(), 1000000}})
}

// rbStriderSlow is the slow a raid boss casts on a strider rider.
const rbStriderSlow = modelskill.ID(4258)

// rbAggroSpellBoot is ind1AggroSpellBoot with a self buff of aggro points.
func rbAggroSpellBoot(t *testing.T, aggro int) raidBoot {
	t.Helper()
	db := sqltest.SharedDB(t)
	def := modelskill.Definition{
		ID: ind1AggroBuffID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true, AggroPoints: aggro,
	}
	table := modelskill.NewTable([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}, def})
	return raidBoot{
		opts: []gameservertest.Option{
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(def),
		},
		ready: func(srv *gameservertest.Server, objID int32) {
			if err := srv.KnownSkills.SetKnownSkill(t.Context(), objID, 0, int(ind1AggroBuffID), 1); err != nil {
				t.Fatalf("seed the hate buff: %v", err)
			}
		},
	}
}

// onPlayer reports whether h stands where the character does.
func (w *ind7World) onPlayer(t *testing.T, h *npc.Hostile) bool {
	t.Helper()
	x, y, z := w.srv.PlayerPosition(t, w.player.ObjectID())
	return positionOf(h) == location.Location{X: x, Y: y, Z: z}
}

// TestRaidBossStandardSlowsAStriderRider ports the attacked hook's mount
// rule: hit by the character riding a strider (mount type 1), the boss
// wants its strider slow (4258) on it at 1000000, the character holding no
// effect of the slow, beside its attack.
func TestRaidBossStandardSlowsAStriderRider(t *testing.T) {
	t.Parallel()
	w := bootRaidBosses(t, raidBoot{}, rbTemplate(25019, 20, nil, nil))
	boss := w.spawn(t, 25019, 400, 0)
	var mountType int32
	w.onCharacter(t, func(c *player.Character) {
		c.Mount(12526, 0)
		mountType = c.MountType()
	})
	if mountType != 1 {
		t.Fatalf("setup: mount type = %d, want 1 (a strider)", mountType)
	}

	w.hit(t, boss, 10)
	w.wantCasts(t, "after the hit", boss, map[modelskill.ID]ind1Cast{rbStriderSlow: {w.player.ObjectID(), 1000000}})
	if got, want := w.attackWeight(boss), rbHate(10, 20); got != want {
		t.Fatalf("attack desire = %v, want %v", got, want)
	}
}

// TestRaidBossStandardJumpsWhenLost ports the attacked hook's path-failure
// branch: after more than 10 failed paths, a hit makes a boss whose top
// desire target stands within 1000 jump onto it. One whose top desire
// target stands 1200 away drops its attack desires, wants the attacker
// again for this hit alone and jumps onto the attacker. Every draw is 1,
// so neither boss's minute check sends it home.
func TestRaidBossStandardJumpsWhenLost(t *testing.T) {
	t.Parallel()
	near, far := rbTemplate(25019, 20, nil, nil), rbTemplate(25050, 20, nil, nil)
	near.CanMove, far.CanMove = false, false
	w := bootRaidBosses(t, raidBoot{}, near, far)
	w.draws.set(1)
	for _, row := range []struct {
		id     int32
		dx     int
		weight float64
	}{{25019, 400, 2 * rbHate(10, 20)}, {25050, 1200, rbHate(10, 20)}} {
		boss := w.spawn(t, row.id, row.dx, 0)
		w.hit(t, boss, 10)
		if top := boss.AI().TopDesireTarget(); top == nil || top.ObjectID() != w.player.ObjectID() {
			t.Fatalf("npc %d top desire target = %v, want the character", row.id, top)
		}
		w.onQueue(t, boss, func() {
			for range 11 {
				boss.AddGeoPathFailCount()
			}
		})
		w.hit(t, boss, 10)
		w.srv.AdvanceUntil(t, "the jump onto the character", func() bool { return w.onPlayer(t, boss) })
		if got := w.attackWeight(boss); got != row.weight {
			t.Fatalf("npc %d attack desire after the jump = %v, want %v", row.id, got, row.weight)
		}
	}
}

// TestRaidBossStandardHitInPeaceGoesHome ports the attacked hook's peace
// zone branch: a boss hit inside a peace zone drops its attack desires, the
// one the hit just added included, and goes back to its spawn point.
func TestRaidBossStandardHitInPeaceGoesHome(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(200, 3_000, -1_000, 1_000, -1_000, 1_000)
	if err != nil {
		t.Fatalf("zone form: %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewPeace(1, form))
	tmpl := rbTemplate(25019, 20, nil, nil)
	tmpl.CanMove = false
	w := bootRaidBosses(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithZones(zones)}}, tmpl)
	w.draws.set(1)
	boss := w.spawn(t, 25019, 400, 0)
	home := positionOf(boss)
	w.teleport(t, boss, 600, 0)
	if !boss.InsideZone(zone.FlagPeace) || positionOf(boss) == home {
		t.Fatal("setup: the boss is not in the peace zone, off its spawn point")
	}

	w.hit(t, boss, 10)
	w.srv.AdvanceUntil(t, "the boss back home", func() bool { return positionOf(boss) == home })
	if d, ok := desire(boss, actorai.IntentionAttack); ok {
		t.Fatalf("attack desire after a hit in peace = %+v, want none", d)
	}
}

// TestRaidBossStandardWantsTheAggroCasterWhileFighting ports the see-spell
// hook's aggro rule: a boss attacking another creature that sees the
// character cast a self buff of 100000 aggro points wants to attack the
// character at (100000 / its maximum HP, an integer division) * 4000 * 150.
func TestRaidBossStandardWantsTheAggroCasterWhileFighting(t *testing.T) {
	t.Parallel()
	tmpl := rbTemplate(25019, 20, nil, nil)
	tmpl.CanMove = false
	w := bootRaidBosses(t, rbAggroSpellBoot(t, 100000), tmpl, warriorTemplate(unboundID, nil))
	w.draws.set(1)
	boss := w.spawn(t, 25019, 400, 0)
	other := w.spawn(t, unboundID, 450, 0)
	w.onQueue(t, boss, func() { script.NPCOf(boss).AddAttackDesire(script.NPCOf(other), 1000) })
	w.ticks(t, 2)
	if got := boss.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("setup: boss intention = %v, want attack", got)
	}

	w.castAggroBuff()
	w.srv.AdvanceUntil(t, "the attack desire on the caster", func() bool {
		_, ok := attackOn(boss, w.player)
		return ok
	})
	if got, want := w.attackWeight(boss), float64(((int32(100000)/int32(boss.MaxHP()))*4000)*150); got != want {
		t.Fatalf("attack desire on the caster = %v, want %v", got, want)
	}
}

// TestRaidBossStandardJumpsOntoTheLostCaster ports the see-spell hook's
// path-failure branch: a hurt boss whose top desire target is the
// character, after more than 10 failed paths, jumps onto the character when
// it sees it cast. Every draw is 1, so its minute check, out of its
// territory, does not send it home.
func TestRaidBossStandardJumpsOntoTheLostCaster(t *testing.T) {
	t.Parallel()
	tmpl := rbTemplate(25019, 20, nil, nil)
	tmpl.CanMove = false
	w := bootRaidBosses(t, raidSpellBoot(t), tmpl)
	w.draws.set(1)
	boss := w.spawn(t, 25019, 400, 0)
	w.hit(t, boss, 100)
	if top := boss.AI().TopDesireTarget(); top == nil || top.ObjectID() != w.player.ObjectID() {
		t.Fatalf("top desire target = %v, want the character", top)
	}
	w.onQueue(t, boss, func() {
		script.NPCOf(boss).InstantTeleportTo(script.Loc{X: w.at.X + 500, Y: w.at.Y, Z: w.at.Z})
		for range 11 {
			boss.AddGeoPathFailCount()
		}
	})
	w.srv.ReadQueued(t, w.srv.Client)
	if boss.HP() == float64(boss.MaxHP()) {
		t.Fatal("setup: the boss is not hurt")
	}

	w.castSelfBuff()
	w.srv.AdvanceUntil(t, "the jump onto the caster", func() bool { return w.onPlayer(t, boss) })
}
