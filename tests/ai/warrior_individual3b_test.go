package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/catalog"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

// The NPC ids the scenarios of the third individual behavior batch's
// second PR spawn (#188 PR 2), each a fixture monster of level 1.
const (
	ind3bSummonID       = int32(22029) // PartyLeaderPhysicalSpecialSummon
	ind3bSummonAggroID  = int32(22032) // PartyLeaderPhysicalSpecialSummonAggressive
	ind3bAggroID        = int32(20376) // PartyLeaderWarriorAggressive
	ind3bGardenerID     = int32(22100) // FreyaGardener
	ind3bCannibaliID    = int32(22113) // PartyLeaderCannibaliAggressiva
	ind3bCannibaliPvtID = int32(22112) // PartyPrivateCannibaliAggressive
	ind3bTransformedID  = int32(22114) // the transformed cannibal
	ind3bCurseID        = int32(20773) // PartyLeaderCastCurseAggressive
	ind3bCurseTwoID     = int32(21081) // PartyLeaderCastCurseAggressive
	ind3bSilhouetteID   = int32(29981) // the summon leader's silhouette, unbound
	ind3bListenerID     = int32(29982) // the garden sculpture, recording its events
)

// The skills the batch's fixture NPCs list by type.
const (
	ind3bDebuffSkill   = modelskill.ID(9701)
	ind3bHoldSkill     = modelskill.ID(9702)
	ind3bBuffSkill     = modelskill.ID(9703)
	ind3bSpecial1Skill = modelskill.ID(9704)
	ind3bSummonSkill   = modelskill.ID(9705)
	ind3bEffectSkill   = modelskill.ID(9706)
	ind3bEffect2Skill  = modelskill.ID(9707)
)

// ind3bBatch are the PR's classes, in scripts.xml order, with the private
// the cannibal leader's broadcast reaches.
var ind3bBatch = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.FreyaGardener", warrior.FreyaGardener},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCannibaliAggressiva", warrior.PartyLeaderCannibaliAggressiva},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastCurseAggressive", warrior.PartyLeaderCastCurseAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecialSummon", warrior.PartyLeaderPhysicalSpecialSummon},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecialSummonAggressive", warrior.PartyLeaderPhysicalSpecialSummonAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderWarriorAggressive", warrior.PartyLeaderWarriorAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCannibaliAggressive", warrior.PartyPrivateCannibaliAggressive},
}

// bootIndividual3b boots the PR's classes after the party Warriors beside
// templates, every draw 0 until a scenario queues others, with the
// director script sendEvent sends through and a listener on
// ind3bListenerID recording the script events it hears.
func bootIndividual3b(t *testing.T, extra raidBoot, templates ...*npc.Template) *ind1World {
	t.Helper()
	list, catalog := partyScripts()
	for _, c := range ind3bBatch {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	heard := &ind1Heard{}
	const listener, director = "test.Ind3bListener", "test.Ind1Director"
	catalog[listener] = func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{ind3bListenerID}, Hooks: script.Hooks{
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				heard.mu.Lock()
				defer heard.mu.Unlock()
				heard.events = append(heard.events, fmt.Sprintf("event=%d arg1=%d arg2=%d", e.EventID, e.Arg1, e.Arg2))
			},
		}}
	}
	catalog[director] = func() script.Script { return script.Script{} }
	list = append(list, script.Listing{Path: listener}, script.Listing{Path: director})
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	draws := &ind1Draws{}
	opts := append([]gameservertest.Option{
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(kinds, list, catalog),
	}, extra.opts...)
	w := bootRaid(t, raidBoot{opts: opts, ready: extra.ready}, templates...)
	return &ind1World{warriorWorld: w, draws: draws, heard: heard}
}

// ind3bTemplate is a fixture batch NPC of id with ai as its AI parameters,
// listing skills by type; with clan set, in the clan the scenario's NPCs
// share.
func ind3bTemplate(id int32, ai npc.AIParams, clan bool, skills map[npc.SkillType]modelskill.ID) *npc.Template {
	tmpl := warriorTemplate(id, ai)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{}
	for typ, skill := range skills {
		tmpl.SkillsByType[typ] = modelskill.Ref{ID: skill, Level: 1}
	}
	if clan {
		tmpl.Clans, tmpl.ClanRange = []string{"party"}, 500
	}
	return tmpl
}

// ind3bHeld is skill id reaching 600 whose effect its target holds for a
// minute once it lands.
func ind3bHeld(id modelskill.ID) modelskill.Definition {
	d := ind1Skill(id, 600, false)
	d.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: fmt.Sprintf("ind3b_%d", id), StackOrder: 1}}
	return d
}

// ind3bSkills are the batch NPCs' skills: the debuff, special strike and
// summon magic reach 600; the hold, buff and second effect skill are self
// buffs; the first effect skill is a one-target buff reaching 600.
func ind3bSkills() *modelskill.Table {
	effect := ind1Skill(ind3bEffectSkill, 600, true)
	effect.Target = modelskill.TargetOne
	return modelskill.NewTable([]modelskill.Definition{
		ind3bHeld(ind3bDebuffSkill), ind1Skill(ind3bHoldSkill, 0, true), ind1Skill(ind3bBuffSkill, 0, true),
		ind1Skill(ind3bSpecial1Skill, 600, false), ind1Skill(ind3bSummonSkill, 600, false), effect,
		ind1Skill(ind3bEffect2Skill, 0, true),
	})
}

// landOnCharacter applies the effects of skill id, level 1, on the
// character from h, and waits until the character holds them.
func (w *ind1World) landOnCharacter(t *testing.T, id modelskill.ID, h *npc.Hostile) {
	t.Helper()
	ref := modelskill.Ref{ID: id, Level: 1}
	me := w.player.ObjectID()
	if !w.srv.RunScript(t, me, "Ind1Director", func(s *script.Script, p *script.Player) {
		s.ApplyEffects(ref, script.NPCOf(h), p)
	}) {
		t.Fatal("the effect landing panicked")
	}
	w.srv.AdvanceUntil(t, fmt.Sprintf("skill %d landed on the character", id), func() bool {
		var level int32
		w.srv.RunScript(t, me, "Ind1Director", func(s *script.Script, p *script.Player) {
			level = s.AbnormalLevel(p, ref)
		})
		return level > 0
	})
}

// setHPShare sets h's HP to share of its maximum, on its queue.
func (w *ind1World) setHPShare(t *testing.T, h *npc.Hostile, share float64) {
	t.Helper()
	w.onQueue(t, h, func() { h.SetHP(share * float64(script.NPCOf(h).MaxHP())) })
}

// joinCharacter logs a second client in as account with a character of
// name beside the first one, and returns that character in the world.
func (w *ind1World) joinCharacter(t *testing.T, account, name string) attackable.Combatant {
	t.Helper()
	id := w.srv.SeedCharacterFor(t, account, name, 20, 0).ID
	c := w.srv.DialClient(t, account, 1)
	c.Send(encodeGameStart())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatalf("%s: no CharSelected within 100 frames", name)
		}
		if c.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	w.srv.ReadQueued(t, c)
	w.srv.ReadQueued(t, w.srv.Client)
	obj, ok := w.srv.State.Object(id)
	if !ok {
		t.Fatalf("%s is not in the world", name)
	}
	return obj.(attackable.Combatant)
}

// hitBy lands a hit of damage on h from attacker, a character, on its
// queue.
func (w *ind1World) hitBy(t *testing.T, h *npc.Hostile, attacker attackable.Combatant, damage int) {
	t.Helper()
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, attacker.ObjectID()).Post(func() {
		defer close(done)
		h.TakeDamage(damage, attacker)
	}) {
		t.Fatal("the attacker's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}

// TestPartyLeaderPhysicalSpecialSummonCallsItsSilhouette ports
// PartyLeaderPhysicalSpecialSummon's created, attacked and timer hooks.
// Its created hook calls no parent: it clears i_ai0 and i_ai1, and spawns
// with none of its declared privates and its weight point unset. Hit at
// half its HP or more it calls nothing. Under half, the hit calls the
// silhouette its AI parameters name as its private, at the two draws less
// 25 on x and y, with spawn parameters 1000 and the attacker, and the
// client sees it appear; i_ai0 counts the call and i_ai1 blocks the next
// until the 5 s timer "6006" clears it. A third call never comes.
func TestPartyLeaderPhysicalSpecialSummonCallsItsSilhouette(t *testing.T) {
	t.Parallel()
	leaderTmpl := leaderTemplate(ind3bSummonID, npc.AIParams{"silhouette": fmt.Sprint(ind3bSilhouetteID)}, 2)
	w := bootIndividual3b(t, raidBoot{}, leaderTmpl, warriorTemplate(partyPrivateID, nil), warriorTemplate(ind3bSilhouetteID, nil))
	n := w.spawn(t, ind3bSummonID, 300, 0)
	me := w.player.ObjectID()
	if a0, a1, wp := ind1Slot(n, script.SlotIntAI0), ind1Slot(n, script.SlotIntAI1), ind1Slot(n, script.SlotWeightPoint); a0 != 0 || a1 != 0 || wp != 1 {
		t.Fatalf("created i_ai0, i_ai1, weight point = %d, %d, %d, want 0, 0, 1", a0, a1, wp)
	}
	privatesOf(t, n, 0)

	w.hit(t, n, 1)
	privatesOf(t, n, 0)

	x, y, _ := n.Position()
	w.setHPShare(t, n, 0.4)
	w.draws.set(0, 30, 10)
	w.hit(t, n, 1)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	first := privatesOf(t, n, 1)[0]
	if first.NpcID() != int(ind3bSilhouetteID) {
		t.Fatalf("private npc id = %d, want the silhouette %d", first.NpcID(), ind3bSilhouetteID)
	}
	if px, py, _ := first.Position(); px != x+5 || py != y-15 {
		t.Fatalf("silhouette at %d,%d, want %d,%d", px, py, x+5, y-15)
	}
	if p1, p2, p3 := ind1Slot(first, script.SlotParam1), ind1Slot(first, script.SlotParam2), ind1Slot(first, script.SlotParam3); p1 != 1000 || p2 != me || p3 != 0 {
		t.Fatalf("silhouette spawn parameters = %d, %d, %d, want 1000, %d, 0", p1, p2, p3, me)
	}
	if got := framesOf(frames, serverpackets.OpcodeNPCInfo, first.ObjectID()); len(got) != 2 {
		t.Fatalf("NpcInfo frames of the silhouette = %d, want 2 (appears walking, then running)", len(got))
	}
	if a0, a1 := ind1Slot(n, script.SlotIntAI0), ind1Slot(n, script.SlotIntAI1); a0 != 1 || a1 != 1 {
		t.Fatalf("i_ai0, i_ai1 after the first call = %d, %d, want 1, 1", a0, a1)
	}

	w.hit(t, n, 1)
	privatesOf(t, n, 1)
	w.srv.Advance(t, 5*time.Second)
	if got := ind1Slot(n, script.SlotIntAI1); got != 0 {
		t.Fatalf("i_ai1 after timer 6006 = %d, want 0", got)
	}
	w.hit(t, n, 1)
	privatesOf(t, n, 2)
	if got := ind1Slot(n, script.SlotIntAI0); got != 2 {
		t.Fatalf("i_ai0 after the second call = %d, want 2", got)
	}
	w.srv.Advance(t, 5*time.Second)
	w.hit(t, n, 1)
	privatesOf(t, n, 2)
}

// TestPartyLeaderAggressiveLeadersAttackOnSight ports the see-creature
// hooks of PartyLeaderWarriorAggressive and
// PartyLeaderPhysicalSpecialSummonAggressive: the character each sees
// gets hate 200.
func TestPartyLeaderAggressiveLeadersAttackOnSight(t *testing.T) {
	t.Parallel()
	for _, id := range []int32{ind3bAggroID, ind3bSummonAggroID} {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			t.Parallel()
			w := bootIndividual3b(t, raidBoot{}, warriorTemplate(id, nil))
			n := w.spawn(t, id, 1500, 0)
			w.ticks(t, 3)
			if got := n.AggroHate(w.player); got != 0 {
				t.Fatalf("hate before it saw the character = %v, want 0", got)
			}
			w.moveCharacter(t, n, -150, 0)
			w.ticks(t, 1)
			if got := n.AggroHate(w.player); got != 200 {
				t.Fatalf("hate of the character it sees = %v, want 200", got)
			}
		})
	}
}

// gardenerTemplate is the fixture FreyaGardener: its debuff and hold.
func gardenerTemplate() *npc.Template {
	return ind3bTemplate(ind3bGardenerID, nil, false, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeDebuff: ind3bDebuffSkill, npc.SkillTypeRangeHoldA: ind3bHoldSkill,
	})
}

// TestFreyaGardenerDebuffsItsFirstAttackers ports FreyaGardener's created
// and attacked hooks. The first hit by the character, free of the debuff,
// stores it in c_ai0 and wants the debuff on it, weight 1000000. The next
// two hits by the same character count on (i_ai1) but want nothing: it is
// already in c_ai0. A second character's hit at i_ai1 1 stores it in c_ai1
// and wants the debuff on it; at i_ai1 2 only a character in neither slot
// gets it. Past three, a draw under 20 wants the hold on itself;
// a draw of 20 wants nothing. While the character holds the debuff, even
// the first hit goes to the hold.
func TestFreyaGardenerDebuffsItsFirstAttackers(t *testing.T) {
	t.Parallel()
	t.Run("counts", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual3b(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(ind3bHeld(ind3bDebuffSkill))}}, gardenerTemplate())
		n := w.spawn(t, ind3bGardenerID, 300, 0)
		w.srv.InstallNPCCastSeam(t, n, ind3bSkills())
		me := w.player.ObjectID()
		if got := ind1Slot(n, script.SlotIntAI1); got != 0 {
			t.Fatalf("created i_ai1 = %d, want 0", got)
		}
		w.hit(t, n, 1)
		w.wantCasts(t, "after the first hit", n, map[modelskill.ID]ind1Cast{ind3bDebuffSkill: {me, 1000000}})
		if c0 := script.NPCOf(n).Slot(script.SlotCreature0); c0 == nil || c0.ObjectID() != me {
			t.Fatalf("c_ai0 after the first hit = %v, want the character", c0)
		}
		for i, want := range []int32{2, 3} {
			w.clearDesires(t, n)
			w.hit(t, n, 1)
			w.wantCasts(t, fmt.Sprintf("after hit %d", i+2), n, nil)
			if got := ind1Slot(n, script.SlotIntAI1); got != want {
				t.Fatalf("i_ai1 after hit %d = %d, want %d", i+2, got, want)
			}
		}
		w.draws.set(0, 20)
		w.hit(t, n, 1)
		w.wantCasts(t, "past three on a draw of 20", n, nil)
		w.hit(t, n, 1)
		w.wantCasts(t, "past three on a draw of 0", n, map[modelskill.ID]ind1Cast{ind3bHoldSkill: {n.ObjectID(), 1000000}})
		if got := ind1Slot(n, script.SlotIntAI1); got != 3 {
			t.Fatalf("i_ai1 past three = %d, want 3", got)
		}
	})
	t.Run("three attackers", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual3b(t, raidBoot{}, gardenerTemplate())
		n := w.spawn(t, ind3bGardenerID, 300, 0)
		w.srv.InstallNPCCastSeam(t, n, ind3bSkills())
		mate := w.joinCharacter(t, "mate", "Mate")
		third := w.joinCharacter(t, "third", "Third")
		h := script.NPCOf(n)
		slot := func(s script.CreatureSlot) int32 {
			if c := h.Slot(s); c != nil {
				return c.ObjectID()
			}
			return 0
		}

		w.hit(t, n, 1)
		w.wantCasts(t, "after the first attacker's hit", n, map[modelskill.ID]ind1Cast{ind3bDebuffSkill: {w.player.ObjectID(), 1000000}})
		w.clearDesires(t, n)
		w.hitBy(t, n, mate, 1)
		w.wantCasts(t, "after the second attacker's hit", n, map[modelskill.ID]ind1Cast{ind3bDebuffSkill: {mate.ObjectID(), 1000000}})
		if c0, c1, a1 := slot(script.SlotCreature0), slot(script.SlotCreature1), ind1Slot(n, script.SlotIntAI1); c0 != w.player.ObjectID() || c1 != mate.ObjectID() || a1 != 2 {
			t.Fatalf("c_ai0, c_ai1, i_ai1 after two attackers = %d, %d, %d, want %d, %d, 2", c0, c1, a1, w.player.ObjectID(), mate.ObjectID())
		}

		// The second attacker again at i_ai1 2: it is in c_ai1.
		w.clearDesires(t, n)
		w.hitBy(t, n, mate, 1)
		w.wantCasts(t, "after the second attacker's second hit", n, nil)
		if got := ind1Slot(n, script.SlotIntAI1); got != 3 {
			t.Fatalf("i_ai1 after the second attacker's second hit = %d, want 3", got)
		}

		// A third attacker at i_ai1 2 is in neither slot.
		w.onQueue(t, n, func() { h.SetInt(script.SlotIntAI1, 2) })
		w.clearDesires(t, n)
		w.hitBy(t, n, third, 1)
		w.wantCasts(t, "after the third attacker's hit", n, map[modelskill.ID]ind1Cast{ind3bDebuffSkill: {third.ObjectID(), 1000000}})
		if c0, c1, a1 := slot(script.SlotCreature0), slot(script.SlotCreature1), ind1Slot(n, script.SlotIntAI1); c0 != w.player.ObjectID() || c1 != mate.ObjectID() || a1 != 3 {
			t.Fatalf("c_ai0, c_ai1, i_ai1 after the third attacker = %d, %d, %d, want %d, %d, 3", c0, c1, a1, w.player.ObjectID(), mate.ObjectID())
		}
	})
	t.Run("held", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual3b(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(ind3bHeld(ind3bDebuffSkill))}}, gardenerTemplate())
		n := w.spawn(t, ind3bGardenerID, 300, 0)
		w.srv.InstallNPCCastSeam(t, n, ind3bSkills())
		w.landOnCharacter(t, ind3bDebuffSkill, n)
		w.hit(t, n, 1)
		w.wantCasts(t, "after a hit by a debuffed character", n, map[modelskill.ID]ind1Cast{ind3bHoldSkill: {n.ObjectID(), 1000000}})
		if got := ind1Slot(n, script.SlotIntAI1); got != 0 {
			t.Fatalf("i_ai1 after a hit by a debuffed character = %d, want 0", got)
		}
	})
}

// TestFreyaGardenerForgetsWhomItDebuffed ports FreyaGardener's
// skill-finished hook: once its debuff's cast on the character ends
// (MagicSkillUse on the character), it stops hating the character.
func TestFreyaGardenerForgetsWhomItDebuffed(t *testing.T) {
	t.Parallel()
	w := bootIndividual3b(t, raidBoot{}, gardenerTemplate())
	n := w.spawn(t, ind3bGardenerID, 300, 0)
	w.srv.InstallNPCCastSeam(t, n, ind3bSkills())
	me := w.player.ObjectID()
	w.hit(t, n, 1)
	if got := n.AggroHate(w.player); got <= 0 {
		t.Fatalf("setup: hate after the hit = %v, want some", got)
	}
	frames := w.ticks(t, 1)
	if got := castsOf(frames, n, ind3bDebuffSkill); len(got) != 1 || got[0] != me {
		t.Fatalf("debuff MagicSkillUse targets = %v, want one on %d", got, me)
	}
	w.srv.AdvanceUntil(t, "the debuff cast ended", func() bool { return n.AggroHate(w.player) == 0 })
	if d, ok := desire(n, actorai.IntentionAttack); ok && d.FinalTarget != nil && d.FinalTarget.ObjectID() == me {
		t.Fatalf("attack desire on the character after the debuff = %+v, want none", d)
	}
}

// gardenSpawnlist places the Garden Sculpture's npcmaker under its
// datapack name, with the listener as its NPC.
const gardenSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="garden_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="schuttgart13_npc2314_2m1" territory="garden_field" maximumNpcs="1">
		<ai type="sculpture_garden_maker"/>
		<npc id="29982" total="1" pos="50;20;30;0" respawn="1min"/>
	</npcmaker>
</list>`

// ind3bSpawnTable loads spawnlist as the population's makers.
func ind3bSpawnTable(t *testing.T, spawnlist string) *spawn.Table {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "garden.xml"), []byte(spawnlist), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	return table
}

// TestFreyaGardenerTellsTheGardenSculptureAsItDies ports FreyaGardener's
// dying hook: it runs maker script event 10005 on the npcmaker
// schuttgart13_npc2314_2m1, whose Garden Sculpture maker sends its NPC
// event 11038. Its dying hook calls no parent.
func TestFreyaGardenerTellsTheGardenSculptureAsItDies(t *testing.T) {
	t.Parallel()
	garden := catalog.Makers()["makers_3"]["sculpture_garden_maker"]
	if garden == nil {
		t.Fatal("no maker is bound to sculpture_garden_maker")
	}
	w := bootIndividual3b(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithNpcSpawns(ind3bSpawnTable(t, gardenSpawnlist)),
		gameservertest.WithMakers(script.MakerCatalog{"sculpture_garden_maker": garden}),
	}},
		gardenerTemplate(), warriorTemplate(ind3bListenerID, nil))
	if got := ind3Live(w, ind3bListenerID); got != 1 {
		t.Fatalf("setup: garden sculptures in the world = %d, want 1", got)
	}
	n := w.spawn(t, ind3bGardenerID, 300, 0)
	if got := w.heard.take(); len(got) != 0 {
		t.Fatalf("the sculpture heard %q before the gardener died", got)
	}
	w.die(t, n)
	if got := w.heard.take(); !slices.Equal(got, []string{"event=11038 arg1=0 arg2=0"}) {
		t.Fatalf("the sculpture heard %q after the gardener died, want event 11038 once", got)
	}
}

// cannibaliTemplate is the fixture PartyLeaderCannibaliAggressiva with ai
// as its AI parameters and its five skills by type.
func cannibaliTemplate(ai npc.AIParams) *npc.Template {
	return ind3bTemplate(ind3bCannibaliID, ai, false, map[npc.SkillType]modelskill.ID{
		npc.SkillTypeBuff: ind3bBuffSkill, npc.SkillTypePhysicalSpecial1: ind3bSpecial1Skill,
		npc.SkillTypeSummonMagic: ind3bSummonSkill, npc.SkillTypeEffectSkill: ind3bEffectSkill,
		npc.SkillTypeEffectSkill2: ind3bEffect2Skill,
	})
}

// TestPartyLeaderCannibaliAggressivaHatesByDamage ports
// PartyLeaderCannibaliAggressiva's created and attacked hooks. Created, it
// clears i_ai0 and i_ai3 and stores itself in c_ai0. Its attacked hook
// calls no parent: a hit stores the attacker in c_ai0 and wants to attack
// it with weight damage / (level + 7) x 1000, a hit of 0 counting as 1;
// it calls no private even at a SummonPrivateRate of 100. Under half its HP
// it wants its buff on itself once (i_ai1). A draw under 33 wants its
// physical special on the attacker; standing between 300 and 1000 away, a
// draw under 10 wants its summon magic on it.
func TestPartyLeaderCannibaliAggressivaHatesByDamage(t *testing.T) {
	t.Parallel()
	tmpl := cannibaliTemplate(npc.AIParams{"SummonPrivateRate": "100"})
	tmpl.Privates = []npc.PrivateEntry{{NpcID: int(partyPrivateID), Weight: 1, RespawnDelay: 240 * time.Second}}
	w := bootIndividual3b(t, raidBoot{}, tmpl, warriorTemplate(partyPrivateID, nil))
	n := w.spawn(t, ind3bCannibaliID, 100, 0)
	w.srv.InstallNPCCastSeam(t, n, ind3bSkills())
	me := w.player.ObjectID()
	h := script.NPCOf(n)
	if a0, a3 := ind1Slot(n, script.SlotIntAI0), ind1Slot(n, script.SlotIntAI3); a0 != 0 || a3 != 0 || !script.Same(h.Slot(script.SlotCreature0), h) {
		t.Fatalf("created i_ai0, i_ai3, c_ai0 = %d, %d, %v, want 0, 0, itself", a0, a3, h.Slot(script.SlotCreature0))
	}
	privates := len(n.Minions())

	w.draws.set(99)
	w.zeroHit(t, n)
	w.wantCasts(t, "after a hit of 0 on draws of 99", n, nil)
	if c0 := h.Slot(script.SlotCreature0); c0 == nil || c0.ObjectID() != me {
		t.Fatalf("c_ai0 after the hit = %v, want the character", c0)
	}
	if d, ok := desire(n, actorai.IntentionAttack); !ok || d.Weight != 1000.0/8 {
		t.Fatalf("attack desire after a hit of 0 = %+v (%v), want weight %v", d, ok, 1000.0/8)
	}
	if got := len(n.Minions()); got != privates {
		t.Fatalf("privates after the hit = %d, want %d: the parent's private call never runs", got, privates)
	}

	w.clearDesires(t, n)
	w.setHPShare(t, n, 0.4)
	w.draws.set(99, 32)
	w.hit(t, n, 16)
	w.wantCasts(t, "under half on a draw of 32", n, map[modelskill.ID]ind1Cast{ind3bBuffSkill: {n.ObjectID(), 1000000}, ind3bSpecial1Skill: {me, 1000000}})
	if d, ok := desire(n, actorai.IntentionAttack); !ok || d.Weight != 2000 {
		t.Fatalf("attack desire after a hit of 16 = %+v (%v), want weight 2000", d, ok)
	}
	w.clearDesires(t, n)
	w.hit(t, n, 1)
	w.wantCasts(t, "under half again on draws of 99", n, nil)

	for _, row := range []struct {
		name   string
		dx     int
		draw   int
		summon bool
	}{{"near", 200, 0, false}, {"far", 500, 9, true}, {"far draw 10", 500, 10, false}, {"too far", 1100, 0, false}} {
		w.clearDesires(t, n)
		w.moveCharacter(t, n, row.dx, 0)
		w.draws.set(99, 99, row.draw)
		w.hit(t, n, 1)
		want := map[modelskill.ID]ind1Cast{}
		if row.summon {
			want[ind3bSummonSkill] = ind1Cast{me, 1000000}
		}
		w.wantCasts(t, row.name, n, want)
	}
}

// TestPartyLeaderCannibaliAggressivaPullsItsAttacker ports
// PartyLeaderCannibaliAggressiva's skill-finished hook for its summon
// magic, set up after a hit: once the cast on the character ends, a draw
// under 10 with the character within 1000 brings the character to it
// (TeleportToLocation) and wants to attack it with weight 100000; a draw
// of 10 does neither.
func TestPartyLeaderCannibaliAggressivaPullsItsAttacker(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		draw int
		pull bool
	}{{"pulls", 9, true}, {"draw 10", 10, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual3b(t, raidBoot{}, cannibaliTemplate(nil))
			n := w.spawn(t, ind3bCannibaliID, 500, 0)
			w.srv.InstallNPCCastSeam(t, n, ind3bSkills())
			me := w.player.ObjectID()
			w.draws.set(99, 99, 0)
			w.hit(t, n, 1)
			w.wantCasts(t, "after the hit", n, map[modelskill.ID]ind1Cast{ind3bSummonSkill: {me, 1000000}})
			w.onQueue(t, n, func() { script.NPCOf(n).RemoveAllAttackDesire(); n.AI().Desires().Clear() })
			for i := 0; fightingIntention(n); i++ {
				if i == 5 {
					t.Fatalf("setup: intention after the desires were dropped = %v, want neither attack nor cast", n.AI().CurrentIntention())
				}
				w.ticks(t, 1)
			}
			w.onQueue(t, n, func() {
				script.NPCOf(n).AddCastDesire(script.PlayerOf(w.player), modelskill.Ref{ID: ind3bSummonSkill, Level: 1}, 1000000)
			})
			w.draws.set(row.draw)
			frames := w.ticks(t, 2)
			if got := castsOf(frames, n, ind3bSummonSkill); len(got) != 1 || got[0] != me {
				t.Fatalf("summon magic MagicSkillUse targets = %v, want one on %d", got, me)
			}
			nx, ny, _ := n.Position()
			w.srv.AdvanceUntil(t, "the summon magic ended", func() bool { return n.AI().CurrentIntention() != actorai.IntentionCast })
			frames = w.srv.ReadQueued(t, w.srv.Client)
			x, y, _ := w.srv.PlayerPosition(t, me)
			pulled := x == nx && y == ny
			if pulled != row.pull {
				t.Fatalf("character at %d,%d, cannibal at %d,%d: pulled = %v, want %v", x, y, nx, ny, pulled, row.pull)
			}
			if got := len(framesOf(frames, serverpackets.OpcodeTeleportToLocation, me)) == 1; got != row.pull {
				t.Fatalf("TeleportToLocation of the character sent = %v, want %v", got, row.pull)
			}
			// Pulled into its sight, the character also draws the 200 of
			// its see-creature hook.
			d, ok := desire(n, actorai.IntentionAttack)
			if row.pull && (!ok || d.Weight < 100000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != me) {
				t.Fatalf("attack desire after the pull = %+v (%v), want weight 100000 or more on the character", d, ok)
			}
			if !row.pull && ok {
				t.Fatalf("attack desire without a pull = %+v, want none", d)
			}
		})
	}
}

// cannibaliParty boots a cannibal leader with one PartyPrivateCannibaliAggressive
// private, both able to cast.
func cannibaliParty(t *testing.T) (*ind1World, *npc.Hostile, *npc.Hostile) {
	t.Helper()
	tmpl := cannibaliTemplate(nil)
	tmpl.Privates = []npc.PrivateEntry{{NpcID: int(ind3bCannibaliPvtID), Weight: 1, RespawnDelay: 240 * time.Second}}
	w := bootIndividual3b(t, raidBoot{}, tmpl, warriorTemplate(ind3bCannibaliPvtID, nil), warriorTemplate(ind3bTransformedID, nil))
	leader := w.spawn(t, ind3bCannibaliID, 1500, 0)
	private := privatesOf(t, leader, 1)[0]
	w.srv.InstallNPCCastSeam(t, leader, ind3bSkills())
	return w, leader, private
}

// TestPartyLeaderCannibaliAggressivaTransforms ports
// PartyLeaderCannibaliAggressiva's party-attacked and skill-finished hooks
// for its first effect skill, every draw 99. A hit on its private while
// the private is under a third of its HP marks the leader (i_ai3) and
// stores the private in c_ai1. Hit itself under 70% of its HP, the leader
// wants its first effect skill on the private, weight 1000000, and sets
// i_ai0, so no later hit wants it again. Once that cast ends, the
// transformed cannibal (22114) appears as its private where it stands,
// with spawn parameters 1000 and the creature in c_ai0, and the leader
// leaves the world (DeleteObject). Its broadcast of event 10028 comes
// after it left the world and reaches nobody.
func TestPartyLeaderCannibaliAggressivaTransforms(t *testing.T) {
	t.Parallel()
	w, leader, private := cannibaliParty(t)
	w.draws.set(99)
	me := w.player.ObjectID()

	w.setHPShare(t, private, 0.5)
	w.hit(t, private, 1)
	if got := ind1Slot(leader, script.SlotIntAI3); got != 0 {
		t.Fatalf("i_ai3 after a hit on a healthy private = %d, want 0", got)
	}
	w.setHPShare(t, private, 0.3)
	w.hit(t, private, 1)
	if got, c1 := ind1Slot(leader, script.SlotIntAI3), script.NPCOf(leader).Slot(script.SlotCreature1); got != 1 || c1 == nil || c1.ObjectID() != private.ObjectID() {
		t.Fatalf("i_ai3, c_ai1 after a hit on a weak private = %d, %v, want 1 and the private", got, c1)
	}

	w.clearDesires(t, leader)
	w.setHPShare(t, leader, 0.8)
	w.hit(t, leader, 1)
	w.wantCasts(t, "at 80%", leader, nil)
	w.setHPShare(t, leader, 0.6)
	w.hit(t, leader, 1)
	w.wantCasts(t, "under 70%", leader, map[modelskill.ID]ind1Cast{ind3bEffectSkill: {private.ObjectID(), 1000000}})
	if got := ind1Slot(leader, script.SlotIntAI0); got != 1 {
		t.Fatalf("i_ai0 after the effect skill = %d, want 1", got)
	}
	w.clearDesires(t, leader)
	w.hit(t, leader, 1)
	w.wantCasts(t, "a second hit under 70%", leader, nil)

	w.onQueue(t, leader, func() {
		script.NPCOf(leader).RemoveAllAttackDesire()
		script.NPCOf(leader).AddCastDesire(script.NPCOf(private), modelskill.Ref{ID: ind3bEffectSkill, Level: 1}, 1000000)
	})
	lx, ly, lz := leader.Position()
	if px, py, _ := private.Position(); (px-lx)*(px-lx)+(py-ly)*(py-ly) >= 500*500 {
		t.Fatalf("setup: private at %d,%d, leader at %d,%d: want within 500", px, py, lx, ly)
	}
	frames := w.ticks(t, 2)
	if got := castsOf(frames, leader, ind3bEffectSkill); len(got) != 1 || got[0] != private.ObjectID() {
		t.Fatalf("effect skill MagicSkillUse targets = %v, want one on the private", got)
	}
	w.srv.AdvanceUntil(t, "the leader left the world", func() bool { return !w.inWorld(leader) })
	frames = w.srv.ReadQueued(t, w.srv.Client)
	if got := framesOf(frames, serverpackets.OpcodeDeleteObject, leader.ObjectID()); len(got) != 1 {
		t.Fatalf("DeleteObject frames of the leader = %d, want 1", len(got))
	}
	// Its broadcast of event 10028 runs once it has left the world, and
	// reaches nobody: the private within 500, which leaves on that event,
	// stays.
	if !w.inWorld(private) {
		t.Fatal("the private left the world with the transformation")
	}
	var transformed *npc.Hostile
	for _, m := range leader.Minions() {
		if h := m.(*npc.Hostile); h.NpcID() == int(ind3bTransformedID) {
			transformed = h
		}
	}
	if transformed == nil {
		t.Fatalf("the leader's privates hold no transformed cannibal")
	}
	if x, y, z := transformed.Position(); x != lx || y != ly || z != lz {
		t.Fatalf("transformed cannibal at %d,%d,%d, want %d,%d,%d", x, y, z, lx, ly, lz)
	}
	if p1, p2 := ind1Slot(transformed, script.SlotParam1), ind1Slot(transformed, script.SlotParam2); p1 != 1000 || p2 != me {
		t.Fatalf("transformed spawn parameters = %d, %d, want 1000, %d (the last attacker)", p1, p2, me)
	}
	if got := framesOf(frames, serverpackets.OpcodeNPCInfo, transformed.ObjectID()); len(got) != 2 {
		t.Fatalf("NpcInfo frames of the transformed cannibal = %d, want 2 (appears walking, then running)", len(got))
	}
}

// TestPartyLeaderCannibaliAggressivaSendsItsPrivatesAway ports
// PartyLeaderCannibaliAggressiva's party-attacked and skill-finished hooks
// for its second effect skill, every draw 99. Hit under a third of its HP,
// the leader wants its second effect skill on itself, weight 1000000,
// beside its buff (under half its HP).
// Once that cast ends it broadcasts event 10028 within 500, on which its
// PartyPrivateCannibaliAggressive private leaves the world.
func TestPartyLeaderCannibaliAggressivaSendsItsPrivatesAway(t *testing.T) {
	t.Parallel()
	w, leader, private := cannibaliParty(t)
	w.draws.set(99)
	w.setHPShare(t, leader, 0.3)
	w.hit(t, leader, 1)
	w.wantCasts(t, "under a third", leader, map[modelskill.ID]ind1Cast{ind3bEffect2Skill: {leader.ObjectID(), 1000000}, ind3bBuffSkill: {leader.ObjectID(), 1000000}})
	if got := ind1Slot(leader, script.SlotIntAI0); got != 1 {
		t.Fatalf("i_ai0 after the second effect skill = %d, want 1", got)
	}
	w.onQueue(t, leader, func() {
		script.NPCOf(leader).RemoveAllAttackDesire()
		leader.AI().Desires().Clear()
		script.NPCOf(leader).AddCastDesire(script.NPCOf(leader), modelskill.Ref{ID: ind3bEffect2Skill, Level: 1}, 1000000)
	})
	frames := w.ticks(t, 2)
	if got := castsOf(frames, leader, ind3bEffect2Skill); len(got) != 1 || got[0] != leader.ObjectID() {
		t.Fatalf("second effect skill MagicSkillUse targets = %v, want one on the leader", got)
	}
	w.srv.AdvanceUntil(t, "the private left the world", func() bool { return !w.inWorld(private) })
	if !w.inWorld(leader) {
		t.Fatal("the leader left the world with its private")
	}
	if got := framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeDeleteObject, private.ObjectID()); len(got) != 1 {
		t.Fatalf("DeleteObject frames of the private = %d, want 1", len(got))
	}
}

// curseTemplate is a fixture PartyLeaderCastCurseAggressive of id listing
// its debuff, in the clan the scenario's NPCs share.
func curseTemplate(id int32) *npc.Template {
	return ind3bTemplate(id, nil, true, map[npc.SkillType]modelskill.ID{npc.SkillTypeDebuff: ind3bDebuffSkill})
}

// TestPartyLeaderCastCurseAggressiveCursesWhatItFights ports
// PartyLeaderCastCurseAggressive's created, attacked and timer hooks,
// every draw 0. Created, it clears i_ai1 and i_ai2, which its parent's
// created hook then sets as its summon flag, so the first tick is
// skipped. The first hit arms the 5 s timer "2001"; the second tick wants
// the debuff on the most hated, weight 1000000, which it then casts
// (MagicSkillUse). A hit by the most hated marks the next tick to skip and
// wants the debuff on the attacker. Neither wants it on a character
// holding the debuff. Out of a fight the tick clears both slots and the
// timer stops.
func TestPartyLeaderCastCurseAggressiveCursesWhatItFights(t *testing.T) {
	t.Parallel()
	w := bootIndividual3b(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptSkills(ind3bHeld(ind3bDebuffSkill))}}, curseTemplate(ind3bCurseID))
	n := w.spawn(t, ind3bCurseID, 300, 0)
	w.srv.InstallNPCCastSeam(t, n, ind3bSkills())
	me := w.player.ObjectID()
	if a1, a2 := ind1Slot(n, script.SlotIntAI1), ind1Slot(n, script.SlotIntAI2); a1 != 0 || a2 != 1 {
		t.Fatalf("created i_ai1, i_ai2 = %d, %d, want 0, 1", a1, a2)
	}

	w.hit(t, n, 10)
	w.wantCasts(t, "after the first hit", n, nil)
	if got := ind1Slot(n, script.SlotIntAI1); got != 1 {
		t.Fatalf("i_ai1 after the first hit = %d, want 1", got)
	}
	w.ticks(t, 1)
	if !fightingIntention(n) {
		t.Fatalf("intention after the first hit = %v, want attack", n.AI().CurrentIntention())
	}
	w.srv.Advance(t, 5*time.Second)
	w.wantCasts(t, "after the first tick", n, nil)
	w.srv.Advance(t, 5*time.Second)
	w.wantCasts(t, "after the second tick", n, map[modelskill.ID]ind1Cast{ind3bDebuffSkill: {me, 1000000}})
	if got := castsOf(w.ticks(t, 2), n, ind3bDebuffSkill); len(got) != 1 || got[0] != me {
		t.Fatalf("debuff MagicSkillUse targets = %v, want one on %d", got, me)
	}

	w.clearDesires(t, n)
	w.hit(t, n, 10)
	w.wantCasts(t, "after a hit by the most hated", n, map[modelskill.ID]ind1Cast{ind3bDebuffSkill: {me, 1000000}})
	if got := ind1Slot(n, script.SlotIntAI2); got != 1 {
		t.Fatalf("i_ai2 after a hit by the most hated = %d, want 1", got)
	}
	w.clearDesires(t, n)
	w.ticks(t, 1)
	w.srv.Advance(t, 5*time.Second)
	w.wantCasts(t, "after the skipped tick", n, nil)

	w.landOnCharacter(t, ind3bDebuffSkill, n)
	w.clearDesires(t, n)
	w.hit(t, n, 10)
	w.wantCasts(t, "after a hit while the character holds the debuff", n, nil)
	w.clearDesires(t, n)
	w.ticks(t, 1)
	w.srv.Advance(t, 5*time.Second)
	w.wantCasts(t, "at a tick while the character holds the debuff", n, nil)

	w.onQueue(t, n, func() { script.NPCOf(n).RemoveAllAttackDesire(); n.AI().Desires().Clear() })
	w.ticks(t, 1)
	if fightingIntention(n) {
		t.Fatalf("intention after the desires were dropped = %v, want neither attack nor cast", n.AI().CurrentIntention())
	}
	w.srv.Advance(t, 5*time.Second)
	if a1, a2 := ind1Slot(n, script.SlotIntAI1), ind1Slot(n, script.SlotIntAI2); a1 != 0 || a2 != 0 {
		t.Fatalf("i_ai1, i_ai2 after the tick out of a fight = %d, %d, want 0, 0", a1, a2)
	}
}

// TestPartyLeaderCastCurseAggressiveTickDrawsOnlyWithDebuff pins the
// tick's draw order: a template that lists no debuff draws nothing, so
// the queued draw of 99 is still there for the next caller; one that
// lists it draws 99, which is not under 50, and wants nothing.
func TestPartyLeaderCastCurseAggressiveTickDrawsOnlyWithDebuff(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		listed bool
		left   int
	}{{"unlisted", false, 99}, {"listed", true, 0}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := curseTemplate(ind3bCurseID)
			if !row.listed {
				delete(tmpl.SkillsByType, npc.SkillTypeDebuff)
			}
			w := bootIndividual3b(t, raidBoot{}, tmpl)
			n := w.spawn(t, ind3bCurseID, 300, 0)
			w.hit(t, n, 10)
			w.ticks(t, 1)
			if !fightingIntention(n) {
				t.Fatalf("intention after the hit = %v, want attack", n.AI().CurrentIntention())
			}
			w.srv.Advance(t, 5*time.Second) // the skipped first tick
			w.clearDesires(t, n)
			w.draws.set(0, 99)
			w.srv.Advance(t, 5*time.Second)
			w.wantCasts(t, "after the second tick", n, nil)
			if got := w.draws.rand(100); got != row.left {
				t.Fatalf("next draw after the tick = %d, want %d", got, row.left)
			}
		})
	}
}

// TestPartyLeaderCastCurseAggressiveCursesForItsClan ports
// PartyLeaderCastCurseAggressive's clan-attacked hook, every draw 0: a
// settled clan member wants its debuff, weight 1000000, on the attacker of
// another member; a young member does not.
func TestPartyLeaderCastCurseAggressiveCursesForItsClan(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		settle int
		cast   bool
	}{{"settled", 8, true}, {"young", 0, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual3b(t, raidBoot{}, curseTemplate(ind3bCurseID), curseTemplate(ind3bCurseTwoID))
			victim := w.spawn(t, ind3bCurseID, 1500, 0)
			member := w.spawn(t, ind3bCurseTwoID, 1500, 200)
			w.srv.InstallNPCCastSeam(t, member, ind3bSkills())
			w.ticks(t, row.settle)
			w.clearDesires(t, member)
			w.hit(t, victim, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.cast {
				want[ind3bDebuffSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "member after the hit", member, want)
		})
	}
}
