package ai

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The NPC ids of the fifth individual behavior batch the tests spawn.
const (
	chestID          = int32(18265) // TreasureChest
	mimicID          = int32(21671) // TreasureChestMimic
	splitID          = int32(22088) // PartyLeaderSplit
	splitAggrID      = int32(22094) // PartyLeaderSplitAggressive
	berserkerID      = int32(20086) // WarriorBerserker
	berserkerTwoID   = int32(20248) // WarriorBerserker
	berserkerPSID    = int32(21410) // WarriorBerserkerPhysicalSpecial
	berserkerPSAggID = int32(21382) // WarriorBerserkerPhysicalSpecialAggressive
	berserkerStoneID = int32(21327) // WarriorBerserkerPhysicalSpecialStoneAggressive
)

// The skills the batch's tests cast or hit with.
const (
	unlockID         = modelskill.ID(27)
	keyID            = modelskill.ID(2065)
	deluxeKeyID      = modelskill.ID(2229)
	treasureBombID   = modelskill.ID(4143)
	mimicWrathID     = modelskill.ID(4245)
	mimicStrikeID    = modelskill.ID(4144)
	furyID           = modelskill.ID(4317)
	petrifyID        = modelskill.ID(4515)
	physicalHitID    = modelskill.ID(9310)
	magicHitID       = modelskill.ID(9311)
	berserkerPSProbe = modelskill.ID(9320)
	berserkerDebuff  = modelskill.ID(9321)
	berserkerRange   = modelskill.ID(9322)
)

// individual5Scripts are the behavior chain down to the Warrior behaviors,
// then the batch's classes, in scripts.xml order.
func individual5Scripts() ([]script.Listing, script.Catalog) {
	list, catalog := warriorScripts()
	for _, c := range []struct {
		path string
		ctor func() script.Script
	}{
		{"script.ai.individual.Monster.WarriorBase.Chests.TreasureChest", warrior.TreasureChest},
		{"script.ai.individual.Monster.WarriorBase.Chests.TreasureChestMimic", warrior.TreasureChestMimic},
		{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderSplit", warrior.PartyLeaderSplit},
		{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderSplitAggressive", warrior.PartyLeaderSplitAggressive},
		{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBerserker", warrior.Berserker},
		{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBerserkerPhysicalSpecial", warrior.BerserkerPhysicalSpecial},
		{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBerserkerPhysicalSpecialAggressive", warrior.BerserkerPhysicalSpecialAggressive},
		{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBerserkerPhysicalSpecialStoneAggressive", warrior.BerserkerPhysicalSpecialStoneAggressive},
	} {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	return list, catalog
}

// individual5World is a warriorWorld with the batch registered, the
// script skills defs readable, and every script draw the current value of
// draw.
type individual5World struct {
	*warriorWorld
	draw *atomic.Int32
}

func bootIndividual5(t *testing.T, defs []modelskill.Definition, templates ...*npc.Template) *individual5World {
	t.Helper()
	list, catalog := individual5Scripts()
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draw := new(atomic.Int32)
	w := bootWarriorsWith(t, []gameservertest.Option{
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithScriptRand(func(int) int { return int(draw.Load()) }),
		gameservertest.WithScriptSkills(defs...),
	}, templates...)
	return &individual5World{warriorWorld: w, draw: draw}
}

// skillHit lands a hit of def on h from the character, on its queue.
func (w *individual5World) skillHit(t *testing.T, h *npc.Hostile, def modelskill.Definition) {
	t.Helper()
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		h.ReduceHP(1, w.player, def)
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}

// castDesire returns h's queued cast desire of skill id, and whether it has
// one.
func castDesire(h *npc.Hostile, id modelskill.ID) (actorai.Desire, bool) {
	for _, d := range h.AI().Desires().Snapshot() {
		if d.Kind == actorai.IntentionCast && d.Skill.ID == id {
			return d, true
		}
	}
	return actorai.Desire{}, false
}

// npcSkill is a one-target NPC skill of id and level that the cast seam
// accepts, offensive or not.
func npcSkill(id modelskill.ID, level int, offensive bool) modelskill.Definition {
	d := modelskill.Definition{
		ID: id, Level: level, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		Offensive: offensive, SkillType: "PDAM", CastRange: 600, HitTime: 500, StaticHitTime: true, StaticReuse: true, Power: 1,
	}
	if !offensive {
		d.Target, d.SkillType, d.Power = modelskill.TargetSelf, "BUFF", 0
	}
	return d
}

// chestTemplate is a fixture treasure chest of level 21.
func chestTemplate() *npc.Template {
	tmpl := warriorTemplate(chestID, nil)
	tmpl.Type, tmpl.Name, tmpl.Level, tmpl.CanMove = "Chest", "Treasure Chest", 21, false
	return tmpl
}

// TestTreasureChestKeysAndBomb ports TreasureChest's attacked and
// skill-finished hooks on a level 21 chest. Each level 1 key opens it on a
// draw under its chance, the chest dying to the character, and breaks
// otherwise, the chest deleted with no death and the broken key sound
// played to the character: Unlock 98 less 6 per level of the chest above
// 16 + 4 = 92; key 2065, 60 - 21*1.5 = 28.5 truncated to 28; deluxe key
// (0.0002*2*2 - 0.0264*2 + 0.7695) * 100 = 71.75 truncated to 71. A plain
// hit makes a fresh chest want to cast its treasure bomb of level
// min(10, 21/10 + 1) = 3 on the character; the bomb's cast ends with the
// chest deleted.
func TestTreasureChestKeysAndBomb(t *testing.T) {
	t.Parallel()
	w := bootIndividual5(t, nil, chestTemplate())

	for i, row := range []struct {
		key   modelskill.ID
		draw  int32
		opens bool
	}{
		{unlockID, 91, true},
		{unlockID, 92, false},
		{keyID, 27, true},
		{keyID, 28, false},
		{deluxeKeyID, 70, true},
		{deluxeKeyID, 71, false},
	} {
		chest := w.spawn(t, chestID, 100+50*i, 100)
		w.draw.Store(row.draw)
		w.skillHit(t, chest, modelskill.Definition{ID: row.key, Level: 1})
		frames := w.srv.ReadQueued(t, w.srv.Client)
		died := len(framesOf(frames, serverpackets.OpcodeDie, chest.ObjectID())) == 1
		deleted := !died && len(framesOf(frames, serverpackets.OpcodeDeleteObject, chest.ObjectID())) == 1
		broke := hasSound(frames, "ItemSound2.broken_key")
		// An opened chest stays as a corpse; a broken one leaves the world.
		if w.inWorld(chest) == !row.opens || died != row.opens || deleted == row.opens || broke == row.opens {
			t.Fatalf("level 1 key %d with a draw of %d: died %v, deleted %v, broken key sound %v; want opened %v", row.key, row.draw, died, deleted, broke, row.opens)
		}
	}

	bomb := w.spawn(t, chestID, 100, -100)
	w.srv.InstallNPCCastSeam(t, bomb, modelskill.NewTable([]modelskill.Definition{npcSkill(treasureBombID, 3, true)}))
	w.ticks(t, 2)
	w.hit(t, bomb, 10)
	d, ok := castDesire(bomb, treasureBombID)
	if !ok || d.Skill.Level != 3 || d.Weight != 1000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("chest cast desire after a plain hit = %+v (%v), want the level 3 treasure bomb on the character, weight 1000000", d, ok)
	}
	if bomb.Dead() || !w.inWorld(bomb) {
		t.Fatal("a plain hit took the chest out of the world before its bomb")
	}
	frames := w.ticks(t, 1)
	w.srv.AdvanceUntil(t, "the chest leaving after its bomb", func() bool { return !w.inWorld(bomb) })
	frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
	if len(framesOf(frames, serverpackets.OpcodeMagicSkillUse, bomb.ObjectID())) != 1 || len(framesOf(frames, serverpackets.OpcodeDie, bomb.ObjectID())) != 0 {
		t.Fatal("the chest did not cast its bomb once and leave without a death")
	}
}

// hasSound reports whether frames hold a PlaySound of file; a built frame
// starts with its two length bytes, a read one does not.
func hasSound(frames [][]byte, file string) bool {
	want := serverpackets.FramePlaySound(file).Bytes()[2:]
	for _, f := range frames {
		if string(f) == string(want) {
			return true
		}
	}
	return false
}

// TestTreasureChestMimicAnswersKeys ports TreasureChestMimic's attacked
// hook on a level 30 mimic. A plain hit of 10 adds hate 10 / (30 + 7) * 100
// and no cast. An Unlock makes it want to cast its wrath (4245) on itself
// and its strong attack (4144) of level 30 - 5 on the character; the
// attack desire the key's level gives, 1 / 37 * 150 in integers, is 0, and
// the key's damage of 1 adds 1 / 37 * 100 more hate.
func TestTreasureChestMimicAnswersKeys(t *testing.T) {
	t.Parallel()
	tmpl := warriorTemplate(mimicID, nil)
	tmpl.Level = 30
	w := bootIndividual5(t, nil, tmpl)
	mimic := w.spawn(t, mimicID, 700, 0)
	w.srv.InstallNPCCastSeam(t, mimic, modelskill.NewTable([]modelskill.Definition{
		npcSkill(mimicWrathID, 1, false), npcSkill(mimicStrikeID, 25, true),
	}))

	w.hit(t, mimic, 10)
	if got, want := mimic.AggroHate(w.player), 10.0/37*100; got < want-0.01 || got > want+0.01 {
		t.Fatalf("mimic hate after a hit of 10 = %v, want %v", got, want)
	}
	if d, ok := castDesire(mimic, mimicStrikeID); ok {
		t.Fatalf("mimic wants %+v after a plain hit, want no cast", d)
	}

	before := mimic.AggroHate(w.player)
	w.skillHit(t, mimic, modelskill.Definition{ID: unlockID, Level: 1})
	if d, ok := castDesire(mimic, mimicWrathID); !ok || d.Weight != 1000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != mimic.ObjectID() {
		t.Fatalf("mimic wrath desire = %+v (%v), want on itself, weight 1000000", d, ok)
	}
	if d, ok := castDesire(mimic, mimicStrikeID); !ok || d.Skill.Level != 25 || d.Weight != 1000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("mimic strong attack desire = %+v (%v), want level 25 on the character, weight 1000000", d, ok)
	}
	if got, want := mimic.AggroHate(w.player)-before, 1.0/37*100; got < want-0.01 || got > want+0.01 {
		t.Fatalf("mimic hate gained from the key = %v, want %v", got, want)
	}
}

// splitTemplate is a fixture PartyLeaderSplit NPC of id whose silhouettes
// are Warrior NPCs (warriorTwoID), with a creation shout the Warrior
// created hook would say.
func splitTemplate(id int32) *npc.Template {
	return warriorTemplate(id, npc.AIParams{"silhouette": "20003", "ShoutMsg1": "1000403"})
}

// TestPartyLeaderSplitSplitsOnAPhysicalSkill ports PartyLeaderSplit's
// created and attacked hooks. Created, it does not run the Warrior created
// hook: no shout. A magic skill does nothing; the first physical skill
// places six silhouettes at its point, +20 and +40 on x, +20, +40 and +60
// on y, each told (1000, the attacker) as its spawn parameters, so the
// WarriorBase created hook makes it hate the character by 500, and takes
// the leader out of the world.
func TestPartyLeaderSplitSplitsOnAPhysicalSkill(t *testing.T) {
	t.Parallel()
	physical := modelskill.Definition{ID: physicalHitID, Level: 1}
	magic := modelskill.Definition{ID: magicHitID, Level: 1, Magic: true}
	w := bootIndividual5(t, []modelskill.Definition{physical, magic}, splitTemplate(splitID), warriorTemplate(warriorTwoID, nil))
	leader := w.spawn(t, splitID, 700, 0)
	x, y, z := leader.Position()

	w.skillHit(t, leader, magic)
	if !w.inWorld(leader) || len(w.liveNPCs(warriorTwoID)) != 0 {
		t.Fatal("a magic skill split the leader")
	}
	w.skillHit(t, leader, physical)
	if w.inWorld(leader) {
		t.Fatal("the leader stays in the world after its split")
	}
	silhouettes := w.liveNPCs(warriorTwoID)
	if len(silhouettes) != 6 {
		t.Fatalf("silhouettes = %d, want 6", len(silhouettes))
	}
	want := map[[2]int]int{{x, y}: 1, {x + 20, y}: 1, {x + 40, y}: 1, {x, y + 20}: 1, {x, y + 40}: 1, {x, y + 60}: 1}
	for _, s := range silhouettes {
		sx, sy, sz := s.Position()
		if want[[2]int{sx, sy}] == 0 || sz != z {
			t.Errorf("silhouette at %d,%d,%d, want one of the split offsets from %d,%d,%d", sx, sy, sz, x, y, z)
		}
		want[[2]int{sx, sy}]--
		if got := s.AggroHate(w.player); got != 500 {
			t.Errorf("silhouette hate of the character = %v, want 500", got)
		}
	}
}

// TestPartyLeaderSplitCreatedShoutsNothing compares the created hooks: a
// WarriorBerserker with ShoutMsg1 runs the Warrior created hook and shouts
// it; a PartyLeaderSplit and a WarriorBerserkerPhysicalSpecial with the
// same parameter override it and shout nothing.
func TestPartyLeaderSplitCreatedShoutsNothing(t *testing.T) {
	t.Parallel()
	shout := npc.AIParams{"ShoutMsg1": "1000403"}
	w := bootIndividual5(t, nil, warriorTemplate(splitID, shout), warriorTemplate(berserkerID, shout), warriorTemplate(berserkerPSID, shout))
	for _, row := range []struct {
		id    int32
		lines int
	}{{splitID, 0}, {berserkerID, 1}, {berserkerPSID, 0}} {
		n, err := w.srv.NpcSpawns.AddSpawn(row.id, w.at.X+700, w.at.Y, w.at.Z, 0, false, 0)
		if err != nil {
			t.Fatalf("spawn %d: %v", row.id, err)
		}
		w.srv.Settle(t)
		frames := w.srv.ReadQueued(t, w.srv.Client)
		if got := framesOf(frames, serverpackets.OpcodeNpcSay, n.ObjectID()); len(got) != row.lines {
			t.Errorf("npc %d creation lines = %d, want %d", row.id, len(got), row.lines)
		}
	}
}

// TestAggressiveSplitAndBerserkerAttackOnSight ports the see-creature hook
// of PartyLeaderSplitAggressive and WarriorBerserkerPhysicalSpecialAggressive:
// each wants to attack the character it sees with weight 200.
func TestAggressiveSplitAndBerserkerAttackOnSight(t *testing.T) {
	t.Parallel()
	w := bootIndividual5(t, nil, warriorTemplate(splitAggrID, nil), warriorTemplate(berserkerPSAggID, nil))
	split := w.spawn(t, splitAggrID, 150, 0)
	berserker := w.spawn(t, berserkerPSAggID, 0, 150)
	w.ticks(t, 2)
	for _, h := range []*npc.Hostile{split, berserker} {
		if got := h.AggroHate(w.player); got != 200 {
			t.Errorf("npc %d hate of the character it sees = %v, want 200", h.NpcID(), got)
		}
	}
}

// berserkerTemplate is a fixture berserker of id in the scenario's clan,
// roaring at most twice, listing the fury and the probe skills by type.
func berserkerTemplate(id int32) *npc.Template {
	tmpl := warriorTemplate(id, npc.AIParams{"MaxRoarCount": "2"})
	tmpl.Clans, tmpl.ClanRange = []string{"ant_clan"}, 500
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{
		npc.SkillTypeFury:                     {ID: furyID, Level: 1},
		npc.SkillTypePhysicalSpecial:          {ID: berserkerPSProbe, Level: 1},
		npc.SkillTypeDebuff:                   {ID: berserkerDebuff, Level: 1},
		npc.SkillTypeSelfRangePhysicalSpecial: {ID: berserkerRange, Level: 1},
	}
	return tmpl
}

// berserkerSkills are the skills a fixture berserker casts.
func berserkerSkills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{
		npcSkill(furyID, 1, false), npcSkill(furyID, 2, false), npcSkill(furyID, 3, false),
		npcSkill(berserkerPSProbe, 1, true), npcSkill(berserkerDebuff, 1, true), npcSkill(berserkerRange, 1, false),
	})
}

// clanMember is a fixture Warrior of id in the berserkers' clan.
func clanMember(id int32) *npc.Template {
	tmpl := warriorTemplate(id, nil)
	tmpl.Clans, tmpl.ClanRange = []string{"ant_clan"}, 500
	return tmpl
}

// TestBerserkerRoarsAsItsClanDies ports WarriorBerserker's clan-died hook:
// each death of a clan member makes it want to cast its fury on itself,
// one level higher each time (level 1, then 2), up to its MaxRoarCount of
// 2: a third death adds no level 3 fury.
func TestBerserkerRoarsAsItsClanDies(t *testing.T) {
	t.Parallel()
	w := bootIndividual5(t, nil, berserkerTemplate(berserkerID), clanMember(warriorID), clanMember(warriorTwoID), clanMember(warriorFarID))
	b := w.spawn(t, berserkerID, 700, 0)
	w.srv.InstallNPCCastSeam(t, b, berserkerSkills())
	victims := []*npc.Hostile{w.spawn(t, warriorID, 700, 100), w.spawn(t, warriorTwoID, 700, -100), w.spawn(t, warriorFarID, 800, 0)}

	for i, wantLevel := range []int{1, 2, 2} {
		w.hit(t, victims[i], 5000)
		if !victims[i].Dead() {
			t.Fatalf("clan member %d survived a hit of 5000", i)
		}
		top := 0
		for _, d := range b.AI().Desires().Snapshot() {
			if d.Kind == actorai.IntentionCast && d.Skill.ID == furyID {
				top = max(top, d.Skill.Level)
				if d.Weight != 1000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != b.ObjectID() {
					t.Fatalf("fury desire %+v, want on itself, weight 1000000", d)
				}
			}
		}
		if top != wantLevel {
			t.Fatalf("after %d deaths the highest fury level wanted = %d, want %d", i+1, top, wantLevel)
		}
	}
	if got := script.NPCOf(b).Int(script.SlotIntAI1); got != 2 {
		t.Fatalf("roar count = %d, want 2", got)
	}
}

// TestBerserkerPhysicalSpecialStrikesAndRoars ports
// WarriorBerserkerPhysicalSpecial's attacked, clan-attacked and clan-died
// hooks, every draw 0. The first hit makes the character its top desire
// target, so only the second wants its physical special and debuff on the
// character. A settled clan member that is not attacking wants both at the
// first hit on its clan. A clan death, with a top desire target, adds its
// fury and its ranged strike on itself.
func TestBerserkerPhysicalSpecialStrikesAndRoars(t *testing.T) {
	t.Parallel()
	w := bootIndividual5(t, nil, berserkerTemplate(berserkerPSID), berserkerTemplate(berserkerPSAggID), clanMember(warriorID))
	victim := w.spawn(t, berserkerPSID, 700, 0)
	member := w.spawn(t, berserkerPSAggID, 700, 100)
	dying := w.spawn(t, warriorID, 700, -100)
	for _, h := range []*npc.Hostile{victim, member} {
		w.srv.InstallNPCCastSeam(t, h, berserkerSkills())
	}
	w.ticks(t, 8)
	if member.LifeTime() <= 7 {
		t.Fatalf("clan member life time = %d, want past 7", member.LifeTime())
	}

	on := func(h *npc.Hostile, id modelskill.ID, target int32) bool {
		d, ok := castDesire(h, id)
		return ok && d.Weight == 1000000 && d.FinalTarget != nil && d.FinalTarget.ObjectID() == target
	}
	w.hit(t, victim, 10)
	if _, ok := castDesire(victim, berserkerPSProbe); ok {
		t.Fatal("the victim wants its physical special after the hit that made the character its top target")
	}
	for _, id := range []modelskill.ID{berserkerPSProbe, berserkerDebuff} {
		if !on(member, id, w.player.ObjectID()) {
			t.Fatalf("settled clan member wants no skill %d on the attacker, weight 1000000", id)
		}
	}
	w.hit(t, victim, 10)
	for _, id := range []modelskill.ID{berserkerPSProbe, berserkerDebuff} {
		if !on(victim, id, w.player.ObjectID()) {
			t.Fatalf("victim wants no skill %d on its top desire target, weight 1000000", id)
		}
	}

	w.hit(t, dying, 5000)
	for _, id := range []modelskill.ID{furyID, berserkerRange} {
		if !on(victim, id, victim.ObjectID()) {
			t.Fatalf("victim wants no skill %d on itself after a clan death", id)
		}
	}
}

// TestStoneBerserkerPetrifiesAlliedPlayers ports
// WarriorBerserkerPhysicalSpecialStoneAggressive's see-creature and
// attacked hooks for a Ketra berserker. A character allied with the Ketra
// Orcs is not attacked on sight; its hit makes the berserker want to
// petrify it (4515, weight 1000000 from its attacked hook and as much again
// from its own clan call, which the hit makes first to itself), stop
// hating it and do nothing else. A
// character with no alliance is attacked on sight with weight 200.
func TestStoneBerserkerPetrifiesAlliedPlayers(t *testing.T) {
	t.Parallel()
	stone := func(id int32) *npc.Template {
		tmpl := berserkerTemplate(id)
		tmpl.Clans = []string{"ketra_orc_clan"}
		return tmpl
	}
	w := bootIndividual5(t, nil, stone(berserkerStoneID), stone(21332))
	obj, ok := w.srv.State.Player(w.player.ObjectID())
	if !ok {
		t.Fatal("the character is not in the world")
	}
	ch, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("the character is %T, not an online character", obj)
	}
	ch.SetVarkaKetraAlliance(1)

	allied := w.spawn(t, berserkerStoneID, 150, 0)
	w.srv.InstallNPCCastSeam(t, allied, modelskill.NewTable([]modelskill.Definition{npcSkill(petrifyID, 1, true)}))
	w.ticks(t, 2)
	if got := allied.AggroHate(w.player); got != 0 {
		t.Fatalf("Ketra berserker hate of a Ketra ally it sees = %v, want none", got)
	}
	w.hit(t, allied, 10)
	if d, ok := castDesire(allied, petrifyID); !ok || d.Weight != 2000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("petrify desire = %+v (%v), want on the character, weight 2000000", d, ok)
	}
	if got := allied.AggroHate(w.player); got != 0 {
		t.Fatalf("Ketra berserker hate of the ally that hit it = %v, want none", got)
	}

	ch.SetVarkaKetraAlliance(0)
	open := w.spawn(t, 21332, 0, 150)
	w.ticks(t, 2)
	if got := open.AggroHate(w.player); got != 200 {
		t.Fatalf("Ketra berserker hate of a character with no alliance it sees = %v, want 200", got)
	}
}

// The character's self buffs the stone berserker sees cast: one carrying
// aggro points, one carrying none. Neither is offensive.
const (
	aggroBuffID = modelskill.ID(9330)
	plainBuffID = modelskill.ID(9331)
)

// castOwnSkill has the character cast its skill id.
func castOwnSkill(w *warriorWorld, id modelskill.ID) {
	p := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	p.WriteInt32(int32(id))
	p.WriteInt32(0)
	p.WriteUint8(0)
	w.srv.Client.Send(p.Bytes())
}

// TestStoneBerserkerPetrifiesAlliedCaster ports
// WarriorBerserkerPhysicalSpecialStoneAggressive's see-spell hook for a
// Ketra berserker that sees the character cast a self buff. A buff with
// aggro points cast by a character with no alliance, or a buff without
// aggro points cast by a Ketra ally, goes on to the parent hooks, which
// want nothing here. A buff with aggro points cast by a Ketra ally makes
// the berserker want to petrify the caster (4515, weight 1000000) and
// leaves it no hate.
func TestStoneBerserkerPetrifiesAlliedCaster(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	buff := func(id modelskill.ID, aggro int) modelskill.Definition {
		return modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true, AggroPoints: aggro,
		}
	}
	defs := []modelskill.Definition{buff(aggroBuffID, 100), buff(plainBuffID, 0)}
	table := modelskill.NewTable(append([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}}, defs...))
	tmpl := berserkerTemplate(berserkerStoneID)
	tmpl.Clans = []string{"ketra_orc_clan"}
	list, catalog := individual5Scripts()
	w := bootRaid(t, raidBoot{
		opts: []gameservertest.Option{
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(defs...),
			gameservertest.WithNPCScripts(map[int32]script.NPCKind{berserkerStoneID: script.KindHostile}, list, catalog),
		},
		ready: func(srv *gameservertest.Server, objID int32) {
			for _, d := range defs {
				if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(d.ID), 1); err != nil {
					t.Fatalf("seed skill %d: %v", d.ID, err)
				}
			}
		},
	}, tmpl)
	obj, ok := w.srv.State.Player(w.player.ObjectID())
	if !ok {
		t.Fatal("the character is not in the world")
	}
	ch, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("the character is %T, not an online character", obj)
	}
	stone := w.spawn(t, berserkerStoneID, 150, 0)
	w.srv.InstallNPCCastSeam(t, stone, modelskill.NewTable([]modelskill.Definition{npcSkill(petrifyID, 1, true)}))

	for _, row := range []struct {
		standing int
		skill    modelskill.ID
	}{{0, aggroBuffID}, {1, plainBuffID}} {
		ch.SetVarkaKetraAlliance(row.standing)
		castOwnSkill(w, row.skill)
		w.srv.Advance(t, 2*time.Second)
		w.srv.ReadQueued(t, w.srv.Client)
		if d, ok := castDesire(stone, petrifyID); ok {
			t.Fatalf("standing %d, buff %d: the berserker wants %+v, want no petrify", row.standing, row.skill, d)
		}
	}

	castOwnSkill(w, aggroBuffID)
	w.srv.AdvanceUntil(t, "the petrify desire", func() bool {
		_, ok := castDesire(stone, petrifyID)
		return ok
	})
	if d, _ := castDesire(stone, petrifyID); d.Weight != 1000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("petrify desire = %+v, want on the caster, weight 1000000", d)
	}
	if got := stone.AggroHate(w.player); got != 0 {
		t.Fatalf("berserker hate of the allied caster = %v, want none", got)
	}
}
