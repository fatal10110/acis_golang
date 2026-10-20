package ai

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/raid"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The raid boss fixtures' skills by type: a physical special on one target,
// two specials around the boss and its self buff, and the raid mute.
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
		ind1Skill(rbSelfBuff, 0, true), ind1Skill(rbMuteID, 600, false),
	}
}

// rbTemplate is a raid boss fixture of id and level, with ai, listing
// skills by type.
func rbTemplate(id int32, level int, ai npc.AIParams, skills map[npc.SkillType]modelskill.ID) *npc.Template {
	tmpl := wusTemplate(id, ai, skills)
	tmpl.Type, tmpl.Level = "RaidBoss", level
	return tmpl
}

// bootRaidBosses boots the raid boss chain (RaidBossStandard, Gordon,
// RaidBossType1) beside templates, with extra, every NPC casting the raid
// boss fixture skills and the raid curse, every draw 0 until a test queues
// others.
func bootRaidBosses(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind7World {
	t.Helper()
	paths := []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.individual.Monster.MonsterAI",
		"script.ai.individual.Monster.RaidBoss.RaidBossStandard",
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.Gordon",
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1",
	}
	catalog := script.Catalog{
		paths[0]: defaultnpc.New, paths[1]: monster.New, paths[2]: raid.BossStandard, paths[3]: raid.Gordon, paths[4]: raid.BossType1,
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

// gordonWeapons is the cursed weapon data of the Gordon scenarios: Zariche
// alone.
func gordonWeapons(t *testing.T) *entity.CursedWeaponTable {
	t.Helper()
	table, err := entity.NewCursedWeaponTable([]entity.CursedWeapon{{
		ItemID: zaricheItemID, Skill: modelskill.Ref{ID: 3603, Level: 1}, Name: "Demonic Sword Zariche",
		DropRate: 1, Duration: 72, DurationLost: 24, DisappearChance: 50, StageKills: 10,
	}})
	if err != nil {
		t.Fatalf("NewCursedWeaponTable: %v", err)
	}
	return table
}

// TestGordonPicksUpACursedWeaponAndLeaves ports Gordon's created, timer,
// see-item and picked-item hooks. Gordon appears with no raid music, in
// its walk stance, wanting to walk the gordon route at 10. 3 s later, and
// every 3 s, it looks for Zariche or Akamanah within 500: Zariche lies 15
// from it, so it wants to pick it up at 1000000. Its AI picks it up; 3 s
// later Gordon leaves the world.
func TestGordonPicksUpACursedWeaponAndLeaves(t *testing.T) {
	t.Parallel()
	tmpl := rbTemplate(29095, 20, nil, nil)
	w := bootRaidBosses(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithCursedWeapons(gordonWeapons(t))}}, tmpl)
	s := w.spawnFrames(t, 29095, 200, 0)
	gordon := s.h
	if got := ind8Pick(s.frames, serverpackets.OpcodePlaySound); len(got) != 0 {
		t.Fatalf("Gordon's PlaySound frames = %x, want none", got)
	}
	if gordon.Running() {
		t.Fatal("Gordon runs, want its walk stance")
	}
	if d, ok := desire(gordon, actorai.IntentionMoveRoute); !ok || d.Weight != 10 {
		t.Fatalf("route desire = %+v (%v), want weight 10", d, ok)
	}

	if id, ok := w.srv.CursedWeapons.RollDrop(func(int) int { return 0 }); !ok || id != zaricheItemID {
		t.Fatalf("RollDrop = %d, %v; want Zariche", id, ok)
	}
	at := positionOf(gordon)
	w.srv.SeedGroundItem(t, 0, zaricheItemID, 1, at.X+15, at.Y, at.Z)
	zariche := groundItemOf(t, w.srv, zaricheItemID)
	x, y, z := zariche.Position()
	w.srv.CursedWeapons.PlaceOnGround(zaricheItemID, zariche.ObjectID(), location.Location{X: x, Y: y, Z: z})

	w.srv.Advance(t, 3*time.Second)
	d, ok := desire(gordon, actorai.IntentionPickUp)
	if !ok || d.Weight != 1000000 {
		t.Fatalf("pick-up desire after the look = %+v (%v), want weight 1000000", d, ok)
	}
	w.srv.AdvanceUntil(t, "Zariche picked up", func() bool {
		cycle(t, w.srv)
		_, lying := w.srv.State.Object(zariche.ObjectID())
		return !lying
	})
	if got := w.liveNPCs(29095); len(got) != 1 {
		t.Fatalf("Gordon left the world as it picked Zariche up")
	}
	w.srv.Advance(t, 3*time.Second)
	if got := w.liveNPCs(29095); len(got) != 0 {
		t.Fatalf("Gordon still in the world 3 s after picking Zariche up")
	}
}

// TestGordonHuntsTheCarrierOfACursedWeapon ports Gordon's attacked hook: a
// hit of 10 from the character carrying Zariche adds 10 per point of
// damage, 100, to the RaidBossStandard attack desire of 10 / 27 * 20000.
func TestGordonHuntsTheCarrierOfACursedWeapon(t *testing.T) {
	t.Parallel()
	ready := func(srv *gameservertest.Server, objID int32) { srv.GiveItem(t, objID, zaricheItemID, 1) }
	w := bootRaidBosses(t, raidBoot{ready: ready}, rbTemplate(29095, 20, nil, nil))
	gordon := w.spawn(t, 29095, 400, 0)

	w.hit(t, gordon, 10)
	if got, want := w.attackWeight(gordon), 100+rbHate(10, 20); got != want {
		t.Fatalf("attack desire after the hit = %v, want %v", got, want)
	}
}

// groundItemOf returns the one itemID lying in the world.
func groundItemOf(t *testing.T, srv *gameservertest.Server, itemID int32) *grounditem.Item {
	t.Helper()
	for _, obj := range srv.State.Objects() {
		if g, ok := obj.(*grounditem.Item); ok && g.ItemID() == itemID {
			return g
		}
	}
	t.Fatalf("no item %d on the ground", itemID)
	return nil
}

// TestGordonHuntsTheWielderOfACursedWeapon ports Gordon's see-creature
// hook, which its look at the creatures around it raises every 40 s: a
// raid boss sees within its aggro range, here 1000. Seeing the character,
// Gordon wants nothing of it; once the character wields Zariche, picked up
// at its feet, Gordon's next look wants to attack it at 1000000.
func TestGordonHuntsTheWielderOfACursedWeapon(t *testing.T) {
	t.Parallel()
	tmpl := rbTemplate(29095, 20, nil, nil)
	tmpl.AggroRange, tmpl.CanMove = 1000, false
	w := bootRaidBosses(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithCursedWeapons(gordonWeapons(t))}}, tmpl)
	gordon := w.spawn(t, 29095, 400, 0)

	w.srv.Advance(t, 40*time.Second)
	if d, ok := attackOn(gordon, w.player); ok {
		t.Fatalf("attack desire on the unarmed character = %+v, want none", d)
	}

	if id, ok := w.srv.CursedWeapons.RollDrop(func(int) int { return 0 }); !ok || id != zaricheItemID {
		t.Fatalf("RollDrop = %d, %v; want Zariche", id, ok)
	}
	w.srv.SeedGroundItem(t, 0, zaricheItemID, 1, w.at.X, w.at.Y, w.at.Z)
	zariche := groundItemOf(t, w.srv, zaricheItemID)
	w.srv.CursedWeapons.PlaceOnGround(zaricheItemID, zariche.ObjectID(), w.at)
	p := wire.NewPacketWriter(clientpackets.OpcodeAction)
	for _, v := range []int32{zariche.ObjectID(), int32(w.at.X), int32(w.at.Y), int32(w.at.Z)} {
		p.WriteInt32(v)
	}
	p.WriteUint8(0)
	w.srv.Client.Send(p.Bytes())
	w.srv.AdvanceUntil(t, "Zariche picked up", func() bool {
		_, lying := w.srv.State.Object(zariche.ObjectID())
		return !lying
	})
	w.srv.SettlePickup(t, w.player.ObjectID())

	w.srv.Advance(t, 40*time.Second)
	if got := w.attackWeight(gordon); got != 1000000 {
		t.Fatalf("attack desire on the wielder = %v, want 1000000", got)
	}
}
