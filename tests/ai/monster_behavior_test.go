package ai

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monsterbehavior"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The MonsterBehavior scenarios' NPC ids, each a fixture monster of level
// 1: WarriorUseSkill ids in the reference for the warriors, a
// WizardUseSkill id for the wizard.
const (
	behaviorWarriorID = int32(20136)
	behaviorLeaderID  = int32(20214)
	behaviorPrivateID = int32(20215)
	behaviorWizardID  = int32(20216)
	behaviorMuteID    = modelskill.ID(9304)
)

// behaviorScripts are the behavior chain down to WarriorBehavior and
// WizardBehavior, in scripts.xml order, and two behaviors bound to the
// fixture ids that take their hooks unchanged: WarriorBehavior's for the
// warriors, WizardBehavior's for the wizard. They stand in for the
// reference's children, which are ported later.
func behaviorScripts() ([]script.Listing, script.Catalog) {
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":                                              defaultnpc.New,
		"script.ai.individual.Monster.MonsterAI":                                       monster.New,
		"script.ai.individual.Monster.MonsterBehavior.MonsterBehavior":                 monsterbehavior.MonsterBehavior,
		"script.ai.individual.Monster.MonsterBehavior.WarriorBehavior.WarriorBehavior": monsterbehavior.WarriorBehavior,
		"script.ai.individual.Monster.MonsterBehavior.WizardBehavior.WizardBehavior":   monsterbehavior.WizardBehavior,
		"ai.BehaviorWarrior": func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{behaviorWarriorID, behaviorLeaderID, behaviorPrivateID}, Hooks: monsterbehavior.WarriorHooks()}
		},
		"ai.BehaviorWizard": func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{behaviorWizardID}, Hooks: monsterbehavior.WizardHooks()}
		},
	}
	var list []script.Listing
	for _, path := range []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.individual.Monster.MonsterAI",
		"script.ai.individual.Monster.MonsterBehavior.MonsterBehavior",
		"script.ai.individual.Monster.MonsterBehavior.WarriorBehavior.WarriorBehavior",
		"script.ai.individual.Monster.MonsterBehavior.WizardBehavior.WizardBehavior",
		"ai.BehaviorWarrior",
		"ai.BehaviorWizard",
	} {
		list = append(list, script.Listing{Path: path})
	}
	return list, catalog
}

// bootBehaviors boots one character in the world beside the fixture
// templates, the behavior chain registered and every script draw 0.
func bootBehaviors(t *testing.T, templates ...*npc.Template) *warriorWorld {
	t.Helper()
	return bootBehaviorsWith(t, nil, templates...)
}

// bootBehaviorsWith is bootBehaviors with extra boot options, applied last.
func bootBehaviorsWith(t *testing.T, extra []gameservertest.Option, templates ...*npc.Template) *warriorWorld {
	t.Helper()
	list, catalog := behaviorScripts()
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(kinds, list, catalog),
	}, extra...)...)
	c := srv.Client
	c.Send(encodeGameStart())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("no CharSelected within 100 frames")
		}
		if c.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	srv.ReadQueued(t, c)
	id := srv.SoleObjectID(t)
	x, y, z := srv.PlayerPosition(t, id)
	obj, ok := srv.State.Object(id)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	return &warriorWorld{srv: srv, at: location.Location{X: x, Y: y, Z: z}, player: obj.(attackable.Combatant)}
}

// behaviorLeader is a fixture party leader (Party_Type 2) that calls one
// private of behaviorPrivateID, respawning after 240 s, onto its top
// desire target at every hit by it (Party_OneShot 10000).
func behaviorLeader() *npc.Template {
	tmpl := warriorTemplate(behaviorLeaderID, npc.AIParams{"Party_Type": "2", "Party_OneShot": "10000"})
	tmpl.Privates = []npc.PrivateEntry{{NpcID: int(behaviorPrivateID), Weight: 1, RespawnDelay: 240 * time.Second}}
	return tmpl
}

// behaviorPrivate is the fixture leader's private: a following private
// (Party_Type 1) of a loyalty no party rule answers (Party_Loyalty 3), so
// only its master's call makes it attack.
func behaviorPrivate() *npc.Template {
	return warriorTemplate(behaviorPrivateID, npc.AIParams{"Party_Type": "1", "Party_Loyalty": "3"})
}

// attackOn returns h's queued attack desire on target, and whether it has
// one.
func attackOn(h *npc.Hostile, target attackable.Combatant) (actorai.Desire, bool) {
	for _, d := range h.AI().Desires().Snapshot() {
		if d.Kind == actorai.IntentionAttack && d.FinalTarget != nil && d.FinalTarget.ObjectID() == target.ObjectID() {
			return d, true
		}
	}
	return actorai.Desire{}, false
}

// TestWarriorBehaviorAttacksByItsMovingAttack ports WarriorBehavior's
// attacked hook. A warrior that moves to attack (MovingAttack 1, the
// default) wants to chase its attacker by 10 per point of damage. One that
// holds its ground (MovingAttack 0) wants to attack from where it stands
// only a creature it may attack on its own: an aggressive warrior the
// character stands within aggro range of does, one with no aggro range
// wants nothing.
func TestWarriorBehaviorAttacksByItsMovingAttack(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name       string
		ai         npc.AIParams
		aggroRange int
		want       bool
		moves      bool
	}{
		{"moving", nil, 0, true, true},
		{"hold aggressive", npc.AIParams{"MovingAttack": "0"}, 400, true, false},
		{"hold peaceful", npc.AIParams{"MovingAttack": "0"}, 0, false, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := warriorTemplate(behaviorWarriorID, row.ai)
			tmpl.AggroRange = row.aggroRange
			w := bootBehaviors(t, tmpl)
			h := w.spawn(t, behaviorWarriorID, 100, 0)

			w.hit(t, h, 10)
			d, ok := attackOn(h, w.player)
			if ok != row.want {
				t.Fatalf("attack desire on the character = %+v (%v), want one: %v", d, ok, row.want)
			}
			if ok && (d.Weight != 100 || d.MoveToTarget != row.moves) {
				t.Fatalf("attack desire = weight %v, move to target %v; want 100, %v", d.Weight, d.MoveToTarget, row.moves)
			}
		})
	}
}

// TestWarriorBehaviorLeaderCallsItsPrivateOntoItsTarget ports the
// WarriorBehavior party call. The party leader creates its private as it
// spawns. Its first hit by the character makes the character its top
// desire target once it takes the attack up; the next hit by that target
// broadcasts the call, which reaches the private inline: the private drops
// its attack desires and answers the character as if hit by it for 100,
// wanting to attack it at 1000.
func TestWarriorBehaviorLeaderCallsItsPrivateOntoItsTarget(t *testing.T) {
	t.Parallel()
	w := bootBehaviors(t, behaviorLeader(), behaviorPrivate())
	leader := w.spawn(t, behaviorLeaderID, 100, 0)
	private := privatesOf(t, leader, 1)[0]

	w.hit(t, leader, 10)
	if _, ok := attackOn(private, w.player); ok {
		t.Fatal("the private wants to attack the character before its leader called it")
	}
	w.ticks(t, 1)
	if top := script.NPCOf(leader).TopDesireTarget(); !script.Same(top, script.PlayerOf(w.player)) {
		t.Fatalf("leader top desire target = %v, want the character", top)
	}

	w.hit(t, leader, 10)
	if d, ok := attackOn(private, w.player); !ok || d.Weight != 1000 {
		t.Fatalf("called private's attack desire on the character = %+v (%v), want weight 1000", d, ok)
	}
}

// TestMonsterBehaviorPrivateLeavesOnceItsLeaderDies ports MonsterBehavior's
// private checks. The private's 20 s check after it spawns finds its
// leader alive and keeps it. Its leader's death arms the check again; 20 s
// later the private, with no live master and neither fighting nor casting,
// leaves the world.
func TestMonsterBehaviorPrivateLeavesOnceItsLeaderDies(t *testing.T) {
	t.Parallel()
	w := bootBehaviors(t, behaviorLeader(), behaviorPrivate())
	leader := w.spawn(t, behaviorLeaderID, 100, 0)
	private := privatesOf(t, leader, 1)[0]

	w.srv.Advance(t, 20*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if !w.inWorld(private) {
		t.Fatal("the 20 s check hid a private whose leader lives")
	}

	w.kill(t, leader)
	w.srv.Advance(t, 19*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if !w.inWorld(private) {
		t.Fatal("the private left before its check came due")
	}
	w.srv.Advance(t, time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if w.inWorld(private) {
		t.Fatal("the check after its leader's death kept a private with no live master")
	}
}

// TestMonsterBehaviorLeaderRespawnsADeadPrivate ports MonsterBehavior's
// party-died hook on a live party leader (Party_Type 2): its private's
// death asks the private's spawn to bring it back.
func TestMonsterBehaviorLeaderRespawnsADeadPrivate(t *testing.T) {
	t.Parallel()
	w := bootBehaviors(t, behaviorLeader(), behaviorPrivate())
	leader := w.spawn(t, behaviorLeaderID, 100, 0)
	private := privatesOf(t, leader, 1)[0]

	w.kill(t, private)
	w.armedPrivateRespawn(t)
}

// TestMonsterBehaviorShortRangedMonsterRunsFromItsAttacker ports
// MonsterBehavior's AttackRange 1 branch: a hit from closer than 100 makes
// the monster remember its attacker, and 10 s later it wants to run from
// that attacker by the drift range, at 10000000. A hit from farther away
// starts nothing.
func TestMonsterBehaviorShortRangedMonsterRunsFromItsAttacker(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		away int
		runs bool
	}{
		{"near", 50, true},
		{"far", 150, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootBehaviors(t, warriorTemplate(behaviorWarriorID, npc.AIParams{"AttackRange": "1"}))
			h := w.spawn(t, behaviorWarriorID, row.away, 0)

			w.hit(t, h, 10)
			if _, ok := desire(h, actorai.IntentionFlee); ok {
				t.Fatal("the monster wants to run before its 10 s timer")
			}
			w.srv.Advance(t, 10*time.Second)
			d, ok := desire(h, actorai.IntentionFlee)
			if ok != row.runs {
				t.Fatalf("attacker %d away: flee desire = %+v (%v), want one: %v", row.away, d, ok, row.runs)
			}
			if ok && (d.Weight != 10000000 || d.Distance != 200 || d.FinalTarget.ObjectID() != w.player.ObjectID()) {
				t.Fatalf("flee desire = %+v, want from the character, 200 (the shipped drift range) far, weight 10000000", d)
			}
		})
	}
}

// TestWizardBehaviorFillsItsHateList ports WizardBehavior's attacked hook
// and its 5001 timer, every draw 0. A hit of 10 enters the character in the
// wizard's empty hate list at 10 per point of damage plus the opening 300;
// a second hit adds 100. A hit while silenced arms the timer: it waits out
// the silence, then drops the wizard's attack desires and answers its most
// hated as if hit by it for 100, adding 1000.
func TestWizardBehaviorFillsItsHateList(t *testing.T) {
	t.Parallel()
	w := bootBehaviors(t, warriorTemplate(behaviorWizardID, nil))
	h := w.spawn(t, behaviorWizardID, 100, 0)

	w.hit(t, h, 10)
	if got := h.AI().Hates().Hate(w.player); got != 400 {
		t.Fatalf("hate list entry of the character after the first hit = %v, want 400", got)
	}
	w.hit(t, h, 10)
	if got := h.AI().Hates().Hate(w.player); got != 500 {
		t.Fatalf("hate list entry of the character after the second hit = %v, want 500", got)
	}

	mute, err := effect.New(effect.Skill{ID: behaviorMuteID, Level: 1}, modelskill.EffectTemplate{Name: "Mute", Time: 600})
	if err != nil {
		t.Fatalf("effect.New(Mute): %v", err)
	}
	mute.Effector, mute.Effected = h, h
	w.onQueue(t, h, func() { h.EffectList().Add(mute) })
	w.hit(t, h, 10)
	if got := script.NPCOf(h).Int(script.SlotIntAI4); got != 1 {
		t.Fatalf("silenced wizard int slot 4 = %d, want 1", got)
	}
	w.srv.Advance(t, 10*time.Second)
	if got := h.AI().Hates().Hate(w.player); got != 600 {
		t.Fatalf("hate list entry while silenced = %v, want 600", got)
	}

	w.onQueue(t, h, func() { h.EffectList().Remove(mute) })
	w.srv.Advance(t, 10*time.Second)
	if got := script.NPCOf(h).Int(script.SlotIntAI4); got != 0 {
		t.Fatalf("freed wizard int slot 4 = %d, want 0", got)
	}
	if got := h.AI().Hates().Hate(w.player); got != 1600 {
		t.Fatalf("hate list entry after the timer's attacked call = %v, want 600 + 1000", got)
	}
}

// TestWizardBehaviorForgetsWhoWalksAway ports WizardBehavior's 5002 timer:
// every 10 s the wizard's hate list drops the creatures 2000 or more away.
// A character 1999 away stays listed, one 2000 away is dropped.
func TestWizardBehaviorForgetsWhoWalksAway(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		away int
		kept bool
	}{
		{"near", 1999, true},
		{"far", 2000, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootBehaviors(t, warriorTemplate(behaviorWizardID, nil))
			h := w.spawn(t, behaviorWizardID, 100, 0)
			w.hit(t, h, 10)
			x, y, z := h.Position()
			w.instantTeleport(t, location.Location{X: x - row.away, Y: y, Z: z})
			if !h.Knows(w.player) {
				t.Fatal("the wizard no longer knows the character")
			}

			w.srv.Advance(t, 10*time.Second)
			if listed := h.AI().Hates().Hate(w.player) != 0; listed != row.kept {
				t.Fatalf("character %d away: in the hate list %v, want %v", row.away, listed, row.kept)
			}
		})
	}
}

// TestWarriorBehaviorRootedWarriorDropsAnUnreachableTarget ports the
// rooted branch of WarriorBehavior's attack. A warrior that moves to attack
// wants its attacker by 10 per point of damage at each hit: 200 after two
// hits of 10, the first making the character its top desire target. A
// rooted one more than 40 from that top target, which it may not attack on
// its own (it has no aggro range), drops its attack desire on it at the
// second hit and wants it again at that hit's weight alone.
func TestWarriorBehaviorRootedWarriorDropsAnUnreachableTarget(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		rooted bool
		want   float64
	}{
		{"free", false, 200},
		{"rooted", true, 100},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootBehaviors(t, warriorTemplate(behaviorWarriorID, nil))
			h := w.spawn(t, behaviorWarriorID, 300, 0)
			if row.rooted {
				root, err := effect.New(effect.Skill{ID: behaviorMuteID, Level: 1}, modelskill.EffectTemplate{Name: "Root", Time: 600})
				if err != nil {
					t.Fatalf("effect.New(Root): %v", err)
				}
				root.Effector, root.Effected = h, h
				w.onQueue(t, h, func() { h.EffectList().Add(root) })
				if !script.NPCOf(h).Rooted() {
					t.Fatal("the warrior is not rooted")
				}
			}

			w.hit(t, h, 10)
			w.ticks(t, 1)
			if top := script.NPCOf(h).TopDesireTarget(); !script.Same(top, script.PlayerOf(w.player)) {
				t.Fatalf("warrior top desire target = %v, want the character", top)
			}
			w.hit(t, h, 10)
			if d, ok := attackOn(h, w.player); !ok || d.Weight != row.want {
				t.Fatalf("attack desire on the character = %+v (%v), want weight %v", d, ok, row.want)
			}
		})
	}
}
